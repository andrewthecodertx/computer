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
	"sync"
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

// previewClient is reused for all preview fetches (review PERF-1): a new
// Transport per call leaked idle connections because IdleConnTimeout
// defaulted to 0. A single client with bounded idle pool and timeout keeps
// the connection count and file descriptors under control.
var previewClient = &http.Client{
	Timeout: 8 * time.Second,
	Transport: &http.Transport{
		DialContext:         publicDial,
		IdleConnTimeout:     30 * time.Second,
		MaxIdleConns:        32,
		MaxIdleConnsPerHost: 8,
	},
	CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if len(via) > 5 {
			return fmt.Errorf("too many redirects")
		}
		if r.URL.Scheme != "http" && r.URL.Scheme != "https" {
			return fmt.Errorf("invalid scheme")
		}
		return nil
	},
}

// Shared preview-work budget (review CR-06). EVERY entry point that fetches
// preview metadata — the dedicated /api/preview route, bookmark creation and
// URL-changing updates — reserves against this one budget, so mutations can
// no longer bypass the preview route's throttle and generate unbounded
// outbound work. Two layers:
//   - per-identity rate: at most keyLimit reservations per keyWindow;
//   - global concurrency: at most maxConcurrent fetches in flight.
//
// Exhaustion returns an empty Preview immediately instead of queueing:
// previews are decorative, and a bookmark save must never be coupled to
// preview availability (the row persists without og metadata).
const (
	keyLimit      = 20
	keyWindow     = time.Minute
	maxConcurrent = 8
)

type rateWindow struct {
	count int
	reset time.Time
}

type budgetStore struct {
	mu       sync.Mutex
	inFlight int
	keys     map[string]*rateWindow
}

var sharedBudget = &budgetStore{keys: map[string]*rateWindow{}}

// Reserve claims one preview-work slot for key (the requesting identity).
// The returned release function MUST be called when the fetch finishes.
// ok is false when either budget layer is exhausted.
func Reserve(key string) (release func(), ok bool) {
	if key == "" {
		key = "anonymous"
	}
	sharedBudget.mu.Lock()
	defer sharedBudget.mu.Unlock()
	if sharedBudget.inFlight >= maxConcurrent {
		return nil, false
	}
	now := time.Now()
	// Opportunistic prune keeps the map bounded by active identities.
	if len(sharedBudget.keys) > 4096 {
		for k, w := range sharedBudget.keys {
			if now.After(w.reset) {
				delete(sharedBudget.keys, k)
			}
		}
	}
	w := sharedBudget.keys[key]
	if w == nil || now.After(w.reset) {
		w = &rateWindow{reset: now.Add(keyWindow)}
		sharedBudget.keys[key] = w
	}
	if w.count >= keyLimit {
		return nil, false
	}
	w.count++
	sharedBudget.inFlight++
	return func() {
		sharedBudget.mu.Lock()
		sharedBudget.inFlight--
		sharedBudget.mu.Unlock()
	}, true
}

// FetchPreview resolves metadata for raw on behalf of key (the requesting
// identity, e.g. "user:<id>"). The caller's context bounds the fetch (review
// CORR-02): when the incoming request is cancelled — the user closed the tab
// or aborted the save — the outbound I/O and HTML parsing stop instead of
// running to the 8-second client timeout. An exhausted shared budget (review
// CR-06) yields an empty Preview without any outbound work.
func FetchPreview(ctx context.Context, key, raw string) Preview {
	release, ok := Reserve(key)
	if !ok {
		return Preview{}
	}
	defer release()
	u, e := url.Parse(raw)
	if e != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil {
		return Preview{}
	}
	req, e := http.NewRequestWithContext(ctx, "GET", raw, nil)
	if e != nil {
		return Preview{}
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; computer/1.0)")
	resp, e := previewClient.Do(req)
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
