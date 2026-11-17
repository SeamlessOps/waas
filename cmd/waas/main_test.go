package main

import (
	"bytes"
	"encoding/json"
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

// Without environments from the file, the flags give the two defaults.
func TestRoute_Defaults(t *testing.T) {
	services := `{"staging": [{"name": "a"}], "production": [{"name": "a"}, {"name": "b"}]}`
	got := outputs(t, "route", "--branch", "main", "--services", services)
	for k, want := range map[string]string{
		"deploy": "true", "environment": "production", "domain": "production", "config": "Release",
		"promote_from_environment": "staging", "promote_from_domain": "staging",
	} {
		if got[k] != want {
			t.Errorf("%s = %q, want %q", k, got[k], want)
		}
	}
	var matrix []map[string]any
	if err := json.Unmarshal([]byte(got["matrix"]), &matrix); err != nil || len(matrix) != 2 {
		t.Errorf("matrix = %s", got["matrix"])
	}
}

// The caller's inputs replace every default.
func TestRoute_Flags(t *testing.T) {
	got := outputs(t, "route", "--branch", "develop", "--staging-branch", "develop", "--staging-env", "dev",
		"--staging-domain", "shop-dev", "--staging-config", "Debug")
	if got["environment"] != "dev" || got["domain"] != "shop-dev" || got["config"] != "Debug" {
		t.Errorf("outputs = %v", got)
	}
}

func TestRoute_FileEnvironments(t *testing.T) {
	envs := `[{"name":"uat","branch":"develop","domain":"u"},{"name":"live","branch":"trunk","domain":"l"}]`
	got := outputs(t, "route", "--branch", "trunk", "--environments", envs, "--services", `{"live":[{"name":"x"}]}`)
	if got["environment"] != "live" || got["domain"] != "l" || got["promote_from_environment"] != "uat" || got["matrix"] != `[{"name":"x"}]` {
		t.Errorf("outputs = %v", got)
	}
	// The defaults' branches mean nothing once the file declares environments.
	if got := outputs(t, "route", "--branch", "main", "--environments", envs); got["deploy"] != "false" || got["matrix"] != "[]" {
		t.Errorf("main with file environments: %v", got)
	}
}

func TestRoute_Polyrepo(t *testing.T) {
	// Nothing loaded: deployed by name.
	got := outputs(t, "route", "--branch", "dev", "--monorepo=false", "--service", "api")
	if got["matrix"] != `[{"name":"api"}]` || got["service"] != "api" {
		t.Errorf("by name: %v", got)
	}
	// Loaded, and left out of this environment: nothing to deploy.
	got = outputs(t, "route", "--branch", "dev", "--monorepo=false", "--service", "api", "--services", `{"staging": []}`)
	if got["matrix"] != "[]" {
		t.Errorf("left out: matrix = %s", got["matrix"])
	}
}

func TestRoute_UnmappedBranchDoesNotDeploy(t *testing.T) {
	got := outputs(t, "route", "--branch", "feature/x")
	if got["deploy"] != "false" || got["environment"] != "" || got["matrix"] != "[]" {
		t.Errorf("outputs = %v", got)
	}
}
