package handlers

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/andrew/go-computer/internal/db"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Collections (Notion-like upgrades phase 2, N-01/N-02/N-03).
//
// Locked semantics:
//   - Membership is contextual: items reference bookmarks without owning
//     them; removing an item or a whole collection never touches bookmarks.
//   - Membership never broadens access: every item surface re-checks
//     db.BookmarkVisible, and items whose bookmark was revoked behave as
//     not-found (N-08). Creating an item requires current visibility.
//   - Every mutating PATCH requires the expected `version` (400 missing,
//     409 stale) — the DATA-05 lesson applied from day one.
//   - Views are queries: filters/sorts/grouping are stored definitions,
//     validated against registered properties with fixed operator allowlists;
//     values are bound parameters, never interpolated SQL.
//   - Property types are immutable after creation and RELATION is rejected
//     until cross-collection access is designed (draft §6 open issue).

var (
	collectionPropTypes = map[string]bool{
		"TEXT": true, "NUMBER": true, "SELECT": true, "MULTI_SELECT": true,
		"DATE": true, "CHECKBOX": true, "URL": true,
		// RELATION exists in the schema CHECK but is API-rejected for now.
	}
	collectionViewTypes  = map[string]bool{"TABLE": true, "BOARD": true, "CALENDAR": true, "LIST": true}
	collectionGroupTypes = map[string]bool{"SELECT": true, "MULTI_SELECT": true, "CHECKBOX": true, "TEXT": true}
	collectionSortTypes  = map[string]bool{"TEXT": true, "URL": true, "NUMBER": true, "SELECT": true, "DATE": true, "CHECKBOX": true}
	// Filter operator allowlist per property type.
	collectionFilterOps = map[string]map[string]bool{
		"TEXT":         {"eq": true, "ne": true, "contains": true, "empty": true, "notEmpty": true},
		"URL":          {"eq": true, "ne": true, "contains": true, "empty": true, "notEmpty": true},
		"NUMBER":       {"eq": true, "ne": true, "gt": true, "gte": true, "lt": true, "lte": true, "empty": true, "notEmpty": true},
		"SELECT":       {"eq": true, "ne": true, "empty": true, "notEmpty": true},
		"MULTI_SELECT": {"contains": true, "empty": true, "notEmpty": true},
		"DATE":         {"eq": true, "ne": true, "gt": true, "gte": true, "lt": true, "lte": true, "empty": true, "notEmpty": true},
		"CHECKBOX":     {"eq": true, "ne": true, "empty": true, "notEmpty": true},
	}
	dateOnlyPattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	// Fixed-width so lexicographic comparison equals chronological order.
	dateTimeStorageFormat = "2006-01-02T15:04:05.000000000Z07:00"
	likeEscaper           = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
)

func collectionOwned(r *http.Request, q db.Queryer, id string) (object, error) {
	return db.One(r.Context(), q, `SELECT to_jsonb(c) FROM "Collection" c WHERE c.id=$1 AND c."ownerId"=$2`, id, user(r))
}
func collectionProperties(r *http.Request, q db.Queryer, collectionID string) ([]object, error) {
	return db.Many(r.Context(), q, `SELECT to_jsonb(p) FROM "CollectionProperty" p WHERE p."collectionId"=$1 ORDER BY p.position,p.id`, collectionID)
}
func collectionViews(r *http.Request, q db.Queryer, collectionID string) ([]object, error) {
	return db.Many(r.Context(), q, `SELECT to_jsonb(v) FROM "CollectionView" v WHERE v."collectionId"=$1 ORDER BY v.position,v.id`, collectionID)
}
func propertyIndex(props []object) map[string]object {
	byID := map[string]object{}
	for _, p := range props {
		byID[str(p, "id")] = p
	}
	return byID
}
func collectionDetail(r *http.Request, q db.Queryer, col object) (object, error) {
	props, e := collectionProperties(r, q, str(col, "id"))
	if e != nil {
		return nil, e
	}
	views, e := collectionViews(r, q, str(col, "id"))
	if e != nil {
		return nil, e
	}
	// Same authorized-count shape as the list endpoint: items only counts
	// bookmarks the caller can currently see (N-10 edge case).
	counts, e := db.One(r.Context(), q, `SELECT jsonb_build_object(
		'items',(SELECT count(*) FROM "CollectionItem" ci JOIN "Bookmark" b ON b.id=ci."bookmarkId" WHERE ci."collectionId"=$1 AND `+db.BookmarkVisible+`),
		'properties',(SELECT count(*) FROM "CollectionProperty" p WHERE p."collectionId"=$1),
		'views',(SELECT count(*) FROM "CollectionView" v WHERE v."collectionId"=$1))`, str(col, "id"), user(r))
	if e != nil {
		return nil, e
	}
	out := object{}
	for k, v := range col {
		out[k] = v
	}
	out["properties"], out["views"], out["_count"] = props, views, counts
	return out, nil
}

// patchVersion enforces the DATA-05 rule on every mutating collection PATCH:
// the base version is required, never optional.
func patchVersion(b object) (int, error) {
	v, ok := b["version"]
	if !ok {
		return 0, bad("version is required (optimistic concurrency); send the value from GET")
	}
	f, isNum := v.(float64)
	if !isNum || f < 0 || f != float64(int64(f)) {
		return 0, bad("version must be a non-negative integer")
	}
	return int(f), nil
}
func intField(b object, k string) (int, bool, error) {
	v, ok := b[k]
	if !ok {
		return 0, false, nil
	}
	f, isNum := v.(float64)
	if !isNum || f < 0 || f != float64(int64(f)) {
		return 0, false, bad(k + " must be a non-negative integer")
	}
	return int(f), true, nil
}
func trimLimit(v string, max int) string {
	r := []rune(v)
	if len(r) > max {
		r = r[:max]
	}
	return string(r)
}

func collectionData(b object) (object, error) {
	d := fields(b, "name description icon")
	if e := stringFields(d, "description icon", "name"); e != nil {
		return nil, e
	}
	if v, ok := d["name"]; ok {
		s := strings.TrimSpace(v.(string))
		if s == "" {
			return nil, bad("Name required")
		}
		d["name"] = trimLimit(s, 120)
	}
	if v, ok := d["description"]; ok && v != nil {
		d["description"] = trimLimit(v.(string), 2000)
	}
	if v, ok := d["icon"]; ok && v != nil {
		d["icon"] = trimLimit(v.(string), 64)
	}
	return d, nil
}

