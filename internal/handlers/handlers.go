package handlers

import (
	"encoding/json"
	"net/http"
)

// Common helpers shared by all handlers. Porting guide section 4.
//
// Every data endpoint follows the same pattern: resolve the effective user,
// return 401 if none, check ownership with a filtered lookup, return 404 if
// the record is not theirs, then act.
//
// All routes return JSON unless marked .md. Errors are {error: string}.

// OK is the success response shape for mutations.
type OK struct {
	OK bool `json:"ok"`
}

// Error is the error response shape.
type Error struct {
	Error string `json:"error"`
}

// NotFound returns a 404 {error} response.
func NotFound(message string) Error {
	if message == "" {
		message = "not found"
	}
	return Error{Error: message}
}

// Unauthorized returns a 401 {error} response.
func Unauthorized(message string) Error {
	if message == "" {
		message = "unauthorized"
	}
	return Error{Error: message}
}

// Forbidden returns a 403 {error} response.
func Forbidden(message string) Error {
	if message == "" {
		message = "forbidden"
	}
	return Error{Error: message}
}

// BadRequest returns a 400 {error} response.
func BadRequest(message string) Error {
	if message == "" {
		message = "bad request"
	}
	return Error{Error: message}
}

// Conflict returns a 409 {error} response.
func Conflict(message string) Error {
	return Error{Error: message}
}

// WriteJSON writes v as JSON with the given status.
func WriteJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeJSON is the lowercase alias used by the stub handlers.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	WriteJSON(w, status, v)
}
