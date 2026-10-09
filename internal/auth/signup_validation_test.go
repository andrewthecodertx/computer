package auth

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

// Signup input validation must reject before any database access, so these
// run without a DB context.
func TestSignupValidation(t *testing.T) {
	cases := []struct {
		name string
		body map[string]any
	}{
		{"empty", map[string]any{}},
		{"missing password", map[string]any{"email": "ok@computer.test"}},
		{"invalid email", map[string]any{"email": "not-an-email", "password": "long-enough-password"}},
		{"email with display name", map[string]any{"email": "Name <ok@computer.test>", "password": "long-enough-password"}},
		{"short password", map[string]any{"email": "ok@computer.test", "password": "short"}},
		{"7 boundary password", map[string]any{"email": "ok@computer.test", "password": "1234567"}},
		{"overlong password", map[string]any{"email": "ok@computer.test", "password": strings.Repeat("a", 73)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw, _ := json.Marshal(tc.body)
			req := httptest.NewRequest("POST", "/api/signup", bytes.NewReader(raw))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			HandleSignup(w, req)
			if w.Code != 400 {
				t.Fatalf("wanted 400, got %d: %s", w.Code, w.Body.String())
			}
		})
	}
}
