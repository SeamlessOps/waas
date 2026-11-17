package services

import (
	"strings"
	"testing"
)

func signingOf(t *testing.T, signing string) (*Signing, error) {
	t.Helper()
	f, err := Parse("s.json", []byte(`{"metadata": {"signing": `+signing+`}, "services": []}`))
	if err != nil {
		return nil, err
	}
	return f.Signing, nil
}

func TestSigning(t *testing.T) {
	s, err := signingOf(t, `{"key": "gcpkms://projects/p/locations/global/keyRings/r/cryptoKeys/images"}`)
	if err != nil || s.Key == "" || s.Keyless != nil {
		t.Errorf("key: %+v, %v", s, err)
	}

	// Enabled alone: GitHub's issuer and any workflow; the repository is
	// checked when verifying.
	s, err = signingOf(t, `{"keyless": {"enabled": true}}`)
	if err != nil || s.Keyless == nil || s.Keyless.Identity != AnyGitHubWorkflow || s.Keyless.Issuer != GitHubIssuer {
		t.Errorf("keyless enabled: %+v, %v", s, err)
	}

	s, err = signingOf(t, `{"keyless": {"enabled": true, "identity": "^https://github.com/org/", "issuer": "https://idp"}}`)
	if err != nil || s.Keyless.Identity != "^https://github.com/org/" || s.Keyless.Issuer != "https://idp" {
		t.Errorf("keyless narrowed: %+v, %v", s, err)
	}

	// Not enabled: nothing is signed.
	if s, err := signingOf(t, `{"keyless": {"enabled": false}}`); err != nil || s != nil {
		t.Errorf("keyless disabled: %+v, %v", s, err)
	}
	if f, _ := Parse("s.json", []byte(`{"services": []}`)); f.Signing != nil {
		t.Error("no signing set, yet signing")
	}
}

func TestSigning_Problems(t *testing.T) {
	for name, tc := range map[string]struct{ signing, want string }{
		"not an object":      {`"yes"`, "must be an object"},
		"unknown setting":    {`{"cert": "x"}`, "signing.cert is not a setting"},
		"empty key":          {`{"key": ""}`, "cosign key reference"},
		"both":               {`{"key": "k", "keyless": {"enabled": true}}`, "exclusive"},
		"keyless not object": {`{"keyless": true}`, `{"enabled": true}`},
		"enabled not bool":   {`{"keyless": {"enabled": "yes"}}`, "true or false"},
		"bad identity":       {`{"keyless": {"enabled": true, "identity": "("}}`, "regular expression"},
		"unknown keyless":    {`{"keyless": {"enabled": true, "subject": "x"}}`, "keyless.subject is not a setting"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := signingOf(t, tc.signing); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want it to mention %q", err, tc.want)
			}
		})
	}
}
