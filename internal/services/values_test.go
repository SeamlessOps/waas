package services

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// withValues sets values at every layer: staging writes the full image into
// one file, the legacy repository its own key and a repository with a tag,
// and its "pinned" service a digest beside the tag.
const withValues = `{
  "metadata": {
    "environments": [
      {"name": "staging", "branch": "dev", "domain": "shop-stg",
       "values": {"path": "clusters/{environment}/apps.yaml", "image": {"ref": "image"}}},
      {"name": "production", "branch": "main", "domain": "shop"}
    ]
  },
  "repositories": {
    "legacy": {"project_type": "dotnet",
               "values": {"key": "apps.{name}", "image": {"repository": "image.repository", "tag": "image.tag"}}}
  },
  "services": [
    {"repository": "shop", "name": "web"},
    {"repository": "legacy", "name": "old"},
    {"repository": "legacy", "name": "pinned", "values": {"image": {"tag": "image.tag", "digest": "image.digest"}}}
  ]
}`

func TestTargetFor_Default(t *testing.T) {
	got, err := (*File)(nil).TargetFor(Environment{Name: "staging", Domain: "shop-stg"}, Entry{"name": "web"})
	if err != nil {
		t.Fatal(err)
	}
	want := Target{
		Path: "environment/staging",
		Key:  `."services"."web-shop-stg"`,
		Tag:  `."services"."web-shop-stg"."tag"`,
	}
	if got != want {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
}

func TestTargetFor_Layers(t *testing.T) {
	f, err := Parse("s.json", []byte(withValues))
	if err != nil {
		t.Fatal(err)
	}
	staging, production := f.Environments[0], f.Environments[1]
	entry := func(name string) Entry {
		for _, e := range f.Services {
			if e.Name() == name {
				return f.WithDefaults(e)
			}
		}
		t.Fatalf("no entry %s", name)
		return nil
	}
	for name, tc := range map[string]struct {
		env  Environment
		e    string
		want Target
	}{
		"environment's file and full image": {staging, "web", Target{
			Path: "clusters/staging/apps.yaml",
			Key:  `."services"."web-shop-stg"`,
			Ref:  `."services"."web-shop-stg"."image"`,
		}},
		"default where nothing is set": {production, "web", Target{
			Path: "environment/production",
			Key:  `."services"."web-shop"`,
			Tag:  `."services"."web-shop"."tag"`,
		}},
		// The repository's image replaces the environment's ref whole; its key replaces the default's.
		"repository over environment": {staging, "old", Target{
			Path:       "clusters/staging/apps.yaml",
			Key:        `."apps"."old"`,
			Tag:        `."apps"."old"."image"."tag"`,
			Repository: `."apps"."old"."image"."repository"`,
		}},
		// The entry's image replaces the repository's whole: no repository.
		"entry over repository": {production, "pinned", Target{
			Path:   "environment/production",
			Key:    `."apps"."pinned"`,
			Tag:    `."apps"."pinned"."image"."tag"`,
			Digest: `."apps"."pinned"."image"."digest"`,
		}},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := f.TargetFor(tc.env, entry(tc.e))
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("got %+v\nwant %+v", got, tc.want)
			}
		})
	}
}

func TestTargetFor_NameNotAKeySegment(t *testing.T) {
	_, err := (*File)(nil).TargetFor(Environment{Name: "staging", Domain: "stg"}, Entry{"name": `a"b`})
	if err == nil || !strings.Contains(err.Error(), "values.key") {
		t.Errorf("err = %v, want a values.key problem", err)
	}
}

func TestParse_ValuesProblems(t *testing.T) {
	doc := func(values string) string {
		return `{"metadata": {"environments": [{"name": "staging", "values": ` + values + `}]}, "services": []}`
	}
	for name, tc := range map[string]struct{ doc, want string }{
		"not an object":       {doc(`"x"`), "values must be an object"},
		"unknown field":       {doc(`{"file": "a.yaml"}`), "values.file is not a values field"},
		"path not text":       {doc(`{"path": 1}`), "values.path must be text"},
		"absolute path":       {doc(`{"path": "/etc/values.yaml"}`), "is not a relative path"},
		"path leaves":         {doc(`{"path": "../other/values.yaml"}`), "leaves the GitOps repository"},
		"path with spaces":    {doc(`{"path": "my values.yaml"}`), "is not a relative path"},
		"unknown placeholder": {doc(`{"key": "services.{service}"}`), "{service} is not a placeholder"},
		"key with a quote":    {doc(`{"key": "services.\"x\""}`), "is not a dotted path"},
		"empty key segment":   {doc(`{"key": "services..x"}`), "is not a dotted path"},
		"unknown image field": {doc(`{"image": {"name": "image.name"}}`), "image.name is not an image field"},
		"image field path":    {doc(`{"image": {"tag": "image tag"}}`), "is not a dotted path"},
		"ref beside tag":      {doc(`{"image": {"ref": "image", "tag": "tag"}}`), "ref is the whole image"},
		"repository alone":    {doc(`{"image": {"repository": "image.repository"}}`), "repository needs tag"},
		"empty image":         {doc(`{"image": {}}`), "set tag, ref or digest"},
		"on a repository": {`{"repositories": {"shop": {"values": {"key": "a b"}}}, "services": []}`,
			"repositories.shop.values.key"},
		"on an entry": {`{"services": [{"name": "a", "values": {"image": {"ref": "x", "repository": "y"}}}]}`,
			"service at index 0: values.image"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Parse("s.json", []byte(tc.doc))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want it to mention %q", err, tc.want)
			}
		})
	}
}

func TestWithDefaults_LeavesRepositoryValuesOut(t *testing.T) {
	f, err := Parse("s.json", []byte(withValues))
	if err != nil {
		t.Fatal(err)
	}
	if got := f.WithDefaults(f.Services[1]); got["values"] != nil {
		t.Errorf("old over its repository has values %v; the repository's are where it deploys, not a field", got["values"])
	}
	if got := f.WithDefaults(f.Services[2]); got["values"] == nil {
		t.Error("pinned lost its own values")
	}
}

func TestPlanPush_Targets(t *testing.T) {
	dir := write(t, map[string]string{"services.json": withValues})
	r := Request{File: filepath.Join(dir, "none"), Catalog: filepath.Join(dir, "services.json"), Repository: "shop", Service: "web"}

	p, err := PlanPush(r, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Services) != 1 {
		t.Fatalf("services = %v", names(p.Services))
	}
	web := p.Services[0]
	if got, want := web["gitops"], (Target{Path: "environment/production", Key: `."services"."web-shop"`, Tag: `."services"."web-shop"."tag"`}); !reflect.DeepEqual(got, want) {
		t.Errorf("gitops = %+v\nwant %+v", got, want)
	}
	// Promotion reads staging the way staging is written: the full image, in its file.
	if got, want := web["gitops_from"], (Target{Path: "clusters/staging/apps.yaml", Key: `."services"."web-shop-stg"`, Ref: `."services"."web-shop-stg"."image"`}); !reflect.DeepEqual(got, want) {
		t.Errorf("gitops_from = %+v\nwant %+v", got, want)
	}
	// The matrix's entries are left as they were.
	for _, e := range p.Matrix.All {
		if _, has := e["gitops"]; has {
			t.Errorf("matrix entry %s gained gitops", e.Name())
		}
	}

	// The first environment promotes from none.
	p, err = PlanPush(r, "dev")
	if err != nil {
		t.Fatal(err)
	}
	if _, has := p.Services[0]["gitops_from"]; has {
		t.Errorf("staging has gitops_from: %v", p.Services[0]["gitops_from"])
	}
}
