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
	// repository's defaults; empty when the branch deploys nowhere.
	Services []Entry `json:"services"`
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
	p := &Plan{Route: route, Monorepo: m.Monorepo, Service: m.Service, Source: m.Source, Services: []Entry{}, Signing: m.Signing, Matrix: m}
	if route.Deploy && m.Environments[route.Name] != nil {
		p.Services = m.Environments[route.Name]
	}
	return p, nil
}
