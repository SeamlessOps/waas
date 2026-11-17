// Package services reads a services file: the services a repository deploys,
// each with the environments it deploys to and its build options, and a
// repository's defaults under its entries.
//
//	{
//	  "metadata": { "default_environments": ["staging", "production"] },
//	  "repositories": { "shop": { "project_type": "nx" } },
//	  "services": [ { "repository": "shop", "name": "web", "environments": ["staging"] } ]
//	}
//
// An entry is kept as it is written: WaaS reads name, repository and
// environments, and passes every other field through to the workflow that
// builds and deploys it.
package services

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

// DefaultEnvironments are the environments an entry deploys to when neither
// it nor the file's metadata.default_environments says.
var DefaultEnvironments = []string{"staging", "production"}

// Entry is one service as the file declares it.
type Entry map[string]any

// Name is what the service deploys as.
func (e Entry) Name() string { s, _ := e["name"].(string); return s }

// Repository is the repository the service is built from, "" when unset.
func (e Entry) Repository() string { s, _ := e["repository"].(string); return s }

// File is a services file.
type File struct {
	Path string
	// Services are the entries, in file order.
	Services []Entry
	// Repositories are each repository's defaults.
	Repositories map[string]any
	// DefaultEnvironments is metadata.default_environments, or
	// DefaultEnvironments when the file has none.
	DefaultEnvironments []string
	// Environments are metadata.environments: where the file's repositories
	// deploy, and from which branch. Empty: the caller's defaults apply.
	Environments []Environment
}

// Invalid is a services file that can't be used, with every reason.
type Invalid struct {
	Path     string
	Problems []string
}

func (e *Invalid) Error() string {
	msg := e.Path + " is not a valid services file:"
	for _, p := range e.Problems {
		msg += "\n  - " + p
	}
	return msg
}

// Load reads and checks the services file at path: valid JSON, a "services"
// array, a name on every entry, environments (when set) a list, and each
// repository's defaults an object. An *Invalid lists everything wrong.
func Load(path string) (*File, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(path, raw)
}

// Parse is Load on raw, read from path.
func Parse(path string, raw []byte) (*File, error) {
	var doc map[string]any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber() // numbers pass through as written
	if err := dec.Decode(&doc); err != nil {
		return nil, &Invalid{Path: path, Problems: []string{"invalid JSON: " + err.Error()}}
	}
	if dec.More() {
		return nil, &Invalid{Path: path, Problems: []string{"invalid JSON: more than one value"}}
	}

	f := &File{Path: path, DefaultEnvironments: DefaultEnvironments, Repositories: map[string]any{}}
	inv := &Invalid{Path: path}
	add := func(format string, args ...any) { inv.Problems = append(inv.Problems, fmt.Sprintf(format, args...)) }

	list, ok := doc["services"]
	switch {
	case !ok:
		add(`missing the "services" key: {"services": [{"name": "service-name", "environments": ["staging", "production"]}]}`)
	default:
		items, isList := list.([]any)
		if !isList {
			add(`"services" must be a list`)
			break
		}
		for i, it := range items {
			obj, isObj := it.(map[string]any)
			if !isObj {
				add("service at index %d is not an object", i)
				continue
			}
			e := Entry(obj)
			if v, set := e["name"]; !set || v == nil || v == "" {
				add("service at index %d is missing its name", i)
			}
			if v, set := e["environments"]; set && v != nil {
				if _, isList := v.([]any); !isList {
					add("service %s: environments must be a list", e.Name())
				}
			}
			f.Services = append(f.Services, e)
		}
	}

	if r, set := doc["repositories"]; set && r != nil {
		repos, isObj := r.(map[string]any)
		if !isObj {
			add(`"repositories" must be an object of each repository's defaults`)
		}
		for name, d := range repos {
			if _, isObj := d.(map[string]any); !isObj {
				add("repositories.%s must be an object", name)
			}
		}
		if isObj {
			f.Repositories = repos
		}
	}

	if md, isObj := doc["metadata"].(map[string]any); isObj {
		if envs, isList := md["default_environments"].([]any); isList {
			f.DefaultEnvironments = stringsOf(envs)
		}
		f.Environments = parseEnvironments(md["environments"], add)
	}
	checkBranches(f, add)

	if len(inv.Problems) > 0 {
		return nil, inv
	}
	return f, nil
}

// IsNotExist reports whether err says the file is not there.
func IsNotExist(err error) bool { return errors.Is(err, os.ErrNotExist) }

// Names are the entries' names, in file order.
func (f *File) Names() []string {
	out := make([]string, 0, len(f.Services))
	for _, e := range f.Services {
		out = append(out, e.Name())
	}
	return out
}

// WithDefaults returns e over its repository's defaults: e's own fields win,
// and fields that are objects on both sides (such as "version") merge key by
// key. The repository's "branches" are routing, not a service field, and are
// left out.
func (f *File) WithDefaults(e Entry) Entry {
	d, _ := f.Repositories[e.Repository()].(map[string]any)
	if _, has := d["branches"]; has {
		trimmed := make(map[string]any, len(d))
		for k, v := range d {
			if k != "branches" {
				trimmed[k] = v
			}
		}
		d = trimmed
	}
	return Entry(merge(d, e))
}

// EnvironmentsOf are the environments e deploys to: its own, else the file's
// defaults.
func (f *File) EnvironmentsOf(e Entry) []string {
	if v, set := e["environments"]; set && v != nil {
		list, _ := v.([]any)
		return stringsOf(list)
	}
	return f.DefaultEnvironments
}

// merge returns over laid on base: keys of both, over's value winning unless
// both are objects, which merge the same way.
func merge(base, over map[string]any) map[string]any {
	out := make(map[string]any, len(base)+len(over))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range over {
		b, bObj := out[k].(map[string]any)
		o, oObj := v.(map[string]any)
		if bObj && oObj {
			out[k] = merge(b, o)
			continue
		}
		out[k] = v
	}
	return out
}

// stringsOf keeps the strings of list.
func stringsOf(list []any) []string {
	out := make([]string, 0, len(list))
	for _, v := range list {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
