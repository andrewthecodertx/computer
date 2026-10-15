# Computer — Features and Roadmap

A self-hosted, multi-user bookmark workspace. URLs are first-class records
with Markdown notes, live previews, contacts, and email signals attached;
pages, kanban boards, and Notion-style collections provide the workspace
context around them.

**Stack:** Go 1.26 API (`internal/`), Next.js + NextAuth web client (`web/`),
PostgreSQL with checksummed additive migrations (`db/`), Docker Compose
deployment with an nginx TLS template (`deploy/`). A release gate
(`make check` + CI) runs DB-backed integration tests, browser regressions,
and real-stack e2e on every push.

---

## Features

### Bookmarks and link management
- URL records with title, description, Markdown notes, due dates, and tags.
- **Live link previews**: server-side fetch of Open Graph metadata and
  favicon (SSRF-guarded, connection-pooled, rate-limited).
- Search and filtering (`search`, `tagId`, `contactId`, `kanban`, `date`,
  `shared`) with keyset-cursor pagination (`X-Next-Cursor`, max 200/page).
- Markdown export per bookmark (`.md` attachment).
- Permission-specific responses: owners see everything; shared readers get a
  narrowed projection (never contacts, recipients, alerts, or mailbox
  watches); public links carry no owner attribution.

### Pages (Markdown workspace)
- Markdown editor with preview/edit toggle, Notion-style **slash menu**
  (`/` for headings, to-dos, lists, quotes, dividers, code) and `[]` + space
  to-do shortcut.
- Notion-style paste: a bare URL offers to upgrade to a titled link once the
  preview resolves — only on explicit click, so undo stays intact.
- **Versioned autosave**: every content save carries a base `version`; a
  stale base returns 409 and the editor pauses with an Overwrite / Save as
  copy / Discard banner. Typing is never consent to overwrite.
- Tab-local recovery drafts (sessionStorage) that persist their base version.
- Pinning, sidebar/tab reordering, inline rename, Markdown export, and
  curated bookmark links per page.

### Collections (Notion-like databases)
- Owner-scoped collections whose members are **existing bookmarks** —
  membership never owns, copies, or publishes a bookmark.
- **Typed properties**: TEXT, NUMBER, SELECT (stable option ids),
  MULTI_SELECT, DATE (date-only vs datetime modes that never shift across
  timezones), CHECKBOX, URL. Types are immutable after creation; option
  removal is refused while items still carry the value. RELATION is
  schema-reserved but API-rejected pending cross-collection access design.
- **Saved views** (TABLE, BOARD; CALENDAR/LIST stored, rendering as tables
  for now) with server-side AND/OR filters over per-type operator
  allowlists, up to 3 stored sorts (first key drives ordering), grouping,
  and display settings. Board views group by a SELECT property; a card move
  is a property edit the table reads back immediately.
- Keyset pagination with deterministic id tie-break; filter ids and values
  are bind parameters end to end.
- **Membership never broadens access**: every surface re-checks bookmark
  visibility; a revoked share makes items not-found while the membership row
  survives, so re-sharing restores it. Item counts are authorized counts.
- Optimistic concurrency on every mutating PATCH (400 missing / 409 stale).

### Boards, calendar, tags
- **Global kanban board** with custom columns, card moves, column reordering;
  deleting a column relocates its cards instead of dropping them.
- **Calendar**: bookmarks by due date plus shared dates.
- **Tags**: colored, group-wide sharing (every recipient sees all bookmarks
  under a shared tag — disclosed in the UI).

### Sharing and publishing
- Per-bookmark shares (revocable), tag shares, and **shared dates** that
  grant read access to what lands on a day.
- **Public bookmark links** deliberately publish title, description, notes,
  due date, tags, and preview; the share panel discloses this before the
  toggle. Public responses exclude owner identity, contacts, recipients,
  mailbox queries, and alert metadata. Public endpoints are rate-limited
  per IP.

### Contacts (CardDAV)
- Sync from Nextcloud (or any CardDAV server): HTTPS required outside test
  fixtures, redirects never followed, credentials used per-request and never
  stored. Contacts attach to bookmarks.

### Email signals (IMAP watchers)
- Per-bookmark mailbox watchers (`from`/`subject` filters) over a configured
  IMAP mailbox; passwords encrypted at rest (AES-GCM, HKDF-SHA256 derived
  key with transparent legacy rotation).
- **Checkpointed incremental ingestion**: durable per-source UID checkpoints,
  bounded 2,500-message passes, monotonic checkpoint saves, UIDVALIDITY
  resets handled correctly; deduplication by source + Message-ID.
- Background scheduler with per-user advisory leases, bounded concurrency,
  exponential backoff, and verified-TLS-only connections; manual "Check now"
  takes the same lease (409 while held) under a 45-second cap.
- Per-source `lastStatus` surfacing failures without reporting false
  success. **Alerts**: bookmarks due within 15 minutes, dismissible.

### Quick Find and workspace shell
- **Ctrl/Cmd+K palette** over pages, bookmarks, tags, and views plus commands
  (new page, add bookmark, theme toggle), deep-linking into the bookmark
  detail sheet.
