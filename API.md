# API reference

Go API: `http://localhost:8080`. The web UI uses the same paths through its
server-side proxy at `http://localhost:3000` and its NextAuth session.

Successful responses are JSON unless marked Markdown. Errors are
`{"error":"message"}`. Data endpoints require authentication: **401** without
a valid session, **404** for another user's private record, **403** for
non-admin administration, **400** for invalid input, **409** for duplicates.

## Authentication

| Method | Path | Returns |
|---|---|---|
| GET | `/healthz` | `{"status":"ok"}` (process liveness) |
| GET | `/readyz` | `{"status":"ready"}` after database ping; 503 when unavailable |
| POST | `/api/signup` | 201 `{ok:true,userId}`; body `{email,password,name?}` |
| POST | `/api/auth/login` | `{ok:true,user:{id,name,email,image,role}}` and signed HttpOnly cookie |
| GET | `/api/auth/session` | `{user}` or `null` for no valid cookie |
| POST | `/api/auth/signout` | `{ok:true}` and expired cookie |
| GET | `/api/me` | `{user,effectiveUser,isAdmin,viewingAs}`; real and effective identities |
| GET | `/api/auth/oidc/status` | `{enabled,issuerSet,clientIdSet,clientSecretSet,issuer}`; never includes secrets |
| GET | `/api/auth/oidc/login` | **501**, client-owned Authelia integration stub |
| GET, POST | `/api/auth/callback/authelia` | **501**, client-owned callback stub |

The browser's NextAuth `/api/auth/*` endpoints remain on the Next server.
Direct Go API consumers use the `computer_session` cookie from login.

## Bookmarks

A bookmark includes camelCase scalar fields plus `tags:[{tag}]`, `contact`,
`sharedWith:[{userId,user}]`, `owner`. Detail responses also include
`pageLinks:[{pageId,page}]`, limited to the effective user's pages.

| Method | Path | Returns / behavior |
|---|---|---|
| GET | `/api/bookmarks` | Array, newest-updated first, maximum 200; query `search,tagId,contactId,kanban,date,shared` |
| POST | `/api/bookmarks` | 201 bookmark; body `{url,title?,description?,notes?,dueDate?,alertAt?,kanbanStatus?,contactId?,tagIds?,isPublic?,imapWatchEnabled?,imapQuery?}` |
| GET | `/api/bookmarks/{id}` | Bookmark if owned, shared, or public |
| PUT | `/api/bookmarks/{id}` | Updated bookmark, owner only; fields above plus `kanbanColumnId`; omitted fields stay unchanged; nullable fields can be cleared |
| DELETE | `/api/bookmarks/{id}` | `{ok:true}`, owner only |
| POST | `/api/bookmarks/{id}/share` | `{ok:true}`; body `{userIds?,isPublic?}`; adds shares |
| DELETE | `/api/bookmarks/{id}/share/{userId}` | `{ok:true}`; revokes that direct share |
| GET | `/api/bookmarks/{id}/markdown` | Markdown attachment, owned/shared only |
| GET | `/api/preview?url=...` | `{title,description,ogTitle,ogDescription,ogImage,favicon}`; nullable on fetch failure |
| GET | `/api/public/bookmarks/{id}` | Public bookmark projection without login; private records return 404 |

Public HTML pages remain at `/share/bookmark/{id}` on the Next.js server.
Updating `alertAt` resets `alertSent`; replacing `tagIds` is transactional.
Tags, columns and contacts are checked for accessibility/ownership.

## Tags, kanban and pages

