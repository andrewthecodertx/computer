package handlers

import (
	"database/sql"
	"encoding/xml"
	"github.com/andrew/go-computer/internal/config"
	crypt "github.com/andrew/go-computer/internal/crypto"
	"github.com/andrew/go-computer/internal/db"
	"github.com/andrew/go-computer/internal/signals"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func HandleAlertsCheck(w http.ResponseWriter, r *http.Request) {
	v, e := db.Many(r.Context(), database(r), `SELECT to_jsonb(b) FROM "Bookmark" b WHERE "ownerId"=$1 AND "alertAt"<=now()+interval '15 minutes' AND NOT "alertSent" ORDER BY "alertAt"`, user(r))
	respond(w, 200, v, e)
}
func HandleAlertsDismiss(w http.ResponseWriter, r *http.Request) {
	_, e := db.Update(r.Context(), database(r), "Bookmark", r.PathValue("bookmarkId"), "ownerId", user(r), object{"alertSent": true, "updatedAt": time.Now().UTC()})
	respond(w, 200, OK{true}, e)
}
func HandleUsersSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len([]rune(q)) < 2 {
		respond(w, 200, []object{}, nil)
		return
	}
	v, e := db.Many(r.Context(), database(r), `SELECT jsonb_build_object('id',id,'name',name,'email',email,'image',image) FROM "User" WHERE id<>$1 AND (name ILIKE $2 OR email ILIKE $2) ORDER BY name LIMIT 10`, user(r), "%"+q+"%")
	respond(w, 200, v, e)
}
func HandleSharedDatesList(w http.ResponseWriter, r *http.Request) {
	v, e := db.Many(r.Context(), database(r), `SELECT to_jsonb(s)||jsonb_build_object('creator',jsonb_build_object('id',c.id,'name',c.name,'email',c.email),'recipient',jsonb_build_object('id',u.id,'name',u.name,'email',u.email)) FROM "SharedDate" s JOIN "User" c ON c.id=s."creatorId" JOIN "User" u ON u.id=s."recipientId" WHERE s."creatorId"=$1 OR s."recipientId"=$1 ORDER BY date`, user(r))
	respond(w, 200, v, e)
}
func HandleSharedDatesCreate(w http.ResponseWriter, r *http.Request) {
	b, ok := read(w, r)
	if !ok {
		return
	}
	dt, e := dateValue(b["date"])
	if e != nil || dt == nil {
		respond(w, 0, nil, bad("Date required"))
		return
	}
	list, e := ids(b, "recipientIds")
	if e != nil || len(list) == 0 {
		respond(w, 0, nil, bad("Recipients required"))
		return
	}
	if e = stringFields(b, "note", ""); e != nil {
		respond(w, 0, nil, e)
		return
	}
	out := []object{}
	e = db.Transaction(r.Context(), database(r), func(tx *sql.Tx) error {
		for _, id := range list {
			v, e := db.Insert(r.Context(), tx, "SharedDate", object{"id": db.NewID(), "date": dt, "creatorId": user(r), "recipientId": id, "note": b["note"]})
			if e != nil {
				return e
			}
			out = append(out, v)
		}
		return nil
	})
	respond(w, 201, out, e)
}
func HandleContactsList(w http.ResponseWriter, r *http.Request) {
	v, e := db.Many(r.Context(), database(r), `SELECT to_jsonb(c)||jsonb_build_object('_count',jsonb_build_object('bookmarks',(SELECT count(*) FROM "Bookmark" b WHERE b."contactId"=c.id AND b."ownerId"=$1))) FROM "Contact" c WHERE "userId"=$1 ORDER BY "displayName"`, user(r))
	respond(w, 200, v, e)
}

type davResponse struct {
	Responses []struct {
		Href      string `xml:"href"`
		Propstats []struct {
			Status string `xml:"status"`
			Prop   struct {
				Data string `xml:"address-data"`
			} `xml:"prop"`
		} `xml:"propstat"`
	} `xml:"response"`
}

