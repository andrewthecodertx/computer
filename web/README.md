# computer — web UI

This app now runs inside the root Compose stack. See [../README.md](../README.md)
for current setup and commands. The Go API owns database access; NextAuth owns
browser sessions. The sections below describe the original pre-port application.

A multi-user, web-based "link operating system". You collect URLs from apps
and sites and organize them by calendar date, tag, Nextcloud contact, or a
kanban board. Each URL can have markdown notes, previews, date alerts and IMAP
mailbox watching, and can be shared.

## Requirements

- Node.js 20.9 or newer
- Yarn (Corepack: `corepack enable`)
- PostgreSQL 13 or newer

## Setup

```bash
cp .env.example .env          # fill in DATABASE_URL, secrets, NEXTAUTH_URL
yarn install
yarn prisma generate
yarn prisma db push           # creates or updates the tables (non-destructive)
# optional first user:
SEED_EMAIL=you@example.org SEED_PASSWORD='choose-one' yarn prisma db seed
```

## Run

```bash
yarn dev                      # development: http://localhost:3000
yarn build && yarn start      # production
```

`yarn build && yarn start` is the simplest way to self-host. Put it behind a
reverse proxy (Caddy, nginx) for HTTPS.

## Authentication

- **Email and password** work out of the box. Sign up at `/signup`.
- **Authelia OIDC** is stubbed. It switches on automatically once
  `OIDC_ISSUER`, `OIDC_CLIENT_ID` and `OIDC_CLIENT_SECRET` hold real values.
  See **[docs/AUTHELIA.md](docs/AUTHELIA.md)**.

## Project layout

| Path | Contents |
|---|---|
| `app/(app)/` | Signed-in screens: pages (primary workspace), bookmark filters (all, calendar, tags, contacts, kanban), settings; `/dashboard` redirects to `/bookmarks` |
| `app/api/` | API routes: bookmarks, tags, sharing, preview, contacts sync, IMAP, alerts |
| `app/share/bookmark/[id]` | Public read-only bookmark page |
| `components/` | UI components |
| `lib/` | Prisma client, crypto (IMAP passwords), OIDC config, helpers |
| `prisma/schema.prisma` | Database schema |
| `auth.ts` | Auth.js configuration |
| `docs/` | Integration docs |

## Notes

- IMAP passwords are encrypted (AES-256-GCM) with a key derived from `NEXTAUTH_SECRET`.
- URL previews are fetched server-side with `open-graph-scraper` when a bookmark is saved.
- Date alerts are polled by the browser every 5 minutes and shown as browser notifications.

## Admins
The first account to sign up becomes admin. You can also list emails in the optional `ADMIN_EMAILS` env var (comma-separated). Admins can open any account via Admin → "View as".