| Method | Path | Returns / behavior |
|---|---|---|
| GET | `/api/tags` | Owned/shared tags with `_count.bookmarks` and `sharedWith` |
| POST | `/api/tags` | 201 tag; body `{name,color?}` |
| PUT | `/api/tags/{id}` | Updated tag; body `{name?,color?,shareUserIds?}`, owner only |
| DELETE | `/api/tags/{id}` | `{ok:true}` |
| DELETE | `/api/tags/{id}/share/{userId}` | `{ok:true}`, revokes tag share |
| GET | `/api/kanban/columns` | Ordered columns; creates four defaults if none exist |
| POST | `/api/kanban/columns` | 201 column; body `{label,color?}` |
| PUT | `/api/kanban/columns` | Ordered columns; body `{order:[id,...]}`, complete permutation required |
| PATCH | `/api/kanban/columns/{id}` | Updated column; body `{label?,color?}` |
| DELETE | `/api/kanban/columns/{id}` | `{ok:true,movedTo,moved}`; moves cards, never deletes bookmarks; refuses last column |
| GET | `/api/pages?search=...` | Pages, pinned first, with `_count.bookmarks` |
| POST | `/api/pages` | 201 page; body `{title?,content?}` |
| PUT | `/api/pages` | `{ok:true}`; complete `{order:[id,...]}` |
| GET | `/api/pages/{id}` | Page plus ordered `bookmarks:[{bookmark}]` |
| PATCH | `/api/pages/{id}` | Updated page; `{title?,content?,pinned?,icon?,addBookmarkId?,removeBookmarkId?}` |
| DELETE | `/api/pages/{id}` | `{ok:true}`; keeps bookmarks |
| GET | `/api/pages/{id}/markdown` | Markdown attachment |

## Signals and integrations

| Method | Path | Returns / behavior |
|---|---|---|
| GET | `/api/bookmarks/{id}/signals` | `{sources,signals,types}`, newest 50 events |
| POST | `/api/bookmarks/{id}/signals` | 201 watcher for `{type:"email",config:{from?,subject?}}`; `{checked,created}` for `{action:"check"}` |
| PATCH | `/api/signal-sources/{id}` | Updated watcher for `{enabled:boolean}` |
| DELETE | `/api/signal-sources/{id}` | `{ok:true}` |
| GET | `/api/alerts/check` | Undismissed bookmarks due within 15 minutes, soonest first |
| DELETE | `/api/alerts/{bookmarkId}` | `{ok:true}`; sets `alertSent` |
| GET | `/api/contacts` | Synced contacts with `_count.bookmarks` |
| POST | `/api/contacts/sync` | `{synced}`; `{nextcloudUrl,username,password,cardDavPath?}` |
| GET | `/api/settings/imap` | Configuration with masked password, or `null` |
| POST | `/api/settings/imap` | `{ok:true}`; `{host,port?,tls?,username,password,folder?}`; empty/masked password preserves an existing password |
| POST | `/api/imap/test` | `{ok:true}` after real connection/login; same mailbox fields |
| GET | `/api/imap/check` | `{matches:[{bookmarkId,title,from,externalId,occurredAt}]}` for legacy watchers |
| GET | `/api/shared-dates` | Created/received dates with `creator` and `recipient` |
| POST | `/api/shared-dates` | 201 array of created shares for `{date,note?,recipientIds:[...]}` |
| DELETE | `/api/shared-dates/{id}` | `{ok:true}`, creator only |
| GET | `/api/users/search?q=...` | Safe user projections; minimum two characters, maximum ten results |

Email signals are deduplicated by source and Message-ID. Failed mailbox checks
are recorded on `lastStatus` rather than reported as successful email matches.
Sharing a tag or date exposes associated bookmarks read-only; direct-share
revocation does not remove access independently granted by a tag/date share.

## Administration

Always uses the real user, even while viewing another account.

| Method | Path | Returns / behavior |
|---|---|---|
| GET | `/api/admin/users` | Safe user projections with bookmark/page/tag counts; excludes `@example.com` test users |
| PATCH | `/api/admin/users` | `{ok:true}` for `{userId,role:"USER"\|"ADMIN"}`; refuses self-demotion |
| POST | `/api/admin/view-as` | `{ok:true}`; `{userId}` sets an eight-hour HttpOnly cookie, `{userId:null}` clears it |

Route registration: `internal/http/server.go`. Only the client-owned OIDC
login/callback remain stubs; application data routes use PostgreSQL.
