package signals

import (
	"context"
	"crypto/tls"
	"database/sql"
	"errors"
	"fmt"
	"github.com/andrew/go-computer/internal/config"
	crypt "github.com/andrew/go-computer/internal/crypto"
	"github.com/andrew/go-computer/internal/db"
	"github.com/andrew/go-computer/internal/netguard"
	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	"log"
	"math"
	"net"
	"os"
	"sort"
	"strings"
	"time"
)

type SignalInput struct {
	ExternalID, Title, Summary string
	OccurredAt                 time.Time
}
type Mailbox struct {
	Host, Username, Password, Folder string
	Port                             int
	TLS                              bool
}

// AllowInsecure reports whether plaintext IMAP logins are permitted. Only
// explicit development/test fixtures may set INTEGRATIONS_ALLOW_INSECURE_IMAP;
// production configurations must always authenticate over verified TLS
// (review NET-02). Never set it in production Compose.
func AllowInsecure() bool { return os.Getenv("INTEGRATIONS_ALLOW_INSECURE_IMAP") == "1" }

// checkTransport refuses password login on a plaintext socket (review NET-02).
// Implicit TLS is the only production transport; there is no STARTTLS
// downgrade path.
func checkTransport(m Mailbox) error {
	if !m.TLS && !AllowInsecure() {
		return errors.New("signals: refusing plaintext IMAP connection; TLS is required")
	}
	return nil
}

// Connect dials through netguard, so user-supplied IMAP hosts cannot point
// the server at internal infrastructure. The 15-second deadline covers
// dial/TLS/login only and is cleared afterwards; later commands are bounded
// by the caller's context (AfterFunc termination), not by a stale absolute
// deadline.
func Connect(ctx context.Context, m Mailbox) (*client.Client, error) {
	if e := checkTransport(m); e != nil {
		return nil, e
	}
	if m.Port == 0 {
		m.Port = 993
	}
	if m.Folder == "" {
		m.Folder = "INBOX"
	}
	conn, e := netguard.DialContext(ctx, "tcp", net.JoinHostPort(m.Host, fmt.Sprint(m.Port)))
	if e != nil {
		return nil, e
	}
	if e = conn.SetDeadline(time.Now().Add(15 * time.Second)); e != nil {
		conn.Close()
		return nil, e
	}
	if m.TLS {
		t := tls.Client(conn, &tls.Config{ServerName: m.Host, MinVersion: tls.VersionTLS12})
		if e = t.HandshakeContext(ctx); e != nil {
			conn.Close()
			return nil, e
		}
		conn = t
	}
	c, e := client.New(conn)
	if e != nil {
		conn.Close()
		return nil, e
	}
	if e = c.Login(m.Username, m.Password); e != nil {
		c.Terminate()
		return nil, e
	}
	if e = conn.SetDeadline(time.Time{}); e != nil {
		c.Terminate()
		return nil, e
	}
	return c, nil
}
func Search(ctx context.Context, m Mailbox, from, subject string) ([]SignalInput, error) {
	c, e := Connect(ctx, m)
	if e != nil {
		return nil, e
	}
	stop := context.AfterFunc(ctx, func() { c.Terminate() })
	defer stop()
	defer c.Logout()
	return SearchConn(c, m, from, subject)
}

// Checkpoint is the durable per-source ingestion state (review SYNC-02):
// the folder UIDVALIDITY it was recorded against and the highest UID already
// considered. A zero value means "nothing ingested yet".
type Checkpoint struct{ UIDValidity, LastUID uint32 }

func LoadCheckpoint(ctx context.Context, q db.Queryer, sourceID string) (Checkpoint, error) {
	var cp Checkpoint
	var validity, last int64
	e := q.QueryRowContext(ctx, `SELECT "uidValidity","lastUid" FROM "SignalCheckpoint" WHERE "sourceId"=$1`, sourceID).Scan(&validity, &last)
	if errors.Is(e, sql.ErrNoRows) {
		return Checkpoint{}, nil
	}
	if e != nil {
		return Checkpoint{}, e
	}
	cp.UIDValidity, cp.LastUID = uint32(validity), uint32(last)
	return cp, nil
}

