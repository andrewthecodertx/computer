package oidc

// Provider is the Authelia OIDC sign-in provider.
//
// Porting guide 3.1: the Authelia provider is registered only when
// getOidcStatus().enabled. Scope "openid profile email". Profile mapping:
// id=sub, name = name ?? preferred_username ?? sub, email, image = picture.
// It links to an existing account with the same email.
//
// STUB: the full OIDC code flow (authorization code exchange, token endpoint,
// userinfo) is NOT implemented in Go — it will be driven by the client, which
// runs NextAuth and handles the provider itself. These handlers exist so the
// route table matches the original app and the Settings/login pages can read
// the status. They return 501 until the client-side flow is wired.

import (
	"encoding/json"
	"net/http"
)

// HandleStatus returns the current OIDC configuration state. This is what the
// Settings page and the login page read to decide whether to show the
// "Sign in with Authelia" button.
//
//	GET /api/auth/oidc/status  ->  {enabled, issuerSet, clientIdSet, clientSecretSet, issuer}
func HandleStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "method not allowed"})
		return
	}
	s := Status()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"enabled":         s.Enabled,
		"issuerSet":       s.IssuerSet,
		"clientIdSet":     s.ClientIDSet,
		"clientSecretSet": s.ClientSecretSet,
		"issuer":          s.Issuer,
	})
}

// HandleLogin starts the OIDC authorization-code flow. STUB: returns 501 until
// the client drives the flow via NextAuth.
//
//	GET /api/auth/oidc/login
func HandleLogin(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotImplemented)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error":  "oidc not implemented",
		"detail": "Authelia OIDC is driven by the client (NextAuth); this handler is a stub",
	})
}

// HandleCallback is the Authelia OIDC callback. STUB: returns 501 until the
// client drives the flow via NextAuth.
//
//	GET /api/auth/callback/authelia
func HandleCallback(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotImplemented)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error":  "oidc not implemented",
		"detail": "Authelia OIDC is driven by the client (NextAuth); this handler is a stub",
	})
}

// Profile maps an Authelia userinfo response to a user identity. Porting guide
// 3.1: id=sub, name = name ?? preferred_username ?? sub, email, image = picture.
type Profile struct {
	Sub               string `json:"sub"`
	Name              string `json:"name"`
	PreferredUsername string `json:"preferred_username"`
	Email             string `json:"email"`
	Picture           string `json:"picture"`
}

// MapProfile converts a raw Authelia profile into the identity fields.
func MapProfile(p Profile) (id, name, email, image string) {
	id = p.Sub
	if p.Name != "" {
		name = p.Name
	} else if p.PreferredUsername != "" {
		name = p.PreferredUsername
	} else {
		name = p.Sub
	}
	email = p.Email
	image = p.Picture
	return
}
