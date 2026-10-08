package models

// KanbanStatus is the legacy enum. Porting guide 2.1.
type KanbanStatus string

const (
	Inbox      KanbanStatus = "INBOX"
	Todo       KanbanStatus = "TODO"
	InProgress KanbanStatus = "IN_PROGRESS"
	Done       KanbanStatus = "DONE"
	Archived   KanbanStatus = "ARCHIVED"
)

// User. Porting guide 2.2.
type User struct {
	ID            string
	Name          *string
	Email         *string
	EmailVerified *string
	Image         *string
	Password      *string
	Role          string // USER | ADMIN
	CreatedAt     string
	UpdatedAt     string
}

// Account, Session, VerificationToken are auth-library tables. Porting guide 2.3.
// Session and VerificationToken are unused (sessions are JWT-based).
// Account is only needed if keeping OIDC.
type Account struct {
	ID                string
	UserID            string
	Type              string
	Provider          string
	ProviderAccountID string
	RefreshToken      *string
	AccessToken       *string
	ExpiresAt         *int
	TokenType         *string
	Scope             *string
	IDToken           *string
	SessionState      *string
}

// Bookmark is the core object: one saved URL. Porting guide 2.4.
type Bookmark struct {
	ID               string
	URL              string
	Title            *string
	Description      *string
	Favicon          *string
	OgImage          *string
	OgTitle          *string
	OgDescription    *string
	Notes            *string
	DueDate          *string
	AlertAt          *string
	AlertSent        bool
	KanbanStatus     KanbanStatus
	KanbanColumnID   *string
	ImapWatchEnabled bool
	ImapQuery        *string
	IsPublic         bool
	OwnerID          string
	ContactID        *string
	CreatedAt        string
	UpdatedAt        string
}

// Tag. Porting guide 2.5.
type Tag struct {
	ID        string
	Name      string
	Color     string
	OwnerID   string
	CreatedAt string
	UpdatedAt string
}

// BookmarkTag join. Porting guide 2.6.
type BookmarkTag struct {
	BookmarkID string
	TagID      string
}

// BookmarkShare. Porting guide 2.7.
type BookmarkShare struct {
	BookmarkID string
	UserID     string
	CreatedAt  string
}

// TagShare. Porting guide 2.8.
type TagShare struct {
	TagID     string
	UserID    string
	CreatedAt string
}

// Contact, synced from Nextcloud CardDAV. Porting guide 2.9.
type Contact struct {
	ID          string
	UID         string
	DisplayName string
	Email       *string
	Phone       *string
	CardDavURL  *string
	UserID      string
	CreatedAt   string
	UpdatedAt   string
}

// ImapConfig, one per user; password is encrypted. Porting guide 2.10.
type ImapConfig struct {
	ID       string
	Host     string
	Port     int
	TLS      bool
	Username string
	Password string // encrypted
	Folder   string
	UserID   string
}

// KanbanColumn, user-editable board column. Porting guide 2.11.
type KanbanColumn struct {
	ID        string
	Label     string
	Color     string
	Position  int
	Key       *string // INBOX/TODO/IN_PROGRESS/DONE for defaults, null for custom
	UserID    string
	CreatedAt string
}

// Page, a Markdown workspace. Porting guide 2.12.
type Page struct {
	ID        string
	Title     string
	Content   string
	Icon      *string
	Pinned    bool
	Position  int
	OwnerID   string
	CreatedAt string
	UpdatedAt string
}

// PageBookmark links a bookmark onto a page. Porting guide 2.13.
type PageBookmark struct {
	PageID     string
	BookmarkID string
	Position   int
	CreatedAt  string
}

// SignalSource, a watcher attached to a bookmark. Porting guide 2.14.
type SignalSource struct {
	ID            string
	Type          string
	Config        string // JSON, default "{}"
	Enabled       bool
	LastCheckedAt *string
	LastStatus    *string
	BookmarkID    string
	OwnerID       string
	CreatedAt     string
	UpdatedAt     string
}

// Signal, one outside event about a bookmark. Porting guide 2.15.
type Signal struct {
	ID         string
	SourceType string
	Title      string
	Summary    *string
	URL        *string
	ExternalID *string
	OccurredAt string
	Read       bool
	SourceID   *string
	BookmarkID string
	OwnerID    string
	CreatedAt  string
}

// SharedDate. Porting guide 2.16.
type SharedDate struct {
	ID          string
	Date        string
	Note        *string
	CreatorID   string
	RecipientID string
	CreatedAt   string
}
