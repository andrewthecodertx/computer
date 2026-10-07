package http

import (
	"net/http"
	"os"
	"time"

	"github.com/andrew/go-computer/internal/auth"
	"github.com/andrew/go-computer/internal/authguard"
	"github.com/andrew/go-computer/internal/config"
	"github.com/andrew/go-computer/internal/db"
	"github.com/andrew/go-computer/internal/handlers"
)

// NewServer builds the HTTP server with all routes registered.
// Porting guide section 4 — every data endpoint resolves the effective user,
// returns 401 if none, checks ownership, returns 404 if not theirs, then acts.
//
// All routes return JSON unless marked .md. Errors are {error: string}.
func NewServer(cfg *config.Config, database *db.DB) *http.Server {
	mux := http.NewServeMux()

	// Health / readiness. No auth, no DB.
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	// 4.1 Auth and account
	mux.HandleFunc("POST /api/signup", auth.HandleSignup)
	mux.HandleFunc("POST /api/auth/login", auth.HandleLogin)
	mux.HandleFunc("GET /api/auth/", auth.HandleAuth) // csrf, callbacks, session, signout
	mux.HandleFunc("POST /api/auth/", auth.HandleAuth)
	mux.HandleFunc("GET /api/me", authed(auth.HandleMe))

	// 4.2 Admin (use real user; 403 unless ADMIN)
	mux.HandleFunc("GET /api/admin/users", admin(authguard.HandleAdminListUsers))
	mux.HandleFunc("PATCH /api/admin/users", admin(authguard.HandleAdminPatchUser))
	mux.HandleFunc("POST /api/admin/view-as", admin(authguard.HandleAdminViewAs))

	// 4.3 Bookmarks
	mux.HandleFunc("GET /api/bookmarks", authed(handlers.HandleBookmarksList))
	mux.HandleFunc("POST /api/bookmarks", authed(handlers.HandleBookmarksCreate))
	mux.HandleFunc("GET /api/bookmarks/{id}", authed(handlers.HandleBookmarksGet))
	mux.HandleFunc("PUT /api/bookmarks/{id}", authed(handlers.HandleBookmarksUpdate))
	mux.HandleFunc("DELETE /api/bookmarks/{id}", authed(handlers.HandleBookmarksDelete))
	mux.HandleFunc("POST /api/bookmarks/{id}/share", authed(handlers.HandleBookmarksShare))
	mux.HandleFunc("GET /api/bookmarks/{id}/markdown", authed(handlers.HandleBookmarksMarkdown))
	mux.HandleFunc("GET /api/preview", handlers.HandlePreview)

	// 4.4 Tags
	mux.HandleFunc("GET /api/tags", authed(handlers.HandleTagsList))
	mux.HandleFunc("POST /api/tags", authed(handlers.HandleTagsCreate))
	mux.HandleFunc("PUT /api/tags/{id}", authed(handlers.HandleTagsUpdate))
	mux.HandleFunc("DELETE /api/tags/{id}", authed(handlers.HandleTagsDelete))

	// 4.5 Kanban columns
	mux.HandleFunc("GET /api/kanban/columns", authed(handlers.HandleKanbanColumnsList))
	mux.HandleFunc("POST /api/kanban/columns", authed(handlers.HandleKanbanColumnsCreate))
	mux.HandleFunc("PUT /api/kanban/columns", authed(handlers.HandleKanbanColumnsReorder))
	mux.HandleFunc("PATCH /api/kanban/columns/{id}", authed(handlers.HandleKanbanColumnsUpdate))
	mux.HandleFunc("DELETE /api/kanban/columns/{id}", authed(handlers.HandleKanbanColumnsDelete))

	// 4.6 Pages
	mux.HandleFunc("GET /api/pages", authed(handlers.HandlePagesList))
	mux.HandleFunc("POST /api/pages", authed(handlers.HandlePagesCreate))
	mux.HandleFunc("PUT /api/pages", authed(handlers.HandlePagesReorder))
	mux.HandleFunc("GET /api/pages/{id}", authed(handlers.HandlePagesGet))
	mux.HandleFunc("PATCH /api/pages/{id}", authed(handlers.HandlePagesUpdate))
	mux.HandleFunc("DELETE /api/pages/{id}", authed(handlers.HandlePagesDelete))
	mux.HandleFunc("GET /api/pages/{id}/markdown", authed(handlers.HandlePagesMarkdown))

	// 4.7 Signals
	mux.HandleFunc("GET /api/bookmarks/{id}/signals", authed(handlers.HandleSignalsList))
	mux.HandleFunc("POST /api/bookmarks/{id}/signals", authed(handlers.HandleSignalsCreate))
	mux.HandleFunc("PATCH /api/signal-sources/{id}", authed(handlers.HandleSignalSourcesUpdate))
	mux.HandleFunc("DELETE /api/signal-sources/{id}", authed(handlers.HandleSignalSourcesDelete))

	// 4.8 Alerts, contacts, email, dates, users
	mux.HandleFunc("GET /api/alerts/check", authed(handlers.HandleAlertsCheck))
	mux.HandleFunc("DELETE /api/alerts/{bookmarkId}", authed(handlers.HandleAlertsDismiss))
	mux.HandleFunc("GET /api/contacts", authed(handlers.HandleContactsList))
	mux.HandleFunc("POST /api/contacts/sync", authed(handlers.HandleContactsSync))
	mux.HandleFunc("GET /api/settings/imap", authed(handlers.HandleImapGet))
	mux.HandleFunc("POST /api/settings/imap", authed(handlers.HandleImapSave))
	mux.HandleFunc("POST /api/imap/test", authed(handlers.HandleImapTest))
	mux.HandleFunc("GET /api/imap/check", authed(handlers.HandleImapCheck))
	mux.HandleFunc("GET /api/shared-dates", authed(handlers.HandleSharedDatesList))
	mux.HandleFunc("POST /api/shared-dates", authed(handlers.HandleSharedDatesCreate))
	mux.HandleFunc("GET /api/users/search", authed(handlers.HandleUsersSearch))

	// 4.9 Public page (no login)
	mux.HandleFunc("GET /share/bookmark/{id}", handlers.HandlePublicBookmark)

	return &http.Server{
		Handler:      withDB(database, mux),
		Addr:         ":" + port(),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
}

// withDB carries the database into request context so handlers can pull it via
// db.FromContext. This keeps the DB out of global state (testable, swappable).
func withDB(database *db.DB, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := db.NewContext(r.Context(), database)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// authed wraps a data route with session authentication (401 if no valid
// session). All data endpoints follow: resolve effective user -> 401 ->
// ownership -> 404 -> act (porting guide 5.1).
func authed(fn http.HandlerFunc) http.HandlerFunc {
	return auth.AuthMiddleware(http.HandlerFunc(fn)).ServeHTTP
}

// admin wraps a route with auth + admin-only (403 unless ADMIN).
// AuthMiddleware runs first (attaches the session), then RequireAdmin checks it.
func admin(fn http.HandlerFunc) http.HandlerFunc {
	return auth.AuthMiddleware(auth.RequireAdmin(http.HandlerFunc(fn))).ServeHTTP
}

// port returns the listen port from PORT env, defaulting to 8080.
func port() string {
	if p := os.Getenv("PORT"); p != "" {
		return p
	}
	return "8080"
}