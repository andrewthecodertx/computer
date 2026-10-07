# computer: porting guide

This guide lists every data object, server function, HTTP endpoint and client behavior in **computer**, so it can be rebuilt in another language or framework. It describes the code as it is today (October 2026), including the parts that are stubs or unfinished.

The guide uses neutral terms wherever possible. "Session user" means the authenticated person; "effective user" means the account a request acts on (these differ only while an admin uses *View as*).

---

## 1. Architecture at a glance

| Layer | Current implementation | What a port needs |
|---|---|---|
| Storage | PostgreSQL (database `linkos`), accessed through an ORM | Any relational DB; schema in section 2 |
| Auth | Email + password (bcrypt, cost 12) and optional OpenID Connect (Authelia); stateless signed JWT session cookie | Password hashing, OIDC client, signed session cookie |
| Server API | JSON over HTTP, one handler per route (section 4) | Any HTTP framework |
| Client | Single-page app with sidebar layout; polls alerts every 5 minutes | Any UI stack (section 6) |
| Outbound calls | OpenGraph scraping, CardDAV (Nextcloud), raw TCP/TLS to IMAP | HTTP client, XML parser, TLS sockets |

Every data endpoint follows the same pattern: resolve the effective user, return **401** if none, check ownership with a filtered lookup, return **404** if the record is not theirs, then act.

---

## 2. Data objects (database schema)

IDs are strings (currently CUIDs; any unique string such as UUID works). `createdAt` and `updatedAt` are timestamps set automatically. "Cascade" means deleting the parent deletes this row; "SetNull" means the foreign key is cleared.

### 2.1 Enum `KanbanStatus` (legacy)
`INBOX`, `TODO`, `IN_PROGRESS`, `DONE`, `ARCHIVED`. Kept for older rows. New code uses `KanbanColumn` (2.11); `resolveColumnId` (3.3) maps between them.

### 2.2 `User`
| Field | Type | Notes |
|---|---|---|
| id | string PK | |
| name | string? | |
| email | string? unique | Login identifier |
| emailVerified | timestamp? | Used by the OIDC adapter |
| image | string? | Avatar URL |
| password | string? | bcrypt hash; null for OIDC-only users |
| role | string, default `USER` | `USER` or `ADMIN` |
| createdAt, updatedAt | timestamp | |

### 2.3 `Account`, `Session`, `VerificationToken` (auth library tables)
`Account` links a user to an external login: `userId`, `type`, `provider`, `providerAccountId` (unique together), plus token fields `refresh_token`, `access_token`, `expires_at`, `token_type`, `scope`, `id_token`, `session_state`. `Session` (`sessionToken` unique, `userId`, `expires`) and `VerificationToken` (`identifier`, `token`, `expires`) exist for the auth library but are **unused**, because sessions are JWT-based. A port only needs `Account` if it keeps OIDC.

### 2.4 `Bookmark` (the core object: one saved URL)
| Field | Type | Notes |
|---|---|---|
| id | string PK | |
| url | string | Required |
| title, description | string? | Defaults to the OpenGraph title/description when saved |
| favicon, ogImage, ogTitle, ogDescription | string? | Filled by the preview scraper (3.6) |
| notes | text? | Markdown |
| dueDate | timestamp? | Places it on the calendar |
| alertAt | timestamp? | Browser alert time |
| alertSent | bool, default false | Reset to false whenever `alertAt` changes |
| kanbanStatus | KanbanStatus, default INBOX | Legacy |
| kanbanColumnId | FK → KanbanColumn? | SetNull |
| imapWatchEnabled | bool | Legacy email watch flag |
| imapQuery | string? | Legacy email search text |
| isPublic | bool, default false | Enables the public share page |
| ownerId | FK → User | Cascade |
| contactId | FK → Contact? | SetNull |

Indexes: ownerId, contactId, kanbanStatus, kanbanColumnId, dueDate, alertAt.

### 2.5 `Tag`
`id`, `name`, `color` (hex, default `#6366f1`), `ownerId` → User (Cascade). Unique on (`name`, `ownerId`).

### 2.6 `BookmarkTag` (join)
Composite PK (`bookmarkId`, `tagId`); both Cascade.

### 2.7 `BookmarkShare` (bookmark visible to another user)
Composite PK (`bookmarkId`, `userId`), `createdAt`; both Cascade.

