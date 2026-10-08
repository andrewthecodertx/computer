package main

import (
	"context"
	"errors"
	"github.com/andrew/go-computer/internal/config"
	"github.com/andrew/go-computer/internal/db"
	"github.com/andrew/go-computer/internal/http"
	"github.com/andrew/go-computer/internal/scripts"
	"github.com/andrew/go-computer/internal/signals"
	"log"
	stdhttp "net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// computer — link OS. Go is the API; the Next front-end is kept.
//
// Run: go run .            (or build and serve)
// Env: see porting guide section 7 (DATABASE_URL, NEXTAUTH_SECRET/AUTH_SECRET,
// NEXTAUTH_URL, optional OIDC_*, ADMIN_EMAILS, DEMO_PASSWORD).

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--healthcheck" {
		client := stdhttp.Client{Timeout: 3 * time.Second}
		res, err := client.Get("http://127.0.0.1:8080/readyz")
		if err != nil {
			os.Exit(1)
		}
		res.Body.Close()
		if res.StatusCode != 200 {
			os.Exit(1)
		}
		return
	}
	cfg := config.Load()
	d, err := db.New(cfg)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer d.Close()
	if len(os.Args) > 1 && os.Args[1] == "--seed" {
		if err := scripts.Seed(context.Background(), d); err != nil {
			log.Fatal(err)
		}
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go signals.RunLoop(ctx, d)
	srv := http.NewServer(cfg, d)
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		srv.Shutdown(shutdown)
	}()
	log.Printf("computer API listening on %s", srv.Addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, stdhttp.ErrServerClosed) {
		log.Fatal(err)
	}
}
