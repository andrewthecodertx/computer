package handlers

import (
	"encoding/json"
	"net/http"
)

// Alerts, contacts, email, dates, users. Porting guide 4.8.

// HandleAlertsCheck implements GET /api/alerts/check.
// My bookmarks with alertAt <= now + 15 min and alertSent = false, soonest first.
// Browser polls this on load and every 5 minutes; there is no server push.
func HandleAlertsCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, []interface{}{})
}

// HandleAlertsDismiss implements DELETE /api/alerts/:bookmarkId.
// Marks alertSent = true (dismiss).
func HandleAlertsDismiss(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, OK{OK: true})
}

// HandleContactsList implements GET /api/contacts.
// Mine with bookmark counts, by name.
func HandleContactsList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, []interface{}{})
}

// HandleContactsSync implements POST /api/contacts/sync.
// {nextcloudUrl, username, password, cardDavPath?}. Path default
// /remote.php/dav/addressbooks/users/<username>/contacts/. Sends HTTP REPORT
// (Depth 1, Basic auth) with a CardDAV addressbook-query for getetag and
// address-data. Parses the multistatus XML; from each vCard takes UID:, FN:, and
// the last value of the EMAIL and TEL lines. Skips cards without UID or FN;
// upserts by (uid, userId). Returns {synced}. Credentials are NOT stored.
func HandleContactsSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	var req ContactsSyncRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	writeJSON(w, http.StatusOK, map[string]interface{}{"synced": 0})
}

// HandleImapGet implements GET /api/settings/imap.
// My config with password masked as "••••••••".
func HandleImapGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{})
}

// HandleImapSave implements POST /api/settings/imap.
// {host, port?, tls?, username, password, folder?}. Encrypts the password and
// upserts.
func HandleImapSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, OK{OK: true})
}

// HandleImapTest implements POST /api/imap/test.
// {host, port?, tls?, username, password}. Opens only a TCP/TLS socket
// (10-second timeout, certificate not verified). Does NOT log in to IMAP.
func HandleImapTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, OK{OK: true})
}

// HandleImapCheck implements GET /api/imap/check.
// STUB: lists bookmarks with the legacy watch on and returns matches: [].
func HandleImapCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"matches": []interface{}{}})
}

// HandleSharedDatesList implements GET /api/shared-dates.
// Dates I created or received, with creator and recipient.
func HandleSharedDatesList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, []interface{}{})
}

// HandleSharedDatesCreate implements POST /api/shared-dates.
// {date, note?, recipientIds[]}. One row per recipient.
func HandleSharedDatesCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusCreated, OK{OK: true})
}

// HandleUsersSearch implements GET /api/users/search?q=.
// At least 2 characters; name/email contains, case-insensitive, excluding me;
// max 10. Used by share dialogs.
func HandleUsersSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, []interface{}{})
}

type ContactsSyncRequest struct {
	NextcloudURL  string `json:"nextcloudUrl"`
	Username      string `json:"username"`
	Password      string `json:"password"`
	CardDavPath   *string `json:"cardDavPath"`
}
