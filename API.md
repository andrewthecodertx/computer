# API endpoints

computer — Go backend. All routes live on `http://localhost:8080`.

> **Status:** every handler is a stub. They are wired and respond with hardcoded
> JSON; none touch the database yet. Real behavior is the `// TODO:` in each
> handler under `internal/handlers/`. Errors are `{error: string}`.

## Health

| Route | Method | Stub returns |
|---|---|---|
| `/healthz` | GET | `{"status":"ok"}` |

## Auth and account (4.1)

| Route | Method | Stub returns |
|---|---|---|
| `/api/signup` | POST | `{"ok":true}` |
| `/api/auth/login` | POST | `{"ok":true}` |
| `/api/auth/*` | GET/POST | (routed, not implemented — csrf, callbacks, session, signout) |
| `/api/me` | GET | `{}` |

## Admin (4.2) — uses the real user; 403 unless ADMIN

| Route | Method | Stub returns |
|---|---|---|
| `/api/admin/users` | GET | `[]` |
| `/api/admin/users` | PATCH | `{"ok":true}` |
| `/api/admin/view-as` | POST | `{"ok":true}` |

## Bookmarks (4.3)

| Route | Method | Stub returns |
|---|---|---|
| `/api/bookmarks` | GET | `[]` |
| `/api/bookmarks` | POST | `{"ok":true}` |
| `/api/bookmarks/{id}` | GET | `{}` |
| `/api/bookmarks/{id}` | PUT | `{"ok":true}` |
| `/api/bookmarks/{id}` | DELETE | `{"ok":true}` |
| `/api/bookmarks/{id}/share` | POST | `{"ok":true}` |
| `/api/bookmarks/{id}/markdown` | GET | (.md download stub) |
| `/api/preview` | GET | `{}` |

## Tags (4.4)

| Route | Method | Stub returns |
|---|---|---|
| `/api/tags` | GET | `[]` |
| `/api/tags` | POST | `{"ok":true}` |
| `/api/tags/{id}` | PUT | `{"ok":true}` |
| `/api/tags/{id}` | DELETE | `{"ok":true}` |

## Kanban columns (4.5)

| Route | Method | Stub returns |
|---|---|---|
| `/api/kanban/columns` | GET | `[]` |
| `/api/kanban/columns` | POST | `{"ok":true}` |
| `/api/kanban/columns` | PUT | `[]` |
| `/api/kanban/columns/{id}` | PATCH | `{"ok":true}` |
| `/api/kanban/columns/{id}` | DELETE | `{}` |

## Pages (4.6)

| Route | Method | Stub returns |
|---|---|---|
| `/api/pages` | GET | `[]` |
| `/api/pages` | POST | `{"ok":true}` |
| `/api/pages` | PUT | `{"ok":true}` |
| `/api/pages/{id}` | GET | `{}` |
| `/api/pages/{id}` | PATCH | `{"ok":true}` |
| `/api/pages/{id}` | DELETE | `{"ok":true}` |
| `/api/pages/{id}/markdown` | GET | (.md download stub) |

## Signals (4.7)

| Route | Method | Stub returns |
|---|---|---|
| `/api/bookmarks/{id}/signals` | GET | `{types: []}` |
| `/api/bookmarks/{id}/signals` | POST | `{"ok":true}` |
| `/api/signal-sources/{id}` | PATCH | `{"ok":true}` |
| `/api/signal-sources/{id}` | DELETE | `{"ok":true}` |

## Alerts, contacts, email, dates, users (4.8)

| Route | Method | Stub returns |
|---|---|---|
| `/api/alerts/check` | GET | `[]` |
| `/api/alerts/{bookmarkId}` | DELETE | `{"ok":true}` |
| `/api/contacts` | GET | `[]` |
| `/api/contacts/sync` | POST | `{"synced":0}` |
| `/api/settings/imap` | GET | `{}` |
| `/api/settings/imap` | POST | `{"ok":true}` |
| `/api/imap/test` | POST | `{"ok":true}` |
| `/api/imap/check` | GET | `{"matches":[]}` |
| `/api/shared-dates` | GET | `[]` |
| `/api/shared-dates` | POST | `{"ok":true}` |
| `/api/users/search` | GET | `[]` |

## Public page (4.9) — no login

| Route | Method | Stub returns |
|---|---|---|
| `/share/bookmark/{id}` | GET | `{}` |

## Notes

- Every data endpoint resolves the effective user, returns 401 if none,
  checks ownership, returns 404 if not theirs, then acts (porting guide 5.1).
- The Next.js front-end calls these APIs; it is not yet wired to this backend.
- Route registration lives in `internal/http/server.go`.