- Collapsible sidebar with hover actions, breadcrumb top bar, light/dark
  theme.

### Multi-user administration
- USER/ADMIN roles; **view-as** impersonation that is read-only, audited,
  eight-hour bounded, and cleared on sign-out.
- Last-admin invariant: self-demotion refused and demoting the final other
  admin blocked; role changes are audit-logged.
- One-shot first-signup admin bootstrap (`adminBootstrap` AppSetting),
  `ADMIN_EMAILS` reservation for operator provisioning (`make seed`),
  `ALLOW_BOOTSTRAP_SIGNUP=0` to disable open registration entirely.
- OIDC/Authelia login endpoints exist as deliberate 501 stubs (client-owned).

### Security and operations
- NextAuth sessions bridged to Go with 60-second HMAC-signed assertions;
  channel-bound tokens (`typ` claim), token-version revocation, and
  **global sign-out** across both channels.
- Per-client-IP rate limiting for login/signup (signed forwarded IP from the
  trusted Next proxy), consecutive-failure lockout, timing-equalized
  unknown-email logins, accurate `Retry-After`.
- Outbound-fetch SSRF guard (`internal/netguard`) blocking
  private/link-local/CGNAT ranges for previews, CardDAV, and IMAP.
- All SQL parameterized; dynamic identifiers only from server allowlists.
- Migration ledger with atomic apply+record under an advisory lock and a
  schema-convergence check (`make db-converge`) — fresh and upgraded installs
  can never drift.
- `PORT`-aware healthchecks, nginx template with HSTS + CSP, `COOKIE_SECURE`
  auto-detection from `NEXTAUTH_URL`.

---

## Roadmap — next 3–6 months

### Near term (0–3 months): hardening and debt

Completing the remaining code-review backlog (16 open items, none critical):

- **SSRF guard gaps (SEC-8)**: deny NAT64 (`64:ff9b::/96`), 6to4
  (`2002::/16`), and IPv4-mapped/embedded forms; recheck embedded IPv4 after
  resolution; restrict preview to ports 80/443 and IMAP to 143/993.
- **IMAP check robustness**: return partial matches alongside per-query
  errors (COR-6); error-handling cleanups — `errors.Is` for sentinels
  (ERR-1), no `log.Fatalf` inside the server goroutine (ERR-2), cancellable
  scheduler semaphore (ERR-4).
- **Input limits and parsing**: explicit length caps on user-supplied text —
  name, title, url, notes (CQ-3); Markdown export link-target escaping and
  Unicode-safe slugs (CQ-4); vCard edge cases — preferred EMAIL/TEL, `\N`
  unescaping, deletion of contacts removed upstream (CQ-5).
- **Performance**: single-statement kanban column deletion (PERF-2), escaped
  wildcards + `pg_trgm` GIN index (or tsvector) for page search, scheduler
  `failures`-map pruning (PERF-3).
- **Code quality**: load config once and inject it (CQ-1), remove dead
  models and redundant JSON round-trips (CQ-2), rename the module path to
  match the repository (CQ-6), transactionalize the last-admin check
  (SEC-11 follow-up).
- **Testing and CI**: fail (not skip) DB tests when `CI=true` without
  `TEST_DATABASE_URL`; unit tests for `markdown`, `kanban`, `parseVCard`;
  e2e assertion that two browsers' failed logins don't share a lockout
  (TEST-1); run `make audit` (govulncheck + yarn/npm audit) in the release
  gate (TEST-2).
- **Dependencies**: migrate `go-imap` v1 → v2 (unmaintained line); pin
  `next-auth` to stable when it leaves beta (DEP-5).

### Mid term (3–6 months): product phases 3–4

From the phased Notion-like roadmap (phase-gated: each ships only after the
previous exit gate passes):

- **Connected pages (phase 3)**
  - Page hierarchy (`parentId` with cycle rejection).
  - Internal links and **backlinks** that never reveal inaccessible titles.
  - Embedded live collection views inside pages — host page *and* source
    collection must be authorized; embedding never publishes.
- **Trust (phase 4)**
  - Server-side **revision history** for pages (restore creates a new
    revision).
  - **Trash / restore** — restored items never resurrect expired grants.
  - Structured export alongside Markdown.
  - **Collection sharing**: explicit, disclosed, revocable — designed against
    the consistent-permissions requirement so membership can never become an
    access-widening vector. This unlocks the **RELATION** property
    (cross-collection access checks) afterward.
- **View depth**
  - Real CALENDAR and LIST view renderers (currently stored, rendered as
    tables).
  - Multi-key server-side sort (all three stored keys, not just the first).
- **Integrations**
  - OIDC/Authelia login implementation (replacing the 501 stubs) once the
    client integration is scheduled.
  - Alert delivery beyond in-app polling (e.g. email/webhook notifications
    for due bookmarks and matched signals).

### Deliberate non-goals (unchanged)

- No block editor — Markdown-first until block storage, serialization
  versioning, export, undo, and concurrency are fully specified.
- No real-time collaboration, formulas, rollups, plugins, or AI-authored
  mutations.
- No dynamic cross-workspace collections — membership stays explicit.
