package http

import (
	"os"
	"testing"
)

func TestUpgradeBackfillsLegacyWatchesWithoutChangingExistingSources(t *testing.T) {
	srv, d := testServerDB(t)
	_, uid := accountClient(t, srv.URL, "watcher@computer.test")
	if _, err := d.Exec(`INSERT INTO "Bookmark"(id,url,"ownerId","imapWatchEnabled","imapQuery") VALUES('old','https://127.0.0.1',$1,true,'Invoices'),('paused','https://127.0.0.1',$1,true,'Receipts'),('off','https://127.0.0.1',$1,false,'Other')`, uid); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec(`INSERT INTO "SignalSource"(id,type,config,enabled,"bookmarkId","ownerId") VALUES('modern','email','{"subject":"Modern"}',true,'old',$1),('existing','email','{"legacy":true,"subject":"Paused"}',false,'paused',$1)`, uid); err != nil {
		t.Fatal(err)
	}
	upgrade, err := os.ReadFile("../../db/upgrade.sql")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err := d.Exec(string(upgrade)); err != nil {
			t.Fatal(err)
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