// SaveCheckpoint persists cp. Callers must only advance a checkpoint after
// the batch's signals are durably handled. The update is monotonic (review
// COR-4): when uidValidity is unchanged, lastUid only advances via GREATEST,
// so a slower concurrent pass can never regress the checkpoint and force a
// re-scan of messages already ingested.
func SaveCheckpoint(ctx context.Context, q db.Queryer, sourceID string, cp Checkpoint) error {
	_, e := q.ExecContext(ctx, `INSERT INTO "SignalCheckpoint"("sourceId","uidValidity","lastUid","updatedAt") VALUES($1,$2,$3,now()) ON CONFLICT("sourceId") DO UPDATE SET "uidValidity"=EXCLUDED."uidValidity","lastUid"=CASE WHEN "SignalCheckpoint"."uidValidity"=EXCLUDED."uidValidity" THEN GREATEST("SignalCheckpoint"."lastUid",EXCLUDED."lastUid") ELSE EXCLUDED."lastUid" END,"updatedAt"=now()`, sourceID, int64(cp.UIDValidity), int64(cp.LastUID))
	return e
}

const (
	// searchBatch bounds one UID FETCH round trip.
	searchBatch = 50
	// maxIngestBatches caps a single checkpointed pass at 2500 messages per
	// source; a later pass continues from the persisted checkpoint.
	maxIngestBatches = 50
)

// SearchConn runs one bounded search on an established connection and
// reports the most recent matches. It is preview semantics for interactive
// checks ("Check now", legacy mailbox watch): no checkpoint is consulted or
// advanced. Durable ingestion uses SearchConnFrom. An empty criteria would
// match the entire mailbox, so it yields nothing instead.
func SearchConn(c *client.Client, m Mailbox, from, subject string) ([]SignalInput, error) {
	events, _, e := search(c, m, from, subject, nil)
	return events, e
}

// SearchConnFrom processes only messages newer than cp and returns the
// checkpoint to persist after — and only after — the returned inputs are
// durably handled (review SYNC-02). Batches are processed until the source
// is caught up or the per-pass cap is reached. When the folder's UIDVALIDITY
// changed, the checkpoint resets and the mailbox is re-scanned; Message-ID
// deduplication keeps re-scans from duplicating signals that carry one.
func SearchConnFrom(c *client.Client, m Mailbox, from, subject string, cp Checkpoint) ([]SignalInput, Checkpoint, error) {
	events, next, e := search(c, m, from, subject, &cp)
	return events, next, e
}

