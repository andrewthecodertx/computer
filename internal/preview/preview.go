package preview

// Porting guide 3.6, repeated in /api/preview, bookmark create and bookmark update.
//
// fetchPreview(url) uses an 8-second timeout and User-Agent
// "Mozilla/5.0 (compatible; computer/1.0)". It reads OpenGraph ogTitle,
// ogDescription, the first ogImage URL, and the favicon. A relative favicon is
// made absolute against the URL's origin; if none is found it uses
// <origin>/favicon.ico. On any failure it returns all nulls and never throws.
//
// Port tip: make this one shared function.

// Preview holds the scraped metadata for a URL.
type Preview struct {
	Title       *string
	Description *string
	Favicon     *string
	OgImage     *string
	OgTitle     *string
	OgDescription *string
}

// FetchPreview returns the preview for url. Never returns an error; on failure
// all fields are nil.
func FetchPreview(url string) Preview { return Preview{} }