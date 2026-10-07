package config

import "os"

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
	DatabaseURL     string
	NextAuthSecret  string // == AUTH_SECRET; signs sessions and derives IMAP key
	AuthSecret      string
	NextAuthURL     string
	OIDCIssuer      string
	OIDCClientID    string
	OIDCClientSecret string
	AdminEmails     []string
	DEMODemoPassword string
}

// Load reads configuration from the environment. Placeholders keep OIDC off.
func Load() *Config {
	return &Config{
		DatabaseURL:      os.Getenv("DATABASE_URL"),
		NextAuthSecret:   os.Getenv("NEXTAUTH_SECRET"),
		AuthSecret:       os.Getenv("AUTH_SECRET"),
		NextAuthURL:      os.Getenv("NEXTAUTH_URL"),
		OIDCIssuer:       os.Getenv("OIDC_ISSUER"),
		OIDCClientID:     os.Getenv("OIDC_CLIENT_ID"),
		OIDCClientSecret: os.Getenv("OIDC_CLIENT_SECRET"),
		AdminEmails:      splitEmails(os.Getenv("ADMIN_EMAILS")),
		DEMODemoPassword: os.Getenv("DEMO_PASSWORD"),
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

func splitEmails(s string) []string {
	// TODO: split on commas, trim, lowercase.
	return nil
}