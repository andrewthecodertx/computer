package signals

import (
	"context"
	"github.com/andrew/go-computer/internal/db"
	"log"
	"time"
)

// Browser alerts remain browser-only. Email watchers are checked independently
// of an open tab, with source/external-ID deduplication on every run.
func RunLoop(ctx context.Context, d *db.DB) {
	tick := time.NewTicker(5 * time.Minute)
	defer tick.Stop()
	run := func() {
		users, e := db.Many(ctx, d, `SELECT jsonb_build_object('id',"ownerId") FROM "SignalSource" WHERE enabled GROUP BY "ownerId"`)
		if e != nil {
			log.Printf("watchers: query failed: %v", e)
			return
		}
		for _, u := range users {
			if ctx.Err() != nil {
				return
			}
			job, cancel := context.WithTimeout(ctx, 45*time.Second)
			_, _, e := RunSignalChecks(job, d, u["id"].(string), "")
			cancel()
			if e != nil {
				log.Printf("watchers: check failed: %v", e)
			}
		}
	}
	run()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			run()
		}
	}
}