### 2.8 `TagShare` (tag visible to another user)
Composite PK (`tagId`, `userId`), `createdAt`; both Cascade.

### 2.9 `Contact` (synced from Nextcloud CardDAV)
`id`, `uid` (vCard UID), `displayName`, `email?`, `phone?`, `cardDavUrl?`, `userId` → User (Cascade). Unique on (`uid`, `userId`).

### 2.10 `ImapConfig` (one per user)
`id`, `host`, `port` (default 993), `tls` (default true), `username`, `password` (**encrypted**, see 3.5), `folder` (default `INBOX`), `userId` unique → User (Cascade).

### 2.11 `KanbanColumn` (user-editable board column)
`id`, `label`, `color` (default `#64748b`), `position` int, `key?` (`INBOX`/`TODO`/`IN_PROGRESS`/`DONE` for the default set, null for custom columns), `userId` → User (Cascade). Index (`userId`, `position`).

### 2.12 `Page` (Markdown workspace)
`id`, `title`, `content` (text, default empty), `icon?`, `pinned` bool, `position` int, `ownerId` → User (Cascade). Index (`ownerId`, `position`).

### 2.13 `PageBookmark` (link a bookmark onto a page)
Composite PK (`pageId`, `bookmarkId`), `position` int, `createdAt`; both Cascade.

### 2.14 `SignalSource` (a watcher attached to a bookmark)
`id`, `type` (e.g. `email`), `config` (JSON, default `{}`), `enabled` (default true), `lastCheckedAt?`, `lastStatus?`, `bookmarkId` → Bookmark (Cascade), `ownerId` → User (Cascade).

### 2.15 `Signal` (one outside event about a bookmark)
`id`, `sourceType`, `title`, `summary?`, `url?`, `externalId?`, `occurredAt` (default now), `read` (default false), `sourceId?` → SignalSource (SetNull), `bookmarkId` → Bookmark (Cascade), `ownerId` → User (Cascade). **Unique on (`sourceId`, `externalId`)**, which prevents duplicates.

### 2.16 `SharedDate`
`id`, `date` (date only), `note?`, `creatorId` → User, `recipientId` → User (both Cascade). One row per recipient.

---

## 3. Server-side modules and functions

### 3.1 Authentication (`auth.ts`)
| Item | Behavior |
|---|---|
| Session strategy | Signed JWT cookie. `jwt` callback stores `token.id = user.id` and `token.provider`; `session` callback copies `token.id` to `session.user.id` |
| Login page | `/login` |
| `credentials` provider → `authorize(email, password)` | Find user by email; return null if missing, no password, or bcrypt compare fails; otherwise return `{id, name, email, image}` |
| `authelia` provider (OIDC) | Registered **only** if `getOidcStatus().enabled`. Scope `openid profile email`. Profile mapping: `id=sub`, `name = name ?? preferred_username ?? sub`, `email`, `image = picture`. Links to an existing account with the same email |
| Exports | `auth()` (current session), `signIn`, `signOut`, HTTP handlers for `/api/auth/*` |

Secrets: `NEXTAUTH_SECRET` / `AUTH_SECRET` (same value) sign the cookie.

### 3.2 Authorization (`lib/auth-guard.ts`)
| Symbol | Signature | Behavior |
|---|---|---|
| `VIEW_AS_COOKIE` | const `"computer_view_as"` | Cookie holding the target user ID |
| `AuthUser` | `{id, name?, email?, image?, role: USER\|ADMIN, actingAdminId?}` | |
| `adminEmails()` | `→ string[]` | Splits env `ADMIN_EMAILS` on commas, trims, lowercases |
| `resolveRole(user)` | `→ USER\|ADMIN` | ADMIN if DB role is ADMIN; otherwise if email is in `adminEmails()`, **writes** role ADMIN to the DB and returns ADMIN |
| `getRealUser()` | `→ AuthUser\|null` | Session user, ignoring View as. Used by admin endpoints and `/api/me` |
| `getAuthUser()` | `→ AuthUser\|null` | Effective user. If the real user is ADMIN and the cookie holds another existing user's ID, returns that user with `actingAdminId` set; otherwise returns the real user. **All data endpoints use this** |

