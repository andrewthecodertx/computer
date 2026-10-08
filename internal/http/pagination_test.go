package http

import (
	"encoding/json"
	"fmt"
	"net/url"
	"testing"
)

func TestBookmarkPaginationPreservesCollectionAndOwnership(t *testing.T) {
	srv, d := testServerDB(t)
	owner, uid := accountClient(t, srv.URL, "owner@computer.test")
	other, oid := accountClient(t, srv.URL, "other@computer.test")
	for _, user := range []string{uid, oid} {
		if _, err := d.Exec(`INSERT INTO "Bookmark"(id,url,title,"ownerId","updatedAt") SELECT $1||'-'||lpad(n::text,4,'0'),'https://127.0.0.1/test','Match '||n,$1,'2026-10-07T12:00:00Z' FROM generate_series(1,205) n`, user); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range []apiClient{owner, other} {
		seen := map[string]bool{}
		cursor := ""
		for {
			res, err := c.c.Get(srv.URL + "/api/bookmarks?limit=100&search=Match&cursor=" + url.QueryEscape(cursor))
			if err != nil {
				t.Fatal(err)
			}
			var records []map[string]any
			if err := json.NewDecoder(res.Body).Decode(&records); err != nil {
				t.Fatal(err)
			}
			res.Body.Close()
			if res.StatusCode != 200 || len(records) > 100 {
				t.Fatal("invalid page", res.StatusCode, len(records))
			}
			for _, row := range records {
				id := row["id"].(string)
				if seen[id] {
					t.Fatal("duplicate bookmark across pages", id)
				}
				seen[id] = true
			}
			cursor = res.Header.Get("X-Next-Cursor")
			if cursor == "" {
				break
			}
		}
		if len(seen) != 205 {
			t.Fatalf("wanted 205 bookmarks, got %d", len(seen))
		}
		expected := uid
		if c.c == other.c {
			expected = oid
		}
		for i := 1; i <= 205; i++ {
			if !seen[fmt.Sprintf("%s-%04d", expected, i)] {
				t.Fatal("bookmark missing or foreign collection included")
			}
		}
	}
	owner.request("GET", "/api/bookmarks?limit=201", nil, 400)
	owner.request("GET", "/api/bookmarks?limit=0", nil, 400)
	owner.request("GET", "/api/bookmarks?cursor=invalid", nil, 400)
}
