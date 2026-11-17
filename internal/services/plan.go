package services

// Plan is what a push deploys: the environment its branch takes, and the
// repository's services there.
type Plan struct {
	Route
	// Monorepo says the repository deploys as a monorepo.
	Monorepo bool `json:"monorepo"`
	// Service is the polyrepo's service name, "" for a monorepo.
	Service string `json:"service"`
	// Source is the file the services came from, "" when deployed by name.
	Source string `json:"source"`
	// Services are the services deploying to the environment, each over its
	// repository's defaults; empty when the branch deploys nowhere. Each
	// carries "gitops", the Target its image is written to, and, when the
	// environment promotes from another, "gitops_from", the Target its image
	// is read from there.
	Services Entries `json:"services"`
	// Signing is how the images are signed; nil, not at all.
	Signing *Signing `json:"signing,omitempty"`
	// Matrix is everything Resolve found, for the outputs per environment.
	Matrix *Matrix `json:"-"`
}

// PlanPush resolves what r's repository deploys and routes branch through
// its environments (Resolve's routes).
func PlanPush(r Request, branch string) (*Plan, error) {
	r.Environments = nil
	m, err := Resolve(r)
	if err != nil {
		return nil, err
	}
	route, err := RouteBranch(m.Routes, branch)
	if err != nil {
		return nil, err
	}
	p := &Plan{Route: route, Monorepo: m.Monorepo, Service: m.Service, Source: m.Source, Services: Entries{}, Signing: m.Signing, Matrix: m}
	if !route.Deploy {
		return p, nil
	}
	for _, e := range m.Environments[route.Name] {
		withTargets, err := targets(m.File, route, e)
		if err != nil {
			return nil, err
		}
		p.Services = append(p.Services, withTargets)
	}
	return p, nil
}

// targets is a copy of e with "gitops", where it is written in route's
// environment, and "gitops_from", where it is read in the one route promotes
// from.
func targets(f *File, route Route, e Entry) (Entry, error) {
	out := make(Entry, len(e)+2)
	for k, v := range e {
		out[k] = v
	}
	to, err := f.TargetFor(route.Environment, e)
	if err != nil {
		return nil, err
	}
	out["gitops"] = to
	if route.PromoteFrom != nil {
		from, err := f.TargetFor(*route.PromoteFrom, e)
		if err != nil {
			return nil, err
		}
		out["gitops_from"] = from
	}
	return out, nil
}