### 3.3 Kanban (`lib/kanban.ts`)
| Symbol | Behavior |
|---|---|
| `DEFAULT_COLUMNS` | `INBOX` Inbox `#64748b`, `TODO` To Do `#3b82f6`, `IN_PROGRESS` In Progress `#f59e0b`, `DONE` Done `#22c55e` |
| `getColumns(userId)` | Columns ordered by position; if the user has none, creates the four defaults (positions 0–3) first |
| `resolveColumnId(bookmark, columns)` | Pure function: if `bookmark.kanbanColumnId` exists among `columns`, return it; else the column whose `key == bookmark.kanbanStatus`; else the first column; else null |

### 3.4 Signals (`lib/signals.ts`)
| Symbol | Shape / behavior |
|---|---|
| `SignalInput` | `{externalId, title, summary?, url?, occurredAt?}` |
| `ConfigField` | `{key, label, placeholder?}` |
| `SignalSourceType` | `{type, label, description, stub: bool, configFields: ConfigField[], check(ctx{userId, config}) → {status, signals: SignalInput[]}}` |
| `emailSource` | type `email`, label "Email (IMAP)", **stub = true**, fields `from` and `subject`. `check`: if the user has no ImapConfig, status "No IMAP mailbox configured"; otherwise status "Stub: mailbox search not implemented yet". Always returns no signals |
| `SIGNAL_SOURCES` | Registry map `type → SignalSourceType`. Adding a source type = adding one entry |
| `publicSourceTypes()` | Registry entries without the `check` function (safe to send to the client) |
| `runSignalChecks(userId, bookmarkId?)` | For each enabled SignalSource of the user (optionally one bookmark): run `check`, insert each signal unless (`sourceId`, `externalId`) already exists, catch errors as status `Error: …`, then save `lastCheckedAt = now` and `lastStatus`. Returns `{checked, created}` |

When porting, the real email check should: decrypt the IMAP password, connect, search `folder` for messages matching `from`/`subject`, and return one `SignalInput` per message with `externalId` = Message-ID.

### 3.5 Encryption (`lib/crypto.ts`)
| Function | Behavior |
|---|---|
| `getKey()` | SHA-256 of `NEXTAUTH_SECRET` (or `AUTH_SECRET`), giving a 32-byte key. The code also has a hard-coded fallback string if neither is set; a port should **fail instead** |
| `encrypt(text)` | AES-256-GCM, random 12-byte IV, 16-byte auth tag. Output = base64(IV ‖ tag ‖ ciphertext) |
| `decrypt(data)` | Reverse of the above. Keep the exact byte layout so existing stored IMAP passwords stay readable |

Changing the secret makes saved IMAP passwords unreadable.

### 3.6 Link preview (repeated in `/api/preview`, bookmark create and bookmark update)
`fetchPreview(url)` uses an 8-second timeout and User-Agent `Mozilla/5.0 (compatible; computer/1.0)`. It reads OpenGraph `ogTitle`, `ogDescription`, the first `ogImage` URL, and the favicon. A relative favicon is made absolute against the URL's origin; if none is found it uses `<origin>/favicon.ico`. On any failure it returns all nulls and never throws. **Port tip:** make this one shared function.

### 3.7 Markdown export (`lib/markdown-export.ts`)
| Function | Output |
|---|---|
| `slugify(s)` | Lowercase, runs of non `[a-z0-9]` become `-`, trim dashes, max 60 chars, fallback `untitled` |
| `pageToMarkdown(page)` | `# title`, blank line, trimmed content; if links exist, `## Links` then `- [title](url) — description` (title has `[` `]` removed; description whitespace collapsed, max 160 chars); final line `<!-- exported from computer <updatedAt ISO> -->` |
| `bookmarkToMarkdown(b)` | `# title-or-url`, then `- URL: <url>`, `- Date: YYYY-MM-DD` (if dueDate), `- Tags: a, b` (if any), blank line, trimmed notes |
| `markdownResponse(body, name)` | Content-Type `text/markdown; charset=utf-8`, `Content-Disposition: attachment; filename="<name>.md"` |

### 3.8 OIDC configuration check (`lib/oidc-config.ts`)
`isRealValue(v)` is false when empty or matching a placeholder pattern: `replace_me`/`replace-me`/`replaceme`, `example.com/org/net`, `changeme`, `placeholder`, `<...>`, `your_…`/`your-…`, `todo`. `getOidcStatus()` returns `{enabled, issuerSet, clientIdSet, clientSecretSet, issuer}`, with `enabled` only when all three env values are real. It never exposes the secret.

