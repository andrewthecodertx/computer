package preview

import (
	"context"
	"fmt"
	"github.com/andrew/go-computer/internal/netguard"
	"golang.org/x/net/html"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Preview struct {
	Title         *string `json:"title"`
	Description   *string `json:"description"`
	Favicon       *string `json:"favicon"`
	OgImage       *string `json:"ogImage"`
	OgTitle       *string `json:"ogTitle"`
	OgDescription *string `json:"ogDescription"`
}

// DNS is resolved and checked on each connection, including redirects, so
// URL previews cannot reach another user's internal service or Docker DB.
// The shared range logic lives in netguard (also used by CardDAV and IMAP).
func publicDial(ctx context.Context, network, address string) (net.Conn, error) {
	return netguard.DialContext(ctx, network, address)
}
// FetchPreview resolves metadata for raw. The caller's context bounds the
// fetch (review CORR-02): when the incoming request is cancelled — the user
// closed the tab or aborted the save — the outbound I/O and HTML parsing stop
// instead of running to the 8-second client timeout.
func FetchPreview(ctx context.Context, raw string) Preview {
	u, e := url.Parse(raw)
	if e != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil {
		return Preview{}
	}
	client := http.Client{Timeout: 8 * time.Second, Transport: &http.Transport{DialContext: publicDial}, CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if len(via) > 5 {
			return fmt.Errorf("too many redirects")
		}
		if r.URL.Scheme != "http" && r.URL.Scheme != "https" {
			return fmt.Errorf("invalid scheme")
		}
		return nil
	}}
	req, e := http.NewRequestWithContext(ctx, "GET", raw, nil)
	if e != nil {
		return Preview{}
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; computer/1.0)")
	resp, e := client.Do(req)
	if e != nil {
		return Preview{}
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return Preview{}
	}
	// Review CORR-01: resolve relative favicons/og:images against the final
	// response URL, not the caller-supplied one — a redirect can change the
	// host or base path and break every relative asset.
	base := u
	if resp.Request != nil && resp.Request.URL != nil {
		base = resp.Request.URL
	}
	return ParseHTML(io.LimitReader(resp.Body, 1<<20), base)
}
func ParseHTML(r io.Reader, base *url.URL) Preview {
	out := Preview{}
	z := html.NewTokenizer(r)
	var title strings.Builder
	inTitle := false
	absolute := func(s string) *string {
		s = strings.TrimSpace(s)
		if s == "" {
			return nil
		}
		u, e := base.Parse(s)
		if e != nil || (u.Scheme != "http" && u.Scheme != "https") {
			return nil
		}
		s = u.String()
		return &s
	}
	for {
		t := z.Next()
		if t == html.ErrorToken {
			break
		}
		tok := z.Token()
		if t == html.TextToken && inTitle {
			title.WriteString(tok.Data)
		}
		if t == html.EndTagToken && tok.Data == "title" {
			inTitle = false
		}
		if t != html.StartTagToken && t != html.SelfClosingTagToken {
			continue
		}
		if tok.Data == "title" {
			inTitle = true
		}
		attrs := map[string]string{}
		for _, a := range tok.Attr {
			attrs[a.Key] = a.Val
		}
		if tok.Data == "meta" {
			key := strings.ToLower(attrs["property"])
			if key == "" {
				key = strings.ToLower(attrs["name"])
			}
			v := strings.TrimSpace(attrs["content"])
			if v != "" {
				switch key {
				case "og:title":
					if out.OgTitle == nil {
						out.OgTitle = &v
					}
				case "og:description":
					if out.OgDescription == nil {
						out.OgDescription = &v
					}
				case "description":
					if out.Description == nil {
						out.Description = &v
					}
				case "og:image":
					if out.OgImage == nil {
						out.OgImage = absolute(v)
					}
				}
			}
		}
		if tok.Data == "link" && strings.Contains(strings.ToLower(attrs["rel"]), "icon") && out.Favicon == nil {
			out.Favicon = absolute(attrs["href"])
		}
	}
	if out.OgTitle != nil {
		out.Title = out.OgTitle
	} else if s := strings.TrimSpace(title.String()); s != "" {
		out.Title = &s
	}
	if out.OgDescription != nil {
		out.Description = out.OgDescription
	}
	if out.Favicon == nil {
		s := base.Scheme + "://" + base.Host + "/favicon.ico"
		out.Favicon = &s
	}
	return out
}
