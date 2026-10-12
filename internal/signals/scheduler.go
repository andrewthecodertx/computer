package signals

import (
	"context"
	"github.com/andrew/go-computer/internal/db"
	"log"
	"sync"
	"time"
)

const (
	// pollConcurrency bounds simultaneous mailbox passes; each pass holds one
	// IMAP login and a 45-second budget (review SCALE-01).
	pollConcurrency = 3
	perUserTimeout  = 45 * time.Second
	// maxBackoffShift caps failure backoff at 2^4 ticks (~80 minutes).
	maxBackoffShift = 4
)

// Scheduler polls users with enabled signal sources. Review SCALE-01:
// bounded concurrency, per-user exponential backoff on consecutive failures,
// logged outcomes (counts only — never credentials or mail content), and a
// database-backed advisory lease so multiple app instances never poll the
// same user simultaneously. Database deduplication would hide duplicate
// records but not the duplicated external work the lease prevents.
type Scheduler struct {
	mu       sync.Mutex
	failures map[string]int
	tick     int
}

func NewScheduler() *Scheduler { return &Scheduler{failures: map[string]int{}} }

// RunOnce performs a single scheduler pass over every user with enabled
// sources. Extracted from RunLoop so tests can drive one tick directly.
func RunOnce(ctx context.Context, d *db.DB) { NewScheduler().RunOnce(ctx, d) }

func (s *Scheduler) RunOnce(ctx context.Context, d *db.DB) {
	users, e := db.Many(ctx, d, `SELECT jsonb_build_object('id',"ownerId") FROM "SignalSource" WHERE enabled GROUP BY "ownerId"`)
	if e != nil {
		log.Printf("watchers: query failed: %v", e)
		return
	}
	s.mu.Lock()
	s.tick++
	tick := s.tick
	s.mu.Unlock()
	sem := make(chan struct{}, pollConcurrency)
	var wg sync.WaitGroup
	for _, u := range users {
		if ctx.Err() != nil {
			break
		}
		id, _ := u["id"].(string)
		if id == "" || s.skipTick(id, tick) {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			s.checkUser(ctx, d, id)
		}()
	}
	wg.Wait()
}

// skipTick applies per-user exponential backoff: after n consecutive failed
// passes the user is polled on every 2^n-th tick instead of every tick.
func (s *Scheduler) skipTick(id string, tick int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	f := s.failures[id]
	if f == 0 {
		return false
	}
	shift := min(f, maxBackoffShift)
	return tick%(1<<shift) != 0
}

func (s *Scheduler) checkUser(ctx context.Context, d *db.DB, id string) {
	// Database-backed lease held on a dedicated connection for the duration
	// of the pass: a second poller instance skips this user instead of
	// repeating the same external mailbox work.
	conn, e := d.Conn(ctx)
	if e != nil {
		s.note(id, false)
		log.Printf("watchers: lease connection for user %s failed: %v", id, e)
		return
	}
	defer conn.Close()
	var locked bool
	if e := conn.QueryRowContext(ctx, `SELECT pg_try_advisory_lock(hashtextextended($1,0))`, "signal-poll:"+id).Scan(&locked); e != nil {
		s.note(id, false)
		log.Printf("watchers: lease for user %s failed: %v", id, e)
		return
	}
	if !locked {
		return // another instance owns this user's pass
	}
	defer conn.ExecContext(context.WithoutCancel(ctx), `SELECT pg_advisory_unlock(hashtextextended($1,0))`, "signal-poll:"+id)
	job, cancel := context.WithTimeout(ctx, perUserTimeout)
	defer cancel()
	sources, created, e := RunSignalChecks(job, d, id, "")
	if e != nil {
		s.note(id, false)
		log.Printf("watchers: check failed for user %s: %v", id, e)
		return
	}
	s.note(id, true)
	if sources > 0 || created > 0 {
		log.Printf("watchers: user %s: %d sources checked, %d signals created", id, sources, created)
	}
}

func (s *Scheduler) note(id string, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ok {
		delete(s.failures, id)
	} else {
		s.failures[id]++
	}
}

// Browser alerts remain browser-only. Email watchers are checked independently
// of an open tab, with source/external-ID deduplication and a durable per-source
// UID checkpoint on every run. A slow pass delays the next one because the
// ticker drops missed ticks.
func RunLoop(ctx context.Context, d *db.DB) {
	tick := time.NewTicker(5 * time.Minute)
	defer tick.Stop()
	s := NewScheduler()
	s.RunOnce(ctx, d)
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			s.RunOnce(ctx, d)
		}
	}
}
