package services

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const catalog = `{
  "metadata": {"default_environments": ["staging", "production"]},
  "repositories": {
    "shop": {"project_type": "nx", "version": {"node": "24.x", "pnpm": "9"}}
  },
  "services": [
    {"repository": "shop", "name": "web", "version": {"node": "22.x"}},
    {"repository": "shop", "name": "admin", "environments": ["staging"]},
    {"repository": "billing", "name": "billing", "replicas": 2},
    {"name": "loner"}
  ]
}`

// write puts files (name → content) in a temp dir and returns its path.
func write(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func names(entries Entries) []string {
	out := []string{}
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

func TestParse_Problems(t *testing.T) {
	for name, tc := range map[string]struct{ doc, want string }{
		"invalid JSON":            {`{"services": [`, "invalid JSON"},
		"no services key":         {`{}`, `missing the "services" key`},
		"services not a list":     {`{"services": {}}`, `"services" must be a list`},
		"entry without name":      {`{"services": [{"name": "a"}, {"name": ""}]}`, "index 1 is missing its name"},
		"environments not list":   {`{"services": [{"name": "a", "environments": "staging"}]}`, "a: environments must be a list"},
		"repositories not object": {`{"repositories": [], "services": []}`, `"repositories" must be an object`},
		"repository defaults":     {`{"repositories": {"shop": 1}, "services": []}`, "repositories.shop must be an object"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Parse("s.json", []byte(tc.doc))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want it to mention %q", err, tc.want)
			}
		})
	}
	// Every problem at once, not only the first.
	_, err := Parse("s.json", []byte(`{"services": [{"name": ""}, {"name": "b", "environments": 1}]}`))
	if inv, ok := err.(*Invalid); !ok || len(inv.Problems) != 2 {
		t.Errorf("err = %v, want both problems", err)
	}
}

func TestWithDefaults_MergesObjectsEntryWins(t *testing.T) {
	f, err := Parse("s.json", []byte(catalog))
	if err != nil {
		t.Fatal(err)
	}
	got := f.WithDefaults(f.Services[0])
	want := Entry{
		"repository": "shop", "name": "web", "project_type": "nx",
		"version": map[string]any{"node": "22.x", "pnpm": "9"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v\nwant %v", got, want)
	}
	if f.Services[0]["project_type"] != nil {
		t.Error("the file's entry was changed")
	}
}

func TestResolve(t *testing.T) {
	envs := []string{"staging", "production"}
	dir := write(t, map[string]string{
		"services.json": catalog,
		"mono.json":     `{"services": [{"name": "a"}, {"name": "b", "environments": ["production"]}]}`,
		"poly.json":     `{"services": [{"name": "a"}, {"name": "b"}]}`,
	})
	p := func(name string) string { return filepath.Join(dir, name) }

	for name, tc := range map[string]struct {
		req                      Request
		monorepo                 bool
		service                  string
		all, staging, production []string
	}{
		"by name": {
			req:     Request{ByName: true, Service: "solo"},
			service: "solo",
			all:     []string{"solo"}, staging: []string{"solo"}, production: []string{"solo"},
		},
		"own file, monorepo": {
			req:      Request{File: p("mono.json"), Monorepo: true},
			monorepo: true,
			all:      []string{"a", "b"}, staging: []string{"a"}, production: []string{"a", "b"},
		},
		"own file, polyrepo takes its entry": {
			req:     Request{File: p("poly.json"), Service: "b"},
			service: "b",
			all:     []string{"b"}, staging: []string{"b"}, production: []string{"b"},
		},
		"catalog, several entries are a monorepo": {
			req:      Request{File: p("shop.json"), Catalog: p("services.json"), Repository: "shop", Service: "shop", Monorepo: false},
			monorepo: true,
			all:      []string{"web", "admin"}, staging: []string{"web", "admin"}, production: []string{"web"},
		},
		"catalog, the service input picks one of the repository's": {
			req:     Request{File: p("shop.json"), Catalog: p("services.json"), Repository: "shop", Service: "admin", Monorepo: true},
			service: "admin",
			all:     []string{"admin"}, staging: []string{"admin"}, production: []string{},
		},
		"catalog, one entry is a polyrepo under the entry's name": {
			req:     Request{File: p("billing.json"), Catalog: p("services.json"), Repository: "billing-svc", Service: "billing", Monorepo: true},
			service: "billing",
			all:     []string{"billing"}, staging: []string{"billing"}, production: []string{"billing"},
		},
		"catalog by repository": {
			req:     Request{File: p("x.json"), Catalog: p("services.json"), Repository: "billing", Service: "billing-repo"},
			service: "billing",
			all:     []string{"billing"}, staging: []string{"billing"}, production: []string{"billing"},
		},
		"no file, not in the catalog, polyrepo deploys by name": {
			req:     Request{File: p("x.json"), Catalog: p("services.json"), Repository: "x", Service: "x"},
			service: "x",
			all:     []string{"x"}, staging: []string{"x"}, production: []string{"x"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			tc.req.Environments = envs
			m, err := Resolve(tc.req)
			if err != nil {
				t.Fatal(err)
			}
			if m.Monorepo != tc.monorepo || m.Service != tc.service {
				t.Errorf("monorepo, service = %v, %q; want %v, %q", m.Monorepo, m.Service, tc.monorepo, tc.service)
			}
			for what, pair := range map[string][2][]string{
				"all":        {names(m.All), tc.all},
				"staging":    {names(m.Environments["staging"]), tc.staging},
				"production": {names(m.Environments["production"]), tc.production},
			} {
				if !reflect.DeepEqual(pair[0], pair[1]) {
					t.Errorf("%s = %v, want %v", what, pair[0], pair[1])
				}
			}
		})
	}
}

func TestResolve_Errors(t *testing.T) {
	dir := write(t, map[string]string{
		"services.json": catalog,
		"poly.json":     `{"services": [{"name": "a"}]}`,
		"empty.json":    `{"services": []}`,
	})
	p := func(name string) string { return filepath.Join(dir, name) }
	for name, tc := range map[string]struct {
		req  Request
		want string
	}{
		"monorepo with no file and not in the catalog": {Request{File: p("x.json"), Catalog: p("services.json"), Repository: "x", Monorepo: true}, "not in the catalog"},
		"polyrepo's service not in its file":           {Request{File: p("poly.json"), Service: "b"}, `no service named "b"`},
		"no services":                                  {Request{File: p("empty.json"), Monorepo: true}, "at least one"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Resolve(tc.req); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want it to mention %q", err, tc.want)
			}
		})
	}
}

// Fields WaaS doesn't read pass through, numbers as written.
func TestResolve_PassesFieldsThrough(t *testing.T) {
	dir := write(t, map[string]string{"s.json": `{"services": [{"name": "a", "replicas": 2, "ratio": 0.10, "extra": {"k": [1]}}]}`})
	m, err := Resolve(Request{File: filepath.Join(dir, "s.json"), Monorepo: true})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := json.Marshal(m.All)
	if want := `[{"extra":{"k":[1]},"name":"a","ratio":0.10,"replicas":2}]`; string(got) != want {
		t.Errorf("got %s, want %s", got, want)
	}
}
