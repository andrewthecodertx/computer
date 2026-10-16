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
| POST | `/api/signup` | 201 `{ok:true}` — the same uniform body for fresh, duplicate and operator-reserved addresses (review CR-03: no existence oracle); body `{email,password,name?}` |
| POST | `/api/auth/login` | `{ok:true,user:{id,name,email,image,role}}` and signed HttpOnly cookie |
| GET | `/api/auth/session` | `{user}` or `null` for no valid cookie |
| POST | `/api/auth/signout` | `{ok:true}` and expired cookie |
| GET | `/api/me` | `{user,effectiveUser,isAdmin,viewingAs}`; real and effective identities |
| GET | `/api/auth/oidc/status` | `{enabled,issuerSet,clientIdSet,clientSecretSet,issuer}`; never includes secrets |
| GET | `/api/auth/oidc/login` | **501**, client-owned Authelia integration stub |
| GET, POST | `/api/auth/callback/authelia` | **501**, client-owned callback stub |

The browser's NextAuth `/api/auth/*` endpoints remain on the Next server.
Direct Go API consumers use the `computer_session` cookie from login.
Go login accepts JSON and rejects foreign browser origins. The custom Next.js
login endpoint also requires the configured same-origin `Origin` header.
Login and signup are throttled per client IP (429 with `Retry-After`), plus a
consecutive-failure lockout per IP+email on login; loopback clients are
exempt. Browser traffic arrives through the trusted Next proxy, which
forwards the observed client IP in `X-Computer-Client-Ip` signed with
HMAC-SHA256 under the shared secret (`X-Computer-Client-Sig`); Go buckets on
that identity only when the signature verifies and falls back to the direct
peer otherwise, so proxied browsers neither share one bucket nor forge
another identity.
`ADMIN_EMAILS` reserves administrator addresses for operator provisioning and
disables public first-admin bootstrap while set. Automatic promotion of an
existing allowlisted user requires `emailVerified`; DB administrator roles
remain authoritative. First-signup administrator promotion is one-shot: once
bootstrap completion is recorded (AppSetting `adminBootstrap`), the absence
of a qualifying administrator — e.g. after demoting the last one — never
re-opens promotion through signup. Recovery is operator-driven
(`ADMIN_EMAILS` + `make seed`, or promotion by an existing admin).

## Bookmarks

Bookmark responses are permission-specific. Owners receive camelCase scalar
fields plus `tags:[{tag}]`, `contact`, `sharedWith:[{userId,user}]` and
`owner` (with email). Authenticated shared readers receive a narrowed
allowlist — URL, title/description, notes, preview metadata, `dueDate`,
`isPublic`, `kanbanStatus`, timestamps, `tags` and `owner:{id,name}` — never
the owner's contact record, the recipient list, alerts, mailbox watches or
board placement. Detail responses also include `pageLinks:[{pageId,page}]`,
limited to the effective user's pages.

| Method | Path | Returns / behavior |
|---|---|---|
| GET | `/api/bookmarks` | Array, newest-updated then ID descending; query `search,tagId,contactId,kanban,date,shared,limit,cursor`; default/max page size 200 |
| POST | `/api/bookmarks` | 201 bookmark; body `{url,title?,description?,notes?,dueDate?,alertAt?,kanbanStatus?,contactId?,tagIds?,isPublic?,imapWatchEnabled?,imapQuery?}` |
| GET | `/api/bookmarks/{id}` | Owner-full or shared-reader projection if visible; public-only access uses the redacted public endpoint |
| PUT | `/api/bookmarks/{id}` | Updated bookmark, owner only; fields above plus `kanbanColumnId`; omitted fields stay unchanged; nullable fields can be cleared. **Requires `version`** (the revision the client edited from): 400 when missing, 409 when stale — success bumps `version` (review CR-04) |
| DELETE | `/api/bookmarks/{id}` | `{ok:true}`, owner only |
| POST | `/api/bookmarks/{id}/share` | `{ok:true}`; body `{userIds?,isPublic?}`; adds shares |
| DELETE | `/api/bookmarks/{id}/share/{userId}` | `{ok:true}`; revokes that direct share |
| GET | `/api/bookmarks/{id}/markdown` | Markdown attachment, owned/shared only |
| GET | `/api/preview?url=...` | `{title,description,ogTitle,ogDescription,ogImage,favicon}`; nullable on fetch failure |
| GET | `/api/public/bookmarks/{id}` | Public bookmark projection without login; private records return 404 |

Public HTML pages remain at `/share/bookmark/{id}` on the Next.js server.
For pagination, read the `X-Next-Cursor` response header and pass that opaque
value as `cursor` with the same filters. An absent header marks the final page.
`limit` must be between 1 and 200. The web proxy forwards the cursor header.
Public responses allowlist URL/preview/notes/date/tag fields only; owner
identity, contacts, share recipients, mailbox queries, and alert metadata are
excluded.
Updating `alertAt` resets `alertSent`; replacing `tagIds` is transactional.
Tags, columns and contacts are checked for accessibility/ownership.
The `version` contract covers the owner-driven PUT (fields and tag
replacement): kanban column deletion moves bookmarks server-side and bumps
their `version` too, so a stale board save 409s instead of moving a card
back into a deleted column. Sharing (`POST /share`) and the server-driven
`alertSent` flag are deliberately outside the contract (review CR-04).

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
| PATCH | `/api/pages/{id}` | Updated page; `{title?,content?,pinned?,icon?,version?,addBookmarkId?,removeBookmarkId?}`. `version` (the base revision from GET) is REQUIRED with `content`: 400 when missing, 409 when stale. Metadata-only patches stay version-free and do not bump the revision |
| DELETE | `/api/pages/{id}` | `{ok:true}`; keeps bookmarks |
| GET | `/api/pages/{id}/markdown` | Markdown attachment |

