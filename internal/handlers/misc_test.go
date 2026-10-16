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

// TestParseVCardSubset pins the documented parser subset (review CR-08):
// repeated properties are preserved, PREF selection is deterministic,
// groups/params/escapes/folding are handled, and base64-encoded properties
// are dropped instead of silently misparsed.
func TestParseVCardSubset(t *testing.T) {
	card := "BEGIN:VCARD\r\n" +
		"VERSION:3.0\r\n" +
		"UID:uid-1\r\n" +
		"item1.FN:Preferred Name\r\n" +
		"item1.X-ABLabel:Main\r\n" +
		"EMAIL;TYPE=WORK:work@computer.test\r\n" +
		"EMAIL;TYPE=PREF:pref@computer.test\r\n" +
		"EMAIL:plain@computer.test\r\n" +
		"TEL;TYPE=HOME,VOICE:+1 555 0100\r\n" +
		"NOTE:folded line\r\n" +
		"  continuation\r\n" +
		"X-ESCAPED:a\\,b\\;c\\\\d\\ne\r\n" +
		"PHOTO;ENCODING=b;TYPE=JPEG:/9j/4AAQSkZJRg==\r\n" +
		"END:VCARD"
	v := parseVCard(card)

	if got := vCardPick(v["UID"]); got != "uid-1" {
		t.Fatalf("UID = %q", got)
	}
	// Group prefix stripped (item1.FN -> FN).
	if got := vCardPick(v["FN"]); got != "Preferred Name" {
		t.Fatalf("FN = %q", got)
	}
	// All three EMAIL occurrences survive; TYPE=PREF wins deterministically.
	if len(v["EMAIL"]) != 3 {
		t.Fatalf("repeated EMAILs not preserved: %d", len(v["EMAIL"]))
	}
	if got := vCardPick(v["EMAIL"]); got != "pref@computer.test" {
		t.Fatalf("PREF selection = %q", got)
	}
	// vCard 4.0 numeric PREF ranks below any TYPE=PREF-free occurrence.
	num := parseVCard("EMAIL:a@x.test\r\nEMAIL;PREF=2:b@x.test\r\nEMAIL;PREF=1:c@x.test\r\n")
	if got := vCardPick(num["EMAIL"]); got != "c@x.test" {
		t.Fatalf("numeric PREF selection = %q", got)
	}
	// Multi-value parameters (TYPE=HOME,VOICE) are split.
	tel := v["TEL"]
	if len(tel) != 1 || tel[0].params["TYPE"][0] != "HOME" || tel[0].params["TYPE"][1] != "VOICE" {
		t.Fatalf("TEL params = %+v", tel)
	}
	// CRLF-space folding unfolds into one value: the first space is the fold
	// marker, the second is content.
	if got := vCardPick(v["NOTE"]); got != "folded line continuation" {
		t.Fatalf("NOTE unfolding = %q", got)
	}
	// Escapes decode.
	if got := vCardPick(v["X-ESCAPED"]); got != "a,b;c\\d\ne" {
		t.Fatalf("escapes = %q", got)
	}
	// Base64-encoded properties are dropped, not stored as garbage.
	if _, ok := v["PHOTO"]; ok {
		t.Fatal("base64-encoded PHOTO was parsed instead of dropped")
	}
	// Missing properties pick to "".
	if got := vCardPick(v["ORG"]); got != "" {
		t.Fatalf("missing property = %q", got)
	}
}
