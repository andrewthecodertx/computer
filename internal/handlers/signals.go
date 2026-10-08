package handlers

import (
	"encoding/json"
	"github.com/andrew/go-computer/internal/db"
	"github.com/andrew/go-computer/internal/signals"
	"net/http"
	"strings"
	"time"
)

func HandleSignalsList(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, e := bookmark(r, id, false); e != nil {
		respond(w, 0, nil, e)
		return
	}
	sources, e := db.Many(r.Context(), database(r), `SELECT to_jsonb(s) FROM "SignalSource" s WHERE "bookmarkId"=$1 AND "ownerId"=$2 ORDER BY "createdAt"`, id, user(r))
	if e != nil {
		respond(w, 0, nil, e)
		return
	}
	events, e := db.Many(r.Context(), database(r), `SELECT to_jsonb(s) FROM "Signal" s WHERE "bookmarkId"=$1 AND "ownerId"=$2 ORDER BY "occurredAt" DESC LIMIT 50`, id, user(r))
	respond(w, 200, object{"sources": sources, "signals": events, "types": signals.PublicSourceTypes()}, e)
}
func HandleSignalsCreate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, e := owned(r, database(r), "Bookmark", id, "ownerId"); e != nil {
		respond(w, 0, nil, e)
		return
	}
	b, ok := read(w, r)
	if !ok {
		return
	}
	if str(b, "action") == "check" {
		checked, created, e := signals.RunSignalChecks(r.Context(), database(r), user(r), id)
		respond(w, 200, object{"checked": checked, "created": created}, e)
		return
	}
	if str(b, "type") != "email" {
		respond(w, 0, nil, bad("Unknown signal type"))
		return
	}
	cfg, ok := b["config"].(map[string]any)
	if !ok {
		respond(w, 0, nil, bad("Config must be an object"))
		return
	}
	clean := object{}
	for _, key := range []string{"from", "subject"} {
		if v, ok := cfg[key]; ok {
			s, ok := v.(string)
			if !ok {
				respond(w, 0, nil, bad("Config values must be text"))
				return
			}
			rs := []rune(strings.TrimSpace(s))
			if len(rs) > 200 {
				rs = rs[:200]
			}
			clean[key] = string(rs)
		}
	}
	raw, _ := json.Marshal(clean)
	v, e := db.Insert(r.Context(), database(r), "SignalSource", object{"id": db.NewID(), "bookmarkId": id, "ownerId": user(r), "type": "email", "config": string(raw)})
	respond(w, 201, v, e)
}
func HandleSignalSourcesUpdate(w http.ResponseWriter, r *http.Request) {
	b, ok := read(w, r)
	if !ok {
		return
	}
	if _, ok = b["enabled"].(bool); !ok {
		respond(w, 0, nil, bad("Enabled must be boolean"))
		return
	}
	v, e := db.Update(r.Context(), database(r), "SignalSource", r.PathValue("id"), "ownerId", user(r), object{"enabled": b["enabled"], "updatedAt": time.Now().UTC()})
	respond(w, 200, v, e)
}
func HandleSignalSourcesDelete(w http.ResponseWriter, r *http.Request) {
	respond(w, 200, OK{true}, remove(r, "SignalSource", r.PathValue("id"), "ownerId"))
}