func HandleContactsSync(w http.ResponseWriter, r *http.Request) {
	b, ok := read(w, r)
	if !ok {
		return
	}
	if !validURL(str(b, "nextcloudUrl")) || str(b, "username") == "" || str(b, "password") == "" {
		respond(w, 0, nil, bad("Nextcloud URL and credentials required"))
		return
	}
	base, _ := url.Parse(str(b, "nextcloudUrl"))
	path := str(b, "cardDavPath")
	if path == "" {
		path = "/remote.php/dav/addressbooks/users/" + url.PathEscape(str(b, "username")) + "/contacts/"
	}
	target, e := base.Parse(path)
	if e != nil || target.Host != base.Host {
		respond(w, 0, nil, bad("CardDAV path must use the Nextcloud host"))
		return
	}
	payload := `<?xml version="1.0"?><card:addressbook-query xmlns:d="DAV:" xmlns:card="urn:ietf:params:xml:ns:carddav"><d:prop><d:getetag/><card:address-data/></d:prop></card:addressbook-query>`
	req, e := http.NewRequestWithContext(r.Context(), "REPORT", target.String(), strings.NewReader(payload))
	if e != nil {
		respond(w, 0, nil, bad("Invalid CardDAV URL"))
		return
	}
	req.SetBasicAuth(str(b, "username"), str(b, "password"))
	req.Header.Set("Depth", "1")
	req.Header.Set("Content-Type", "application/xml")
	c := http.Client{Timeout: 20 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }}
	res, e := c.Do(req)
	if e != nil {
		respond(w, 0, nil, bad("Could not connect to Nextcloud"))
		return
	}
	defer res.Body.Close()
	if res.StatusCode != 207 && res.StatusCode != 200 {
		respond(w, 0, nil, bad("Nextcloud rejected the CardDAV request"))
		return
	}
	var dav davResponse
	if e = xml.NewDecoder(io.LimitReader(res.Body, 8<<20)).Decode(&dav); e != nil {
		respond(w, 0, nil, bad("Invalid CardDAV response"))
		return
	}
	synced := 0
	e = db.Transaction(r.Context(), database(r), func(tx *sql.Tx) error {
		for _, entry := range dav.Responses {
			for _, prop := range entry.Propstats {
				if !strings.Contains(prop.Status, "200") {
					continue
				}
				v := parseVCard(prop.Prop.Data)
				if v["UID"] == "" || v["FN"] == "" {
					continue
				}
				_, e := tx.ExecContext(r.Context(), `INSERT INTO "Contact"(id,uid,"displayName",email,phone,"cardDavUrl","userId") VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(uid,"userId") DO UPDATE SET "displayName"=EXCLUDED."displayName",email=EXCLUDED.email,phone=EXCLUDED.phone,"cardDavUrl"=EXCLUDED."cardDavUrl","updatedAt"=now()`, db.NewID(), v["UID"], v["FN"], nullable(v["EMAIL"]), nullable(v["TEL"]), entry.Href, user(r))
				if e != nil {
					return e
				}
				synced++
			}
		}
		return nil
	})
	respond(w, 200, object{"synced": synced}, e)
}
func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}
func parseVCard(s string) map[string]string {
	s = strings.ReplaceAll(s, "\r\n ", "")
	s = strings.ReplaceAll(s, "\r\n\t", "")
	out := map[string]string{}
	for _, line := range strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n") {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.ToUpper(strings.SplitN(parts[0], ";", 2)[0])
		if i := strings.LastIndex(key, "."); i >= 0 {
			key = key[i+1:]
		}
		value := strings.NewReplacer(`\n`, "\n", `\,`, ",", `\;`, ";", `\\`, `\`).Replace(parts[1])
		out[key] = value
	}
	return out
}
func mailboxData(b object) (signals.Mailbox, error) {
	if e := stringFields(b, "", "host username password folder"); e != nil {
		return signals.Mailbox{}, e
	}
	if e := boolFields(b, "tls"); e != nil {
		return signals.Mailbox{}, e
	}
	m := signals.Mailbox{Host: str(b, "host"), Username: str(b, "username"), Password: str(b, "password"), Folder: str(b, "folder"), Port: 993, TLS: true}
	if n, ok := b["port"]; ok {
		v, ok := n.(float64)
		if !ok || v < 1 || v > 65535 || float64(int(v)) != v {
			return m, bad("Invalid port")
		}
		m.Port = int(v)
	}
	if v, ok := b["tls"].(bool); ok {
		m.TLS = v
	}
	if m.Folder == "" {
		m.Folder = "INBOX"
	}
	if m.Host == "" || m.Username == "" {
		return m, bad("Host and username required")
	}
	return m, nil
}
func HandleImapGet(w http.ResponseWriter, r *http.Request) {
	v, e := db.One(r.Context(), database(r), `SELECT to_jsonb(m)-'password'||jsonb_build_object('password','••••••••') FROM "ImapConfig" m WHERE "userId"=$1`, user(r))
	if e == db.ErrNotFound {
		respond(w, 200, nil, nil)
		return
	}
	respond(w, 200, v, e)
}
func HandleImapSave(w http.ResponseWriter, r *http.Request) {
	b, ok := read(w, r)
	if !ok {
		return
	}
	m, e := mailboxData(b)
	if e != nil {
		respond(w, 0, nil, e)
		return
	}
	var pw string
	if m.Password == "" || m.Password == "••••••••" {
		v, e := db.One(r.Context(), database(r), `SELECT jsonb_build_object('password',password) FROM "ImapConfig" WHERE "userId"=$1`, user(r))
		if e != nil {
			respond(w, 0, nil, bad("Password required"))
			return
		}
		pw = str(v, "password")
	} else {
		pw, e = crypt.Encrypt(m.Password, config.Load().Secret())
		if e != nil {
			respond(w, 0, nil, e)
			return
		}
	}
	_, e = database(r).ExecContext(r.Context(), `INSERT INTO "ImapConfig"(id,host,port,tls,username,password,folder,"userId") VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT("userId") DO UPDATE SET host=EXCLUDED.host,port=EXCLUDED.port,tls=EXCLUDED.tls,username=EXCLUDED.username,password=EXCLUDED.password,folder=EXCLUDED.folder`, db.NewID(), m.Host, m.Port, m.TLS, m.Username, pw, m.Folder, user(r))
	respond(w, 200, OK{true}, e)
}
func HandleImapTest(w http.ResponseWriter, r *http.Request) {
	b, ok := read(w, r)
	if !ok {
		return
	}
	m, e := mailboxData(b)
	if e != nil {
		respond(w, 0, nil, e)
		return
	}
	if m.Password == "" || m.Password == "••••••••" {
		saved, e := signals.LoadMailbox(r.Context(), database(r), user(r))
		if e != nil {
			respond(w, 0, nil, bad("Password required"))
			return
		}
		m.Password = saved.Password
	}
	c, e := signals.Connect(r.Context(), m)
	if e != nil {
		respond(w, 0, nil, bad("Mailbox connection or login failed"))
		return
	}
	c.Logout()
	respond(w, 200, OK{true}, nil)
}
func HandleImapCheck(w http.ResponseWriter, r *http.Request) {
	bms, e := db.Many(r.Context(), database(r), `SELECT to_jsonb(b) FROM "Bookmark" b WHERE "ownerId"=$1 AND "imapWatchEnabled"`, user(r))
	if e != nil {
		respond(w, 0, nil, e)
		return
	}
	matches := []object{}
	if len(bms) > 0 {
		m, e := signals.LoadMailbox(r.Context(), database(r), user(r))
		if e != nil {
			respond(w, 0, nil, bad("Configure IMAP first"))
			return
		}
		for _, b := range bms {
			events, e := signals.Search(r.Context(), m, "", str(b, "imapQuery"))
			if e != nil {
				respond(w, 0, nil, bad("Mailbox search failed"))
				return
			}
			for _, event := range events {
				matches = append(matches, object{"bookmarkId": b["id"], "title": event.Title, "from": event.Summary, "externalId": event.ExternalID, "occurredAt": event.OccurredAt})
			}
		}
	}
	respond(w, 200, object{"matches": matches}, nil)
}
