package services

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const withEnvironments = `{
  "metadata": {
    "environments": [
      {"name": "uat", "branch": "develop", "domain": "shop-uat", "build_configuration": "Debug"},
      {"name": "staging", "branch": "release", "domain": "shop-stg", "build_configuration": "Release"},
      {"name": "production", "branch": "main", "domain": "shop", "build_configuration": "Release"}
    ]
  },
  "repositories": {
    "legacy": {"project_type": "dotnet", "branches": {"dev": "staging", "master": "production"}}
  },
  "services": [
    {"repository": "shop", "name": "web"},
    {"repository": "shop", "name": "admin", "environments": ["uat", "staging"]},
    {"repository": "legacy", "name": "old"}
  ]
}`

func TestEnvironments_Parsed(t *testing.T) {
	f, err := Parse("s.json", []byte(withEnvironments))
	if err != nil {
		t.Fatal(err)
	}
	want := []Environment{
		{Name: "uat", Branch: "develop", Domain: "shop-uat", BuildConfiguration: "Debug"},
		{Name: "staging", Branch: "release", Domain: "shop-stg", BuildConfiguration: "Release"},
		{Name: "production", Branch: "main", Domain: "shop", BuildConfiguration: "Release"},
	}
	if !reflect.DeepEqual(f.Environments, want) {
		t.Errorf("environments = %+v", f.Environments)
	}
}

func TestEnvironmentsFor_RepositoryBranchesReplaceTheFiles(t *testing.T) {
	f, err := Parse("s.json", []byte(withEnvironments))
	if err != nil {
		t.Fatal(err)
	}
	branches := func(envs []Environment) []string {
		var out []string
		for _, e := range envs {
			out = append(out, e.Name+"="+e.Branch)
		}
		return out
	}
	if got, want := branches(f.EnvironmentsFor("shop")), []string{"uat=develop", "staging=release", "production=main"}; !reflect.DeepEqual(got, want) {
		t.Errorf("shop: %v, want %v", got, want)
	}
	// legacy maps dev and master; uat, which it doesn't map, takes no branch.
	if got, want := branches(f.EnvironmentsFor("legacy")), []string{"uat=", "staging=dev", "production=master"}; !reflect.DeepEqual(got, want) {
		t.Errorf("legacy: %v, want %v", got, want)
	}
	// branches is routing, not a field of the repository's services.
	if _, has := f.WithDefaults(f.Services[2])["branches"]; has {
		t.Error("branches merged into a service")
	}
}

func TestEnvironments_Problems(t *testing.T) {
	for name, tc := range map[string]struct{ doc, want string }{
		"not a list":       {`{"metadata": {"environments": {}}, "services": []}`, "metadata.environments must be a list"},
		"no name":          {`{"metadata": {"environments": [{"branch": "dev"}]}, "services": []}`, "[0] is missing its name"},
		"name twice":       {`{"metadata": {"environments": [{"name": "a"}, {"name": "a"}]}, "services": []}`, "a is listed twice"},
		"branch twice":     {`{"metadata": {"environments": [{"name": "a", "branch": "dev"}, {"name": "b", "branch": "dev"}]}, "services": []}`, "branch dev deploys to both a and b"},
		"field not text":   {`{"metadata": {"environments": [{"name": "a", "domain": 1}]}, "services": []}`, "[0].domain must be text"},
		"unknown env":      {`{"metadata": {"environments": [{"name": "a"}]}, "repositories": {"r": {"branches": {"dev": "b"}}}, "services": []}`, "b is not an environment"},
		"two branches":     {`{"metadata": {"environments": [{"name": "a"}]}, "repositories": {"r": {"branches": {"dev": "a", "x": "a"}}}, "services": []}`, "both deploy to a"},
		"no environments":  {`{"repositories": {"r": {"branches": {"dev": "a"}}}, "services": []}`, "needs metadata.environments"},
		"branches not map": {`{"metadata": {"environments": [{"name": "a"}]}, "repositories": {"r": {"branches": ["dev"]}}, "services": []}`, "must be an object of branch"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Parse("s.json", []byte(tc.doc))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want it to mention %q", err, tc.want)
			}
		})
	}
}

func TestRouteBranch(t *testing.T) {
	f, err := Parse("s.json", []byte(withEnvironments))
	if err != nil {
		t.Fatal(err)
	}
	envs := f.EnvironmentsFor("shop")

	r, err := RouteBranch(envs, "main")
	if err != nil || !r.Deploy || r.Name != "production" || r.Domain != "shop" {
		t.Fatalf("main: %+v, %v", r, err)
	}
	if r.PromoteFrom == nil || r.PromoteFrom.Name != "staging" || r.PromoteFrom.Domain != "shop-stg" {
		t.Errorf("main promotes from %+v, want staging", r.PromoteFrom)
	}
	if r, _ := RouteBranch(envs, "develop"); r.PromoteFrom != nil {
		t.Errorf("the first environment promotes from nothing, got %+v", r.PromoteFrom)
	}
	if r, err := RouteBranch(envs, "feature/x"); err != nil || r.Deploy {
		t.Errorf("an unmapped branch: %+v, %v; want no deploy and no error", r, err)
	}
	if _, err := RouteBranch(envs, ""); err == nil {
		t.Error("an empty branch routed")
	}
}

func TestResolve_EnvironmentsFromTheFile(t *testing.T) {
	dir := write(t, map[string]string{"services.json": withEnvironments})
	m, err := Resolve(Request{File: filepath.Join(dir, "shop.json"), Catalog: filepath.Join(dir, "services.json"), Repository: "shop", Service: "shop"})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string][]string{}
	for env, list := range m.Environments {
		got[env] = names(list)
	}
	want := map[string][]string{"uat": {"admin"}, "staging": {"admin"}, "production": {"web"}}
	// web has no environments of its own: it takes the file's defaults,
	// staging and production; admin lists uat and staging.
	want["staging"] = []string{"web", "admin"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("services per environment = %v, want %v", got, want)
	}
	if len(m.Routes) != 3 || m.Routes[0].Branch != "develop" {
		t.Errorf("routes = %+v", m.Routes)
	}
}
