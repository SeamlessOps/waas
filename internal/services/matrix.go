package services

import (
	"fmt"
	"slices"
	"strings"
)

// Request is what a repository's workflow asks to deploy.
type Request struct {
	// File is the repository's own services file; it need not exist.
	File string
	// Catalog is the shared services file a repository without a file of its
	// own is looked up in, by Repository, else by Service. Empty: none.
	Catalog string
	// Repository is the repository's name.
	Repository string
	// Service is the service a polyrepo deploys as.
	Service string
	// Monorepo says the repository deploys several services. A catalog
	// lookup decides it from the number of entries found.
	Monorepo bool
	// ByName skips the files: the one service is deployed by its name alone.
	ByName bool
	// Environments are the environments to list the services of. Empty: the
	// file's metadata.environments, else DefaultEnvironments.
	Environments []string
}

// Matrix is what a repository deploys.
type Matrix struct {
	// Monorepo says the repository deploys as a monorepo.
	Monorepo bool `json:"monorepo"`
	// Service is the polyrepo's service name, "" for a monorepo.
	Service string `json:"service"`
	// Source is the file the services came from, "" when deployed by name.
	Source string `json:"source"`
	// All are the services, each over its repository's defaults.
	All Entries `json:"all"`
	// Environments are the services deploying to each environment.
	Environments map[string]Entries `json:"environments"`
	// Routes are the environments, with the branch that deploys to each for
	// this repository: the file's, else DefaultRoutes.
	Routes Environments `json:"routes"`
	// Signing is how the images are signed; nil, not at all.
	Signing *Signing `json:"signing,omitempty"`
	// File is the services file read; nil when deployed by name.
	File *File `json:"-"`
}

// Resolve finds what r deploys:
//
//   - ByName, or a polyrepo with neither a file nor a catalog entry, is the
//     service by its name alone, in every environment.
//   - The repository's own file: a monorepo deploys every entry, a polyrepo
//     the entry named Service.
//   - Without one, the catalog: the repository's entries (only the one named
//     Service, when it names one), else the entry named Service. One entry
//     deploys as a polyrepo under its name, several as a monorepo.
func Resolve(r Request) (*Matrix, error) {
	if r.ByName {
		return byName(r), nil
	}
	f, err := Load(r.File)
	fromCatalog := false
	if IsNotExist(err) {
		switch {
		case r.Catalog != "" && inCatalog(r.Catalog, r.Repository, r.Service):
			if f, err = Load(r.Catalog); err != nil {
				return nil, err
			}
			fromCatalog = true
		case !r.Monorepo:
			return byName(r), nil
		default:
			return nil, fmt.Errorf("services file %s not found, and %s is not in the catalog %s", r.File, r.Repository, r.Catalog)
		}
	} else if err != nil {
		return nil, err
	}

	m := &Matrix{Monorepo: r.Monorepo, Service: r.Service, Source: f.Path, File: f}
	picked := f.Services
	switch {
	case fromCatalog:
		picked = catalogEntries(f, r.Repository, r.Service)
		m.Monorepo = len(picked) > 1
		if !m.Monorepo {
			m.Service = picked[0].Name()
		}
	case !r.Monorepo:
		picked = byNameIn(f.Services, r.Service)
		if len(picked) == 0 {
			return nil, fmt.Errorf("no service named %q in %s (it has: %s)", r.Service, f.Path, strings.Join(f.Names(), ", "))
		}
	}
	if len(picked) == 0 {
		return nil, fmt.Errorf("no services in %s: at least one must be defined", f.Path)
	}
	if m.Monorepo {
		m.Service = ""
	}

	for _, e := range picked {
		m.All = append(m.All, f.WithDefaults(e))
	}
	m.Routes = f.EnvironmentsFor(r.Repository)
	if len(m.Routes) == 0 {
		m.Routes = defaultRoutes()
	}
	m.Signing = f.Signing
	envs := r.Environments
	if len(envs) == 0 {
		for _, e := range m.Routes {
			envs = append(envs, e.Name)
		}
	}
	if len(envs) == 0 {
		envs = DefaultEnvironments
	}
	m.Environments = map[string]Entries{}
	for _, env := range envs {
		m.Environments[env] = Entries{}
		for i, e := range picked {
			if slices.Contains(f.EnvironmentsOf(e), env) {
				m.Environments[env] = append(m.Environments[env], m.All[i])
			}
		}
	}
	return m, nil
}

func byName(r Request) *Matrix {
	one := Entries{{"name": r.Service}}
	m := &Matrix{Monorepo: r.Monorepo, All: one, Environments: map[string]Entries{}, Routes: defaultRoutes()}
	if !r.Monorepo {
		m.Service = r.Service
	}
	envs := r.Environments
	if len(envs) == 0 {
		envs = DefaultEnvironments
	}
	for _, env := range envs {
		m.Environments[env] = one
	}
	return m
}

// inCatalog reports whether the catalog at path is valid and has an entry of
// repo or named service.
func inCatalog(path, repo, service string) bool {
	f, err := Load(path)
	if err != nil {
		return false
	}
	for _, e := range f.Services {
		if e.Repository() == repo || e.Name() == service {
			return true
		}
	}
	return false
}

// catalogEntries are repo's entries in f (only the one named service, when
// it names one), else the first entry named service.
func catalogEntries(f *File, repo, service string) Entries {
	var own Entries
	for _, e := range f.Services {
		if e.Repository() == repo {
			own = append(own, e)
		}
	}
	if len(own) > 0 {
		if named := byNameIn(own, service); len(named) > 0 {
			return named
		}
		return own
	}
	return byNameIn(f.Services, service)[:1]
}

func byNameIn(entries Entries, name string) Entries {
	var out Entries
	for _, e := range entries {
		if e.Name() == name {
			out = append(out, e)
		}
	}
	return out
}
