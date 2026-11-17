package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// outputs runs args with GITHUB_OUTPUT set and returns the outputs written.
func outputs(t *testing.T, args ...string) map[string]string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "out")
	t.Setenv("GITHUB_OUTPUT", path)
	t.Setenv("GITHUB_STEP_SUMMARY", filepath.Join(t.TempDir(), "summary"))
	var stdout bytes.Buffer
	if err := run(args, &stdout); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	lines := strings.Split(string(raw), "\n")
	for i := 0; i < len(lines); i++ {
		key, delim, ok := strings.Cut(lines[i], "<<")
		if !ok {
			continue
		}
		var val []string
		for i++; i < len(lines) && lines[i] != delim; i++ {
			val = append(val, lines[i])
		}
		out[key] = strings.Join(val, "\n")
	}
	return out
}

func TestPlan_Outputs(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "s.json")
	doc := `{"metadata": {"environments": [{"name": "staging", "branch": "dev", "domain": "s"}, {"name": "live", "branch": "main", "domain": "l", "build_configuration": "Production"}]},
	  "services": [{"name": "a"}, {"name": "b", "environments": ["staging"]}]}`
	if err := os.WriteFile(file, []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	got := outputs(t, "plan", "-f", file, "--branch", "main")
	for k, want := range map[string]string{
		"deploy": "true", "environment": "live", "domain": "l", "config": "Production",
		"promote_from_environment": "staging", "promote_from_domain": "s",
		// Each service carries where its image is written (gitops) and read for promotion (gitops_from).
		"matrix": `[{` +
			`"gitops":{"path":"environment/live","key":".\"services\".\"a-l\"","tag":".\"services\".\"a-l\".\"tag\""},` +
			`"gitops_from":{"path":"environment/staging","key":".\"services\".\"a-s\"","tag":".\"services\".\"a-s\".\"tag\""},` +
			`"name":"a"}]`,
		"monorepo": "true", "staging-services": `[{"name":"a"},{"environments":["staging"],"name":"b"}]`,
	} {
		if got[k] != want {
			t.Errorf("%s = %q, want %q", k, got[k], want)
		}
	}
	// The branch defaults to GITHUB_REF_NAME.
	t.Setenv("GITHUB_REF_NAME", "dev")
	if got := outputs(t, "plan", "-f", file); got["environment"] != "staging" {
		t.Errorf("from GITHUB_REF_NAME: %v", got["environment"])
	}
}

func TestPlan_GitOpsSummary(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "s.json")
	doc := `{"metadata": {"environments": [{"name": "live", "branch": "main", "domain": "l",
	    "values": {"path": "clusters/{environment}/apps.yaml", "image": {"ref": "image"}}}]},
	  "services": [{"name": "a"}, {"name": "b", "values": {"key": "apps.{name}", "image": {"repository": "image.repository", "tag": "image.tag", "digest": "image.digest"}}}]}`
	if err := os.WriteFile(file, []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	summary := filepath.Join(dir, "summary")
	t.Setenv("GITHUB_OUTPUT", filepath.Join(dir, "out"))
	t.Setenv("GITHUB_STEP_SUMMARY", summary)
	if err := run([]string{"plan", "-f", file, "--branch", "main"}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(summary)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"| `a` | `clusters/live/apps.yaml` | `services.a-l` | full image |",
		"| `b` | `clusters/live/apps.yaml` | `apps.b` | repository, tag and digest |",
	} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("summary lacks %q:\n%s", want, raw)
		}
	}
}

func TestPlan_SigningOutputs(t *testing.T) {
	dir := t.TempDir()
	write := func(signing string) string {
		f := filepath.Join(dir, "s.json")
		doc := `{"metadata": {` + signing + `}, "services": [{"name": "a"}]}`
		if err := os.WriteFile(f, []byte(doc), 0o644); err != nil {
			t.Fatal(err)
		}
		return f
	}
	got := outputs(t, "plan", "-f", write(`"signing": {"key": "gcpkms://k"}`), "--branch", "main")
	if got["signing_key"] != "gcpkms://k" || got["signing_keyless"] != "" {
		t.Errorf("key: %v", got)
	}
	got = outputs(t, "plan", "-f", write(`"signing": {"keyless": {"enabled": true}}`), "--branch", "main")
	if got["signing_keyless"] != "true" || got["signing_identity"] != "^https://github.com/" || got["signing_issuer"] != "https://token.actions.githubusercontent.com" {
		t.Errorf("keyless: %v", got)
	}
	got = outputs(t, "plan", "-f", write(`"default_environments": ["staging", "production"]`), "--branch", "main")
	if got["signing_key"] != "" || got["signing_keyless"] != "" {
		t.Errorf("unsigned: %v", got)
	}
}
