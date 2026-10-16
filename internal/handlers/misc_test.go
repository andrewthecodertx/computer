package handlers

import (
	"net/url"
	"testing"
)

// TestResolveCardDAVTarget pins review CR-01: the COMPLETE resolved
// destination is revalidated, so a same-host absolute http:// path cannot
// downgrade an https base and carry Basic credentials over plaintext.
func TestResolveCardDAVTarget(t *testing.T) {
	base, e := url.Parse("https://cloud.example.com/remote.php/dav")
	if e != nil {
		t.Fatal(e)
	}
	accepted := []string{
		"/remote.php/dav/addressbooks/users/alice/contacts/",
		"addressbooks/users/alice/contacts/",
		"https://cloud.example.com/remote.php/dav/addressbooks/users/alice/contacts/",
	}
	for _, path := range accepted {
		target, e := resolveCardDAVTarget(base, path)
		if e != nil {
			t.Fatalf("resolveCardDAVTarget(%q) rejected: %v", path, e)
		}
		if target.Scheme != "https" || target.Host != "cloud.example.com" {
			t.Fatalf("resolveCardDAVTarget(%q) resolved to %s", path, target)
		}
	}
	rejected := []string{
		// Same host, downgraded scheme — the CR-01 credential-leak vector.
		"http://cloud.example.com/remote.php/dav/addressbooks/users/alice/contacts/",
		// Cross-host absolute URLs.
		"https://evil.example.com/remote.php/dav/",
		"http://evil.example.com/remote.php/dav/",
		// Scheme-relative and userinfo tricks.
		"//evil.example.com/dav/",
		"https://alice:pw@cloud.example.com/dav/",
	}
	for _, path := range rejected {
		if target, e := resolveCardDAVTarget(base, path); e == nil {
			t.Fatalf("resolveCardDAVTarget(%q) accepted, resolved to %s", path, target)
		}
	}
}
