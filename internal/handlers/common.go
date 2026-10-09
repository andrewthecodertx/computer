package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/andrew/go-computer/internal/auth"
	"github.com/andrew/go-computer/internal/db"
	"github.com/jackc/pgx/v5/pgconn"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

type object = db.Object
type clientError struct{ message string }

func (e clientError) Error() string   { return e.message }
func bad(s string) error              { return clientError{s} }

// conflictError maps to 409: the request collides with newer server state
// (optimistic concurrency, review DATA-01), not with a unique constraint.
type conflictError struct{ message string }

func (e conflictError) Error() string { return e.message }
func conflict(s string) error         { return conflictError{s} }
func database(r *http.Request) *db.DB { return db.FromContext(r.Context()) }
func user(r *http.Request) string     { s, _ := auth.SessionFromContext(r.Context()); return s.ID }
func read(w http.ResponseWriter, r *http.Request) (object, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var b object
	dec := json.NewDecoder(r.Body)
	if dec.Decode(&b) != nil || b == nil {
		writeJSON(w, 400, Error{"Invalid JSON object"})
		return nil, false
	}
	var extra any
	if dec.Decode(&extra) != io.EOF {
		writeJSON(w, 400, Error{"Unexpected data after JSON"})
		return nil, false
	}
	return b, true
}
func respond(w http.ResponseWriter, status int, v any, e error) {
	if e != nil {
		code, msg := 500, "Server error"
		var ce clientError
		var cfe conflictError
		var pg *pgconn.PgError
		switch {
		case errors.Is(e, db.ErrNotFound):
			code, msg = 404, "Not found"
		case errors.As(e, &ce):
			code, msg = 400, ce.message
		case errors.As(e, &cfe):
			code, msg = 409, cfe.message
		case errors.As(e, &pg) && pg.Code == "23505":
			code, msg = 409, "Already exists"
		case errors.As(e, &pg) && (pg.Code == "23503" || pg.Code == "22P02"):
			code, msg = 400, "Invalid related record or value"
		default:
			log.Printf("request: %v", e)
		}
		writeJSON(w, code, Error{msg})
		return
	}
	writeJSON(w, status, v)
}
func str(b object, k string) string { v, _ := b[k].(string); return v }
func ids(b object, k string) ([]string, error) {
	v, ok := b[k]
	if !ok {
		return nil, nil
	}
	list, ok := v.([]any)
	if !ok {
		return nil, bad(k + " must be an array")
	}
	out := []string{}
	seen := map[string]bool{}
	for _, a := range list {
		s, ok := a.(string)
		if !ok || s == "" {
			return nil, bad("Invalid " + k)
		}
		if !seen[s] {
			out = append(out, s)
			seen[s] = true
		} else {
			return nil, bad("Duplicate " + k)
		}
	}
	return out, nil
}
func fields(b object, allowed string) object {
	out := object{}
	for _, k := range strings.Fields(allowed) {
		if v, ok := b[k]; ok {
			out[k] = v
		}
	}
	return out
}
func owned(r *http.Request, q db.Queryer, table, id, col string) (object, error) {
	return db.One(r.Context(), q, `SELECT to_jsonb(t) FROM `+db.Quote(table)+` t WHERE id=$1 AND `+db.Quote(col)+`=$2`, id, user(r))
}
func remove(r *http.Request, table, id, col string) error {
	res, e := database(r).ExecContext(r.Context(), `DELETE FROM `+db.Quote(table)+` WHERE id=$1 AND `+db.Quote(col)+`=$2`, id, user(r))
	if e != nil {
		return e
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return db.ErrNotFound
	}
	return nil
}
func dateValue(v any) (any, error) {
	if v == nil {
		return nil, nil
	}
	s, ok := v.(string)
	if !ok {
		return nil, bad("Invalid date")
	}
	if s == "" {
		return nil, nil
	}
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04", "2006-01-02T15:04:05", "2006-01-02"} {
		if t, e := time.Parse(layout, s); e == nil {
			return t.UTC(), nil
		}
	}
	return nil, bad("Invalid date")
}

var colorPattern = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func validColor(b object) error {
	if v, ok := b["color"]; ok {
		if s, ok := v.(string); !ok || !colorPattern.MatchString(s) {
			return bad("Color must be #RRGGBB")
		}
	}
	return nil
}
func validURL(s string) bool {
	u, e := url.Parse(s)
	return e == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" && u.User == nil
}
func stringFields(b object, nullable, required string) error {
	for _, k := range strings.Fields(nullable) {
		if v, ok := b[k]; ok && v != nil {
			if _, ok := v.(string); !ok {
				return bad(k + " must be text")
			}
		}
	}
	for _, k := range strings.Fields(required) {
		if v, ok := b[k]; ok {
			if _, ok := v.(string); !ok {
				return bad(k + " must be text")
			}
		}
	}
	return nil
}
func boolFields(b object, keys string) error {
	for _, k := range strings.Fields(keys) {
		if v, ok := b[k]; ok {
			if _, ok := v.(bool); !ok {
				return bad(k + " must be boolean")
			}
		}
	}
	return nil
}
func lockUser(r *http.Request, tx *sql.Tx) error {
	_, e := tx.ExecContext(r.Context(), `SELECT id FROM "User" WHERE id=$1 FOR UPDATE`, user(r))
	return e
}
func reorder(r *http.Request, table, col string, order []string) error {
	return db.Transaction(r.Context(), database(r), func(tx *sql.Tx) error {
		if e := lockUser(r, tx); e != nil {
			return e
		}
		var count int
		if e := tx.QueryRowContext(r.Context(), `SELECT count(*) FROM `+db.Quote(table)+` WHERE `+db.Quote(col)+`=$1`, user(r)).Scan(&count); e != nil {
			return e
		}
		if count != len(order) {
			return bad("Order must contain every record exactly once")
		}
		for i, id := range order {
			res, e := tx.ExecContext(r.Context(), `UPDATE `+db.Quote(table)+` SET position=$1 WHERE id=$2 AND `+db.Quote(col)+`=$3`, i, id, user(r))
			if e != nil {
				return e
			}
			n, _ := res.RowsAffected()
			if n == 0 {
				return bad("Invalid order")
			}
		}
		return nil
	})
}
func shareRows(r *http.Request, tx *sql.Tx, table, key, id string, list []string) error {
	for _, u := range list {
		if _, e := tx.ExecContext(r.Context(), `INSERT INTO `+db.Quote(table)+` (`+db.Quote(key)+`,"userId") VALUES($1,$2) ON CONFLICT DO NOTHING`, id, u); e != nil {
			return e
		}
	}
	return nil
}
