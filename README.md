# computer

A multi-user bookmark workspace: URLs with Markdown notes, tags, calendar
dates, editable kanban boards, project collections with typed properties and
saved views, pages, previews, sharing, contacts and email signals.

## Run the whole app

**Client setup guide:** [Running computer](RUNNING.md) covers cloning, setup,
first login, everyday commands, updates, and troubleshooting on Windows,
macOS, and Linux.

Requires Docker with Compose v2 or newer. On a new installation:

```bash
git clone https://github.com/andrewthecodertx/computer.git
cd computer
cp .env.example .env
docker run --rm node:22-alpine node -e "console.log(require('node:crypto').randomBytes(32).toString('hex'))"
```

Put the generated value in `AUTH_SECRET` in `.env`, save the file, then run:

```bash
docker compose up -d --build  # builds and starts PostgreSQL, Go and Next.js
```

On Windows PowerShell, use `Copy-Item .env.example .env` instead of `cp`.

- Web interface: **http://localhost:3000**. Sign up, then sign in.
- Go API: **http://localhost:8080** (local access; endpoints in [API.md](API.md)).
- PostgreSQL: `db:5432` inside Docker; not published on the host.
- With `ADMIN_EMAILS` empty, the first non-`@example.com` account becomes
  administrator and completes a one-shot bootstrap: afterwards signup never
  auto-promotes again, even if the last administrator is demoted or removed
  — recovery is operator-driven (`ADMIN_EMAILS` + seed, or promotion by an
  existing admin). Addresses in `ADMIN_EMAILS` are reserved for
  operator-provisioned accounts; automatic
  promotion requires a verified email. Existing accounts are preserved on restart.
- `docker compose down` stops the stack. It does not delete the database volume.

`make run` and `make stop` are optional shortcuts for the Compose commands.
`docker-compose.full.yml` is a compatibility alias for the same complete stack.

### Existing Docker database

Set `POSTGRES_VOLUME` to its existing Docker volume name and
`POSTGRES_VOLUME_EXTERNAL=true`. The volume must contain the `computer`
database and `computer` role. Stop the previous PostgreSQL container before
mounting that volume in this stack. The local configuration on this machine
already reuses the existing volume.

`db/schema.sql` initializes a **new, empty** database once. `db/migrations/`
holds the ordered, checksummed additive migrations that `db/migrate.sh` runs
(ledger table `_migration`) before Go starts. Each migration is applied and
recorded in a single transaction under an advisory lock, so a crash never
leaves an applied file unrecorded (or vice versa) and a second concurrent
runner aborts instead of double-applying. Neither path drops a database.
Do not run the initial schema against an already initialized database, and do
not edit a migration that has been applied — add a new numbered file.
Migrations are additive (new columns/tables/indexes with defaults), so the
previous application release keeps working against a migrated database;
rolling the data itself back means restoring the pre-deploy dump (see
RUNNING.md), not reverting SQL. Because schema.sql and the chain are two
paths to the same schema, every schema change needs both a schema.sql edit
and a new migration; `make db-converge` (included in `make check`) fails
when they drift apart.

## Architecture

- `internal/`: Go API, persistence, authorization and integrations. SQL values
  are parameterized; camelCase JSON matches the Next.js client contract.
