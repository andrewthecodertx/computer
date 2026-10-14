package http

import (
	"testing"
)

// Collections (Notion-like upgrades phase 2). Proves the locked semantics:
// contextual membership (same bookmark, independent workflows, bookmark
// never owned by the membership), versioned PATCHes (DATA-05 lesson),
// server-side validated filters/sorts with deterministic cursor pagination,
// and access revocation reaching every item surface (N-08).

func collectionBookmark(t *testing.T, c apiClient, url string) string {
	t.Helper()
	return c.request("POST", "/api/bookmarks", map[string]any{"url": url, "title": "BM " + url}, 201)["id"].(string)
}

func TestCollectionsLifecycle(t *testing.T) {
	srv := testServer(t)
	alice := newAPIUser(t, srv, "coll-life@computer.test")
	bob := newAPIUser(t, srv, "coll-other@computer.test")

	bm1 := collectionBookmark(t, alice, "https://127.0.0.1/one")

	col := alice.request("POST", "/api/collections", map[string]any{"name": "Research", "description": "phase 2"}, 201)
	cid := col["id"].(string)
	views, _ := col["views"].([]any)
	if len(views) != 1 {
		t.Fatalf("new collection did not get a default view: %v", col)
	}

	// Property surface: typed, validated, RELATION reserved but rejected.
	alice.request("POST", "/api/collections/"+cid+"/properties", map[string]any{"name": "Rel", "type": "RELATION"}, 400)
	alice.request("POST", "/api/collections/"+cid+"/properties", map[string]any{"name": "Bad", "type": "PERSON"}, 400)
	status := alice.request("POST", "/api/collections/"+cid+"/properties", map[string]any{
		"name": "Status", "type": "SELECT",
		"config": map[string]any{"options": []any{
			map[string]any{"label": "Backlog"}, map[string]any{"label": "Doing"}, map[string]any{"label": "Done"},
		}},
	}, 201)
	statusID := status["id"].(string)
	opts, _ := status["config"].(map[string]any)["options"].([]any)
	doing := opts[1].(map[string]any)["id"].(string)
	done := opts[2].(map[string]any)["id"].(string)
	priority := alice.request("POST", "/api/collections/"+cid+"/properties", map[string]any{"name": "Priority", "type": "NUMBER"}, 201)
	priorityID := priority["id"].(string)
	notes := alice.request("POST", "/api/collections/"+cid+"/properties", map[string]any{"name": "Notes", "type": "TEXT"}, 201)
	notesID := notes["id"].(string)
	alice.request("POST", "/api/collections/"+cid+"/properties", map[string]any{
		"name": "Due", "type": "DATE", "config": map[string]any{"mode": "bogus"},
	}, 400)
	due := alice.request("POST", "/api/collections/"+cid+"/properties", map[string]any{
		"name": "Due", "type": "DATE", "config": map[string]any{"mode": "date"},
	}, 201)
	dueID := due["id"].(string)

	// Duplicate property name is a unique violation -> 409.
	alice.request("POST", "/api/collections/"+cid+"/properties", map[string]any{"name": "Priority", "type": "TEXT"}, 409)

	// Items: visibility required at creation, values validated, duplicates 409.
	foreign := collectionBookmark(t, bob, "https://127.0.0.1/private")
	alice.request("POST", "/api/collections/"+cid+"/items", map[string]any{"bookmarkId": foreign}, 400)
	alice.request("POST", "/api/collections/"+cid+"/items", map[string]any{"bookmarkId": bm1,
		"propertyValues": map[string]any{statusID: "not-an-option"}}, 400)
	alice.request("POST", "/api/collections/"+cid+"/items", map[string]any{"bookmarkId": bm1,
		"propertyValues": map[string]any{notesID: 5}}, 400)
	alice.request("POST", "/api/collections/"+cid+"/items", map[string]any{"bookmarkId": bm1,
		"propertyValues": map[string]any{dueID: "2026-13-45"}}, 400)
	item := alice.request("POST", "/api/collections/"+cid+"/items", map[string]any{"bookmarkId": bm1,
		"propertyValues": map[string]any{statusID: doing, priorityID: 2, dueID: "2026-12-31", notesID: "context"}}, 201)
	itemID := item["id"].(string)
	alice.request("POST", "/api/collections/"+cid+"/items", map[string]any{"bookmarkId": bm1}, 409)
	pv, _ := item["propertyValues"].(map[string]any)
	if pv[dueID] != "2026-12-31" {
		t.Fatalf("date-only value not stored verbatim: %v", pv[dueID])
	}
	if _, ok := item["bookmark"]; !ok {
		t.Fatal("item response missing the bookmark projection")
	}

	// Item PATCH enforces version (DATA-05 from day one) and merge semantics:
	// omitted keys untouched, null removes.
	alice.request("PATCH", "/api/collections/"+cid+"/items/"+itemID, map[string]any{"propertyValues": map[string]any{notesID: "x"}}, 400)
	alice.request("PATCH", "/api/collections/"+cid+"/items/"+itemID, map[string]any{"version": 99, "propertyValues": map[string]any{notesID: "x"}}, 409)
	updated := alice.request("PATCH", "/api/collections/"+cid+"/items/"+itemID, map[string]any{
		"version": 1, "propertyValues": map[string]any{notesID: nil, statusID: done},
	}, 200)
	if updated["version"].(float64) != 2 {
		t.Fatalf("item patch did not bump version: %v", updated["version"])
	}
	pv, _ = updated["propertyValues"].(map[string]any)
	if _, stillThere := pv[notesID]; stillThere {
		t.Fatal("null did not remove the value")
	}
	if pv[statusID] != done {
		t.Fatal("status not updated")
	}

	// The same bookmark in a second collection: contextual values independent,
	// intrinsic edits propagate to both (test matrix row 1).
	col2 := alice.request("POST", "/api/collections", map[string]any{"name": "Reading"}, 201)
	cid2 := col2["id"].(string)
	status2 := alice.request("POST", "/api/collections/"+cid2+"/properties", map[string]any{
		"name": "Status", "type": "SELECT",
		"config": map[string]any{"options": []any{map[string]any{"label": "Unread"}, map[string]any{"label": "Read"}}},
	}, 201)
	unread := status2["config"].(map[string]any)["options"].([]any)[0].(map[string]any)["id"].(string)
	item2 := alice.request("POST", "/api/collections/"+cid2+"/items", map[string]any{
		"bookmarkId": bm1, "propertyValues": map[string]any{status2["id"].(string): unread},
	}, 201)
	alice.request("PUT", "/api/bookmarks/"+bm1, map[string]any{"title": "Renamed intrinsically"}, 200)
	for _, pair := range [][2]string{{cid, itemID}, {cid2, item2["id"].(string)}} {
		got := alice.getJSON("/api/collections/"+pair[0]+"/items", 200)
		found := false
		for _, it := range got {
			if it["id"] == pair[1] {
				found = true
				bmk := it["bookmark"].(map[string]any)
				if bmk["title"] != "Renamed intrinsically" {
					t.Fatal("intrinsic edit did not propagate to membership", pair[0])
				}
			}
		}
		if !found {
			t.Fatal("item missing from list", pair[0])
		}
	}
	// Contextual independence: collection 1 says Done, collection 2 says Unread.
	first := alice.getJSON("/api/collections/"+cid+"/items", 200)[0]
	if first["propertyValues"].(map[string]any)[statusID] != done {
		t.Fatal("contextual value leaked between collections")
	}

	// Removing membership and then the whole collection never touches the
	// bookmark (N-01 acceptance).
	alice.request("DELETE", "/api/collections/"+cid2+"/items/"+item2["id"].(string), nil, 200)
	alice.request("GET", "/api/bookmarks/"+bm1, nil, 200)
	alice.request("DELETE", "/api/collections/"+cid2, nil, 200)
	alice.request("GET", "/api/bookmarks/"+bm1, nil, 200)
	alice.request("GET", "/api/collections/"+cid2, nil, 404)

	// Foreign users get 404 on every collection surface (ownership flow).
	bob.request("GET", "/api/collections/"+cid, nil, 404)
	bob.request("PATCH", "/api/collections/"+cid, map[string]any{"version": 1, "name": "hijack"}, 404)
	bob.request("GET", "/api/collections/"+cid+"/items", nil, 404)
	bob.request("POST", "/api/collections/"+cid+"/items", map[string]any{"bookmarkId": bm1}, 404)
	bob.request("DELETE", "/api/collections/"+cid, nil, 404)
}