func HandleCollectionsList(w http.ResponseWriter, r *http.Request) {
	// Item counts are authorized counts (N-10 edge case): a bookmark the
	// caller can no longer see never inflates them. Collections are
	// owner-scoped in phase 2, so $1 (owner) and $2 (BookmarkVisible) match.
	v, e := db.Many(r.Context(), database(r), `SELECT to_jsonb(c)||jsonb_build_object('_count',jsonb_build_object(
		'items',(SELECT count(*) FROM "CollectionItem" ci JOIN "Bookmark" b ON b.id=ci."bookmarkId" WHERE ci."collectionId"=c.id AND `+db.BookmarkVisible+`),
		'properties',(SELECT count(*) FROM "CollectionProperty" p WHERE p."collectionId"=c.id),
		'views',(SELECT count(*) FROM "CollectionView" v WHERE v."collectionId"=c.id)))
		FROM "Collection" c WHERE c."ownerId"=$1 ORDER BY c."createdAt",c.id`, user(r), user(r))
	respond(w, 200, v, e)
}

func HandleCollectionsCreate(w http.ResponseWriter, r *http.Request) {
	b, ok := read(w, r)
	if !ok {
		return
	}
	d, e := collectionData(b)
	if e != nil {
		respond(w, 0, nil, e)
		return
	}
	if _, ok := d["name"]; !ok {
		respond(w, 0, nil, bad("Name required"))
		return
	}
	d["id"], d["ownerId"] = db.NewID(), user(r)
	var out object
	e = db.Transaction(r.Context(), database(r), func(tx *sql.Tx) error {
		if e := lockUser(r, tx); e != nil {
			return e
		}
		col, e := db.Insert(r.Context(), tx, "Collection", d)
		if e != nil {
			return e
		}
		// Every collection starts with a default table view (draft §7).
		if _, e := db.Insert(r.Context(), tx, "CollectionView", object{
			"id": db.NewID(), "collectionId": str(col, "id"), "name": "Table", "type": "TABLE", "position": 0,
		}); e != nil {
			return e
		}
		out, e = collectionDetail(r, tx, col)
		return e
	})
	respond(w, 201, out, e)
}

func HandleCollectionsGet(w http.ResponseWriter, r *http.Request) {
	col, e := collectionOwned(r, database(r), r.PathValue("id"))
	if e != nil {
		respond(w, 0, nil, e)
		return
	}
	out, e := collectionDetail(r, database(r), col)
	respond(w, 200, out, e)
}

func HandleCollectionsUpdate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	b, ok := read(w, r)
	if !ok {
		return
	}
	base, e := patchVersion(b)
	if e != nil {
		respond(w, 0, nil, e)
		return
	}
	d, e := collectionData(b)
	if e != nil {
		respond(w, 0, nil, e)
		return
	}
	d["updatedAt"] = time.Now().UTC()
	var out object
	e = db.Transaction(r.Context(), database(r), func(tx *sql.Tx) error {
		if e := lockUser(r, tx); e != nil {
			return e
		}
		cur, e := collectionOwned(r, tx, id)
		if e != nil {
			return e
		}
		stored, _ := cur["version"].(float64)
		if int(stored) != base {
			return conflict("Collection was modified elsewhere; reload to see the latest version")
		}
		d["version"] = base + 1
		out, e = db.Update(r.Context(), tx, "Collection", id, "ownerId", user(r), d)
		return e
	})
	respond(w, 200, out, e)
}

func HandleCollectionsDelete(w http.ResponseWriter, r *http.Request) {
	// Memberships, properties and views cascade; bookmarks are never touched
	// (N-01 acceptance criterion).
	respond(w, 200, OK{true}, remove(r, "Collection", r.PathValue("id"), "ownerId"))
}

// --- properties -----------------------------------------------------------

func optionList(config any) []object {
	m, _ := config.(object)
	arr, _ := m["options"].([]any)
	out := make([]object, 0, len(arr))
	for _, o := range arr {
		if opt, ok := o.(object); ok {
			out = append(out, opt)
		}
	}
	return out
}
func optionIDs(prop object) map[string]bool {
	ids := map[string]bool{}
	for _, o := range optionList(prop["config"]) {
		if s := str(o, "id"); s != "" {
			ids[s] = true
		}
	}
	return ids
}

// validatePropertyConfig validates and normalizes config for a property type.
// existing is the current stored option set (nil on create): incoming options
// may reference existing ids (rename/recolor, identity preserved) or omit ids
// (server assigns a stable one).
func validatePropertyConfig(typ string, raw any, existing []object) (object, []string, error) {
	switch typ {
	case "SELECT", "MULTI_SELECT":
		m, ok := raw.(object)
		if raw != nil && !ok {
			return nil, nil, bad("config must be an object")
		}
		if m == nil {
			m = object{}
		}
		arr, hasOptions := m["options"].([]any)
		if raw != nil && !hasOptions {
			return nil, nil, bad("SELECT properties require config.options")
		}
		if len(arr) > 100 {
			return nil, nil, bad("Too many options (max 100)")
		}
		byID := map[string]object{}
		for _, o := range existing {
			byID[str(o, "id")] = o
		}
		options, seen := []object{}, map[string]bool{}
		for _, entry := range arr {
			o, ok := entry.(object)
			if !ok {
				return nil, nil, bad("Options must be objects with a label")
			}
			label := strings.TrimSpace(str(o, "label"))
			if label == "" {
				return nil, nil, bad("Option label required")
			}
			opt := object{"label": trimLimit(label, 80)}
			if c := str(o, "color"); c != "" {
				if !colorPattern.MatchString(c) {
					return nil, nil, bad("Option color must be #RRGGBB")
				}
				opt["color"] = c
			}
			if id := str(o, "id"); id != "" {
				if _, ok := byID[id]; !ok {
					return nil, nil, bad("Unknown option id; new options must omit id")
				}
				if seen[id] {
					return nil, nil, bad("Duplicate option id")
				}
				seen[id] = true
				opt["id"] = id
			} else {
				opt["id"] = db.NewID()
			}
			options = append(options, opt)
		}
		removed := []string{}
		for id := range byID {
			if !seen[id] {
				removed = append(removed, id)
			}
		}
		return object{"options": options}, removed, nil
	case "DATE":
		m, ok := raw.(object)
		if raw != nil && !ok {
			return nil, nil, bad("config must be an object")
		}
		mode := "date"
		if m != nil {
			if v, ok := m["mode"]; ok {
				s, isStr := v.(string)
				if !isStr || (s != "date" && s != "datetime") {
					return nil, nil, bad("Date mode must be 'date' or 'datetime'")
				}
				mode = s
			}
		}
		return object{"mode": mode}, nil, nil
	default:
		if m, ok := raw.(object); ok && raw != nil && len(m) > 0 {
			return nil, nil, bad(typ + " properties take no config")
		}
		return object{}, nil, nil
	}
}

