package handlers

import (
	"context"
	"database/sql"
	"encoding/xml"
	"errors"
	"github.com/andrew/go-computer/internal/config"
	crypt "github.com/andrew/go-computer/internal/crypto"
	"github.com/andrew/go-computer/internal/db"
	"github.com/andrew/go-computer/internal/netguard"
	"github.com/andrew/go-computer/internal/signals"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// carddavClient dials through netguard so a user-supplied Nextcloud URL can
// never point the server at internal infrastructure (same guard as URL
// previews). The transport is shared to avoid leaking idle sockets per sync.
var carddavClient = &http.Client{
	Timeout:   20 * time.Second,
	Transport: &http.Transport{DialContext: netguard.DialContext},
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

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
	// The share picker needs names and IDs; substring matching on emails would
	// let any user enumerate the whole user base, so emails must match exactly
	// and are returned masked. The name pattern escapes LIKE wildcards
	// (review SEC-3): without that, "%%" or "a%" enumerated every user
	// 10 rows at a time.
	v, e := db.Many(r.Context(), database(r), `SELECT jsonb_build_object('id',id,'name',name,'email',CASE WHEN position('@' in email)=0 THEN NULL ELSE left(email,1)||'***'||substr(email, position('@' in email)) END,'image',image) FROM "User" WHERE id<>$1 AND (name ILIKE $2 ESCAPE '\' OR lower(email)=lower($3)) ORDER BY name LIMIT 10`, user(r), "%"+likeEscaper.Replace(q)+"%", q)
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

// resolveCardDAVTarget resolves the user-supplied cardDavPath against the
// configured base URL and revalidates the COMPLETE resolved destination
// (review CR-01): a same-host absolute path like "http://host/..." must not
// downgrade an https base, because Basic-auth credentials are attached to the
// resulting request. The resolved URL must keep the base's scheme and host
// and carry no embedded userinfo. The scheme check on the base URL itself
// (https unless the netguard fixture flag is set) happens at the caller, so
// pinning the resolved scheme to the base scheme enforces https end to end.
func resolveCardDAVTarget(base *url.URL, path string) (*url.URL, error) {
	target, e := base.Parse(path)
	if e != nil {
		return nil, bad("Invalid CardDAV path")
	}
	if target.Host != base.Host || target.Scheme != base.Scheme || target.User != nil {
		return nil, bad("CardDAV path must stay on the Nextcloud host and scheme")
	}
	return target, nil
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
	// CardDAV transport (review recheck): Basic-auth credentials must never
	// travel plaintext. https is required unless the netguard escape hatch is
	// set (loopback test fakes / deliberate LAN deployments — never
	// production Compose). Redirects are not followed at all
	// (ErrUseLastResponse), so a verified origin cannot be downgraded, and
	// the path resolution below is pinned to the same host AND the same
	// scheme (review CR-01: the final resolved destination is revalidated,
	// not just the configured base URL).
	if base.Scheme != "https" && !netguard.AllowPrivate() {
		respond(w, 0, nil, bad("Nextcloud URL must use https"))
		return
	}
	path := str(b, "cardDavPath")
	if path == "" {
		path = "/remote.php/dav/addressbooks/users/" + url.PathEscape(str(b, "username")) + "/contacts/"
	}
	target, e := resolveCardDAVTarget(base, path)
	if e != nil {
		respond(w, 0, nil, e)
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
	res, e := carddavClient.Do(req)
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
	removed := 0
	seenUIDs := []string{}
	// complete tracks whether the server answered with full data for every
	// card. Deletion reconciliation (review CR-07) runs ONLY on a complete,
	// non-empty snapshot: a failed, truncated or partially-withheld response
	// must never be read as "these contacts were deleted remotely".
	complete := true
	e = db.Transaction(r.Context(), database(r), func(tx *sql.Tx) error {
		for _, entry := range dav.Responses {
			gotData := false
			for _, prop := range entry.Propstats {
				if !strings.Contains(prop.Status, "200") {
					continue
				}
				v := parseVCard(prop.Prop.Data)
				uid, fn := vCardPick(v["UID"]), vCardPick(v["FN"])
				if uid == "" || fn == "" {
					// A card we cannot identify stays untouched locally — and
					// blocks reconciliation, since we cannot prove which
					// stored contact (if any) it corresponds to.
					complete = false
					continue
				}
				gotData = true
				seenUIDs = append(seenUIDs, uid)
				_, e := tx.ExecContext(r.Context(), `INSERT INTO "Contact"(id,uid,"displayName",email,phone,"cardDavUrl",source,"userId") VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(uid,"userId") DO UPDATE SET "displayName"=EXCLUDED."displayName",email=EXCLUDED.email,phone=EXCLUDED.phone,"cardDavUrl"=EXCLUDED."cardDavUrl",source=EXCLUDED.source,"updatedAt"=now()`, db.NewID(), uid, fn, nullable(vCardPick(v["EMAIL"])), nullable(vCardPick(v["TEL"])), entry.Href, target.String(), user(r))
				if e != nil {
					return e
				}
				synced++
			}
			if !gotData {
				complete = false
			}
		}
		// Remote-deletion reconciliation (review CR-07). Policy: the sync
		// MIRRORS the configured address book, scoped by provenance — only
		// contacts previously imported from THIS source URL are candidates,
		// manually created contacts (source NULL) and other address books'
		// imports are never touched, and bookmarks referencing a removed
		// contact keep working (contactId is ON DELETE SET NULL). Guards:
		// a non-empty, fully-answered snapshot only; an empty or partial
		// response preserves everything (a genuinely emptied address book
		// stops mirroring deletions until it has at least one card — the
		// deliberate safe-side tradeoff).
		if complete && synced > 0 {
			res, e := tx.ExecContext(r.Context(), `DELETE FROM "Contact" WHERE "userId"=$1 AND source=$2 AND NOT (uid = ANY($3::text[]))`, user(r), target.String(), seenUIDs)
			if e != nil {
				return e
			}
			n, _ := res.RowsAffected()
			removed = int(n)
		}
		return nil
	})
	if e != nil {
		respond(w, 0, nil, e)
		return
	}
	respond(w, 200, object{"synced": synced, "removed": removed}, nil)
}
func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// vCardValue is one parsed property occurrence: its parameters (TYPE, PREF,
// ENCODING, ...) and its unescaped value. Repeated properties keep EVERY
// occurrence in file order (review CR-08) instead of the last one clobbering
// the rest.
type vCardValue struct {
	params map[string][]string
	value  string
}

// splitVCardName splits the property name on unquoted semicolons so quoted
// parameter values ("...;X=a;b") survive.
func splitVCardName(name string) []string {
	out := []string{}
	var cur strings.Builder
	inQuotes := false
	for _, r := range name {
		switch {
		case r == '"':
			inQuotes = !inQuotes
			cur.WriteRune(r)
		case r == ';' && !inQuotes:
			out = append(out, cur.String())
			cur.Reset()
		default:
			cur.WriteRune(r)
		}
	}
	return append(out, cur.String())
}

// parseVCard parses the documented subset of vCard (RFC 2426/6350) the
// contact sync consumes: CRLF and bare-LF unfolding, group prefixes
// (item1.EMAIL), parameter lists (including bare vCard 2.1 tokens as
// implicit TYPE values), and the standard escapes. Unsupported data is
// dropped, not silently misparsed (review CR-08): properties declaring a
// base64 ENCODING are skipped — the stored fields (UID/FN/EMAIL/TEL) are
// never legitimately encoded, and keeping the raw text would pretend to be
// a correct parse. A colon inside a quoted parameter value is outside the
// supported subset.
func parseVCard(s string) map[string][]vCardValue {
	s = strings.ReplaceAll(s, "\r\n ", "")
	s = strings.ReplaceAll(s, "\r\n\t", "")
	s = strings.ReplaceAll(s, "\n ", "")
	s = strings.ReplaceAll(s, "\n\t", "")
	out := map[string][]vCardValue{}
	for _, line := range strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n") {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		fields := splitVCardName(parts[0])
		key := strings.ToUpper(fields[0])
		if i := strings.LastIndex(key, "."); i >= 0 {
			key = key[i+1:] // strip group prefix (item1.EMAIL -> EMAIL)
		}
		if key == "" {
			continue
		}
		params := map[string][]string{}
		for _, p := range fields[1:] {
			kv := strings.SplitN(p, "=", 2)
			if len(kv) != 2 {
				// Bare token (vCard 2.1 style "EMAIL;INTERNET;WORK"): an
				// implicit TYPE value.
				if tok := strings.TrimSpace(kv[0]); tok != "" {
					params["TYPE"] = append(params["TYPE"], strings.ToUpper(tok))
				}
				continue
			}
			pk := strings.ToUpper(strings.TrimSpace(kv[0]))
			for _, pv := range strings.Split(kv[1], ",") {
				params[pk] = append(params[pk], strings.ToUpper(strings.Trim(strings.TrimSpace(pv), `"`)))
			}
		}
		if enc := params["ENCODING"]; len(enc) > 0 && (enc[0] == "B" || enc[0] == "BASE64") {
			continue // unsupported encoding: drop instead of misparsing
		}
		value := strings.NewReplacer(`\n`, "\n", `\N`, "\n", `\,`, ",", `\;`, ";", `\\`, `\`).Replace(parts[1])
		out[key] = append(out[key], vCardValue{params: params, value: value})
	}
	return out
}

// vCardPick deterministically selects the display value among repeated
// occurrences (review CR-08): the lowest explicit PREF=n wins (vCard 4.0),
// then the first TYPE=PREF occurrence (vCard 3.0), then file order.
func vCardPick(values []vCardValue) string {
	if len(values) == 0 {
		return ""
	}
	best := values[0].value
	bestPref := 1 << 30
	for _, v := range values {
		for _, p := range v.params["PREF"] {
			if n, e := strconv.Atoi(p); e == nil && n < bestPref {
				bestPref, best = n, v.value
			}
		}
	}
	if bestPref != 1<<30 {
		return best
	}
	for _, v := range values {
		for _, p := range v.params["TYPE"] {
			if p == "PREF" {
				return v.value
			}
		}
	}
	return best
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
	// Review NET-02: mailbox credentials must never travel over a plaintext
	// socket. Insecure settings are rejected at the API boundary; the
	// development-only INTEGRATIONS_ALLOW_INSECURE_IMAP escape hatch mirrors
	// netguard's fixture flag and is never set in production Compose.
	if !m.TLS && !signals.AllowInsecure() {
		return m, bad("TLS is required for mailbox connections")
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
	// Review CR-21: a blank/masked password means "keep the stored secret".
	// The old path read the existing ciphertext and re-submitted it in the
	// upsert, so a password saved concurrently between that read and write
	// was silently rolled back. The keep-stored path therefore never assigns
	// the password column at all — whatever is stored at commit time wins.
	if m.Password == "" || m.Password == "••••••••" {
		res, e := database(r).ExecContext(r.Context(), `UPDATE "ImapConfig" SET host=$1,port=$2,tls=$3,username=$4,folder=$5 WHERE "userId"=$6`, m.Host, m.Port, m.TLS, m.Username, m.Folder, user(r))
		if e != nil {
			respond(w, 0, nil, e)
			return
		}
		if n, _ := res.RowsAffected(); n == 0 {
			// No configuration exists to keep a password from (or it was
			// removed concurrently): initial creation requires a real one.
			respond(w, 0, nil, bad("Password required"))
			return
		}
		respond(w, 200, OK{true}, nil)
		return
	}
	pw, e := crypt.Encrypt(m.Password, config.Load().EncryptionSecret())
	if e != nil {
		respond(w, 0, nil, e)
		return
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
		if errors.Is(e, db.ErrNotFound) {
			respond(w, 0, nil, bad("Password required"))
			return
		}
		if e != nil {
			respond(w, 0, nil, e)
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
	// Bounded, single-connection pass: the watched-bookmark query is capped and
	// all searches reuse one IMAP login under an overall deadline (matches the
	// scheduler's 45s budget) instead of one fresh connection per bookmark.
	bms, e := db.Many(r.Context(), database(r), `SELECT to_jsonb(b) FROM "Bookmark" b WHERE "ownerId"=$1 AND "imapWatchEnabled" ORDER BY "createdAt" LIMIT 50`, user(r))
	if e != nil {
		respond(w, 0, nil, e)
		return
	}
	matches := []object{}
	watched := make([]object, 0, len(bms))
	for _, b := range bms {
		if str(b, "imapQuery") != "" {
			watched = append(watched, b)
		}
	}
	if len(watched) > 0 {
		m, e := signals.LoadMailbox(r.Context(), database(r), user(r))
		if e != nil {
			respond(w, 0, nil, bad("Configure IMAP first"))
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
		defer cancel()
		c, e := signals.Connect(ctx, m)
		if e != nil {
			respond(w, 0, nil, bad("Mailbox connection or login failed"))
			return
		}
		stop := context.AfterFunc(ctx, func() { c.Terminate() })
		defer stop()
		defer c.Logout()
		for _, b := range watched {
			events, e := signals.SearchConn(c, m, "", str(b, "imapQuery"))
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