func TestCollectionViewsFilterSortPaginate(t *testing.T) {
	srv := testServer(t)
	alice := newAPIUser(t, srv, "coll-views@computer.test")
	col := alice.request("POST", "/api/collections", map[string]any{"name": "Pipeline"}, 201)
	cid := col["id"].(string)

	status := alice.request("POST", "/api/collections/"+cid+"/properties", map[string]any{
		"name": "Status", "type": "SELECT",
		"config": map[string]any{"options": []any{map[string]any{"label": "Doing"}, map[string]any{"label": "Done"}}},
	}, 201)
	statusID := status["id"].(string)
	doing := status["config"].(map[string]any)["options"].([]any)[0].(map[string]any)["id"].(string)
	done := status["config"].(map[string]any)["options"].([]any)[1].(map[string]any)["id"].(string)
	prio := alice.request("POST", "/api/collections/"+cid+"/properties", map[string]any{"name": "Prio", "type": "NUMBER"}, 201)
	prioID := prio["id"].(string)

	ids := []string{}
	for i, s := range []string{doing, done, doing} {
		bm := collectionBookmark(t, alice, "https://127.0.0.1/v"+string(rune('a'+i)))
		p := float64(i)
		if i == 2 {
			p = 1 // tie with item 1 on the sort key: id must break it deterministically
		}
		it := alice.request("POST", "/api/collections/"+cid+"/items", map[string]any{
			"bookmarkId": bm, "propertyValues": map[string]any{statusID: s, prioID: p},
		}, 201)
		ids = append(ids, it["id"].(string))
	}

	// View validation: unknown properties and type-incompatible operators 400.
	alice.request("POST", "/api/collections/"+cid+"/views", map[string]any{"name": "Bad", "filters": map[string]any{
		"operator": "AND", "clauses": []any{map[string]any{"propertyId": "nope", "op": "eq", "value": "x"}},
	}}, 400)
	alice.request("POST", "/api/collections/"+cid+"/views", map[string]any{"name": "Bad2", "filters": map[string]any{
		"clauses": []any{map[string]any{"propertyId": statusID, "op": "gt", "value": doing}},
	}}, 400)
	alice.request("POST", "/api/collections/"+cid+"/views", map[string]any{"name": "Bad3", "sorts": []any{
		map[string]any{"field": "nope"},
	}}, 400)

	board := alice.request("POST", "/api/collections/"+cid+"/views", map[string]any{
		"name": "Doing board", "type": "BOARD",
		"filters":           map[string]any{"operator": "AND", "clauses": []any{map[string]any{"propertyId": statusID, "op": "eq", "value": doing}}},
		"sorts":             []any{map[string]any{"field": prioID, "dir": "desc"}},
		"groupByPropertyId": statusID,
	}, 201)
	vid := board["id"].(string)

	// Server-side filtering: only the two Doing items come back.
	doingItems := alice.getJSON("/api/collections/"+cid+"/items?viewId="+vid, 200)
	if len(doingItems) != 2 {
		t.Fatalf("filter returned %d items, want 2", len(doingItems))
	}
	// Sorted by priority desc: first page item has prio 1 (the tie item), and
	// cursor pagination with limit=1 walks both pages exactly once.
	first := doingItems[0]
	if first["propertyValues"].(map[string]any)[prioID].(float64) != 1 {
		t.Fatal("sort desc did not put the higher priority first")
	}
	res, e := alice.c.Get(alice.base + "/api/collections/" + cid + "/items?viewId=" + vid + "&limit=1")
	if e != nil {
		t.Fatal(e)
	}
	cursor := res.Header.Get("X-Next-Cursor")
	res.Body.Close()
	if cursor == "" {
		t.Fatal("no cursor despite more rows")
	}
	page := alice.getJSON("/api/collections/"+cid+"/items?viewId="+vid+"&limit=1&cursor="+cursor, 200)
	if len(page) != 1 {
		t.Fatalf("cursor page returned %d items", len(page))
	}
	if page[0]["id"] == first["id"] {
		t.Fatal("cursor page repeated the first row (non-deterministic tie-break)")
	}

	// View PATCH enforces version like everything else.
	alice.request("PATCH", "/api/collections/"+cid+"/views/"+vid, map[string]any{"name": "No version"}, 400)
	alice.request("PATCH", "/api/collections/"+cid+"/views/"+vid, map[string]any{"version": 99, "name": "Stale"}, 409)
	renamed := alice.request("PATCH", "/api/collections/"+cid+"/views/"+vid, map[string]any{"version": 1, "name": "Doing board v2"}, 200)
	if renamed["version"].(float64) != 2 {
		t.Fatalf("view patch did not bump version: %v", renamed["version"])
	}

	// Option removal with existing values is refused (stated policy); after
	// clearing the values it succeeds. Renames keep the option identity.
	alice.request("PATCH", "/api/collections/"+cid+"/properties/"+statusID, map[string]any{"version": 1,
		"config": map[string]any{"options": []any{map[string]any{"id": doing, "label": "In flight"}}}}, 400)
	// Clear the Doing values by moving every item to Done.
	for _, row := range alice.getJSON("/api/collections/"+cid+"/items", 200) {
		alice.request("PATCH", "/api/collections/"+cid+"/items/"+row["id"].(string), map[string]any{
			"version": row["version"].(float64), "propertyValues": map[string]any{statusID: done},
		}, 200)
	}
	renamedProp := alice.request("PATCH", "/api/collections/"+cid+"/properties/"+statusID, map[string]any{"version": 1,
		"config": map[string]any{"options": []any{map[string]any{"id": done, "label": "Finished"}}}}, 200)
	opts := renamedProp["config"].(map[string]any)["options"].([]any)
	if len(opts) != 1 || opts[0].(map[string]any)["id"] != done {
		t.Fatalf("option removal/rename produced %+v", opts)
	}
	// Type changes are forbidden; deleting the property scrubs values and views.
	alice.request("PATCH", "/api/collections/"+cid+"/properties/"+statusID, map[string]any{"version": 2, "type": "TEXT"}, 400)
	alice.request("DELETE", "/api/collections/"+cid+"/properties/"+statusID, nil, 200)
	view := alice.request("GET", "/api/collections/"+cid, nil, 200)
	for _, raw := range view["views"].([]any) {
		v := raw.(map[string]any)
		if v["id"] != vid {
			continue
		}
		f := v["filters"].(map[string]any)
		if len(f["clauses"].([]any)) != 0 {
			t.Fatalf("deleted property left filter clauses behind: %v", f)
		}
		// The sort keys on the surviving Prio property and must be untouched.
		if len(v["sorts"].([]any)) != 1 {
			t.Fatalf("unrelated sorts were scrubbed: %v", v["sorts"])
		}
		if v["groupByPropertyId"] != nil {
			t.Fatalf("groupBy was not cleared: %v", v["groupByPropertyId"])
		}
	}
	for _, raw := range alice.getJSON("/api/collections/"+cid+"/items", 200) {
		pv, _ := raw["propertyValues"].(map[string]any)
		if _, still := pv[statusID]; still {
			t.Fatal("deleted property left item values behind")
		}
	}
}

