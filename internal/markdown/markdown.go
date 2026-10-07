package markdown

// Porting guide 3.7.

// Slugify lowercases, runs of non [a-z0-9] become "-", trim dashes, max 60
// chars, fallback "untitled".
func Slugify(s string) string { return "" }

// Page is the input shape for pageToMarkdown.
type Page struct {
	Title     string
	Content   string
	Links     []PageLink
	UpdatedAt string
}

// PageLink is one link embedded in a page.
type PageLink struct {
	Title       string
	URL         string
	Description string
}

// PageToMarkdown returns "# title", blank line, trimmed content; if links exist,
// "## Links" then "- [title](url) — description" (title has "[" "]" removed;
// description whitespace collapsed, max 160 chars); final line
// "<!-- exported from computer <updatedAt ISO> -->".
func PageToMarkdown(page Page) string { return "" }

// Bookmark is the input shape for bookmarkToMarkdown.
type Bookmark struct {
	Title    string
	URL      string
	DueDate  *string
	Tags     []string
	Notes    string
}

// BookmarkToMarkdown returns "# title-or-url", then "- URL: <url>",
// "- Date: YYYY-MM-DD" (if dueDate), "- Tags: a, b" (if any), blank line,
// trimmed notes.
func BookmarkToMarkdown(b Bookmark) string { return "" }

// MarkdownResponse sets Content-Type text/markdown; charset=utf-8 and
// Content-Disposition: attachment; filename="<name>.md".
type MarkdownResponse struct {
	ContentDisposition string
	Body               string
}

// MarkdownResponseFor builds the response for a download named "name".
func MarkdownResponseFor(body, name string) MarkdownResponse { return MarkdownResponse{} }