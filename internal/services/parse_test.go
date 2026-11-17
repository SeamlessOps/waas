package services

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// catalogYAML is catalog, written in YAML.
const catalogYAML = `
metadata:
  default_environments: [staging, production]
repositories:
  shop:
    project_type: nx
    version: { node: "24.x", pnpm: "9" }
services:
  - repository: shop
    name: web
    version: { node: "22.x" }
  - repository: shop
    name: admin
    environments: [staging]
  - repository: billing
    name: billing
    replicas: 2
  - name: loner
`

func TestFormatOf(t *testing.T) {
	for path, want := range map[string]Format{
		"services.json":    JSON,
		"services.yaml":    YAML,
		"services.yml":     YAML,
		"dir/Services.YML": YAML,
		"services":         JSON,
		"services.txt":     JSON,
	} {
		if got := FormatOf(path); got != want {
			t.Errorf("FormatOf(%q) = %s, want %s", path, got, want)
		}
	}
}

func TestParse_YAMLSameAsJSON(t *testing.T) {
	fromJSON, err := Parse("s.json", []byte(catalog))
	if err != nil {
		t.Fatal(err)
	}
	fromYAML, err := Parse("s.yaml", []byte(catalogYAML))
	if err != nil {
		t.Fatal(err)
	}
	fromYAML.Path = fromJSON.Path
	if !reflect.DeepEqual(fromYAML, fromJSON) {
		t.Errorf("YAML gave %+v\nJSON gave %+v", fromYAML, fromJSON)
	}
}

func TestParse_YAMLNumbersPassThrough(t *testing.T) {
	f, err := Parse("s.yml", []byte(catalogYAML))
	if err != nil {
		t.Fatal(err)
	}
	if got := f.Services[2]["replicas"]; got != json.Number("2") {
		t.Errorf("replicas = %#v, want json.Number(\"2\")", got)
	}
}

func TestParse_DecodeProblems(t *testing.T) {
	for name, tc := range map[string]struct{ path, doc, want string }{
		"invalid YAML":         {"s.yaml", "services: [\n  - name: a\n bad", "invalid YAML"},
		"YAML not an object":   {"s.yaml", "- a\n- b\n", "invalid YAML"},
		"JSON two values":      {"s.json", `{"services": []} {}`, "invalid JSON: more than one value"},
		"JSON not an object":   {"s.json", `[]`, "invalid JSON"},
		"YAML read as JSON":    {"s.json", catalogYAML, "invalid JSON"},
		"YAML checks the same": {"s.yaml", "services: {}\n", `"services" must be a list`},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Parse(tc.path, []byte(tc.doc))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want it to mention %q", err, tc.want)
			}
		})
	}
}

func TestLoad_YAMLByExtension(t *testing.T) {
	dir := write(t, map[string]string{"services.yml": catalogYAML})
	f, err := Load(filepath.Join(dir, "services.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := names(f.Services), []string{"web", "admin", "billing", "loner"}; !reflect.DeepEqual(got, want) {
		t.Errorf("names = %v, want %v", got, want)
	}
}

func TestFind(t *testing.T) {
	for name, tc := range map[string]struct {
		files   []string
		path    string
		want    string // "" when an error is wanted
		wantErr string
	}{
		"exact JSON":              {[]string{"services.json"}, "services.json", "services.json", ""},
		"exact wins over sibling": {[]string{"services.json", "services.yaml"}, "services.json", "services.json", ""},
		"base finds YAML":         {[]string{"services.yaml"}, "services", "services.yaml", ""},
		"base finds yml":          {[]string{"services.yml"}, "services", "services.yml", ""},
		"base finds JSON":         {[]string{"services.json"}, "services", "services.json", ""},
		"JSON path finds YAML":    {[]string{"services.yaml"}, "services.json", "services.yaml", ""},
		"YAML path finds JSON":    {[]string{"services.json"}, "services.yaml", "services.json", ""},
		"both: ambiguous":         {[]string{"services.json", "services.yaml"}, "services", "", "ambiguous"},
		"none":                    {nil, "services", "", "not found"},
		"other extension kept":    {[]string{"services.json"}, "services.txt", "", "not found"},
	} {
		t.Run(name, func(t *testing.T) {
			files := map[string]string{}
			for _, f := range tc.files {
				files[f] = `{"services": []}`
			}
			dir := write(t, files)
			got, err := Find(filepath.Join(dir, tc.path))
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want it to mention %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if want := filepath.Join(dir, tc.want); got != want {
				t.Errorf("Find = %s, want %s", got, want)
			}
		})
	}
}

func TestLoad_NotFoundIsNotExist(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "services"))
	if !IsNotExist(err) {
		t.Errorf("err = %v, want IsNotExist", err)
	}
}

func TestDefaultEnvironments(t *testing.T) {
	declared := Environments{{Name: "dev"}, {Name: "prod"}}
	for name, tc := range map[string]struct {
		v        any
		declared Environments
		want     []string
	}{
		"listed wins":         {[]any{"qa"}, declared, []string{"qa"}},
		"declared":            {nil, declared, []string{"dev", "prod"}},
		"neither":             {nil, nil, DefaultEnvironments},
		"not a list: ignored": {"qa", nil, DefaultEnvironments},
	} {
		t.Run(name, func(t *testing.T) {
			if got := defaultEnvironments(tc.v, tc.declared); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}
