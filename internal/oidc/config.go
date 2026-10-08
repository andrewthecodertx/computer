package oidc

// Authelia OIDC configuration (STUB-AWARE). Porting guide 3.8.
//
// Authelia sign-in is only enabled when all three env vars hold real values:
//   OIDC_ISSUER, OIDC_CLIENT_ID, OIDC_CLIENT_SECRET
//
// Empty values or obvious placeholders (REPLACE_ME, example.com, changeme, ...)
// are treated as "not connected": the Authelia provider is not registered, the
// login button is shown disabled, and email/password login keeps working.
//
// See docs/AUTHELIA.md for the full connection guide.

import (
	"os"
	"regexp"
	"strings"
)

var placeholderPatterns = []*regexp.Regexp{
	regexp.MustCompile(`replace[_-]?me`),
	regexp.MustCompile(`example\.(com|org|net)`),
	regexp.MustCompile(`changeme`),
	regexp.MustCompile(`placeholder`),
	regexp.MustCompile(`^<.*>$`),
	regexp.MustCompile(`^your[_-]`),
	regexp.MustCompile(`^todo$`),
}

func isRealValue(value string) bool {
	v := strings.ToLower(strings.TrimSpace(value))
	if v == "" {
		return false
	}
	for _, p := range placeholderPatterns {
		if p.MatchString(v) {
			return false
		}
	}
	return true
}

// OidcStatus mirrors lib/oidc-config.ts. Enabled only when all three env
// values are real (non-placeholder). Never exposes the secret.
type OidcStatus struct {
	Enabled         bool
	IssuerSet       bool
	ClientIDSet     bool
	ClientSecretSet bool
	Issuer          *string
}

// Status returns the current OIDC configuration state.
func Status() OidcStatus {
	issuerSet := isRealValue(os.Getenv("OIDC_ISSUER"))
	clientIdSet := isRealValue(os.Getenv("OIDC_CLIENT_ID"))
	clientSecretSet := isRealValue(os.Getenv("OIDC_CLIENT_SECRET"))
	var issuer *string
	if issuerSet {
		v := strings.TrimSpace(os.Getenv("OIDC_ISSUER"))
		issuer = &v
	}
	return OidcStatus{
		Enabled:         issuerSet && clientIdSet && clientSecretSet,
		IssuerSet:       issuerSet,
		ClientIDSet:     clientIdSet,
		ClientSecretSet: clientSecretSet,
		Issuer:          issuer,
	}
}

// CallbackPath is where Authelia sends the OIDC code back.
const CallbackPath = "/api/auth/callback/authelia"
