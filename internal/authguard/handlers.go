package authguard

import (
	"encoding/json"
	"github.com/andrew/go-computer/internal/auth"
	"github.com/andrew/go-computer/internal/db"
	"github.com/andrew/go-computer/internal/handlers"
	"net/http"
	"os"
	"time"
)

func HandleAdminListUsers(w http.ResponseWriter, r *http.Request) {
	v, e := db.Many(r.Context(), db.FromContext(r.Context()), `SELECT jsonb_build_object('id',u.id,'name',u.name,'email',u.email,'image',u.image,'role',u.role,'createdAt',u."createdAt",'_count',jsonb_build_object('bookmarks',(SELECT count(*) FROM "Bookmark" b WHERE b."ownerId"=u.id),'pages',(SELECT count(*) FROM "Page" p WHERE p."ownerId"=u.id),'tags',(SELECT count(*) FROM "Tag" t WHERE t."ownerId"=u.id))) FROM "User" u WHERE email NOT LIKE '%@example.com' ORDER BY "createdAt"`)
	if e != nil {
		handlers.WriteJSON(w, 500, handlers.Error{Error: "Database unavailable"})
		return
	}
	handlers.WriteJSON(w, 200, v)
}
func HandleAdminPatchUser(w http.ResponseWriter, r *http.Request) {
	var b struct {
		UserID string `json:"userId"`
		Role   string `json:"role"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&b) != nil || (b.Role != "USER" && b.Role != "ADMIN") {
		handlers.WriteJSON(w, 400, handlers.Error{Error: "Invalid role"})
		return
	}
	s, _ := auth.SessionFromContext(r.Context())
	if b.UserID == s.RealID && b.Role != "ADMIN" {
		handlers.WriteJSON(w, 400, handlers.Error{Error: "You cannot demote yourself"})
		return
	}
	repo := db.NewUserRepo(db.FromContext(r.Context()))
	if _, e := repo.FindByID(r.Context(), b.UserID); e != nil {
		handlers.WriteJSON(w, 404, handlers.Error{Error: "Not found"})
		return
	}
	if e := repo.SetRole(r.Context(), b.UserID, b.Role); e != nil {
		handlers.WriteJSON(w, 500, handlers.Error{Error: "Unable to change role"})
		return
	}
	handlers.WriteJSON(w, 200, handlers.OK{OK: true})
}
func HandleAdminViewAs(w http.ResponseWriter, r *http.Request) {
	var b struct {
		UserID *string `json:"userId"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&b) != nil {
		handlers.WriteJSON(w, 400, handlers.Error{Error: "Invalid JSON"})
		return
	}
	s, _ := auth.SessionFromContext(r.Context())
	c := http.Cookie{Name: auth.ViewAsCookie, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: os.Getenv("COOKIE_SECURE") == "1", MaxAge: -1}
	if b.UserID != nil && *b.UserID != "" && *b.UserID != s.RealID {
		if _, e := db.NewUserRepo(db.FromContext(r.Context())).FindByID(r.Context(), *b.UserID); e != nil {
			handlers.WriteJSON(w, 404, handlers.Error{Error: "Not found"})
			return
		}
		c.Value = *b.UserID
		c.MaxAge = int((8 * time.Hour).Seconds())
	}
	http.SetCookie(w, &c)
	handlers.WriteJSON(w, 200, handlers.OK{OK: true})
}
