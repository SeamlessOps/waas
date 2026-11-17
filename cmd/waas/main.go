// Command waas does the data work of WaaS's workflows: it reads the services
// file, answers what a repository deploys, and routes a push to its
// environment. It prints JSON, and inside GitHub Actions also sets the step's
// outputs and summary.
//
//	waas validate -f services.json
//	waas matrix -f services/shop.json --catalog services/services.json --repository shop
//	waas route --branch dev --environments "$ENVIRONMENTS" --services "$SERVICES"
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

const usage = `waas does the data work of WaaS's workflows.

Commands:
  validate   check a services file
  matrix     what a repository deploys, per environment
  route      the environment a branch deploys to, and its services

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
	case "matrix":
		return matrix(args[1:], stdout)
	case "route":
		return route(args[1:], stdout)
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

func matrix(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("matrix", flag.ContinueOnError)
	var r services.Request
	fs.StringVar(&r.File, "f", "", "the repository's services file; it need not exist")
	fs.StringVar(&r.Catalog, "catalog", "", "shared services file a repository without its own is looked up in")
	fs.StringVar(&r.Repository, "repository", "", "the repository's name, looked up in the catalog")
	fs.StringVar(&r.Service, "service", "", "the service a polyrepo deploys as")
	fs.BoolVar(&r.Monorepo, "monorepo", true, "the repository deploys several services")
	fs.BoolVar(&r.ByName, "by-name", false, "skip the files: deploy -service by its name alone")
	envs := fs.String("environments", "", "environments to list services for, comma-separated (default: the file's metadata.environments, else staging,production)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if r.File == "" && !r.ByName {
		return errors.New("matrix: -f is required (or -by-name)")
	}
	if !r.Monorepo && r.Service == "" {
		return errors.New("matrix: a polyrepo (-monorepo=false) needs -service")
	}
	r.Environments = splitList(*envs)

	m, err := services.Resolve(r)
	if err != nil {
		return err
	}

	all, _ := json.Marshal(m.All)
	byEnv, _ := json.Marshal(m.Environments)
	routes, _ := json.Marshal(m.Routes)
	outs := [][2]string{
		{"monorepo", strconv.FormatBool(m.Monorepo)},
		{"service", m.Service},
		{"all-services", string(all)},
		{"services", string(byEnv)},
		{"environments", string(routes)},
	}
	summary := fmt.Sprintf("## Services\n\n%d from `%s`\n\n| Environment | Services |\n|---|---|\n", len(m.All), sourceOf(m))
	for _, env := range sortedKeys(m.Environments) {
		list, _ := json.Marshal(m.Environments[env])
		outs = append(outs, [2]string{env + "-services", string(list)})
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

func route(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("route", flag.ContinueOnError)
	branch := fs.String("branch", "", "the branch pushed to (required)")
	envsJSON := fs.String("environments", "", "the environments, as matrix's environments output; empty: the two built from the -staging-* and -production-* flags")
	servicesJSON := fs.String("services", "", "services per environment, as matrix's services output")
	monorepo := fs.Bool("monorepo", true, "the repository deploys several services")
	service := fs.String("service", "", "a polyrepo's service, deployed by name when -services has no list for the environment")
	// Two environments for a services file that declares none.
	var staging, production services.Environment
	fs.StringVar(&staging.Name, "staging-env", "staging", "first environment's name")
	fs.StringVar(&staging.Branch, "staging-branch", "dev", "branch that deploys to the first environment")
	fs.StringVar(&staging.Domain, "staging-domain", "staging", "first environment's domain")
	fs.StringVar(&staging.BuildConfiguration, "staging-config", "Release", "first environment's build configuration")
	fs.StringVar(&production.Name, "production-env", "production", "second environment's name")
	fs.StringVar(&production.Branch, "production-branch", "main", "branch that deploys to the second environment")
	fs.StringVar(&production.Domain, "production-domain", "production", "second environment's domain")
	fs.StringVar(&production.BuildConfiguration, "production-config", "Release", "second environment's build configuration")
	if err := fs.Parse(args); err != nil {
		return err
	}

	var envs []services.Environment
	if s := strings.TrimSpace(*envsJSON); s != "" {
		if err := json.Unmarshal([]byte(s), &envs); err != nil {
			return fmt.Errorf("route: -environments: %w", err)
		}
	}
	if len(envs) == 0 {
		envs = []services.Environment{staging, production}
	}
	var byEnv map[string]json.RawMessage
	if s := strings.TrimSpace(*servicesJSON); s != "" {
		if err := json.Unmarshal([]byte(s), &byEnv); err != nil {
			return fmt.Errorf("route: -services: %w", err)
		}
	}

	r, err := services.RouteBranch(envs, *branch)
	if err != nil {
		return err
	}
	out := struct {
		services.Route
		Matrix  json.RawMessage `json:"matrix"`
		Service string          `json:"service"`
	}{Route: r, Matrix: json.RawMessage("[]")}

	if r.Deploy {
		list, listed := byEnv[r.Name]
		switch {
		case listed:
			out.Matrix = list
		case !*monorepo && *service != "":
			// No services file: the polyrepo deploys by its name.
			one, _ := json.Marshal([]map[string]string{{"name": *service}})
			out.Matrix = one
		}
		if !*monorepo {
			out.Service = *service
		}
	}

	from := services.Environment{}
	if r.PromoteFrom != nil {
		from = *r.PromoteFrom
	}
	if err := actions.Output(
		[2]string{"deploy", strconv.FormatBool(r.Deploy)},
		[2]string{"environment", r.Name},
		[2]string{"domain", r.Domain},
		[2]string{"config", r.BuildConfiguration},
		[2]string{"matrix", string(out.Matrix)},
		[2]string{"service", out.Service},
		[2]string{"promote_from_environment", from.Name},
		[2]string{"promote_from_domain", from.Domain},
	); err != nil {
		return err
	}
	summary := fmt.Sprintf("## Route\n\n`%s` deploys nowhere: no environment takes this branch.\n", *branch)
	if r.Deploy {
		summary = fmt.Sprintf("## Route\n\n`%s` deploys to **%s** (domain `%s`, build configuration `%s`).\n", *branch, r.Name, r.Domain, r.BuildConfiguration)
	}
	if err := actions.Summary(summary); err != nil {
		return err
	}
	return printJSON(stdout, out)
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
