// Command waas does the data work of WaaS's workflows: it reads the services
// file, answers what a repository deploys, and routes a push to its
// environment. It prints JSON, and inside GitHub Actions also sets the step's
// outputs and summary.
//
//	waas validate -f services.json
//	waas plan -f services/shop.json --catalog services/services.json --repository shop --branch dev
//	waas matrix -f services/shop.json --catalog services/services.json --repository shop
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/rollwright/waas/internal/actions"
	"github.com/rollwright/waas/internal/services"
)

// version is the release, set at build time (-ldflags "-X main.version=v1.2.3").
var version = "dev"

const usage = `waas does the data work of WaaS's workflows.

Commands:
  validate   check a services file
  plan       what a push deploys: its branch's environment and the services there
  matrix     what a repository deploys, per environment
  version    the CLI's version

Run "waas <command> -h" for a command's flags.
`

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "❌", err)
		_ = actions.Summary("## WaaS failed\n\n```\n" + err.Error() + "\n```")
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, usage)
		return errors.New("no command")
	}
	switch args[0] {
	case "validate":
		return validate(args[1:], stdout)
	case "plan":
		return plan(args[1:], stdout)
	case "matrix":
		return matrix(args[1:], stdout)
	case "version", "--version":
		fmt.Fprintln(stdout, version)
		return nil
	case "-h", "--help", "help":
		fmt.Fprint(stdout, usage)
		return nil
	}
	return fmt.Errorf("unknown command %q\n\n%s", args[0], usage)
}

func validate(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	file := fs.String("f", "", "services file (required)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *file == "" {
		return errors.New("validate: -f is required")
	}
	f, err := services.Load(*file)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "✅ %s: %d services, %d environments\n", f.Path, len(f.Services), len(f.Environments))
	return nil
}

// requestFlags are the flags saying which repository asks, and where its
// services file is.
func requestFlags(fs *flag.FlagSet, r *services.Request) {
	fs.StringVar(&r.File, "f", "", "the repository's services file; it need not exist")
	fs.StringVar(&r.Catalog, "catalog", "", "shared services file a repository without its own is looked up in")
	fs.StringVar(&r.Repository, "repository", repositoryName(), "the repository's name, looked up in the catalog (default: this repository, inside Actions)")
	fs.StringVar(&r.Service, "service", "", "the service a polyrepo deploys as")
	fs.BoolVar(&r.Monorepo, "monorepo", true, "the repository deploys several services")
	fs.BoolVar(&r.ByName, "by-name", false, "skip the files: deploy -service by its name alone")
}

func checkRequest(cmd string, r services.Request) error {
	if r.File == "" && !r.ByName {
		return fmt.Errorf("%s: -f is required (or -by-name)", cmd)
	}
	if !r.Monorepo && r.Service == "" {
		return fmt.Errorf("%s: a polyrepo (-monorepo=false) needs -service", cmd)
	}
	return nil
}

func plan(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("plan", flag.ContinueOnError)
	var r services.Request
	requestFlags(fs, &r)
	branch := fs.String("branch", os.Getenv("GITHUB_REF_NAME"), "the branch pushed to (default: $GITHUB_REF_NAME)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := checkRequest("plan", r); err != nil {
		return err
	}

	p, err := services.PlanPush(r, *branch)
	if err != nil {
		return err
	}
	list, _ := json.Marshal(p.Services)
	from := services.Environment{}
	if p.PromoteFrom != nil {
		from = *p.PromoteFrom
	}
	outs := append(matrixOutputs(p.Matrix),
		[2]string{"deploy", strconv.FormatBool(p.Deploy)},
		[2]string{"environment", p.Name},
		[2]string{"domain", p.Domain},
		[2]string{"config", p.BuildConfiguration},
		[2]string{"matrix", string(list)},
		[2]string{"promote_from_environment", from.Name},
		[2]string{"promote_from_domain", from.Domain},
	)
	outs = append(outs, signingOutputs(p.Signing)...)
	if err := actions.Output(outs...); err != nil {
		return err
	}
	summary := fmt.Sprintf("## Plan\n\n`%s` deploys nowhere: no environment takes this branch.\n", *branch)
	if p.Deploy {
		summary = fmt.Sprintf("## Plan\n\n`%s` deploys %d services from `%s` to **%s** (domain `%s`, build configuration `%s`); images are %s.\n",
			*branch, len(p.Services), sourceOf(p.Matrix), p.Name, p.Domain, p.BuildConfiguration, p.Signing)
	}
	if err := actions.Summary(summary); err != nil {
		return err
	}
	return printJSON(stdout, p)
}

// signingOutputs say how images are signed: signing_key for a key;
// signing_keyless "true", with signing_identity and signing_issuer, for
// keyless; all empty, not at all.
func signingOutputs(s *services.Signing) [][2]string {
	var key, keyless, identity, issuer string
	switch {
	case s == nil:
	case s.Key != "":
		key = s.Key
	case s.Keyless != nil:
		keyless, identity, issuer = "true", s.Keyless.Identity, s.Keyless.Issuer
	}
	return [][2]string{{"signing_key", key}, {"signing_keyless", keyless}, {"signing_identity", identity}, {"signing_issuer", issuer}}
}

// matrixOutputs are matrix's step outputs: whether it is a monorepo, its
// service, and its services, all and per environment.
func matrixOutputs(m *services.Matrix) [][2]string {
	all, _ := json.Marshal(m.All)
	outs := [][2]string{
		{"monorepo", strconv.FormatBool(m.Monorepo)},
		{"service", m.Service},
		{"all-services", string(all)},
	}
	for _, env := range sortedKeys(m.Environments) {
		list, _ := json.Marshal(m.Environments[env])
		outs = append(outs, [2]string{env + "-services", string(list)})
	}
	return outs
}

func matrix(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("matrix", flag.ContinueOnError)
	var r services.Request
	requestFlags(fs, &r)
	envs := fs.String("environments", "", "environments to list services for, comma-separated (default: the file's metadata.environments, else staging,production)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := checkRequest("matrix", r); err != nil {
		return err
	}
	r.Environments = splitList(*envs)

	m, err := services.Resolve(r)
	if err != nil {
		return err
	}

	outs := matrixOutputs(m)
	summary := fmt.Sprintf("## Services\n\n%d from `%s`\n\n| Environment | Services |\n|---|---|\n", len(m.All), sourceOf(m))
	for _, env := range sortedKeys(m.Environments) {
		summary += fmt.Sprintf("| %s | %d |\n", env, len(m.Environments[env]))
	}
	if err := actions.Output(outs...); err != nil {
		return err
	}
	if err := actions.Summary(summary); err != nil {
		return err
	}
	return printJSON(stdout, m)
}

// repositoryName is this repository's name inside GitHub Actions
// ($GITHUB_REPOSITORY without its owner), else "".
func repositoryName() string {
	full := os.Getenv("GITHUB_REPOSITORY")
	return full[strings.LastIndex(full, "/")+1:]
}

func printJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func splitList(s string) []string {
	var out []string
	for _, e := range strings.Split(s, ",") {
		if e = strings.TrimSpace(e); e != "" {
			out = append(out, e)
		}
	}
	return out
}

func sortedKeys(m map[string][]services.Entry) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}

func sourceOf(m *services.Matrix) string {
	if m.Source == "" {
		return "the service name"
	}
	return m.Source
}
