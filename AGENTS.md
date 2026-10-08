# computer

- `make run` starts the entire Compose stack: db → additive migration → Go
  API → Next.js. `make stop` preserves the PostgreSQL volume. Root `.env`
  selects the volume and a stable `AUTH_SECRET`; it is not committed.
- Go packages are under `internal/`; Next UI is `web/`. `auth` must not import
  `handlers` (data handlers already depend on auth). Administration uses
  real identity; data handlers use the effective identity after view-as.
- `db/schema.sql` is initial empty-database SQL; `db/upgrade.sql` is additive.
  Never drop/recreate the database or delete app users to run verification.
- `make test`, `make vet`, `make integration`; integration tests create/drop
  their own random schemas. Browser tests: `npm --prefix scripts ci`, then
  `CHROMIUM_PATH=/usr/bin/chromium make e2e` with the stack running.
- NextAuth sessions are not Go JWT cookies. The trusted Next server bridges
  them with 60-second signed assertions; Go reloads roles and ownership.
- OIDC login/callback are deliberately client-owned stubs. Preserve that
  boundary unless explicitly asked to implement Authelia integration.
- All SQL values are parameters. Dynamic table/column identifiers originate
  only from server allowlists. Nullable updates distinguish omitted and null.
- Keep AES-GCM `base64(IV[12] || tag[16] || ciphertext)` compatible with stored
  IMAP passwords. Go image needs CA certificates for HTTPS/TLS integrations.
