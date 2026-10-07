package http

import (
	"github.com/andrew/go-computer/internal/auth"
	"github.com/andrew/go-computer/internal/authguard"
	"github.com/andrew/go-computer/internal/config"
	"github.com/andrew/go-computer/internal/db"
	"github.com/andrew/go-computer/internal/handlers"
	"net/http"
)

// NewServer builds the HTTP server with all routes registered.
// Porting guide section 4 — every data endpoint resolves the effective user,
// returns 401 if none, checks ownership, returns 404 if not theirs, then acts.
//
// All routes return JSON unless marked .md. Errors are {error: string}.
func NewServer(cfg *config.Config, database *db.DB) *http.Server {
	mux := http.NewServeMux()

	// 4.1 Auth and account
	mux.HandleFunc("POST /api/signup", auth.HandleSignup)
	mux.HandleFunc("POST /api/auth/login", auth.HandleLogin)
	mux.HandleFunc("GET /api/auth/", auth.HandleAuth) // csrf, callbacks, session, signout
	mux.HandleFunc("POST /api/auth/", auth.HandleAuth)
	mux.HandleFunc("GET /api/me", auth.HandleMe)

	// 4.2 Admin (use real user; 403 unless ADMIN)
	mux.HandleFunc("GET /api/admin/users", authguard.HandleAdminListUsers)
	mux.HandleFunc("PATCH /api/admin/users", authguard.HandleAdminPatchUser)
	mux.HandleFunc("POST /api/admin/view-as", authguard.HandleAdminViewAs)

	// 4.3 Bookmarks
	mux.HandleFunc("GET /api/bookmarks", handlers.HandleBookmarksList)
	mux.HandleFunc("POST /api/bookmarks", handlers.HandleBookmarksCreate)
	mux.HandleFunc("GET /api/bookmarks/{id}", handlers.HandleBookmarksGet)
	mux.HandleFunc("PUT /api/bookmarks/{id}", handlers.HandleBookmarksUpdate)
	mux.HandleFunc("DELETE /api/bookmarks/{id}", handlers.HandleBookmarksDelete)
	mux.HandleFunc("POST /api/bookmarks/{id}/share", handlers.HandleBookmarksShare)
	mux.HandleFunc("GET /api/bookmarks/{id}/markdown", handlers.HandleBookmarksMarkdown)
	mux.HandleFunc("GET /api/preview", handlers.HandlePreview)

	// 4.4 Tags
	mux.HandleFunc("GET /api/tags", handlers.HandleTagsList)
	mux.HandleFunc("POST /api/tags", handlers.HandleTagsCreate)
	mux.HandleFunc("PUT /api/tags/{id}", handlers.HandleTagsUpdate)
	mux.HandleFunc("DELETE /api/tags/{id}", handlers.HandleTagsDelete)

	// 4.5 Kanban columns
	mux.HandleFunc("GET /api/kanban/columns", handlers.HandleKanbanColumnsList)
	mux.HandleFunc("POST /api/kanban/columns", handlers.HandleKanbanColumnsCreate)
	mux.HandleFunc("PUT /api/kanban/columns", handlers.HandleKanbanColumnsReorder)
	mux.HandleFunc("PATCH /api/kanban/columns/{id}", handlers.HandleKanbanColumnsUpdate)
	mux.HandleFunc("DELETE /api/kanban/columns/{id}", handlers.HandleKanbanColumnsDelete)

	// 4.6 Pages
	mux.HandleFunc("GET /api/pages", handlers.HandlePagesList)
	mux.HandleFunc("POST /api/pages", handlers.HandlePagesCreate)
	mux.HandleFunc("PUT /api/pages", handlers.HandlePagesReorder)
	mux.HandleFunc("GET /api/pages/{id}", handlers.HandlePagesGet)
	mux.HandleFunc("PATCH /api/pages/{id}", handlers.HandlePagesUpdate)
	mux.HandleFunc("DELETE /api/pages/{id}", handlers.HandlePagesDelete)
	mux.HandleFunc("GET /api/pages/{id}/markdown", handlers.HandlePagesMarkdown)

	// 4.7 Signals
	mux.HandleFunc("GET /api/bookmarks/{id}/signals", handlers.HandleSignalsList)
	mux.HandleFunc("POST /api/bookmarks/{id}/signals", handlers.HandleSignalsCreate)
	mux.HandleFunc("PATCH /api/signal-sources/{id}", handlers.HandleSignalSourcesUpdate)
	mux.HandleFunc("DELETE /api/signal-sources/{id}", handlers.HandleSignalSourcesDelete)

	// 4.8 Alerts, contacts, email, dates, users
	mux.HandleFunc("GET /api/alerts/check", handlers.HandleAlertsCheck)
	mux.HandleFunc("DELETE /api/alerts/{bookmarkId}", handlers.HandleAlertsDismiss)
	mux.HandleFunc("GET /api/contacts", handlers.HandleContactsList)
	mux.HandleFunc("POST /api/contacts/sync", handlers.HandleContactsSync)
	mux.HandleFunc("GET /api/settings/imap", handlers.HandleImapGet)
	mux.HandleFunc("POST /api/settings/imap", handlers.HandleImapSave)
	mux.HandleFunc("POST /api/imap/test", handlers.HandleImapTest)
	mux.HandleFunc("GET /api/imap/check", handlers.HandleImapCheck)
	mux.HandleFunc("GET /api/shared-dates", handlers.HandleSharedDatesList)
	mux.HandleFunc("POST /api/shared-dates", handlers.HandleSharedDatesCreate)
	mux.HandleFunc("GET /api/users/search", handlers.HandleUsersSearch)

	// 4.9 Public page (no login)
	mux.HandleFunc("GET /share/bookmark/{id}", handlers.HandlePublicBookmark)

	return &http.Server{Handler: mux}
}