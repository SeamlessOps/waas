package services

import (
	"fmt"
	"regexp"
)

// Signing is how images are signed, metadata.signing: with a key, or keyless.
// Without it nothing is signed.
//
//	"signing": { "key": "gcpkms://projects/p/locations/global/keyRings/r/cryptoKeys/images" }
//	"signing": { "keyless": { "enabled": true } }
//
// Images are signed with cosign when built or promoted, and a promotion
// verifies its source image first: signed with the key, or keyless by a
// GitHub Actions workflow that ran in the same repository.
type Signing struct {
	// Key is a cosign key reference (gcpkms://, awskms://, azurekms://,
	// hashivault://, or a file). Signatures stay in the registry: nothing is
	// written to a public transparency log.
	Key string `json:"key,omitempty"`
	// Keyless signs with the workflow's GitHub identity. Each signature is
	// recorded in Sigstore's public transparency log, with the image digest
	// and the identity's organization, repository and workflow.
	Keyless *Keyless `json:"keyless,omitempty"`
}

// Keyless is keyless signing, and what a keyless signature must come from to
// be trusted. Enabled alone is the common case: a GitHub Actions workflow
// that ran in the same repository.
type Keyless struct {
	Enabled bool `json:"enabled"`
	// Identity narrows the signing workflows: a regular expression their
	// identity (https://github.com/<owner>/<repo>/.github/workflows/<file>@<ref>)
	// must match. Default: any.
	Identity string `json:"identity,omitempty"`
	// Issuer is the OIDC issuer; GitHub Actions' by default.
	Issuer string `json:"issuer,omitempty"`
}

// AnyGitHubWorkflow is the identity of every GitHub Actions workflow.
const AnyGitHubWorkflow = "^https://github.com/"

// GitHubIssuer is the OIDC issuer of GitHub Actions.
const GitHubIssuer = "https://token.actions.githubusercontent.com"

// parseSigning reads metadata.signing, reporting problems through add:
// exactly one of key and an enabled keyless, and a keyless identity that is
// a valid regular expression. Keyless with enabled false signs nothing.
func parseSigning(v any, add func(string, ...any)) *Signing {
	if v == nil {
		return nil
	}
	obj, ok := v.(map[string]any)
	if !ok {
		add("metadata.signing must be an object: {\"key\": ...} or {\"keyless\": {\"enabled\": true}}")
		return nil
	}
	for k := range obj {
		if k != "key" && k != "keyless" {
			add("metadata.signing.%s is not a setting: key or keyless", k)
		}
	}
	s := &Signing{}
	if k, set := obj["key"]; set {
		key, ok := k.(string)
		if !ok || key == "" {
			add("metadata.signing.key must be a cosign key reference, such as gcpkms://...")
		}
		s.Key = key
	}
	if kl, set := obj["keyless"]; set {
		m, ok := kl.(map[string]any)
		if !ok {
			add("metadata.signing.keyless must be an object: {\"enabled\": true}")
			return nil
		}
		k := &Keyless{Issuer: GitHubIssuer}
		for name, v := range m {
			switch name {
			case "enabled":
				b, ok := v.(bool)
				if !ok {
					add("metadata.signing.keyless.enabled must be true or false")
				}
				k.Enabled = b
			case "identity":
				id, ok := v.(string)
				if _, err := regexp.Compile(id); !ok || id == "" || err != nil {
					add("metadata.signing.keyless.identity must be a regular expression of the workflows trusted to sign")
				}
				k.Identity = id
			case "issuer":
				iss, ok := v.(string)
				if !ok || iss == "" {
					add("metadata.signing.keyless.issuer must be an OIDC issuer URL")
				}
				k.Issuer = iss
			default:
				add("metadata.signing.keyless.%s is not a setting: enabled, identity or issuer", name)
			}
		}
		if k.Enabled {
			if k.Identity == "" {
				k.Identity = AnyGitHubWorkflow
			}
			s.Keyless = k
		}
	}
	if s.Key != "" && s.Keyless != nil {
		add("metadata.signing: key and keyless are exclusive")
	}
	if s.Key == "" && s.Keyless == nil {
		return nil // nothing to sign with: keyless not enabled
	}
	return s
}

// String says how images are signed.
func (s *Signing) String() string {
	switch {
	case s == nil:
		return "not signed"
	case s.Key != "":
		return fmt.Sprintf("signed with %s", s.Key)
	case s.Keyless.Identity == AnyGitHubWorkflow:
		return "signed keyless, trusted from this repository's workflows"
	default:
		return fmt.Sprintf("signed keyless, trusted from this repository's workflows matching %s", s.Keyless.Identity)
	}
}
