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
// NEXTAUTH_URL, optional OIDC_*, ADMIN_EMAILS).

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
	if cfg.Secret() == "" {
		log.Fatal("config: AUTH_SECRET (or NEXTAUTH_SECRET) must be set")
	}
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
	// Shutdown order is owned by main (review OPS-01): drain in-flight HTTP
	// requests first, then await the background loops, and only afterwards let
	// the deferred db close run. Previously Shutdown raced ListenAndServe's
	// return and the process exited mid-drain.
	workersDone := make(chan struct{})
	go func() {
		defer close(workersDone)
		signals.RunLoop(ctx, d)
	}()
	srv := http.NewServer(cfg, d)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, stdhttp.ErrServerClosed) {
			log.Fatalf("server failed: %v", err)
		}
	}()
	log.Printf("computer API listening on %s", srv.Addr)
	<-ctx.Done()
	stop()
	shutdown, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdown); err != nil {
		log.Printf("shutdown: %v", err)
	}
	<-workersDone
}