// normalizePropertyValue validates one value against its property definition
// and returns the canonical storage form (dates are fixed-width UTC strings,
// numbers JSON numbers, multi-selects deduplicated option-id arrays).
func normalizePropertyValue(prop object, v any) (any, error) {
	name := str(prop, "name")
	switch str(prop, "type") {
	case "TEXT":
		s, ok := v.(string)
		if !ok {
			return nil, bad(name + " must be text")
		}
		return trimLimit(s, 4000), nil
	case "URL":
		s, ok := v.(string)
		if !ok || !validURL(s) {
			return nil, bad(name + " must be a valid HTTP(S) URL")
		}
		return trimLimit(s, 2000), nil
	case "NUMBER":
		f, ok := v.(float64)
		if !ok || math.IsNaN(f) || math.IsInf(f, 0) {
			return nil, bad(name + " must be a number")
		}
		return f, nil
	case "CHECKBOX":
		b, ok := v.(bool)
		if !ok {
			return nil, bad(name + " must be a boolean")
		}
		return b, nil
	case "SELECT":
		s, ok := v.(string)
		if !ok || !optionIDs(prop)[s] {
			return nil, bad(name + " must be one of its options")
		}
		return s, nil
	case "MULTI_SELECT":
		arr, ok := v.([]any)
		if !ok || len(arr) > 50 {
			return nil, bad(name + " must be an array of at most 50 options")
		}
		ids := optionIDs(prop)
		out, seen := make([]string, 0, len(arr)), map[string]bool{}
		for _, e := range arr {
			s, ok := e.(string)
			if !ok || !ids[s] {
				return nil, bad(name + " must contain only its options")
			}
			if !seen[s] {
				seen[s] = true
				out = append(out, s)
			}
		}
		return out, nil
	case "DATE":
		s, ok := v.(string)
		if !ok {
			return nil, bad(name + " must be an ISO date string")
		}
		mode := "date"
		if cfg, ok := prop["config"].(object); ok {
			if m, ok := cfg["mode"].(string); ok {
				mode = m
			}
		}
		if mode == "date" {
			if !dateOnlyPattern.MatchString(s) {
				return nil, bad(name + " must be YYYY-MM-DD (date-only values never shift across timezones)")
			}
			if _, e := time.Parse("2006-01-02", s); e != nil {
				return nil, bad(name + " must be a real calendar date")
			}
			return s, nil
		}
		for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04", "2006-01-02T15:04:05", "2006-01-02"} {
			if t, e := time.Parse(layout, s); e == nil {
				return t.UTC().Format(dateTimeStorageFormat), nil
			}
		}
		return nil, bad(name + " must be an RFC3339 timestamp")
	}
	return nil, bad("Unsupported property type")
}

// applyPropertyValues merges a request propertyValues object over the current
// stored values: unknown property ids are rejected, null removes a value
// (omitted keys are untouched — the repo-wide omitted-vs-null rule).
func applyPropertyValues(current object, props []object, raw any) (object, error) {
	m, ok := raw.(object)
	if !ok {
		return nil, bad("propertyValues must be an object keyed by property id")
	}
	byID := propertyIndex(props)
	out := object{}
	for k, v := range current {
		out[k] = v
	}
	for k, v := range m {
		p, known := byID[k]
		if !known {
			return nil, bad("Unknown property id " + k)
		}
		if v == nil {
			delete(out, k)
			continue
		}
		nv, e := normalizePropertyValue(p, v)
		if e != nil {
			return nil, e
		}
		out[k] = nv
	}
	return out, nil
}

func marshalValues(values object) (string, error) {
	raw, e := json.Marshal(values)
	if e != nil {
		return "", e
	}
	return string(raw), nil
}

func HandleCollectionPropertiesCreate(w http.ResponseWriter, r *http.Request) {
	cid := r.PathValue("id")
	b, ok := read(w, r)
	if !ok {
		return
	}
	name := strings.TrimSpace(str(b, "name"))
	if name == "" {
		respond(w, 0, nil, bad("Name required"))
		return
	}
	typ := str(b, "type")
	if typ == "RELATION" {
		respond(w, 0, nil, bad("RELATION properties are not supported yet"))
		return
	}
	if !collectionPropTypes[typ] {
		respond(w, 0, nil, bad("Invalid property type"))
		return
	}
	cfg, _, e := validatePropertyConfig(typ, b["config"], nil)
	if e != nil {
		respond(w, 0, nil, e)
		return
	}
	raw, _ := json.Marshal(cfg)
	var out object
	e = db.Transaction(r.Context(), database(r), func(tx *sql.Tx) error {
		if e := lockUser(r, tx); e != nil {
			return e
		}
		if _, e := collectionOwned(r, tx, cid); e != nil {
			return e
		}
		var n int
		if e := tx.QueryRowContext(r.Context(), `SELECT COALESCE(max(position),-1)+1 FROM "CollectionProperty" WHERE "collectionId"=$1`, cid).Scan(&n); e != nil {
			return e
		}
		out, e = db.Insert(r.Context(), tx, "CollectionProperty", object{
			"id": db.NewID(), "collectionId": cid, "name": trimLimit(name, 80),
			"type": typ, "position": n, "config": string(raw),
		})
		return e
	})
	respond(w, 201, out, e)
}