### 3.9 Seed script (`scripts/seed.ts`)
Upserts a hidden test account. If env `DEMO_PASSWORD` is set, it also upserts five demo users (maya = ADMIN, jordan, sam, priya, alex @computer.demo). Each gets default columns, tags, bookmarks and one page, created only if that user has no bookmarks yet, followed by idempotent share upserts. It never deletes anything.

---

## 4. HTTP API

All routes return JSON unless marked .md. "Owner" means `ownerId = effective user`. Errors are `{error: string}`.

### 4.1 Auth and account
| Method + path | Input | Behavior |
|---|---|---|
| `POST /api/signup` | `{email, password, name?}` | 400 if missing; 409 if email exists; bcrypt(12); name defaults to the part of the email before `@`; role ADMIN if no admin exists yet (ignoring `@example.com` test users). Returns 201 `{ok, userId}` |
| `POST /api/auth/login` | `{email, password}` | Server-side credentials sign-in; 401 on failure |
| `GET/POST /api/auth/*` | | Auth library endpoints (csrf, callback/credentials, callback/authelia, session, signout) |
| `GET /api/me` | | `{user: realUser, isAdmin, viewingAs: {id, name, email} \| null}` |

### 4.2 Admin (use **real** user; 403 unless ADMIN)
| Method + path | Input | Behavior |
|---|---|---|
| `GET /api/admin/users` | | All users except `@example.com`, with counts of bookmarks, pages and tags, oldest first |
| `PATCH /api/admin/users` | `{userId, role}` | role must be USER/ADMIN; you cannot demote yourself |
| `POST /api/admin/view-as` | `{userId \| null}` | Null or own ID clears the cookie. Otherwise checks the user exists and sets an httpOnly, secure, SameSite=Lax cookie (path `/`, 8 hours) |

### 4.3 Bookmarks
| Method + path | Input | Behavior |
|---|---|---|
| `GET /api/bookmarks` | query `search, tagId, contactId, kanban, date, shared` | `shared=true` → bookmarks shared **with** me; otherwise mine. `search` matches title/url/notes/tag name, case-insensitive. `kanban` filters the legacy status. `date` = that calendar day of `dueDate`. Includes tags, contact, shares, owner. Newest-updated first, max 200 |
| `POST /api/bookmarks` | `{url, title?, description?, notes?, dueDate?, alertAt?, kanbanStatus?, contactId?, tagIds?, isPublic?, imapWatchEnabled?, imapQuery?}` | 400 without url; fetches the preview; title falls back to ogTitle, then the url. Returns 201 |
| `GET /api/bookmarks/:id` | | Visible if mine, shared with me, or public. Includes tags, contact, shares with user info, owner, and `pageLinks` (only **my** pages) |
| `PUT /api/bookmarks/:id` | any create field + `kanbanColumnId` | Owner only. Only fields present in the body change. `kanbanColumnId` must be my column (400). A new URL re-fetches the preview. `tagIds` replaces all tags. Setting `alertAt` resets `alertSent` |
| `DELETE /api/bookmarks/:id` | | Owner only |
| `POST /api/bookmarks/:id/share` | `{userIds?, isPublic?}` | Owner only; upserts shares (adding only, never removing); sets public flag |
| `GET /api/bookmarks/:id/markdown` | | .md download, mine or shared with me |
| `GET /api/preview?url=` | | Runs `fetchPreview` (3.6) |

### 4.4 Tags
| Method + path | Input | Behavior |
|---|---|---|
| `GET /api/tags` | | Mine plus tags shared with me, with bookmark count and share list, by name |
| `POST /api/tags` | `{name, color?}` | 400 without name |
| `PUT /api/tags/:id` | `{name?, color?, shareUserIds?}` | Owner only; upserts tag shares |
| `DELETE /api/tags/:id` | | Owner only |

Known gap: a shared tag shows up in the recipient's list, but filtering bookmarks by that tag only returns the recipient's own bookmarks. Fix this in the port if tag sharing should expose the owner's bookmarks.

