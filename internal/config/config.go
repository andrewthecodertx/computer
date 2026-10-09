package config

import (
	"os"
	"strings"
)

// DBURL returns the database connection string, preferring DATABASE_URL and
// falling back to DB_URL (used by the Makefile).
func DBURL() string {
	if v := os.Getenv("DATABASE_URL"); v != "" {
		return v
	}
	return os.Getenv("DB_URL")
}

// Config holds the runtime configuration, loaded from environment variables.
// See porting guide section 7.
type Config struct {
	DatabaseURL      string
	NextAuthSecret   string // == AUTH_SECRET; signs sessions and derives IMAP key
	AuthSecret       string
	NextAuthURL      string
	OIDCIssuer       string
	OIDCClientID     string
	OIDCClientSecret string
	AdminEmails      []string
}

// Load reads configuration from the environment. Placeholders keep OIDC off.
func Load() *Config {
	return &Config{
		DatabaseURL:      DBURL(),
		NextAuthSecret:   os.Getenv("NEXTAUTH_SECRET"),
		AuthSecret:       os.Getenv("AUTH_SECRET"),
		NextAuthURL:      os.Getenv("NEXTAUTH_URL"),
		OIDCIssuer:       os.Getenv("OIDC_ISSUER"),
		OIDCClientID:     os.Getenv("OIDC_CLIENT_ID"),
		OIDCClientSecret: os.Getenv("OIDC_CLIENT_SECRET"),
		AdminEmails:      AdminEmails(),
	}
}

// Secret returns the signing secret, preferring NEXTAUTH_SECRET then AUTH_SECRET.
// The original code falls back to a hard-coded string; a port must fail instead.
func (c *Config) Secret() string {
	if c.NextAuthSecret != "" {
		return c.NextAuthSecret
	}
	return c.AuthSecret
}

// AdminEmails splits env ADMIN_EMAILS on commas, trims, lowercases.
func AdminEmails() []string {
	s := os.Getenv("ADMIN_EMAILS")
	if s == "" {
		return nil
	}
	var out []string
	for _, email := range strings.Split(s, ",") {
		if email = strings.ToLower(strings.TrimSpace(email)); email != "" {
			out = append(out, email)
		}
	}
	return out
}

// IsAdminEmail reports whether email is listed in ADMIN_EMAILS.
func IsAdminEmail(email string) bool {
	e := strings.ToLower(strings.TrimSpace(email))
	for _, a := range AdminEmails() {
		if strings.ToLower(a) == e {
			return true
		}
	}
	return false
}