func HandleCollectionPropertiesUpdate(w http.ResponseWriter, r *http.Request) {
	cid, pid := r.PathValue("id"), r.PathValue("propertyId")
	b, ok := read(w, r)
	if !ok {
		return
	}
	base, e := patchVersion(b)
	if e != nil {
		respond(w, 0, nil, e)
		return
	}
	if _, ok := b["type"]; ok {
		respond(w, 0, nil, bad("Property type cannot be changed; create a new property (conversion must never destroy values)"))
		return
	}
	var out object
	e = db.Transaction(r.Context(), database(r), func(tx *sql.Tx) error {
		if e := lockUser(r, tx); e != nil {
			return e
		}
		if _, e := collectionOwned(r, tx, cid); e != nil {
			return e
		}
		cur, e := db.One(r.Context(), tx, `SELECT to_jsonb(p) FROM "CollectionProperty" p WHERE p.id=$1 AND p."collectionId"=$2`, pid, cid)
		if e != nil {
			return e
		}
		stored, _ := cur["version"].(float64)
		if int(stored) != base {
			return conflict("Property was modified elsewhere; reload to see the latest version")
		}
		typ := str(cur, "type")
		d := object{"version": base + 1, "updatedAt": time.Now().UTC()}
		if v, ok := b["name"]; ok {
			s, isStr := v.(string)
			if !isStr || strings.TrimSpace(s) == "" {
				return bad("Name must be non-empty text")
			}
			d["name"] = trimLimit(strings.TrimSpace(s), 80)
		}
		if pos, ok2, e2 := intField(b, "position"); e2 != nil {
			return e2
		} else if ok2 {
			d["position"] = pos
		}
		if v, ok := b["config"]; ok {
			cfg, removed, e2 := validatePropertyConfig(typ, v, optionList(cur["config"]))
			if e2 != nil {
				return e2
			}
			// Date mode flips would silently reinterpret every stored value.
			if typ == "DATE" {
				oldMode, newMode := "date", "date"
				if c, ok := cur["config"].(object); ok {
					if m, ok := c["mode"].(string); ok {
						oldMode = m
					}
				}
				if m, ok := cfg["mode"].(string); ok {
					newMode = m
				}
				if oldMode != newMode {
					return bad("Date mode cannot be changed; create a new property")
				}
			}
			// Stated policy for option removal with existing values: refuse
			// and report the count — clearing values is a deliberate act.
			for _, oid := range removed {
				var n int
				q := `SELECT count(*) FROM "CollectionItem" WHERE "collectionId"=$1 AND "propertyValues"->>$2=$3`
				if typ == "MULTI_SELECT" {
					q = `SELECT count(*) FROM "CollectionItem" WHERE "collectionId"=$1 AND jsonb_exists("propertyValues"->$2,$3)`
				}
				if e3 := tx.QueryRowContext(r.Context(), q, cid, pid, oid).Scan(&n); e3 != nil {
					return e3
				}
				if n > 0 {
					return bad(fmt.Sprintf("Option is still used by %d item(s); clear those values first", n))
				}
			}
			raw, _ := json.Marshal(cfg)
			d["config"] = string(raw)
		}
		out, e = db.Update(r.Context(), tx, "CollectionProperty", pid, "collectionId", cid, d)
		return e
	})
	respond(w, 200, out, e)
}

func HandleCollectionPropertiesDelete(w http.ResponseWriter, r *http.Request) {
	cid, pid := r.PathValue("id"), r.PathValue("propertyId")
	e := db.Transaction(r.Context(), database(r), func(tx *sql.Tx) error {
		if e := lockUser(r, tx); e != nil {
			return e
		}
		if _, e := collectionOwned(r, tx, cid); e != nil {
			return e
		}
		res, e := tx.ExecContext(r.Context(), `DELETE FROM "CollectionProperty" WHERE id=$1 AND "collectionId"=$2`, pid, cid)
		if e != nil {
			return e
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return db.ErrNotFound
		}
		// Drop the property's values everywhere and scrub it from saved view
		// definitions, so no view keeps filtering/sorting on a dead id
		// (groupByPropertyId clears itself via ON DELETE SET NULL).
		if _, e := tx.ExecContext(r.Context(), `UPDATE "CollectionItem" SET "propertyValues"="propertyValues"-$2,"updatedAt"=now() WHERE "collectionId"=$1 AND jsonb_exists("propertyValues",$2)`, cid, pid); e != nil {
			return e
		}
		views, e := collectionViews(r, tx, cid)
		if e != nil {
			return e
		}
		for _, v := range views {
			filters := scrubFilters(v["filters"], pid)
			sorts := scrubSorts(v["sorts"], pid)
			rawF, _ := json.Marshal(filters)
			rawS, _ := json.Marshal(sorts)
			if _, e := tx.ExecContext(r.Context(), `UPDATE "CollectionView" SET filters=$1,sorts=$2,"updatedAt"=now() WHERE id=$3`, string(rawF), string(rawS), str(v, "id")); e != nil {
				return e
			}
		}
		return nil
	})
	respond(w, 200, OK{true}, e)
}

func scrubFilters(raw any, pid string) object {
	out := object{"operator": "AND", "clauses": []any{}}
	m, _ := raw.(object)
	if op := str(m, "operator"); op == "OR" {
		out["operator"] = "OR"
	}
	arr, _ := m["clauses"].([]any)
	kept := []any{}
	for _, c := range arr {
		cl, _ := c.(object)
		if str(cl, "propertyId") != pid {
			kept = append(kept, cl)
		}
	}
	out["clauses"] = kept
	return out
}
func scrubSorts(raw any, pid string) []any {
	arr, _ := raw.([]any)
	kept := []any{}
	for _, s := range arr {
		so, _ := s.(object)
		if str(so, "field") != pid {
			kept = append(kept, so)
		}
	}
	return kept
}

// --- views ----------------------------------------------------------------

func validateFilters(props []object, raw any) (object, error) {
	m, ok := raw.(object)
	if !ok {
		return nil, bad("filters must be an object")
	}
	operator := str(m, "operator")
	if operator == "" {
		operator = "AND"
	}
	if operator != "AND" && operator != "OR" {
		return nil, bad("Filter operator must be AND or OR (nested groups are not supported)")
	}
	if v, present := m["clauses"]; present {
		if _, ok := v.([]any); !ok && v != nil {
			return nil, bad("Filter clauses must be an array")
		}
	}
	arr, _ := m["clauses"].([]any)
	if len(arr) > 20 {
		return nil, bad("Too many filter clauses (max 20)")
	}
	byID := propertyIndex(props)
	clauses := []any{}
	for _, entry := range arr {
		cl, ok := entry.(object)
		if !ok {
			return nil, bad("Filter clause must be an object")
		}
		pid, op := str(cl, "propertyId"), str(cl, "op")
		p, known := byID[pid]
		if !known {
			return nil, bad("Filter references an unknown property")
		}
		typ := str(p, "type")
		if !collectionFilterOps[typ][op] {
			return nil, bad("Operator " + op + " is not allowed on " + typ + " properties")
		}
		out := object{"propertyId": pid, "op": op}
		if op != "empty" && op != "notEmpty" {
			v, present := cl["value"]
			if !present || v == nil {
				return nil, bad("Operator " + op + " requires a value")
			}
			switch {
			case op == "contains" && typ == "MULTI_SELECT":
				s, isStr := v.(string)
				if !isStr || !optionIDs(p)[s] {
					return nil, bad("contains on a multi-select requires one of its option ids")
				}
				out["value"] = s
			case op == "contains": // TEXT/URL substring
				s, isStr := v.(string)
				if !isStr || s == "" {
					return nil, bad("contains requires a non-empty string")
				}
				out["value"] = trimLimit(s, 2000)
			default:
				nv, e := normalizePropertyValue(p, v)
				if e != nil {
					return nil, e
				}
				out["value"] = nv
			}
		}
		clauses = append(clauses, out)
	}
	return object{"operator": operator, "clauses": clauses}, nil
}