- `web/`: Next.js UI and NextAuth browser sessions. Credentials are verified
  by Go. A server-side API proxy signs 60-second identity assertions; Go
  reloads roles and resolves admin impersonation on every data request.
  Signing out in the browser also revokes backend sessions ("sign out
  everywhere"): it bumps the user's token version, so prior assertions and
  Go cookie sessions stop working.
- The frontend has no runtime Prisma client or direct database connection.
- Go also supports its own HttpOnly `computer_session` cookie for direct API
  consumers. Administration uses the real user; data uses the effective user.
- Containers share the Compose backend network. The app runs as non-root
  with a read-only filesystem; the web container has temporary `/tmp` storage.

## Features and integrations

- Bookmarks: create/edit/delete, tags, contacts, previews, calendar dates,
  alerts, Markdown downloads and public read-only links.
- Bookmark collections use cursor pagination; the UI follows every page so
  collections larger than 200 remain visible on all organization screens.
- Notion-style workspace shell: a Quick Find command palette (Ctrl/Cmd+K)
  jumps to any page, bookmark, tag or view and runs commands (new page, add
  bookmark, theme toggle); the slim top bar shows a breadcrumb path
  (workspace / section / page); sidebar sections collapse and page rows
  expose hover actions (pin, inline rename, delete).
- Pages: Markdown editing/autosave with a Notion-style slash-command block
  menu (type `/` for headings, to-dos, lists, quotes, dividers and code) and
  the `[]` + space to-do shortcut, link collections, pinning, reorder/export.
  Saves are queued per page; unsaved drafts are kept in the current browser tab
  for recovery after a failed save, together with the server version they were
  based on. Page switching waits for pending saves. When another tab or device
  saved first, autosave pauses in an explicit conflict state — the editor
  offers overwrite, save-as-copy and discard, and typing alone never
  overwrites the newer version.
- Kanban: custom columns, card moves, reorder; deleting a column moves cards.
- Collections: per-project workflow state over your existing bookmarks. The
  same link joins many collections without duplication, each with its own
  typed properties (text, number, select, multi-select, date, checkbox, URL),
  saved table/board views, server-side filters with typed operator
  allowlists, and manual ordering. Membership adds context — it never owns
  or publishes the bookmark: removing an item or a whole collection keeps
  the bookmark, and a bookmark whose share was revoked disappears from
  collections everywhere. Every mutating PATCH carries an expected version,
  so concurrent editors get a conflict instead of a silent overwrite;
  reordering and property deletion advance the affected items' and views'
  revisions too, and deletions are idempotent removals.
- Sharing: bookmarks, tags and calendar dates; recipients read shared content.
  Owners can revoke shares. Shared tags and dates expose the associated URLs.
  Tag sharing is group-wide: every recipient of a tag sees all bookmarks
  carrying it, including ones other recipients add. Shared readers get a
  narrowed response: the link and its reader-facing content only — never the
  owner's contact record, the other recipients, alerts, or mailbox watches.
- Public bookmark links deliberately publish the title, description, notes,
  due date, tags and preview; contacts, alerts, mailbox watches and the
  owner identity stay private. The share panel states this before you toggle.
- Remote preview images and favicons are loaded by your browser directly
  from the third-party host, with `referrerPolicy="no-referrer"` so the app
  URL is not disclosed; opening a page can still let those hosts observe the
  request. Server-side fetches (previews, CardDAV, IMAP) never touch private
  network ranges.
- Alerts: browser notifications and in-app dismissal, polled every five minutes.
- Nextcloud: CardDAV REPORT sync using each user's app password. Credentials
  are used for that request only, not stored, and never travel plaintext:
  the Nextcloud URL must use https (redirects are not followed).
- IMAP: encrypted per-user settings, real login tests, sender/subject searches,
  email watchers, and Message-ID deduplication. Verified TLS is required —
  plaintext mailbox logins are rejected. Watcher ingestion is incremental: a
  durable per-source UID checkpoint processes bounded batches until caught up
  instead of re-scanning the last 50 matches. Configure integrations in
  Settings, then check watchers from a bookmark.
  Enabled email watchers also run every five minutes in the Go process,
  including watches enabled with the original mailbox-watch switch. The
  scheduler runs users with bounded concurrency, per-user failure backoff,
  and a database-backed lease so extra instances never double-poll a mailbox.

### Connecting Authelia (OIDC)

**Authelia sign-in is currently stubbed. Connecting it requires both server
configuration and developer integration; setting environment variables alone
does not enable it.** Email/password sign-in works while that integration is
pending. The full [Authelia connection guide](web/docs/AUTHELIA.md) includes
the client registration example and the implementation handoff.

#### 1. Register the app in Authelia

Use an Authelia installation with OIDC enabled and a valid HTTPS issuer URL.
Register `computer` as a confidential client with:

- Scopes: `openid`, `profile`, `email`.
- Grant type: `authorization_code`; response type: `code`.
- Token endpoint authentication: `client_secret_basic`.
- Redirect URI: `<PUBLIC_APP_URL>/api/auth/callback/authelia`, for example
  `https://computer.company.tld/api/auth/callback/authelia`.

Generate a client secret as described in the connection guide. Put its
**hashed digest** in Authelia and its **plain value** in the app's
`OIDC_CLIENT_SECRET`. The redirect URI must exactly match the public web URL
and callback path. It points to the Next.js server, not the Go API on port 8080.

#### 2. Configure computer

Edit the root `.env`, replacing the sample values with your deployment values:

```dotenv
NEXTAUTH_URL=https://computer.company.tld
COOKIE_SECURE=1
OIDC_ISSUER=https://auth.company.tld
OIDC_CLIENT_ID=computer
OIDC_CLIENT_SECRET=<plain-client-secret>
```

Serve the web app through an HTTPS reverse proxy; the default Docker ports
bind to localhost. Keep your existing `AUTH_SECRET` unchanged. Apply the
environment changes with `docker compose up -d`, then check **Settings → OIDC**.
It will report that the variables are configured but integration is pending.
The login button remains disabled at this stage.

#### 3. Complete the developer integration

The trusted Next.js server owns OIDC verification and the callback. The
remaining work is to:

1. Implement Go-backed lookup/provisioning of a local `User` and `Account`
   for the verified Authelia identity. A provisioning endpoint is not yet
   supplied; any new endpoint must authenticate the trusted Next server.
2. Update `web/auth.ts` to resolve the verified identity to the local
   `User.id` and permit sign-in only after that succeeds. The session and
   API assertion must use this local ID, not Authelia's `sub` claim. Existing
   accounts need an explicit, verified account-linking policy.
3. Enable the readiness gate in `web/app/login/client.tsx` after mapping works,
   rebuild with `docker compose up -d --build`, and verify sign-in, first-login
   provisioning, returning users, and Go-backed data access.

The Go OIDC login/callback handlers in `internal/oidc/provider.go` deliberately
return **501**. They are not the NextAuth callback implementation. Config status
(including an `enabled` flag) indicates that values are present, not that an
end-to-end login has been implemented.

## Developer commands

```bash
make build                   # Go compile
make vet                     # Go vet
make test                    # Go unit tests with race detection
make integration             # Docker PostgreSQL + CardDAV/IMAP fixture tests
make db-shell                # psql inside the database container
make logs                    # Go and Next.js logs
make migrate                 # ordered additive database migrations
make db-converge             # schema.sql vs migration-chain drift check
make check                   # release validation (fails without DB coverage)
make audit                   # dependency advisory scans (needs govulncheck)
```

The same release gate runs automatically on every push and pull request
(`.github/workflows/check.yml`), including the browser smoke tests against
the composed stack. Pass `YARN="corepack yarn"` to `make check` when yarn
comes through corepack.

Integration tests create/drop random test schemas; they never clear app data.
They exercise CRUD, sharing, ownership boundaries, impersonation, encryption,
CardDAV sync, IMAP login/search and signal deduplication.

Frontend checks (from `web/`): `yarn lint`, `yarn build`, and
`yarn eslint -c eslint.ssr.config.mjs .`. Builds enforce TypeScript checks.

Browser smoke tests (with the stack running):

```bash
npm --prefix scripts ci
CHROMIUM_PATH=/usr/bin/chromium make e2e
```

These verify login, UI bookmark creation, Go-backed persistence, public links,
Quick Find deep links, kanban, the collections workflow (membership without
duplication, typed properties, table+board over one membership), page
autosave/slash commands/exports and every application screen. Test accounts
have unique names and only those accounts are deleted afterward.

Targeted browser regressions (after `yarn build` in `web/`):

```bash
npm --prefix scripts run test:regressions
```

These run an isolated Next server with API fixtures and test login CSRF,
browser sign-out revocation, autosave switching/ordering/recovery (including
drafts whose base version is unknown, which pause for review), pagination,
and bookmark editing without creating or deleting application users.

Optional operator-provisioned account: set its address in `ADMIN_EMAILS` in
the root `.env`, run `docker compose up -d`, then:

```bash
docker compose exec -e SEED_EMAIL=admin@company.tld -e SEED_PASSWORD='<chosen-password>' app /computer --seed
```

This trusted command creates an admin for an allowlisted address. It never
changes an existing account's password or role, or deletes data. Additional
existing users can be promoted from the Admin screen.

## Deploying to a server

Deployment is target-agnostic — there are no built-in host or domain
defaults. Point DNS for your domain at the server, have an ssh alias (or
host) ready, then run from the repository root:

```bash
HOST=app-server DOMAIN=computer.example.tld CERTBOT_EMAIL=ops@example.tld ./scripts/deploy.sh
```

(`make deploy` runs the same script with the same variables.) It records the
exact git revision, refuses dirty trees unless `ALLOW_DIRTY=1`, rsyncs the
source, and on the server: provisions Docker/nginx/certbot, writes `.env` on
first deploy, takes a pre-deploy database dump plus `.env` snapshot into
`/var/backups/computer` (15 retained), builds revision-tagged images, and
installs the nginx + Let's Encrypt reverse proxy from
`deploy/nginx/computer.conf.template`. Rollback means re-deploying an older
revision or restoring a pre-deploy dump; migrations are additive. See
RUNNING.md → "Running on a remote server" for the operator checklist.

## Client test build

Use [RUNNING.md](RUNNING.md) for the client handoff. Test login, bookmark editing,
tag/contact organization, calendar dates, kanban, a collections workflow
(create a collection, add the same bookmark to two collections, give each a
status, move a card on a board view), Markdown pages/exports,
Quick Find (Ctrl/Cmd+K), slash commands in the page editor, and sharing with
separate accounts.

Current integration boundaries:

- Authelia still requires the developer identity-mapping work described above;
  use email/password for this test build.
- IMAP and Nextcloud require the client's own service credentials.
- Alerts are browser-only and checked every five minutes while the app is open.
- Page autosave needs an API connection. Check the save indicator; use **Save
  now** after a failed save. Tab-local drafts are recovery aids, not backups.
