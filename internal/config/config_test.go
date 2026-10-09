package config

import "testing"

func TestCookieSecureDerivation(t *testing.T) {
	t.Setenv("COOKIE_SECURE", "")
	https := &Config{NextAuthURL: "https://app.example.com"}
	if !https.CookieSecure() {
		t.Fatal("https NEXTAUTH_URL must imply Secure cookies")
	}
	local := &Config{NextAuthURL: "http://localhost:3000"}
	if local.CookieSecure() {
		t.Fatal("http NEXTAUTH_URL must not set Secure")
	}
	if (&Config{}).CookieSecure() {
		t.Fatal("unset NEXTAUTH_URL must not set Secure")
	}
	t.Setenv("COOKIE_SECURE", "1")
	if !local.CookieSecure() {
		t.Fatal("explicit COOKIE_SECURE=1 must win over http URL")
	}
	t.Setenv("COOKIE_SECURE", "0")
	if https.CookieSecure() {
		t.Fatal("explicit COOKIE_SECURE=0 must win over https URL")
	}
}

func TestSecretPrecedence(t *testing.T) {
	c := &Config{NextAuthSecret: "next", AuthSecret: "auth"}
	if c.Secret() != "next" {
		t.Fatal("NEXTAUTH_SECRET must win")
	}
	if (&Config{AuthSecret: "auth"}).Secret() != "auth" {
		t.Fatal("AUTH_SECRET fallback broken")
	}
}