func validateSorts(props []object, raw any) ([]any, error) {
	arr, ok := raw.([]any)
	if !ok {
		return nil, bad("sorts must be an array")
	}
	if len(arr) > 3 {
		return nil, bad("Too many sort keys (max 3)")
	}
	byID := propertyIndex(props)
	out := []any{}
	for _, entry := range arr {
		s, ok := entry.(object)
		if !ok {
			return nil, bad("Sort entry must be an object")
		}
		field, dir := str(s, "field"), str(s, "dir")
		if dir == "" {
			dir = "asc"
		}
		if dir != "asc" && dir != "desc" {
			return nil, bad("Sort dir must be asc or desc")
		}
		switch field {
		case "position", "addedAt", "title":
		default:
			p, known := byID[field]
			if !known {
				return nil, bad("Sort references an unknown property")
			}
			if !collectionSortTypes[str(p, "type")] {
				return nil, bad("Multi-select properties cannot be sorted on")
			}
		}
		out = append(out, object{"field": field, "dir": dir})
	}
	return out, nil
}

func viewData(b object, props []object) (object, error) {
	d := object{}
	if v, ok := b["name"]; ok {
		s, isStr := v.(string)
		if !isStr || strings.TrimSpace(s) == "" {
			return nil, bad("View name must be non-empty text")
		}
		d["name"] = trimLimit(strings.TrimSpace(s), 80)
	}
	if v, ok := b["type"]; ok {
		s, isStr := v.(string)
		if !isStr || !collectionViewTypes[s] {
			return nil, bad("View type must be TABLE, BOARD, CALENDAR or LIST")
		}
		d["type"] = s
	}
	if v, ok := b["filters"]; ok {
		f, e := validateFilters(props, v)
		if e != nil {
			return nil, e
		}
		raw, _ := json.Marshal(f)
		d["filters"] = string(raw)
	}
	if v, ok := b["sorts"]; ok {
		s, e := validateSorts(props, v)
		if e != nil {
			return nil, e
		}
		raw, _ := json.Marshal(s)
		d["sorts"] = string(raw)
	}
	if v, ok := b["groupByPropertyId"]; ok {
		if v == nil {
			d["groupByPropertyId"] = nil
		} else {
			s, isStr := v.(string)
			if !isStr {
				return nil, bad("groupByPropertyId must be text or null")
			}
			found := false
			for _, p := range props {
				if str(p, "id") == s {
					if !collectionGroupTypes[str(p, "type")] {
						return nil, bad("Grouping is supported on select, multi-select, checkbox and text properties")
					}
					found = true
				}
			}
			if !found {
				return nil, bad("groupByPropertyId references an unknown property")
			}
			d["groupByPropertyId"] = s
		}
	}
	if v, ok := b["displaySettings"]; ok {
		if v == nil {
			d["displaySettings"] = "{}"
		} else {
			m, isObj := v.(object)
			if !isObj {
				return nil, bad("displaySettings must be an object")
			}
			raw, e := json.Marshal(m)
			if e != nil || len(raw) > 8<<10 {
				return nil, bad("displaySettings too large (max 8KB)")
			}
			d["displaySettings"] = string(raw)
		}
	}
	return d, nil
}

func HandleCollectionViewsCreate(w http.ResponseWriter, r *http.Request) {
	cid := r.PathValue("id")
	b, ok := read(w, r)
	if !ok {
		return
	}
	var out object
	e := db.Transaction(r.Context(), database(r), func(tx *sql.Tx) error {
		if e := lockUser(r, tx); e != nil {
			return e
		}
		if _, e := collectionOwned(r, tx, cid); e != nil {
			return e
		}
		props, e := collectionProperties(r, tx, cid)
		if e != nil {
			return e
		}
		d, e := viewData(b, props)
		if e != nil {
			return e
		}
		if _, ok := d["name"]; !ok {
			return bad("View name required")
		}
		if _, ok := d["type"]; !ok {
			d["type"] = "TABLE"
		}
		var n int
		if e := tx.QueryRowContext(r.Context(), `SELECT COALESCE(max(position),-1)+1 FROM "CollectionView" WHERE "collectionId"=$1`, cid).Scan(&n); e != nil {
			return e
		}
		d["id"], d["collectionId"], d["position"] = db.NewID(), cid, n
		out, e = db.Insert(r.Context(), tx, "CollectionView", d)
		return e
	})
	respond(w, 201, out, e)
}

func HandleCollectionViewsUpdate(w http.ResponseWriter, r *http.Request) {
	cid, vid := r.PathValue("id"), r.PathValue("viewId")
	b, ok := read(w, r)
	if !ok {
		return
	}
	base, e := patchVersion(b)
	if e != nil {
		respond(w, 0, nil, e)
		return
	}
	var out object
	e = db.Transaction(r.Context(), database(r), func(tx *sql.Tx) error {
		if e := lockUser(r, tx); e != nil {
			return e
		}
		if _, e := collectionOwned(r, tx, cid); e != nil {
			return e
		}
		cur, e := db.One(r.Context(), tx, `SELECT to_jsonb(v) FROM "CollectionView" v WHERE v.id=$1 AND v."collectionId"=$2`, vid, cid)
		if e != nil {
			return e
		}
		stored, _ := cur["version"].(float64)
		if int(stored) != base {
			return conflict("View was modified elsewhere; reload to see the latest version")
		}
		props, e := collectionProperties(r, tx, cid)
		if e != nil {
			return e
		}
		d, e := viewData(b, props)
		if e != nil {
			return e
		}
		if pos, ok2, e2 := intField(b, "position"); e2 != nil {
			return e2
		} else if ok2 {
			d["position"] = pos
		}
		d["version"], d["updatedAt"] = base+1, time.Now().UTC()
		out, e = db.Update(r.Context(), tx, "CollectionView", vid, "collectionId", cid, d)
		return e
	})
	respond(w, 200, out, e)
}

func HandleCollectionViewsDelete(w http.ResponseWriter, r *http.Request) {
	cid, vid := r.PathValue("id"), r.PathValue("viewId")
	res, e := database(r).ExecContext(r.Context(), `DELETE FROM "CollectionView" WHERE id=$1 AND "collectionId"=$2 AND "collectionId" IN (SELECT id FROM "Collection" WHERE "ownerId"=$3)`, vid, cid, user(r))
	if e == nil {
		if n, _ := res.RowsAffected(); n == 0 {
			e = db.ErrNotFound
		}
	}
	respond(w, 200, OK{true}, e)
}

// --- items ----------------------------------------------------------------

// collectionItemRow is the canonical item projection: the membership row plus
// the permission-appropriate bookmark view. Gated on BookmarkVisible so a
// revoked share makes the item not-found everywhere (N-08).
func collectionItemRow(r *http.Request, q db.Queryer, cid, itemID string) (object, error) {
	return db.One(r.Context(), q, `SELECT to_jsonb(ci)||jsonb_build_object('bookmark',`+db.BookmarkViewerView+`)
		FROM "CollectionItem" ci JOIN "Bookmark" b ON b.id=ci."bookmarkId" JOIN "Collection" c ON c.id=ci."collectionId"
		WHERE ci.id=$1 AND ci."collectionId"=$3 AND c."ownerId"=$2 AND `+db.BookmarkVisible, itemID, user(r), cid)
}

