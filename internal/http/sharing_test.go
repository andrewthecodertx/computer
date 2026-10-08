package http

import (
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"testing"
)

func TestRevocationAndForeignRelations(t *testing.T) {
	srv := testServer(t)
	newClient := func() apiClient { jar, _ := cookiejar.New(nil); return apiClient{t, &http.Client{Jar: jar}, srv.URL} }
	a, b := newClient(), newClient()
	account := func(c apiClient, email string) string {
		body := map[string]any{"email": email, "password": "test-password"}
		id := c.request("POST", "/api/signup", body, 201)["userId"].(string)
		c.request("POST", "/api/auth/login", body, 200)
		return id
	}
	account(a, "owner@computer.test")
	bid := account(b, "recipient@computer.test")
	tag := a.request("POST", "/api/tags", map[string]any{"name": "Shared"}, 201)["id"].(string)
	id := a.request("POST", "/api/bookmarks", map[string]any{"url": "https://127.0.0.1/test", "tagIds": []string{tag}, "dueDate": "2026-10-07"}, 201)["id"].(string)
	a.request("POST", "/api/bookmarks/"+id+"/share", map[string]any{"userIds": []string{bid}}, 200)
	b.request("GET", "/api/bookmarks/"+id, nil, 200)
	b.request("DELETE", "/api/bookmarks/"+id+"/share/"+bid, nil, 404)
	a.request("DELETE", "/api/bookmarks/"+id+"/share/"+bid, nil, 200)
	b.request("GET", "/api/bookmarks/"+id, nil, 404)
	a.request("PUT", "/api/tags/"+tag, map[string]any{"shareUserIds": []string{bid}}, 200)
	b.request("GET", "/api/bookmarks/"+id, nil, 200)
	a.request("DELETE", "/api/tags/"+tag+"/share/"+bid, nil, 200)
	b.request("GET", "/api/bookmarks/"+id, nil, 404)
	a.request("POST", "/api/shared-dates", map[string]any{"date": "2026-10-07", "recipientIds": []string{bid}}, 201)
	response, e := a.c.Get(srv.URL + "/api/shared-dates")
	if e != nil {
		t.Fatal(e)
	}
	var dates []map[string]any
	if e = json.NewDecoder(response.Body).Decode(&dates); e != nil {
		t.Fatal(e)
	}
	response.Body.Close()
	b.request("GET", "/api/bookmarks/"+id, nil, 200)
	a.request("DELETE", "/api/shared-dates/"+dates[0]["id"].(string), nil, 200)
	b.request("GET", "/api/bookmarks/"+id, nil, 404)
	foreign := b.request("POST", "/api/kanban/columns", map[string]any{"label": "Private"}, 201)["id"].(string)
	a.request("PUT", "/api/bookmarks/"+id, map[string]any{"kanbanColumnId": foreign}, 400)
	a.request("PUT", "/api/bookmarks/"+id, map[string]any{"dueDate": map[string]any{}}, 400)
}
