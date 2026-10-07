package main

import (
	"github.com/andrew/go-computer/internal/config"
	"github.com/andrew/go-computer/internal/db"
	"github.com/andrew/go-computer/internal/http"
	"log"
)

// computer — link OS. Go is the API; the Next front-end is kept.
//
// Run: go run .            (or build and serve)
// Env: see porting guide section 7 (DATABASE_URL, NEXTAUTH_SECRET/AUTH_SECRET,
// NEXTAUTH_URL, optional OIDC_*, ADMIN_EMAILS, DEMO_PASSWORD).

func main() {
	cfg := config.Load()
	d, err := db.New(cfg)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer d.Close()

	srv := http.NewServer(cfg, d)
	log.Fatal(srv.ListenAndServe())
}