// filterClauseSQL renders one validated clause into a parameterized
// predicate. Property ids and values are always bind parameters — JSONB key
// access takes the key as $n, so no request data ever reaches SQL text.
func filterClauseSQL(byID map[string]object, cl object, args *[]any) (string, error) {
	pid, op := str(cl, "propertyId"), str(cl, "op")
	p, ok := byID[pid]
	if !ok {
		return "", bad("Filter references an unknown property")
	}
	if !collectionFilterOps[str(p, "type")][op] {
		return "", bad("Operator " + op + " is not allowed on this property")
	}
	*args = append(*args, pid)
	k := len(*args)
	typ := str(p, "type")
	missing := fmt.Sprintf(`(NOT jsonb_exists(ci."propertyValues",$%d) OR jsonb_typeof(ci."propertyValues"->$%d)='null')`, k, k)
	present := fmt.Sprintf(`(jsonb_exists(ci."propertyValues",$%d) AND jsonb_typeof(ci."propertyValues"->$%d)<>'null')`, k, k)
	switch op {
	case "empty":
		return missing, nil
	case "notEmpty":
		return present, nil
	}
	v, hasValue := cl["value"]
	if !hasValue || v == nil {
		return "", bad("Operator " + op + " requires a value")
	}
	var expr, cast string
	switch typ {
	case "NUMBER":
		expr = fmt.Sprintf(`(CASE WHEN jsonb_typeof(ci."propertyValues"->$%d)='number' THEN (ci."propertyValues"->>$%d)::numeric END)`, k, k)
		cast = "::numeric"
	case "CHECKBOX":
		expr = fmt.Sprintf(`(CASE WHEN jsonb_typeof(ci."propertyValues"->$%d)='boolean' THEN (ci."propertyValues"->>$%d)::boolean END)`, k, k)
		cast = "::boolean"
	default: // TEXT, URL, SELECT, DATE (canonical fixed-width strings), MULTI_SELECT containment
		expr = fmt.Sprintf(`(CASE WHEN jsonb_typeof(ci."propertyValues"->$%d)='string' THEN ci."propertyValues"->>$%d END)`, k, k)
	}
	// Values bind as text and are cast per type; the formatter keeps numbers
	// in fixed notation so ::numeric never sees exponent forms.
	var vstr string
	switch t := v.(type) {
	case string:
		vstr = t
	case float64:
		vstr = strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		vstr = strconv.FormatBool(t)
	default:
		return "", bad("Unsupported filter value")
	}
	if op == "contains" && typ != "MULTI_SELECT" {
		// Stored filter values stay verbatim; LIKE escaping happens at query
		// build time so user % and _ are matched literally.
		vstr = "%" + likeEscaper.Replace(vstr) + "%"
	}
	*args = append(*args, vstr)
	vn := len(*args)
	switch op {
	case "contains":
		if typ == "MULTI_SELECT" {
			return fmt.Sprintf(`ci."propertyValues"->$%d @> to_jsonb($%d::text)`, k, vn), nil
		}
		return fmt.Sprintf(`%s ILIKE $%d ESCAPE '\'`, expr, vn), nil
	case "eq":
		return fmt.Sprintf(`%s = $%d%s`, expr, vn, cast), nil
	case "ne":
		return fmt.Sprintf(`%s IS DISTINCT FROM $%d%s`, expr, vn, cast), nil
	case "gt", "gte", "lt", "lte":
		sym := map[string]string{"gt": ">", "gte": ">=", "lt": "<", "lte": "<="}[op]
		return fmt.Sprintf(`%s %s $%d%s`, expr, sym, vn, cast), nil
	}
	return "", bad("Unsupported filter operator")
}

// sortPlan describes the single server-side sort key honored by the items
// endpoint in phase 2 (first entry of view.sorts; extra keys are accepted
// and stored but pagination keys on the first — documented in API.md).
type sortPlan struct {
	expr     string // SQL expression, "" for the canonical (position,id) order
	cast     string // cursor parameter cast for expr
	nullable bool
	desc     bool
	// extract pulls the cursor value from a response item ("v" field).
	extract func(item object) *string
}

func cursorEncode(v *string, id string) string {
	raw, _ := json.Marshal(object{"v": v, "id": id})
	return base64.RawURLEncoding.EncodeToString(raw)
}
func jsonbString(v any) *string {
	switch t := v.(type) {
	case string:
		return &t
	case float64:
		s := strconv.FormatFloat(t, 'f', -1, 64)
		return &s
	case bool:
		s := strconv.FormatBool(t)
		return &s
	}
	return nil
}

func buildSortPlan(byID map[string]object, sorts []any, args *[]any) (sortPlan, error) {
	plan := sortPlan{
		expr: `ci.position`, cast: "::bigint", desc: false,
		extract: func(item object) *string { return jsonbString(item["position"]) },
	}
	if len(sorts) == 0 {
		return plan, nil
	}
	first, _ := sorts[0].(object)
	field, dir := str(first, "field"), str(first, "dir")
	plan.desc = dir == "desc"
	switch field {
	case "", "position":
		return plan, nil
	case "addedAt":
		plan.expr, plan.cast = `ci."addedAt"`, "::timestamptz"
		plan.extract = func(item object) *string { return jsonbString(item["addedAt"]) }
		return plan, nil
	case "title":
		plan.expr, plan.cast, plan.nullable = `b.title`, "", true
		plan.extract = func(item object) *string {
			bm, _ := item["bookmark"].(object)
			return jsonbString(bm["title"])
		}
		return plan, nil
	}
	p, ok := byID[field]
	if !ok {
		return plan, bad("Sort references an unknown property")
	}
	if !collectionSortTypes[str(p, "type")] {
		return plan, bad("Multi-select properties cannot be sorted on")
	}
	*args = append(*args, field)
	k := len(*args)
	plan.nullable = true
	switch str(p, "type") {
	case "NUMBER":
		plan.expr, plan.cast = fmt.Sprintf(`(CASE WHEN jsonb_typeof(ci."propertyValues"->$%d)='number' THEN (ci."propertyValues"->>$%d)::numeric END)`, k, k), "::numeric"
	case "CHECKBOX":
		plan.expr, plan.cast = fmt.Sprintf(`(CASE WHEN jsonb_typeof(ci."propertyValues"->$%d)='boolean' THEN (ci."propertyValues"->>$%d)::boolean END)`, k, k), "::boolean"
	default:
		plan.expr = fmt.Sprintf(`(CASE WHEN jsonb_typeof(ci."propertyValues"->$%d)='string' THEN ci."propertyValues"->>$%d END)`, k, k)
	}
	plan.extract = func(item object) *string {
		pv, _ := item["propertyValues"].(object)
		return jsonbString(pv[field])
	}
	return plan, nil
}

