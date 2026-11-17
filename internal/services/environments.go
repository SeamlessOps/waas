package services

import "fmt"

// Environment is where services deploy, and from which branch.
//
//	"metadata": {
//	  "environments": [
//	    { "name": "staging",    "branch": "dev",  "domain": "shop-dev", "build_configuration": "Release" },
//	    { "name": "production", "branch": "main", "domain": "shop",     "build_configuration": "Release" }
//	  ]
//	}
//
// The order is the promotion path: an environment promotes the image the one
// before it built.
type Environment struct {
	// Name is the environment, as the GitOps repository's
	// environment/<name>/ directory and the GitHub environment name it.
	Name string `json:"name"`
	// Branch is the branch whose pushes deploy here. Empty: none does.
	Branch string `json:"branch,omitempty"`
	// Domain completes a service's values key, <name>-<domain>.
	Domain string `json:"domain,omitempty"`
	// BuildConfiguration is the build configuration (e.g. .NET's Release).
	BuildConfiguration string `json:"build_configuration,omitempty"`
}

// parseEnvironments reads metadata.environments, reporting problems through
// add: each needs a name, names and branches are unique, and every field is
// text.
func parseEnvironments(v any, add func(string, ...any)) []Environment {
	if v == nil {
		return nil
	}
	list, ok := v.([]any)
	if !ok {
		add("metadata.environments must be a list")
		return nil
	}
	var out []Environment
	names, branches := map[string]bool{}, map[string]string{}
	for i, it := range list {
		obj, ok := it.(map[string]any)
		if !ok {
			add("metadata.environments[%d] is not an object", i)
			continue
		}
		text := func(key string) string {
			v, set := obj[key]
			if !set || v == nil {
				return ""
			}
			s, ok := v.(string)
			if !ok {
				add("metadata.environments[%d].%s must be text", i, key)
			}
			return s
		}
		e := Environment{Name: text("name"), Branch: text("branch"), Domain: text("domain"), BuildConfiguration: text("build_configuration")}
		switch {
		case e.Name == "":
			add("metadata.environments[%d] is missing its name", i)
		case names[e.Name]:
			add("metadata.environments: %s is listed twice", e.Name)
		}
		if prior, dup := branches[e.Branch]; dup && e.Branch != "" {
			add("metadata.environments: branch %s deploys to both %s and %s", e.Branch, prior, e.Name)
		}
		names[e.Name], branches[e.Branch] = true, e.Name
		out = append(out, e)
	}
	return out
}

// checkBranches reads each repository's "branches", an object of branch →
// environment name: every value must be an environment the file declares.
func checkBranches(f *File, add func(string, ...any)) {
	for repo, d := range f.Repositories {
		obj, _ := d.(map[string]any)
		v, set := obj["branches"]
		if !set {
			continue
		}
		m, ok := v.(map[string]any)
		if !ok {
			add("repositories.%s.branches must be an object of branch → environment", repo)
			continue
		}
		if len(f.Environments) == 0 {
			add("repositories.%s.branches needs metadata.environments to name the environments", repo)
			continue
		}
		used := map[string]string{}
		for branch, env := range m {
			name, ok := env.(string)
			if !ok || f.environment(name) == nil {
				add("repositories.%s.branches.%s: %v is not an environment in metadata.environments", repo, branch, env)
				continue
			}
			if prior, dup := used[name]; dup {
				add("repositories.%s.branches: %s and %s both deploy to %s", repo, prior, branch, name)
			}
			used[name] = branch
		}
	}
}

func (f *File) environment(name string) *Environment {
	for i := range f.Environments {
		if f.Environments[i].Name == name {
			return &f.Environments[i]
		}
	}
	return nil
}

// EnvironmentsFor are the environments repo deploys to, with the branch that
// deploys to each: the repository's "branches" when it declares them (an
// environment it doesn't map gets no branch), else the file's.
func (f *File) EnvironmentsFor(repo string) []Environment {
	out := append([]Environment(nil), f.Environments...)
	obj, _ := f.Repositories[repo].(map[string]any)
	m, ok := obj["branches"].(map[string]any)
	if !ok {
		return out
	}
	for i := range out {
		out[i].Branch = ""
		for branch, env := range m {
			if env == out[i].Name {
				out[i].Branch = branch
			}
		}
	}
	return out
}

// Route is where a push to a branch deploys.
type Route struct {
	// Deploy is false when no environment takes the branch.
	Deploy bool `json:"deploy"`
	Environment
	// PromoteFrom is the environment before this one, whose image this one
	// can promote; nil for the first.
	PromoteFrom *Environment `json:"promote_from,omitempty"`
}

// RouteBranch finds the environment branch deploys to, in envs.
func RouteBranch(envs []Environment, branch string) (Route, error) {
	if branch == "" {
		return Route{}, fmt.Errorf("no branch to route")
	}
	for i, e := range envs {
		if e.Branch != branch {
			continue
		}
		r := Route{Deploy: true, Environment: e}
		if i > 0 {
			prev := envs[i-1]
			r.PromoteFrom = &prev
		}
		return r, nil
	}
	return Route{}, nil
}
