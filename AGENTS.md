# computer

- `make run` starts the entire Compose stack: db → additive migration → Go
  API → Next.js. `make stop` preserves the PostgreSQL volume. Root `.env`
  selects the volume and a stable `AUTH_SECRET`; it is not committed.
- Go packages are under `internal/`; Next UI is `web/` — read `web/AGENTS.md`
  before frontend work. `auth` must not import `handlers` (data handlers
  already depend on auth). Administration uses real identity; data handlers
  use the effective identity after view-as. JSON is camelCase to match the
  Next.js client contract.
- `db/schema.sql` is initial empty-database SQL; `db/upgrade.sql` is additive
  and runs before every app start. Never drop/recreate the database or delete
  app users to run verification.
- `make test` (unit, `-race`), `make vet`, `make integration` (Compose `tests`
  service; creates/drops its own random schemas against the db container).
  Browser tests: `npm --prefix scripts ci`, then
  `CHROMIUM_PATH=/usr/bin/chromium make e2e` with the stack running.
  `npm --prefix scripts run test:regressions` (after `yarn build` in `web/`)
  runs against an isolated Next server and never touches app users.
- NextAuth sessions are not Go JWT cookies. The trusted Next server bridges
  them with 60-second signed assertions (`web/lib/api.ts`); Go reloads roles
  and ownership on every request.
- OIDC login/callback (`internal/oidc/provider.go`) deliberately return 501:
  client-owned stubs. Preserve that boundary unless explicitly asked to
  implement Authelia integration.
- `make seed` provisions an admin for an `ADMIN_EMAILS` address; it never
  changes existing accounts.
- All SQL values are parameters. Dynamic table/column identifiers originate
  only from server allowlists. Nullable updates distinguish omitted and null.
- Keep AES-GCM `base64(IV[12] || tag[16] || ciphertext)` compatible with stored
  IMAP passwords. Go image needs CA certificates for HTTPS/TLS integrations.