### 4.5 Kanban columns
| Method + path | Input | Behavior |
|---|---|---|
| `GET /api/kanban/columns` | | `getColumns` |
| `POST /api/kanban/columns` | `{label, color?}` | Label trimmed to 40 chars; color default `#8b5cf6`; appended at the end |
| `PUT /api/kanban/columns` | `{order: id[]}` | Reorders my columns (in a transaction); returns the columns |
| `PATCH /api/kanban/columns/:id` | `{label?, color?}` | |
| `DELETE /api/kanban/columns/:id` | | Refuses to delete the last column. Bookmarks that resolve to this column move to the first remaining column; the rest are renumbered. Returns `{ok, movedTo, moved}`. **Never deletes bookmarks** |

Moving a card = `PUT /api/bookmarks/:id {kanbanColumnId}`.

### 4.6 Pages
| Method + path | Input | Behavior |
|---|---|---|
| `GET /api/pages?search=` | | Mine; search title/content; pinned first, then position, then created. Includes link count |
| `POST /api/pages` | `{title?, content?}` | Title default "Untitled page", max 120 chars; position = count |
| `PUT /api/pages` | `{order: id[]}` | Reorder |
| `GET /api/pages/:id` | | Page plus linked bookmarks in order |
| `PATCH /api/pages/:id` | `{title?, content?, pinned?, icon?, addBookmarkId?, removeBookmarkId?}` | Added bookmark must be mine or shared with me; appended at the end; adding the same one twice is harmless |
| `DELETE /api/pages/:id` | | |
| `GET /api/pages/:id/markdown` | | .md download |

### 4.7 Signals
| Method + path | Input | Behavior |
|---|---|---|
| `GET /api/bookmarks/:id/signals` | | `{sources, signals (newest 50), types: publicSourceTypes()}` |
| `POST /api/bookmarks/:id/signals` | `{type, config}` or `{action:"check"}` | Adds a watcher, keeping only the declared config keys (trimmed, max 200 chars); or runs `runSignalChecks` for this bookmark |
| `PATCH /api/signal-sources/:id` | `{enabled}` | |
| `DELETE /api/signal-sources/:id` | | |

### 4.8 Alerts, contacts, email, dates, users
| Method + path | Input | Behavior |
|---|---|---|
| `GET /api/alerts/check` | | My bookmarks with `alertAt ≤ now + 15 min` and `alertSent = false`, soonest first |
| `DELETE /api/alerts/:bookmarkId` | | Marks `alertSent = true` (dismiss) |
| `GET /api/contacts` | | Mine with bookmark counts, by name |
| `POST /api/contacts/sync` | `{nextcloudUrl, username, password, cardDavPath?}` | Path default `/remote.php/dav/addressbooks/users/<username>/contacts/`. Sends HTTP `REPORT` (Depth 1, Basic auth) with a CardDAV `addressbook-query` for `getetag` and `address-data`. Parses the multistatus XML; from each vCard takes `UID:`, `FN:`, and the last value of the EMAIL and TEL lines. Skips cards without UID or FN; upserts by (`uid`, `userId`). Returns `{synced}`. Credentials are **not stored** |
| `GET /api/settings/imap` | | My config with password masked as `••••••••` |
| `POST /api/settings/imap` | `{host, port?, tls?, username, password, folder?}` | Encrypts the password and upserts |
| `POST /api/imap/test` | `{host, port?, tls?, username, password}` | Opens only a TCP/TLS socket (10-second timeout, certificate not verified). **Does not log in to IMAP** |
| `GET /api/imap/check` | | **Stub**: lists bookmarks with the legacy watch on and returns `matches: []` |
| `GET /api/shared-dates` | | Dates I created or received, with creator and recipient |
| `POST /api/shared-dates` | `{date, note?, recipientIds[]}` | One row per recipient |
| `GET /api/users/search?q=` | | At least 2 characters; name/email contains, case-insensitive, excluding me; max 10. Used by share dialogs |

### 4.9 Public page (no login)
`GET /share/bookmark/:id` shows a read-only page for a bookmark where `isPublic = true`; otherwise not found.

---

## 5. Business rules checklist

