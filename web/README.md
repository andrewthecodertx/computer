# computer — web UI

This app runs inside the root Compose stack. The root [../README.md](../README.md)
is the single authoritative deployment/operations guide; [../API.md](../API.md)
documents the Go API contract. This file covers only what a frontend
developer needs.

Ownership boundary: the Go API owns persistence, previews, integrations and
authentication semantics; this Next.js server owns browser sessions
(NextAuth), the trusted API bridge (`lib/api.ts` signs short-lived
assertions — the browser never supplies a trusted identity), and rendering.
Data requests go through `app/api/[...path]/route.ts`, not local Prisma
routes; the pre-port Prisma layer is gone. Schema and seeding live in the Go
stack (`../db/schema.sql`, `../db/migrations/`, `make seed`).

## Requirements

- Node.js 20.9 or newer
- Yarn (Corepack: `corepack enable`)
- A running Go API (`API_BASE_URL` — server-only Docker DNS, e.g.
  `http://app:8080`); the browser uses relative `/api/*` URLs

## Setup

```bash
cp .env.example .env          # fill in AUTH_SECRET, NEXTAUTH_URL, API_BASE_URL
yarn install
```

## Run

```bash
yarn dev                      # development: http://localhost:3000
yarn build && yarn start      # production
```

Production deployments run behind the Compose stack and the nginx template
in `../deploy/nginx/`. The Content-Security-Policy is emitted per response
by `middleware.ts` (nonce-based, so Next's inline bootstrap scripts run);
do not add a second `script-src` policy at the proxy — browsers intersect
multiple CSPs and the nonce would be stripped.

## Authentication

- **Email and password** work out of the box. Sign up at `/signup`. Signup
  responses are uniform (`{ok:true}`) for fresh, duplicate and reserved
  addresses — account existence is never disclosed.
- **Authelia OIDC is fail-closed.** Setting `OIDC_ISSUER`,
  `OIDC_CLIENT_ID` and `OIDC_CLIENT_SECRET` only makes the provider appear
  in configuration; the Go login/callback endpoints deliberately answer
  **501** until verified external identities are mapped to local users with
  explicit account-linking rules, and `auth.ts` rejects Authelia sign-ins
  until then. Configured variables are NOT working sign-in. See
  [docs/AUTHELIA.md](docs/AUTHELIA.md) and the root `AGENTS.md` boundary.
- Sign-out means **sign out everywhere**: the UI first calls
  `/api/session/revoke` (Go tokenVersion bump) and reports honestly when
  global revocation fails; the NextAuth `signOut` event is the backstop.
  Don't remove either path.

## Encryption and key retention (operator-relevant)

IMAP mailbox passwords are encrypted with AES-256-GCM. The key derives from
the dedicated `ENCRYPTION_KEY` when set, otherwise from the auth secret
(`NEXTAUTH_SECRET`/`AUTH_SECRET`). Rows encrypted before `ENCRYPTION_KEY`
existed decrypt via the auth-secret fallback and re-encrypt transparently
(compare-and-swap — a concurrent password change always wins). Consequences:

- Back up `ENCRYPTION_KEY`/`AUTH_SECRET` **with** the database; without the
  matching keys, stored mailbox credentials are unrecoverable.
- Rotating an already-dedicated `ENCRYPTION_KEY` needs old-key-aware
  migration, not just a changed environment variable. Keep the old secret
  until every credential row has been re-encrypted and verified.

## Project layout

| Path | Contents |
|---|---|
| `app/(app)/` | Signed-in screens: pages (primary workspace), bookmark filters (all, calendar, tags, contacts, kanban), collections, settings; `/dashboard` redirects to `/bookmarks` |
| `app/api/` | Next routes only: the Go proxy (`[...path]`), NextAuth (`auth/*`), session revoke; everything else is Go's |
| `app/share/bookmark/[id]` | Public read-only bookmark page |
| `components/` | UI components |
| `lib/` | Server-side API bridge (`api.ts`), OIDC config, autosave/pagination helpers |
| `middleware.ts` | Per-response nonce CSP (review CR-10) |
| `auth.ts` | Auth.js configuration |
| `docs/` | Integration docs |

## Notes

- URL previews are fetched **server-side by the Go API**
  (`internal/preview`, guarded by `internal/netguard` and a shared
  per-user/global budget) when a bookmark is saved or its URL changes —
  not by this app and not by `open-graph-scraper`.
- Date alerts are polled by the browser every 5 minutes and shown as
  browser notifications; email signal ingestion is Go-side (IMAP).

## Admins

Administrator provisioning is operator-driven: list addresses in
`ADMIN_EMAILS` and run `make seed` (it never changes existing accounts).
As a bootstrap convenience the FIRST signup on a fresh installation may be
promoted once (AppSetting `adminBootstrap`, disable with
`ALLOW_BOOTSTRAP_SIGNUP=0`); after bootstrap completes, signup never
auto-promotes again — even with no admins left. Promotion of an existing
allowlisted user requires `emailVerified`; DB roles remain authoritative.
Admins can open any account via Admin → "View as" (mutations are blocked
while impersonating).