## Collections

Contextual workflows over existing bookmarks (phase 2 of the Notion-like
upgrades). Collections are owner-scoped; membership references bookmarks
without owning them — removing an item or a whole collection never touches
the bookmark. Membership never broadens access: items whose bookmark the
caller cannot see are omitted from lists/counts and behave as not-found for
mutations; re-sharing restores the membership. Every mutating PATCH requires
the expected `version` (400 missing, 409 stale).

| Method | Path | Returns / behavior |
|---|---|---|
| GET | `/api/collections` | Owned collections with `_count.{items,properties,views}`; item counts include only bookmarks the caller can see |
| POST | `/api/collections` | 201 detail `{...collection,properties:[],views:[default Table view]}`; body `{name,description?,icon?}` |
| GET | `/api/collections/{id}` | Collection plus ordered `properties`, `views` and authorized `_count.{items,properties,views}` |
| PATCH | `/api/collections/{id}` | Updated collection; `{version,name?,description?,icon?}`, bumps `version` |
| DELETE | `/api/collections/{id}` | `{ok:true}`; memberships/properties/views cascade, bookmarks kept |
| POST | `/api/collections/{id}/properties` | 201 property; `{name,type,config?}`. Types: `TEXT,NUMBER,SELECT,MULTI_SELECT,DATE,CHECKBOX,URL` (`RELATION` reserved, rejected). SELECT config `{options:[{label,color?}]}` — the server assigns stable option ids; DATE config `{mode:"date"\|"datetime"}`. Duplicate name → 409 |
| PATCH | `/api/collections/{id}/properties/{propertyId}` | Updated property; `{version,name?,position?,config?}`. Type and date-mode changes are rejected (400) — conversion must never destroy values. Option updates send the full list: existing ids rename/recolor (identity preserved), omitted ids are removed but refused (400 with the using-item count) while any item still carries them |
| DELETE | `/api/collections/{id}/properties/{propertyId}` | `{ok:true}`; strips the property's values from all items and its clauses/sorts/grouping from saved views |
| GET | `/api/collections/{id}/items?viewId=&cursor=&limit=` | Items `{...item,bookmark}` (bookmark uses the permission-specific projection), visible bookmarks only, ≤`limit` (default/max 200) with `X-Next-Cursor`. With `viewId`: the view's filters (AND/OR over typed operator allowlists) and its FIRST sort key run server-side; ordering always tie-breaks on item id. Without `viewId`: canonical manual order (`position,id`) |
| POST | `/api/collections/{id}/items` | 201 item; `{bookmarkId,propertyValues?}`. 400 if the bookmark is not currently accessible; duplicate membership → 409. Values are validated per property type; DATE values store canonically (`YYYY-MM-DD` or fixed-width UTC RFC3339) so comparisons never shift across timezones |
| PUT | `/api/collections/{id}/items` | `{ok:true}`; complete `{order:[itemId,...]}` permutation |
| PATCH | `/api/collections/{id}/items/{itemId}` | Updated item; `{version,propertyValues?,position?}`. `propertyValues` merges: omitted keys untouched, `null` removes a value, unknown property ids 400 |
| DELETE | `/api/collections/{id}/items/{itemId}` | `{ok:true}`; membership only, bookmark untouched |
| POST | `/api/collections/{id}/views` | 201 view; `{name,type?,filters?,sorts?,groupByPropertyId?,displaySettings?}`, types `TABLE,BOARD,CALENDAR,LIST` (default TABLE) |
| PATCH | `/api/collections/{id}/views/{viewId}` | Updated view; `{version,name?,type?,position?,filters?,sorts?,groupByPropertyId?,displaySettings?}` |
| DELETE | `/api/collections/{id}/views/{viewId}` | `{ok:true}` |

Filter shape: `{operator:"AND"|"OR",clauses:[{propertyId,op,value?}]}` (max 20
clauses; no nested groups). Operator allowlists per type: TEXT/URL
`eq,ne,contains,empty,notEmpty`; NUMBER/DATE `eq,ne,gt,gte,lt,lte,empty,
notEmpty`; SELECT/CHECKBOX `eq,ne,empty,notEmpty`; MULTI_SELECT `contains,
empty,notEmpty`. Sorts: max 3 `{field,dir}` where field is `position`,
`addedAt`, `title` or a sortable property id (not multi-select); only the
first drives server-side ordering/pagination in this phase. `ne` matches
missing values (`IS DISTINCT FROM`). Property ids and values are bind
parameters end to end — filter JSON never reaches SQL text. Collection
sharing does not exist yet; views/counts can only expose what the owner can
already see.

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
