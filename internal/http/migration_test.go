package http

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// The ledgered migrations (review MIG-01) must stay idempotent: applying
// every file twice in order cannot change the backfill guarantees.
func TestUpgradeBackfillsLegacyWatchesWithoutChangingExistingSources(t *testing.T) {
	srv, d := testServerDB(t)
	_, uid := accountClient(t, srv.URL, "watcher@computer.test")
	if _, err := d.Exec(`INSERT INTO "Bookmark"(id,url,"ownerId","imapWatchEnabled","imapQuery") VALUES('old','https://127.0.0.1',$1,true,'Invoices'),('paused','https://127.0.0.1',$1,true,'Receipts'),('off','https://127.0.0.1',$1,false,'Other')`, uid); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec(`INSERT INTO "SignalSource"(id,type,config,enabled,"bookmarkId","ownerId") VALUES('modern','email','{"subject":"Modern"}',true,'old',$1),('existing','email','{"legacy":true,"subject":"Paused"}',false,'paused',$1)`, uid); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir("../../db/migrations")
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	if len(names) == 0 {
		t.Fatal("no migration files found")
	}
	for i := 0; i < 2; i++ {
		for _, name := range names {
			raw, err := os.ReadFile(filepath.Join("../../db/migrations", name))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := d.Exec(string(raw)); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
		}
	}
	var count int
	if err := d.QueryRow(`SELECT count(*) FROM "SignalSource"`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("expected existing two sources and one backfill, got %d", count)
	}
	var enabled bool
	var subject string
	if err := d.QueryRow(`SELECT enabled,config->>'subject' FROM "SignalSource" WHERE id='legacy-old'`).Scan(&enabled, &subject); err != nil {
		t.Fatal(err)
	}
	if !enabled || subject != "Invoices" {
		t.Fatal("legacy switch not migrated")
	}
	if err := d.QueryRow(`SELECT enabled,config->>'subject' FROM "SignalSource" WHERE id='existing'`).Scan(&enabled, &subject); err != nil {
		t.Fatal(err)
	}
	if enabled || subject != "Paused" {
		t.Fatal("existing paused watcher modified")
	}
}
