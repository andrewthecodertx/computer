package markdown

import (
	"fmt"
	"github.com/andrew/go-computer/internal/db"
	"net/http"
	"regexp"
	"strings"
)

var slugRE = regexp.MustCompile(`[^a-z0-9]+`)

func Slugify(s string) string {
	s = strings.Trim(slugRE.ReplaceAllString(strings.ToLower(s), "-"), "-")
	if len(s) > 60 {
		s = s[:60]
	}
	if s == "" {
		s = "untitled"
	}
	return s
}
func text(b db.Object, k string) string { s, _ := b[k].(string); return s }
func BookmarkDocument(b db.Object) string {
	title := text(b, "title")
	if title == "" {
		title = text(b, "url")
	}
	out := "# " + title + "\n\n- URL: " + text(b, "url") + "\n"
	if s := text(b, "dueDate"); len(s) >= 10 {
		out += "- Date: " + s[:10] + "\n"
	}
	tags := []string{}
	if a, ok := b["tags"].([]any); ok {
		for _, x := range a {
			bt, _ := x.(map[string]any)
			t, _ := bt["tag"].(map[string]any)
			tags = append(tags, text(t, "name"))
		}
	}
	if len(tags) > 0 {
		out += "- Tags: " + strings.Join(tags, ", ") + "\n"
	}
	return out + "\n" + strings.TrimSpace(text(b, "notes")) + "\n"
}
func PageDocument(p db.Object) string {
	out := "# " + text(p, "title") + "\n\n" + strings.TrimSpace(text(p, "content")) + "\n"
	if a, ok := p["bookmarks"].([]any); ok && len(a) > 0 {
		out += "\n## Links\n"
		for _, x := range a {
			pb, _ := x.(map[string]any)
			b, _ := pb["bookmark"].(map[string]any)
			title := text(b, "title")
			if title == "" {
				title = text(b, "url")
			}
			title = strings.NewReplacer("[", "", "]", "", "\n", " ").Replace(title)
			desc := strings.Join(strings.Fields(text(b, "description")), " ")
			rs := []rune(desc)
			if len(rs) > 160 {
				desc = string(rs[:160])
			}
			out += "- [" + title + "](" + text(b, "url") + ")"
			if desc != "" {
				out += " — " + desc
			}
			out += "\n"
		}
	}
	return out + "\n<!-- exported from computer " + text(p, "updatedAt") + " -->\n"
}
func Send(w http.ResponseWriter, body, name string) {
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.md"`, Slugify(name)))
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(body))
}
