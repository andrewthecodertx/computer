package handlers

import "net/http"

func HandlePublicBookmark(w http.ResponseWriter, r *http.Request) {
	v, e := bookmark(r, r.PathValue("id"), true)
	if e == nil {
		delete(v, "sharedWith")
		delete(v, "contact")
		delete(v, "pageLinks")
		if owner, ok := v["owner"].(map[string]any); ok {
			delete(owner, "email")
			delete(owner, "image")
		}
	}
	respond(w, 200, v, e)
}