func search(c *client.Client, m Mailbox, from, subject string, cp *Checkpoint) ([]SignalInput, Checkpoint, error) {
	next := Checkpoint{}
	if cp != nil {
		next = *cp
	}
	if from == "" && subject == "" {
		return []SignalInput{}, next, nil
	}
	folder := m.Folder
	if folder == "" {
		folder = "INBOX"
	}
	status, e := c.Select(folder, true)
	if e != nil {
		return nil, next, e
	}
	// Effective checkpoint (review CR-15): exactly one normalized checkpoint
	// drives BOTH the UID filtering and the advancement below. When the
	// folder's UIDVALIDITY changed, the previous UID space is meaningless —
	// the effective checkpoint resets to zero so every UID in the new space
	// is (re)ingested, and filtering must never consult the stale LastUID.
	eff := Checkpoint{}
	if cp != nil {
		if cp.UIDValidity == status.UidValidity {
			eff = *cp
		}
		next = eff
	}
	next.UIDValidity = status.UidValidity
	criteria := imap.NewSearchCriteria()
	if from != "" {
		criteria.Header.Add("From", from)
	}
	if subject != "" {
		criteria.Header.Add("Subject", subject)
	}
	// Review CR-19: constrain the server-side SEARCH to the UID window that
	// can still hold unhandled messages, so a caught-up source does not
	// re-transfer its whole match history every pass. "n:*" is inclusive of
	// the mailbox's highest UID even when that UID is below n (RFC 3501
	// seq-number semantics), so the defensive local filter below stays —
	// the range is an optimization, never the correctness boundary.
	if cp != nil && eff.LastUID > 0 && eff.LastUID < math.MaxUint32 {
		criteria.Uid = new(imap.SeqSet)
		criteria.Uid.AddRange(eff.LastUID+1, 0) // Stop 0 renders as "*"
	}
	uids, e := c.UidSearch(criteria)
	if e != nil {
		return nil, next, e
	}
	sort.Slice(uids, func(i, j int) bool { return uids[i] < uids[j] })
	// truncated reports whether the per-pass cap shortened the UID list.
	// Review COR-1: when it did, the SEARCH did NOT consider every matching
	// message, so the high-water shortcut must stay off — otherwise UIDs past
	// the cap would land below the new checkpoint and be dropped forever.
	truncated := false
	if cp != nil {
		// Incremental ingestion: everything at or below the EFFECTIVE
		// checkpoint has already been durably handled by an earlier pass
		// (or the UID space was reset and nothing has — review CR-15).
		filtered := uids[:0]
		for _, uid := range uids {
			if uid > eff.LastUID {
				filtered = append(filtered, uid)
			}
		}
		uids = filtered
		if len(uids) > searchBatch*maxIngestBatches {
			uids = uids[:searchBatch*maxIngestBatches]
			truncated = true
		}
	} else if len(uids) > searchBatch {
		// Preview mode: the most recent matches, best effort.
		uids = uids[len(uids)-searchBatch:]
	}
	out := []SignalInput{}
	for start := 0; start < len(uids); start += searchBatch {
		batch := uids[start:min(start+searchBatch, len(uids))]
		seq := new(imap.SeqSet)
		seq.AddNum(batch...)
		ch := make(chan *imap.Message, len(batch))
		done := make(chan error, 1)
		go func() { done <- c.UidFetch(seq, []imap.FetchItem{imap.FetchUid, imap.FetchEnvelope}, ch) }()
		for msg := range ch {
			if msg.Uid > next.LastUID {
				next.LastUID = msg.Uid
			}
			if msg.Envelope == nil {
				continue
			}
			env := msg.Envelope
			id := env.MessageId
			if id == "" {
				id = fmt.Sprintf("%s:%d:%d", folder, status.UidValidity, msg.Uid)
			}
			names := []string{}
			for _, a := range env.From {
				names = append(names, a.Address())
			}
			out = append(out, SignalInput{ExternalID: id, Title: env.Subject, Summary: strings.Join(names, ", "), OccurredAt: env.Date})
		}
		if e := <-done; e != nil {
			return nil, next, e
		}
	}
	if cp != nil && !truncated && status.UidNext > next.LastUID+1 {
		// The SEARCH considered every message in the folder (the cap did not
		// shorten it), so non-matching UIDs below the mailbox high-water
		// mark can be skipped for good; anything arriving after this snapshot
		// stays above the checkpoint. Review COR-1: this shortcut is OFF
		// after truncation — remaining matches get a future pass instead of
		// being skipped forever.
		next.LastUID = status.UidNext - 1
	}
	return out, next, nil
}

func LoadMailbox(ctx context.Context, d *db.DB, user string) (Mailbox, error) {
	v, e := db.One(ctx, d, `SELECT to_jsonb(m) FROM "ImapConfig" m WHERE "userId"=$1`, user)
	if e != nil {
		return Mailbox{}, e
	}
	s := func(k string) string { v, _ := v[k].(string); return v }
	cfg := config.Load()
	pw, e := crypt.Decrypt(s("password"), cfg.EncryptionSecret())
	if e != nil {
		// Review OPS-01: rows encrypted before ENCRYPTION_KEY existed used the
		// auth-secret-derived key. Decrypt with the legacy key and re-encrypt
		// under the dedicated key so the migration completes transparently.
		legacy, legacyErr := crypt.Decrypt(s("password"), cfg.Secret())
		if legacyErr != nil {
			return Mailbox{}, e
		}
		pw = legacy
		if cfg.EncryptionSecret() != cfg.Secret() {
			if enc, encErr := crypt.Encrypt(pw, cfg.EncryptionSecret()); encErr == nil {
				if _, updErr := d.ExecContext(ctx, `UPDATE "ImapConfig" SET password=$1 WHERE "userId"=$2`, enc, user); updErr != nil {
					log.Printf("imap: re-encrypt stored password under ENCRYPTION_KEY: %v", updErr)
				}
			}
		}
	}
	port, _ := v["port"].(float64)
	tls, _ := v["tls"].(bool)
	return Mailbox{s("host"), s("username"), pw, s("folder"), int(port), tls}, nil
}
func PublicSourceTypes() []db.Object {
	return []db.Object{{"type": "email", "label": "Email (IMAP)", "description": "Search your configured mailbox for matching messages", "stub": false, "configFields": []db.Object{{"key": "from", "label": "From", "placeholder": "sender@domain.com"}, {"key": "subject", "label": "Subject", "placeholder": "Subject contains"}}}}
}
// CheckResult is the structured outcome of one signal pass (review CR-16).
// Sources counts the sources considered, Created the signals inserted, and
// Failed the sources whose check did not complete successfully (mailbox
// connection/search failure, signal insert failure, checkpoint failure or
// status-write failure). Per-source detail is persisted on
// SignalSource.lastStatus for the UI. The returned error is reserved for
// pass-level infrastructure failures (loading the source list): per-source
// failures live in Failed so scheduler backoff reacts to real integration
// failures while a manual check still answers 200 with per-source outcomes.
type CheckResult struct{ Sources, Created, Failed int }

