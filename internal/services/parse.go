package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"sigs.k8s.io/yaml"
)

// Format is how a services file is written.
type Format string

const (
	JSON Format = "JSON"
	YAML Format = "YAML"
)

// FormatOf is the format of the file at path: YAML for .yaml and .yml,
// JSON otherwise.
func FormatOf(path string) Format {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".yaml", ".yml":
		return YAML
	default:
		return JSON
	}
}

// Extensions are the extensions Find tries, in order.
var Extensions = []string{".yaml", ".yml", ".json"}

// Find is the services file path names. A file at path itself wins.
// Otherwise path is a base name, with or without one of Extensions, and Find
// is the one file of base plus an extension that exists: "services" or
// "services.json" finds services.yaml when only that is there. Two or more
// is an error, and none is one IsNotExist reports.
func Find(path string) (string, error) {
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}
	base := trimExtension(path)
	var found []string
	for _, ext := range Extensions {
		if _, err := os.Stat(base + ext); err == nil {
			found = append(found, base+ext)
		}
	}
	switch len(found) {
	case 0:
		return "", fmt.Errorf("services file %s not found (tried %s): %w", path, strings.Join(Extensions, ", "), os.ErrNotExist)
	case 1:
		return found[0], nil
	default:
		which := "all"
		if len(found) == 2 {
			which = "both"
		}
		return "", fmt.Errorf("services file %s is ambiguous: %s %s exist; keep one", path, strings.Join(found, ", "), which)
	}
}

// trimExtension is path without its extension when it is one of Extensions.
func trimExtension(path string) string {
	ext := filepath.Ext(path)
	for _, known := range Extensions {
		if strings.EqualFold(ext, known) {
			return strings.TrimSuffix(path, ext)
		}
	}
	return path
}

// Load finds (see Find) and reads the services file at path, JSON or YAML by
// its extension, and checks it: a "services" list, a name on every entry,
// environments (when set) a list, and each repository's defaults an object.
// An *Invalid lists everything wrong.
func Load(path string) (*File, error) {
	found, err := Find(path)
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(found)
	if err != nil {
		return nil, err
	}
	return Parse(found, raw)
}

// Parse is Load on raw, read from path.
func Parse(path string, raw []byte) (*File, error) {
	doc, err := decode(FormatOf(path), raw)
	if err != nil {
		return nil, &Invalid{Path: path, Problems: []string{err.Error()}}
	}

	f := &File{Path: path, DefaultEnvironments: DefaultEnvironments, Repositories: map[string]any{}}
	inv := &Invalid{Path: path}

	f.Services = parseServices(doc, inv.add)
	if repos := parseRepositories(doc["repositories"], inv.add); repos != nil {
		f.Repositories = repos
	}
	parseMetadata(f, doc["metadata"], inv.add)
	checkBranches(f, inv.add)

	if len(inv.Problems) > 0 {
		return nil, inv
	}
	return f, nil
}

// add records one problem.
func (e *Invalid) add(format string, args ...any) {
	e.Problems = append(e.Problems, fmt.Sprintf(format, args...))
}

// decode reads raw as one document in format. YAML is converted to JSON
// first, so both give the same types: objects as map[string]any, lists as
// []any, and numbers as json.Number, passed through as written.
func decode(format Format, raw []byte) (map[string]any, error) {
	if format == YAML {
		converted, err := yaml.YAMLToJSON(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid YAML: %w", err)
		}
		raw = converted
	}
	doc, err := decodeJSON(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid %s: %w", format, err)
	}
	return doc, nil
}

// decodeJSON reads raw as exactly one JSON object.
func decodeJSON(raw []byte) (map[string]any, error) {
	var doc map[string]any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber() // numbers pass through as written
	if err := dec.Decode(&doc); err != nil {
		return nil, err
	}
	if dec.More() {
		return nil, fmt.Errorf("more than one value")
	}
	return doc, nil
}

// parseServices reads the "services" list, keeping every object entry in
// file order.
func parseServices(doc map[string]any, add func(string, ...any)) Entries {
	list, ok := doc["services"]
	if !ok {
		add(`missing the "services" key: {"services": [{"name": "service-name", "environments": ["staging", "production"]}]}`)
		return nil
	}
	items, isList := list.([]any)
	if !isList {
		add(`"services" must be a list`)
		return nil
	}
	var out Entries
	for i, it := range items {
		obj, isObj := it.(map[string]any)
		if !isObj {
			add("service at index %d is not an object", i)
			continue
		}
		e := Entry(obj)
		checkEntry(i, e, add)
		out = append(out, e)
	}
	return out
}

// checkEntry checks the fields WaaS reads on the entry at index i.
func checkEntry(i int, e Entry, add func(string, ...any)) {
	if v, set := e["name"]; !set || v == nil || v == "" {
		add("service at index %d is missing its name", i)
	}
	if v, set := e["environments"]; set && v != nil {
		if _, isList := v.([]any); !isList {
			add("service %s: environments must be a list", e.Name())
		}
	}
	parseValues(fmt.Sprintf("service at index %d: values", i), e["values"], add)
}

// parseRepositories reads "repositories", each repository's defaults; nil
// when unset or not an object.
func parseRepositories(v any, add func(string, ...any)) map[string]any {
	if v == nil {
		return nil
	}
	repos, isObj := v.(map[string]any)
	if !isObj {
		add(`"repositories" must be an object of each repository's defaults`)
		return nil
	}
	for name, d := range repos {
		obj, isObj := d.(map[string]any)
		if !isObj {
			add("repositories.%s must be an object", name)
			continue
		}
		parseValues("repositories."+name+".values", obj["values"], add)
	}
	return repos
}

// parseMetadata reads "metadata" into f: environments, signing and the
// default environments.
func parseMetadata(f *File, v any, add func(string, ...any)) {
	md, isObj := v.(map[string]any)
	if !isObj {
		return
	}
	f.Environments = parseEnvironments(md["environments"], add)
	f.Signing = parseSigning(md["signing"], add)
	f.DefaultEnvironments = defaultEnvironments(md["default_environments"], f.Environments)
}

// defaultEnvironments is metadata.default_environments when it is a list,
// else every declared environment, else DefaultEnvironments.
func defaultEnvironments(v any, declared Environments) []string {
	if envs, isList := v.([]any); isList {
		return stringsOf(envs)
	}
	if len(declared) == 0 {
		return DefaultEnvironments
	}
	// An entry without environments deploys to every one declared.
	out := make([]string, 0, len(declared))
	for _, e := range declared {
		out = append(out, e.Name)
	}
	return out
}