func TestCollectionRevocationReachesItems(t *testing.T) {
	srv := testServer(t)
	alice := newAPIUser(t, srv, "coll-rev@computer.test")
	aliceID := alice.request("GET", "/api/me", nil, 200)["user"].(map[string]any)["id"].(string)
	bob := newAPIUser(t, srv, "coll-rev-owner@computer.test")

	// Bob owns a bookmark and shares it with Alice; Alice puts it in HER
	// collection. Revocation must make the item vanish from every surface
	// (list, count, mutation) without deleting anything (N-08, decision #4).
	shared := collectionBookmark(t, bob, "https://127.0.0.1/shared")
	bob.request("POST", "/api/bookmarks/"+shared+"/share", map[string]any{"userIds": []string{aliceID}}, 200)
	col := alice.request("POST", "/api/collections", map[string]any{"name": "Watchlist"}, 201)
	cid := col["id"].(string)
	item := alice.request("POST", "/api/collections/"+cid+"/items", map[string]any{"bookmarkId": shared}, 201)
	itemID := item["id"].(string)
	if len(alice.getJSON("/api/collections/"+cid+"/items", 200)) != 1 {
		t.Fatal("shared bookmark item not listed")
	}
	for _, c := range alice.getJSON("/api/collections", 200) {
		if c["id"] == cid && c["_count"].(map[string]any)["items"].(float64) != 1 {
			t.Fatalf("authorized count wrong: %v", c["_count"])
		}
	}

	bob.request("DELETE", "/api/bookmarks/"+shared+"/share/"+aliceID, nil, 200)

	if len(alice.getJSON("/api/collections/"+cid+"/items", 200)) != 0 {
		t.Fatal("revoked bookmark still listed in collection items")
	}
	for _, c := range alice.getJSON("/api/collections", 200) {
		if c["id"] == cid && c["_count"].(map[string]any)["items"].(float64) != 0 {
			t.Fatalf("revoked bookmark still counted: %v", c["_count"])
		}
	}
	alice.request("PATCH", "/api/collections/"+cid+"/items/"+itemID, map[string]any{"version": 1, "position": 3}, 404)
	alice.request("DELETE", "/api/collections/"+cid+"/items/"+itemID, nil, 404)
	// Review COR-2: a hidden membership row must not wedge reorder. Alice
	// adds one of her own bookmarks, then reorders the visible items — the
	// revoked membership stays hidden at the tail instead of making every
	// PUT /items fail "Order must contain every item exactly once".
	own := collectionBookmark(t, alice, "https://127.0.0.1/alice-own")
	ownItem := alice.request("POST", "/api/collections/"+cid+"/items", map[string]any{"bookmarkId": own}, 201)
	ownID := ownItem["id"].(string)
	visible := alice.getJSON("/api/collections/"+cid+"/items", 200)
	if len(visible) != 1 || visible[0]["id"] != ownID {
		t.Fatalf("expected only the own item visible, got %v", visible)
	}
	alice.request("PUT", "/api/collections/"+cid+"/items", map[string]any{"order": []string{ownID}}, 200)
	// The membership row itself survives revocation: re-sharing restores it.
	bob.request("POST", "/api/bookmarks/"+shared+"/share", map[string]any{"userIds": []string{aliceID}}, 200)
	items := alice.getJSON("/api/collections/"+cid+"/items", 200)
	if len(items) != 2 {
		t.Fatal("re-sharing did not restore the existing membership")
	}
}
