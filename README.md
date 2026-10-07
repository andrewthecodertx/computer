# go-computer

Go backend for **computer** (the link OS). Go is the API; the Next front-end is
kept. This is a stub skeleton — every function is intentionally unimplemented
(see `// TODO:` markers). It is wired so it compiles and the route table matches
the requirements guide.

## Layout

All logic lives under `internal/`, organized by domain:

| Package | Purpose |
|---|---|
| `config` | Env config (section 7) |
| `crypto` | AES-256-GCM IMAP password encryption (3.5) |
| `models` | Data objects mirroring the Prisma schema (section 2) |
| `db` | Storage interface (swap Postgres for another RDBMS) |
| `auth` | Signup/login, session JWT, OIDC status (3.1, 4.1) |
| `authguard` | View-as cookie, effective-user resolution (3.2, 4.2) |
| `kanban` | Default columns, `resolveColumnId` (3.3, 4.5) |
| `signals` | Signal source registry, `runSignalChecks` (3.4, 4.7) |
| `preview` | OpenGraph fetcher (3.6) |
| `markdown` | Slugify + page/bookmark export (3.7) |
| `handlers` | HTTP handlers, one file per route group (section 4) |
| `http` | Route registration (`NewServer`) |
| `scripts` | Seed (3.9) |

## Run

```bash
go run .
```

## Env

`DATABASE_URL`, `NEXTAUTH_SECRET` / `AUTH_SECRET`, `NEXTAUTH_URL`, optional
`OIDC_*`, `ADMIN_EMAILS`, `DEMO_PASSWORD`. See `computer_porting_guide.md`
section 7.