1. Every query is scoped to the effective user; never trust a user ID from the client.
2. Admins are determined by the DB role, by `ADMIN_EMAILS` (promoted on first check), or by being the first real signup when no admin exists.
3. View as changes only the effective user; admin endpoints always use the real user. The UI shows a banner while it is active.
4. Sharing only adds; there is no unshare endpoint yet. Recipients see shared bookmarks read-only (only the owner can PUT or DELETE).
5. A user always has at least one kanban column. Deleting a column moves its bookmarks and never deletes them.
6. Signals are deduplicated by (`sourceId`, `externalId`).
7. IMAP passwords are stored encrypted only; the API never returns them.
8. Alerts are browser-only: there is no server push or email.
9. Authelia sign-in is off until all three OIDC env values are real.

---

## 6. Client screens and behaviors

| Screen / component | Responsibilities |
|---|---|
| **App shell** | Loads session and `/api/me`; shows the View as banner with an Exit button (`POST /api/admin/view-as {userId:null}`); asks for notification permission; polls `/api/alerts/check` on load and every **5 minutes**, showing a browser `Notification("computer alert", "<title> is due!")` per alert; provides search text, the add-bookmark dialog and the bookmark detail panel to child screens |
| **Sidebar** | Navigation (Dashboard, Bookmarks, Calendar, Tags, Contacts, Kanban, Pages, Settings, plus Admin for admins); pinned pages list that refreshes on the in-app event `computer:pages-changed` |
| **Dashboard** `/dashboard` | Bookmark card grid with Mine / Shared tabs and search |
| **Bookmarks** `/bookmarks` | Sortable table |
| **Calendar** `/calendar` | Month grid of bookmarks by `dueDate`, plus shared dates |
| **Tags** `/tags` | Tag list (create, color, share) with filtered bookmarks |
| **Contacts** `/contacts` | Synced contacts and their linked bookmarks |
| **Kanban** `/kanban` | Columns from `getColumns`; cards placed with `resolveColumnId`; drag and drop moves a card (PUT `kanbanColumnId`); add, rename, recolor, reorder and delete columns |
| **Pages** `/pages` | Tab bar of pages; Markdown editor with edit/preview toggle and **autosave 800 ms** after the last keystroke (states dirty → saving → saved); link picker searching bookmarks (250 ms delay, top 8); pin/unpin, rename, delete, .md download; sends `computer:pages-changed` after changes |
| **Admin** `/admin` | User list with counts, role toggle, View as button |
| **Settings** `/settings` | Profile; IMAP form (test + save); Nextcloud sync form; Authelia status and callback URL |
| **Add bookmark dialog** | URL with live preview (`/api/preview`), title, notes, tags, dates |
| **Bookmark detail panel** | Edit notes and dates, legacy status buttons, public toggle and copy link, share with users, legacy IMAP watch. **Unfinished:** column picker, Signals section, "On pages" list and .md button are not wired in yet |
| **Login / Signup** | Email + password; Authelia button disabled with a note while stubbed |

Visual design: indigo/violet accent, light and dark themes, sidebar with card layout, fonts DM Sans (body), Plus Jakarta Sans (headings) and JetBrains Mono (code).

---

## 7. Configuration

| Variable | Required | Purpose |
|---|---|---|
| `DATABASE_URL` | yes | Postgres connection |
| `NEXTAUTH_SECRET` / `AUTH_SECRET` | yes | Signs sessions; derives the IMAP encryption key |
| `NEXTAUTH_URL` | yes in production | Public base URL for auth callbacks |
| `OIDC_ISSUER`, `OIDC_CLIENT_ID`, `OIDC_CLIENT_SECRET` | no | Authelia; placeholder values keep it off |
| `ADMIN_EMAILS` | no | Comma-separated list of always-admin emails |
| `DEMO_PASSWORD` | no | Enables demo accounts in the seed script |

OIDC callback path: `/api/auth/callback/authelia`.

---

## 8. Suggested porting order

1. Schema and migrations (section 2); keep table and column names so the existing `linkos` database can be reused.
2. Crypto with the identical byte layout (3.5), then password and session auth (3.1) and the auth guard (3.2).
3. Bookmarks, tags and sharing endpoints, plus the shared preview function.
4. Kanban (3.3, 4.5), then Pages and Markdown export (3.7, 4.6).
5. Alerts, contacts sync, IMAP settings, shared dates, user search.
6. Signals registry (3.4), then a real email source.
7. Admin and View as; then OIDC.
8. Client screens (section 6).