// keysetPredicate is the cursor condition matching the ORDER BY below.
// Deterministic tie-break: ci.id ASC on every plan (draft §5 edge case).
func (p sortPlan) keysetPredicate(vArg, idArg int) string {
	v := fmt.Sprintf("$%d%s", vArg, p.cast)
	id := fmt.Sprintf(`ci.id>$%d`, idArg)
	dir := ">"
	if p.desc {
		dir = "<"
	}
	if !p.nullable {
		return fmt.Sprintf(`(%s %s %s OR (%s = %s AND %s))`, p.expr, dir, v, p.expr, v, id)
	}
	if p.desc {
		return fmt.Sprintf(`((%s IS NOT NULL AND ((%s = %s AND %s) OR %s %s %s OR %s IS NULL)) OR (%s IS NULL AND %s IS NULL AND %s))`,
			v, p.expr, v, id, p.expr, dir, v, p.expr, v, p.expr, id)
	}
	return fmt.Sprintf(`((%s IS NOT NULL AND (%s %s %s OR (%s = %s AND %s) OR %s IS NULL)) OR (%s IS NULL AND %s IS NULL AND %s))`,
		v, p.expr, dir, v, p.expr, v, id, p.expr, v, p.expr, id)
}

func (p sortPlan) orderBy() string {
	dir := "ASC"
	if p.desc {
		dir = "DESC"
	}
	if p.nullable {
		return fmt.Sprintf(`%s %s NULLS LAST,ci.id ASC`, p.expr, dir)
	}
	return fmt.Sprintf(`%s %s,ci.id ASC`, p.expr, dir)
}

func HandleCollectionItemsList(w http.ResponseWriter, r *http.Request) {
	cid := r.PathValue("id")
	sp := r.URL.Query()
	limit := 200
	if raw := sp.Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 200 {
			respond(w, 0, nil, bad("Limit must be between 1 and 200"))
			return
		}
		limit = n
	}
	// Ownership is the gate: a foreign collection is a 404, not an empty list.
	if _, e := collectionOwned(r, database(r), cid); e != nil {
		respond(w, 0, nil, e)
		return
	}
	props, e := collectionProperties(r, database(r), cid)
	if e != nil {
		respond(w, 0, nil, e)
		return
	}
	byID := propertyIndex(props)
	// $1 stays a permanently-nil placeholder (BookmarkVisible binds $2), the
	// same port artifact the bookmarks list documents.
	args := []any{nil, user(r), cid}
	where := `c."ownerId"=$2 AND ci."collectionId"=$3 AND ` + db.BookmarkVisible + ` AND $1::text IS NULL`
	var sorts []any
	if viewID := sp.Get("viewId"); viewID != "" {
		view, e2 := db.One(r.Context(), database(r), `SELECT to_jsonb(v) FROM "CollectionView" v WHERE v.id=$1 AND v."collectionId"=$2`, viewID, cid)
		if e2 != nil {
			respond(w, 0, nil, bad("Unknown view for this collection"))
			return
		}
		if filters, ok := view["filters"].(object); ok {
			clauses, _ := filters["clauses"].([]any)
			joiner := " AND "
			if str(filters, "operator") == "OR" {
				joiner = " OR "
			}
			parts := make([]string, 0, len(clauses))
			for _, raw := range clauses {
				cl, _ := raw.(object)
				part, e3 := filterClauseSQL(byID, cl, &args)
				if e3 != nil {
					respond(w, 0, nil, e3)
					return
				}
				parts = append(parts, part)
			}
			if len(parts) > 0 {
				where += " AND (" + strings.Join(parts, joiner) + ")"
			}
		}
		if s, ok := view["sorts"].([]any); ok {
			sorts = s
		}
	}
	plan, e := buildSortPlan(byID, sorts, &args)
	if e != nil {
		respond(w, 0, nil, e)
		return
	}
	if raw := sp.Get("cursor"); raw != "" {
		var cur struct {
			V  *string `json:"v"`
			ID string  `json:"id"`
		}
		data, err := base64.RawURLEncoding.DecodeString(raw)
		if err != nil || json.Unmarshal(data, &cur) != nil || cur.ID == "" {
			respond(w, 0, nil, bad("Invalid cursor"))
			return
		}
		args = append(args, cur.ID)
		idArg := len(args)
		if cur.V != nil {
			args = append(args, *cur.V)
			where += " AND " + plan.keysetPredicate(len(args), idArg)
		} else {
			// NULL sort value: only NULL-valued rows with a greater id follow.
			where += fmt.Sprintf(` AND (%s IS NULL AND ci.id>$%d)`, plan.expr, idArg)
		}
	}
	args = append(args, limit+1)
	v, e := db.Many(r.Context(), database(r), `SELECT to_jsonb(ci)||jsonb_build_object('bookmark',`+db.BookmarkViewerView+`)
		FROM "CollectionItem" ci JOIN "Bookmark" b ON b.id=ci."bookmarkId" JOIN "Collection" c ON c.id=ci."collectionId"
		WHERE `+where+fmt.Sprintf(` ORDER BY %s LIMIT $%d`, plan.orderBy(), len(args)), args...)
	if e == nil && len(v) > limit {
		v = v[:limit]
		last := v[len(v)-1]
		w.Header().Set("X-Next-Cursor", cursorEncode(plan.extract(last), str(last, "id")))
	}
	respond(w, 200, v, e)
}

