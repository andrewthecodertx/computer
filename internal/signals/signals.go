package signals

import (
	"context"
	"crypto/tls"
	"fmt"
	"github.com/andrew/go-computer/internal/config"
	crypt "github.com/andrew/go-computer/internal/crypto"
	"github.com/andrew/go-computer/internal/db"
	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	"net"
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

func Connect(ctx context.Context, m Mailbox) (*client.Client, error) {
	if m.Port == 0 {
		m.Port = 993
	}
	if m.Folder == "" {
		m.Folder = "INBOX"
	}
	conn, e := (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, "tcp", net.JoinHostPort(m.Host, fmt.Sprint(m.Port)))
	if e != nil {
		return nil, e
	}
	_ = conn.SetDeadline(time.Now().Add(15 * time.Second))
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
	folder := m.Folder
	if folder == "" {
		folder = "INBOX"
	}
	status, e := c.Select(folder, true)
	if e != nil {
		return nil, e
	}
	criteria := imap.NewSearchCriteria()
	if from != "" {
		criteria.Header.Add("From", from)
	}
	if subject != "" {
		criteria.Header.Add("Subject", subject)
	}
	uids, e := c.UidSearch(criteria)
	if e != nil {
		return nil, e
	}
	if len(uids) > 50 {
		uids = uids[len(uids)-50:]
	}
	out := []SignalInput{}
	if len(uids) == 0 {
		return out, nil
	}
	seq := new(imap.SeqSet)
	seq.AddNum(uids...)
	ch := make(chan *imap.Message, 50)
	done := make(chan error, 1)
	go func() { done <- c.UidFetch(seq, []imap.FetchItem{imap.FetchUid, imap.FetchEnvelope}, ch) }()
	for msg := range ch {
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
	return out, <-done
}
func LoadMailbox(ctx context.Context, d *db.DB, user string) (Mailbox, error) {
	v, e := db.One(ctx, d, `SELECT to_jsonb(m) FROM "ImapConfig" m WHERE "userId"=$1`, user)
	if e != nil {
		return Mailbox{}, e
	}
	s := func(k string) string { v, _ := v[k].(string); return v }
	pw, e := crypt.Decrypt(s("password"), config.Load().Secret())
	if e != nil {
		return Mailbox{}, e
	}
	port, _ := v["port"].(float64)
	tls, _ := v["tls"].(bool)
	return Mailbox{s("host"), s("username"), pw, s("folder"), int(port), tls}, nil
}
func PublicSourceTypes() []db.Object {
	return []db.Object{{"type": "email", "label": "Email (IMAP)", "description": "Search your configured mailbox for matching messages", "stub": false, "configFields": []db.Object{{"key": "from", "label": "From", "placeholder": "sender@domain.com"}, {"key": "subject", "label": "Subject", "placeholder": "Subject contains"}}}}
}
func RunSignalChecks(ctx context.Context, d *db.DB, user, bookmark string) (int, int, error) {
	sources, e := db.Many(ctx, d, `SELECT to_jsonb(s) FROM "SignalSource" s WHERE "ownerId"=$1 AND enabled AND ($2='' OR "bookmarkId"=$2)`, user, bookmark)
	if e != nil {
		return 0, 0, e
	}
	created := 0
	for _, src := range sources {
		status := "Checked"
		m, e := LoadMailbox(ctx, d, user)
		events := []SignalInput{}
		if e == nil {
			cfg, _ := src["config"].(map[string]any)
			from, _ := cfg["from"].(string)
			subject, _ := cfg["subject"].(string)
			events, e = Search(ctx, m, from, subject)
		}
		if e != nil {
			status = "Error: mailbox connection or search failed"
		} else {
			for _, s := range events {
				when := s.OccurredAt
				if when.IsZero() {
					when = time.Now()
				}
				res, err := d.ExecContext(ctx, `INSERT INTO "Signal"(id,"sourceType",title,summary,"externalId","occurredAt","sourceId","bookmarkId","ownerId") VALUES($1,'email',$2,$3,$4,$5,$6,$7,$8) ON CONFLICT("sourceId","externalId") DO NOTHING`, db.NewID(), s.Title, s.Summary, s.ExternalID, when, src["id"], src["bookmarkId"], user)
				if err != nil {
					return len(sources), created, err
				}
				n, _ := res.RowsAffected()
				created += int(n)
			}
		}
		if _, err := d.ExecContext(ctx, `UPDATE "SignalSource" SET "lastCheckedAt"=now(),"lastStatus"=$1,"updatedAt"=now() WHERE id=$2`, status, src["id"]); err != nil {
			return len(sources), created, err
		}
	}
	return len(sources), created, nil
}
