package preview

import (
	"context"
	"net/url"
	"strings"
	"testing"
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
