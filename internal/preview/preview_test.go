package preview

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestHTMLMetadata(t *testing.T) {
	u, _ := url.Parse("https://site.test/docs/page")
	p := ParseHTML(strings.NewReader(`<META content='A &amp; B' property='og:title'><meta property="og:image" content="/cover.png"><link href="../icon.png" rel="shortcut icon">`), u)
	if p.OgTitle == nil || *p.OgTitle != "A & B" || p.OgImage == nil || *p.OgImage != "https://site.test/cover.png" || p.Favicon == nil || *p.Favicon != "https://site.test/icon.png" {
		t.Fatalf("metadata: %+v", p)
	}
}
func TestPrivatePreviewIsBlocked(t *testing.T) {
	if c, e := publicDial(context.Background(), "tcp", "127.0.0.1:5432"); e == nil {
		c.Close()
		t.Fatal("private destination accepted")
	}
}

// Review CR-06: every preview entry point draws from ONE shared budget — a
// per-identity rate and a global concurrency cap. Exhaustion must deny
// immediately (callers then persist bookmarks without preview metadata)
// instead of queueing outbound work.
func TestSharedBudget(t *testing.T) {
	stamp := time.Now().UnixNano()
	key := fmt.Sprintf("user:rate-%d", stamp)
	for i := 0; i < keyLimit; i++ {
		release, ok := Reserve(key)
		if !ok {
			t.Fatalf("reservation %d within the per-key limit denied", i)
		}
		release()
	}
	if _, ok := Reserve(key); ok {
		t.Fatal("per-key rate budget exceeded")
	}
	// An unrelated identity is unaffected by the exhausted key.
	if release, ok := Reserve(key + "-other"); !ok {
		t.Fatal("independent key denied")
	} else {
		release()
	}
	// Global concurrency: held (unreleased) slots cap all identities.
	var releases []func()
	for i := 0; i < maxConcurrent; i++ {
		release, ok := Reserve(fmt.Sprintf("user:conc-%d-%d", stamp, i))
		if !ok {
			t.Fatalf("concurrency slot %d denied", i)
		}
		releases = append(releases, release)
	}
	if _, ok := Reserve(fmt.Sprintf("user:conc-overflow-%d", stamp)); ok {
		t.Fatal("global concurrency cap not enforced")
	}
	for _, release := range releases {
		release()
	}
	if release, ok := Reserve(fmt.Sprintf("user:conc-after-%d", stamp)); !ok {
		t.Fatal("slot not returned after release")
	} else {
		release()
	}
}