func HandleCollectionItemsCreate(w http.ResponseWriter, r *http.Request) {
	cid := r.PathValue("id")
	b, ok := read(w, r)
	if !ok {
		return
	}
	bid := str(b, "bookmarkId")
	if bid == "" {
		respond(w, 0, nil, bad("bookmarkId required"))
		return
	}
	var out object
	e := db.Transaction(r.Context(), database(r), func(tx *sql.Tx) error {
		if e := lockUser(r, tx); e != nil {
			return e
		}
		if _, e := collectionOwned(r, tx, cid); e != nil {
			return e
		}
		// Membership must not become an access widening vector: only
		// currently visible bookmarks can be added (decision #4).
		var found string
		err := tx.QueryRowContext(r.Context(), `SELECT b.id FROM "Bookmark" b WHERE b.id=$1 AND `+db.BookmarkVisible, bid, user(r)).Scan(&found)
		if err == sql.ErrNoRows {
			return bad("Bookmark is not accessible")
		}
		if err != nil {
			return err
		}
		props, e := collectionProperties(r, tx, cid)
		if e != nil {
			return e
		}
		values := object{}
		if raw, ok := b["propertyValues"]; ok && raw != nil {
			values, e = applyPropertyValues(object{}, props, raw)
			if e != nil {
				return e
			}
		}
		encoded, e := marshalValues(values)
		if e != nil {
			return e
		}
		var n int
		if e := tx.QueryRowContext(r.Context(), `SELECT COALESCE(max(position),-1)+1 FROM "CollectionItem" WHERE "collectionId"=$1`, cid).Scan(&n); e != nil {
			return e
		}
		itemID := db.NewID()
		if _, e := db.Insert(r.Context(), tx, "CollectionItem", object{
			"id": itemID, "collectionId": cid, "bookmarkId": bid,
			"propertyValues": encoded, "position": n,
		}); e != nil {
			return e
		}
		out, e = collectionItemRow(r, tx, cid, itemID)
		return e
	})
	if e != nil {
		respond(w, 0, nil, e)
		return
	}
	respond(w, 201, out, nil)
}

func HandleCollectionItemsUpdate(w http.ResponseWriter, r *http.Request) {
	cid, itemID := r.PathValue("id"), r.PathValue("itemId")
	b, ok := read(w, r)
	if !ok {
		return
	}
	base, e := patchVersion(b)
	if e != nil {
		respond(w, 0, nil, e)
		return
	}
	var out object
	e = db.Transaction(r.Context(), database(r), func(tx *sql.Tx) error {
		if e := lockUser(r, tx); e != nil {
			return e
		}
		if _, e := collectionOwned(r, tx, cid); e != nil {
			return e
		}
		cur, e := collectionItemRow(r, tx, cid, itemID)
		if e != nil {
			return e
		}
		stored, _ := cur["version"].(float64)
		if int(stored) != base {
			return conflict("Item was modified elsewhere; reload to see the latest version")
		}
		d := object{"version": base + 1, "updatedAt": time.Now().UTC()}
		if raw, ok := b["propertyValues"]; ok {
			current, _ := cur["propertyValues"].(object)
			if current == nil {
				current = object{}
			}
			props, e2 := collectionProperties(r, tx, cid)
			if e2 != nil {
				return e2
			}
			values, e2 := applyPropertyValues(current, props, raw)
			if e2 != nil {
				return e2
			}
			encoded, e2 := marshalValues(values)
			if e2 != nil {
				return e2
			}
			d["propertyValues"] = encoded
		}
		if pos, ok2, e2 := intField(b, "position"); e2 != nil {
			return e2
		} else if ok2 {
			d["position"] = pos
		}
		if _, e := db.Update(r.Context(), tx, "CollectionItem", itemID, "collectionId", cid, d); e != nil {
			return e
		}
		var e2 error
		out, e2 = collectionItemRow(r, tx, cid, itemID)
		return e2
	})
	if e != nil {
		respond(w, 0, nil, e)
		return
	}
	respond(w, 200, out, nil)
}

func HandleCollectionItemsDelete(w http.ResponseWriter, r *http.Request) {
	cid, itemID := r.PathValue("id"), r.PathValue("itemId")
	res, e := database(r).ExecContext(r.Context(), `DELETE FROM "CollectionItem" ci WHERE ci.id=$1 AND ci."collectionId"=$3
		AND EXISTS(SELECT 1 FROM "Collection" c WHERE c.id=ci."collectionId" AND c."ownerId"=$2)
		AND EXISTS(SELECT 1 FROM "Bookmark" b WHERE b.id=ci."bookmarkId" AND `+db.BookmarkVisible+`)`, itemID, user(r), cid)
	if e == nil {
		if n, _ := res.RowsAffected(); n == 0 {
			e = db.ErrNotFound
		}
	}
	// The bookmark itself is untouched — membership removal never deletes
	// records (N-01).
	respond(w, 200, OK{true}, e)
}

func HandleCollectionItemsReorder(w http.ResponseWriter, r *http.Request) {
	cid := r.PathValue("id")
	b, ok := read(w, r)
	if !ok {
		return
	}
	order, e := ids(b, "order")
	if e != nil {
		respond(w, 0, nil, e)
		return
	}
	e = db.Transaction(r.Context(), database(r), func(tx *sql.Tx) error {
		if e := lockUser(r, tx); e != nil {
			return e
		}
		if _, e := collectionOwned(r, tx, cid); e != nil {
			return e
		}
		// Count only items whose bookmark is still visible (review COR-2):
		// a share revocation hides the membership row from the client, so
		// the client can never include it in the order. Counting it here
		// made every reorder permanently fail once any bookmark became
		// invisible. Hidden memberships keep their rows (re-sharing restores
		// them) and are placed after the supplied order below.
		var count int
		// BookmarkVisible binds the user as $2; collectionId is $1.
		if e := tx.QueryRowContext(r.Context(), `SELECT count(*) FROM "CollectionItem" ci JOIN "Bookmark" b ON b.id=ci."bookmarkId" WHERE ci."collectionId"=$1 AND `+db.BookmarkVisible, cid, user(r)).Scan(&count); e != nil {
			return e
		}
		if count != len(order) {
			return bad("Order must contain every item exactly once")
		}
		res, e := tx.ExecContext(r.Context(), `UPDATE "CollectionItem" AS ci SET position=t.ord-1,"updatedAt"=now() FROM unnest($1::text[]) WITH ORDINALITY AS t(iid,ord) WHERE ci.id=t.iid AND ci."collectionId"=$2`, order, cid)
		if e != nil {
			return e
		}
		if n, _ := res.RowsAffected(); int(n) != len(order) {
			return bad("Invalid order")
		}
		// Hidden items (revoked bookmarks) sort after everything the client
		// ordered, deterministically, so they do not collide with supplied
		// positions. BookmarkVisible binds the user as $2; exclusion array is $3.
		if _, e := tx.ExecContext(r.Context(), `UPDATE "CollectionItem" AS ci SET position=sub.pos,"updatedAt"=now() FROM (
			SELECT id, `+strconv.Itoa(len(order))+` + ROW_NUMBER() OVER (ORDER BY id) AS pos
			FROM "CollectionItem"
			WHERE "collectionId"=$1 AND id <> ALL($3::text[])
			AND NOT EXISTS(SELECT 1 FROM "Bookmark" b WHERE b.id="CollectionItem"."bookmarkId" AND `+db.BookmarkVisible+`)
		) sub WHERE ci.id=sub.id`, cid, user(r), order); e != nil {
			return e
		}
		return nil
	})
	respond(w, 200, OK{true}, e)
}