func RunSignalChecks(ctx context.Context, d *db.DB, user, bookmark string) (CheckResult, error) {
	res := CheckResult{}
	sources, e := db.Many(ctx, d, `SELECT to_jsonb(s) FROM "SignalSource" s WHERE "ownerId"=$1 AND enabled AND ($2='' OR "bookmarkId"=$2)`, user, bookmark)
	if e != nil {
		return res, e
	}
	res.Sources = len(sources)
	if len(sources) == 0 {
		return res, nil
	}
	// Load and connect once for the whole pass; every source of a user shares
	// the same mailbox credentials.
	m, connErr := LoadMailbox(ctx, d, user)
	var c *client.Client
	if connErr == nil {
		c, connErr = Connect(ctx, m)
		if connErr == nil {
			stop := context.AfterFunc(ctx, func() { c.Terminate() })
			defer stop()
			defer c.Logout()
		}
	}
	for _, src := range sources {
		sourceID := fmt.Sprint(src["id"])
		status := "Checked"
		events := []SignalInput{}
		e := connErr
		var next Checkpoint
		if e == nil {
			cfg, _ := src["config"].(map[string]any)
			from, _ := cfg["from"].(string)
			subject, _ := cfg["subject"].(string)
			// Incremental, checkpointed ingestion (review SYNC-02): only UIDs
			// newer than the durable checkpoint are processed, in bounded
			// batches, until the source is caught up. Review COR-3: a failed
			// checkpoint load is surfaced as an error for this source instead
			// of silently degrading to a zero checkpoint (full re-scan).
			var cp Checkpoint
			if cp, e = LoadCheckpoint(ctx, d, sourceID); e == nil {
				events, next, e = SearchConnFrom(c, m, from, subject, cp)
			}
		}
		if e != nil {
			// Connection/search failures are recorded per source (lastStatus)
			// and counted in Failed (review CR-16) — they are NOT folded into
			// the pass-level error, so a manual check still answers 200 with
			// the per-source outcome and db.ErrNotFound from a missing
			// mailbox configuration cannot surface as a misleading 404. The
			// scheduler reads Failed and backs off on repeated failures.
			status = "Error: mailbox connection or search failed"
			res.Failed++
		} else {
			var insertErr error
			for _, s := range events {
				when := s.OccurredAt
				if when.IsZero() {
					when = time.Now()
				}
				r, err := d.ExecContext(ctx, `INSERT INTO "Signal"(id,"sourceType",title,summary,"externalId","occurredAt","sourceId","bookmarkId","ownerId") VALUES($1,'email',$2,$3,$4,$5,$6,$7,$8) ON CONFLICT("sourceId","externalId") DO NOTHING`, db.NewID(), s.Title, s.Summary, s.ExternalID, when, src["id"], src["bookmarkId"], user)
				if err != nil {
					insertErr = err
					break
				}
				n, _ := r.RowsAffected()
				res.Created += int(n)
			}
			if insertErr != nil {
				// Review COR-5: record the per-source failure in lastStatus
				// and continue with the remaining sources instead of aborting
				// the whole pass. The checkpoint does NOT advance, so the
				// failed event and the rest of the batch are retried next
				// pass (ON CONFLICT dedupes the ones already inserted).
				status = "Error: signal insert failed"
				res.Failed++
			} else if err := SaveCheckpoint(ctx, d, sourceID, next); err != nil {
				// Advance the checkpoint only after the batch is durably handled.
				status = "Error: checkpoint save failed"
				res.Failed++
			}
		}
		// Record this source's status even on error so the UI can surface it.
		if _, err := d.ExecContext(ctx, `UPDATE "SignalSource" SET "lastCheckedAt"=now(),"lastStatus"=$1,"updatedAt"=now() WHERE id=$2`, status, src["id"]); err != nil {
			res.Failed++
		}
	}
	return res, nil
}
