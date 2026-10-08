# computer

A multi-user bookmark workspace: URLs with Markdown notes, tags, calendar
dates, editable kanban boards, pages, previews, sharing, contacts and email signals.

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
- First non-`@example.com` account becomes administrator. `ADMIN_EMAILS` can
  promote additional accounts. Existing accounts are preserved on restart.
- `docker compose down` stops the stack. It does not delete the database volume.

`make run` and `make stop` are optional shortcuts for the Compose commands.
`docker-compose.full.yml` is a compatibility alias for the same complete stack.

### Existing Docker database

Set `POSTGRES_VOLUME` to its existing Docker volume name and
`POSTGRES_VOLUME_EXTERNAL=true`. The volume must contain the `computer`
database and `computer` role. Stop the previous PostgreSQL container before
mounting that volume in this stack. The local configuration on this machine
already reuses the existing volume.

`db/schema.sql` initializes a **new, empty** database once. `db/upgrade.sql`
runs as an additive migration before Go starts. Neither script drops a
database. Do not run the initial schema against an already initialized database.

## Architecture

- `internal/`: Go API, persistence, authorization and integrations. SQL values
  are parameterized; camelCase JSON matches the Next.js client contract.
- `web/`: Next.js UI and NextAuth browser sessions. Credentials are verified
  by Go. A server-side API proxy signs 60-second identity assertions; Go
  reloads roles and resolves admin impersonation on every data request.
- The frontend has no runtime Prisma client or direct database connection.
- Go also supports its own HttpOnly `computer_session` cookie for direct API
  consumers. Administration uses the real user; data uses the effective user.
- Containers share the Compose backend network. The app runs as non-root
  with a read-only filesystem; the web container has temporary `/tmp` storage.

## Features and integrations

- Bookmarks: create/edit/delete, tags, contacts, previews, calendar dates,
  alerts, Markdown downloads and public read-only links.
- Pages: Markdown editing/autosave, link collections, pinning, reorder/export.
- Kanban: custom columns, card moves, reorder; deleting a column moves cards.
- Sharing: bookmarks, tags and calendar dates; recipients read shared content.
  Owners can revoke shares. Shared tags and dates expose the associated URLs.
- Alerts: browser notifications and in-app dismissal, polled every five minutes.
- Nextcloud: CardDAV REPORT sync using each user's app password. Credentials
  are used for that request only, not stored.
- IMAP: encrypted per-user settings, real login tests, sender/subject searches,
  email watchers, and Message-ID deduplication. TLS certificates are verified.
  Configure integrations in Settings, then check watchers from a bookmark.
  Enabled email watchers also run every five minutes in the Go process,
  including watches enabled with the original mailbox-watch switch.

### Authelia remains a client integration stub

As requested, the ultimate OIDC connection is reserved for the client.
NextAuth retains the Authelia provider configuration and callback location;
Go exposes placeholder-aware status and explicit **501** login/callback stubs.
No unverified OIDC subject is accepted as a local user ID. Before enabling
Authelia, connect verified OIDC identities to local Go User/Account records in
the frontend sign-in callback. See `web/docs/AUTHELIA.md` and `web/auth.ts`.
Email/password sign-in is fully functional in the meantime.

Keep `AUTH_SECRET` stable: it derives the AES key for stored IMAP passwords.
For HTTPS, set `NEXTAUTH_URL` to the public web URL and `COOKIE_SECURE=1`.
OIDC values stay on the server and are never sent to the browser as secrets.

## Developer commands

```bash
make build                   # Go compile
make vet                     # Go vet
make test                    # Go unit tests with race detection
make integration             # Docker PostgreSQL + CardDAV/IMAP fixture tests
make db-shell                # psql inside the database container
make logs                    # Go and Next.js logs
make migrate                 # additive database upgrades
```

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
kanban, page autosave/exports and every application screen. Test accounts have
unique names and only those accounts are deleted afterward.

Optional initial account: `SEED_EMAIL=... SEED_PASSWORD=... make seed`.
The seed command never replaces an existing password or deletes data.
