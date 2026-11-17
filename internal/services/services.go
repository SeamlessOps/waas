// Package services reads a services file, JSON or YAML: the services a repository deploys,
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
//
// The same file in YAML (.yaml or .yml):
//
//	metadata:
//	  default_environments: [staging, production]
//	repositories:
//	  shop: { project_type: nx }
//	services:
//	  - { repository: shop, name: web, environments: [staging] }
package services

import (
	"errors"
	"os"
)

// DefaultEnvironments are the environments an entry deploys to when neither
// it nor the file's metadata.default_environments says.
var DefaultEnvironments = []string{"staging", "production"}

// Entry is one service as the file declares it.
type Entry map[string]any
type Entries []Entry

// Name is what the service deploys as.
func (e Entry) Name() string { s, _ := e["name"].(string); return s }

// Repository is the repository the service is built from, "" when unset.
func (e Entry) Repository() string { s, _ := e["repository"].(string); return s }

// File is a services file.
type File struct {
	Path string
	// Services are the entries, in file order.
	Services Entries
	// Repositories are each repository's defaults.
	Repositories map[string]any
	// DefaultEnvironments is metadata.default_environments, or
	// DefaultEnvironments when the file has none.
	DefaultEnvironments []string
	// Environments are metadata.environments: where the file's repositories
	// deploy, and from which branch. Empty: the caller's defaults apply.
	Environments Environments
	// Signing is metadata.signing: how images are signed; nil, not at all.
	Signing *Signing
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
// key. The repository's "branches" and "values" say where it deploys, not
// how a service builds, and are left out.
func (f *File) WithDefaults(e Entry) Entry {
	d, _ := f.Repositories[e.Repository()].(map[string]any)
	_, hasBranches := d["branches"]
	_, hasValues := d["values"]
	if hasBranches || hasValues {
		trimmed := make(map[string]any, len(d))
		for k, v := range d {
			if k != "branches" && k != "values" {
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
