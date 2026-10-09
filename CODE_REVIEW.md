# Code Review — computer

**Date:** 2026-10-08 · **Commit:** `ba226bc` (main) · **Scope:** entire repository — Go backend (`internal/`, `main.go`), Next.js frontend (`web/`), database (`db/`), infrastructure (Docker, Compose, Makefile), and test tooling (`scripts/`, `*_test.go`).

**Method:** four parallel deep reviews (auth/HTTP core; handlers/integrations; frontend; DB/infra/tests), each reading its scope end-to-end. Every HIGH/MEDIUM finding below was re-verified by a second reader directly against the source; those are marked ✅. Findings are grounded in quoted code — no speculation.

---

## Executive summary

The codebase is in good shape for its size. The identity architecture (trusted Next server signing 60-second assertions; Go reloading roles per request), SQL parameterization discipline, owner-scoped mutations, migration idempotency, container hardening, and test-data isolation are all implemented correctly and consistently. **No critical findings and no exploitable auth bypass, SQL injection, or cross-user data access were found.**

The real risks cluster into four themes:

1. **No abuse protection on auth endpoints** — unthrottled login/signup enables brute-force, enumeration, and bcrypt CPU exhaustion (AUTH-1, AUTH-2).
2. **Inconsistent SSRF hardening** — the preview fetcher is properly hardened, but CardDAV sync and IMAP dialing are unfiltered server-side request primitives (API-1, API-2).
3. **Token-model gaps** — 60-second assertions and 30-day cookie JWTs are cryptographically interchangeable, sessions are irrevocable, and the `Secure` flag defaults off via an undocumented env var (AUTH-3, AUTH-4, AUTH-5).
4. **Frontend resilience** — a discarded SSR shell, a racy per-keystroke fetch loop, missing `try/catch` around network calls that permanently stick loading states, and one genuine hydration mismatch (WEB-1 … WEB-5).

| Severity | Count |
|---|---|
| High | 1 |
| Medium | 14 |
| Low | 17 |
| Nit | 15 |

**If you fix only five things:** AUTH-1 (rate limiting), AUTH-3 (token-type binding), API-1 (CardDAV SSRF), WEB-1 (pass the session to `SessionProvider`), and WEB-5 (UTC date on the public share page).

---

## 1. Auth & HTTP core (Go)

### AUTH-1 · HIGH ✅ · No rate limiting, throttling, or lockout on any auth endpoint
`internal/http/server.go:42-45` registers `POST /api/signup` and `POST /api/auth/login` bare; the only middleware in the chain is `withDB` (server.go:118).

**Impact:** unlimited online password brute-forcing, unlimited mass account creation, and a cheap CPU-exhaustion DoS — every login attempt against a *valid* email forces a bcrypt cost-12 verification (~200-300 ms CPU) that an attacker can trigger in parallel with trivial requests.

**Fix:** per-IP + per-account rate limiting (e.g. `golang.org/x/time/rate` middleware on `/api/auth/*` and `/api/signup`), plus failed-attempt backoff/lockout. At minimum, cap concurrent bcrypt verifications with a semaphore.

### AUTH-2 · MEDIUM ✅ · Login timing oracle enables email enumeration
`internal/auth/handlers.go:104-107`:
```go
u, e := db.NewUserRepo(...).FindByEmail(...)
if e != nil || u.Password == nil || !bcryptCheckImpl(req.Password, *u.Password) {
    failure(w, 401, "Invalid credentials")
```
The `||` short-circuits: unknown emails return 401 in ~1 ms, known emails pay ~250 ms of bcrypt. Registered addresses are enumerable from latency alone, compounding AUTH-1.

**Fix:** when the user is absent or password-less, still run `bcryptCheckImpl(password, dummyCost12Hash)` against a package-level precomputed dummy hash before returning 401.

### AUTH-3 · MEDIUM ✅ · Bearer assertions and 30-day cookie sessions are interchangeable — no token-type binding
`internal/auth/sessions.go:67-76`: `ReadSession` runs the identical `VerifySession` (same HS256 secret, `iss=computer`, `uid` claim) for both the `Authorization: Bearer` channel and the `computer_session` cookie. Next signs 60-second assertions (`web/lib/api.ts:16-18`); Go signs 30-day tokens with the exact same shape (`SignSession`, sessions.go:31-46). No `typ`/`aud` claim scopes either token to its channel, and no test exercises the Bearer path at all.

**Impact:** the documented "60-second assertion" boundary is unenforced. A 30-day cookie JWT replayed as a Bearer token is fully accepted (long-lived credentials end up in upstream logs/tracing where Bearer tokens land), and a captured assertion is accepted as a cookie value.

**Fix:** add a distinguishing claim (`typ: "session"` vs `typ: "assertion"`, or distinct `aud`) in `SignSession`; have Next add it to assertions; validate per channel in `ReadSession`; reject `exp - iat > 120s` on the Bearer channel. Add tests for the assertion path.

### AUTH-4 · MEDIUM ✅ · `Secure` cookie flag defaults off, controlled by an undocumented env var
`internal/auth/sessions.go:109-111` (`os.Getenv("COOKIE_SECURE") == "1"`), duplicated inline in `internal/authguard/handlers.go:55`. `COOKIE_SECURE` appears nowhere in `internal/config/config.go` or `main.go`'s env documentation.

**Impact:** a TLS deployment whose operator doesn't know this variable exists sends the 30-day session cookie (and view-as cookie) over plain HTTP on any downgrade — full session theft. The duplicated read invites drift between the two cookies.

**Fix:** derive `Secure` from `NEXTAUTH_URL`'s scheme, centralize in `config.Config`, default to secure unless explicitly local-dev.

### AUTH-5 · MEDIUM ✅ · Sessions are irrevocable; signout only clears the client cookie
`internal/auth/handlers.go:129-135` — signout calls `ClearSessionCookie(w)` and nothing invalidates the JWT, which stays valid up to 30 days. Mitigant (verified): `AuthMiddleware` re-reads the user every request (`middleware.go:29-34`), so account deletion/role demotion takes effect immediately — but signout itself does not.

**Fix:** add a `tokenVersion`/`sessionEpoch` column, embed it in `SessionClaims`, compare during the per-request `FindByID` the middleware already performs; bump on signout and password change.

### AUTH-6 · LOW · Signup reveals operator-reserved admin emails
`internal/auth/handlers.go:52-55` returns a distinct 403 ("must be provisioned by the operator") for `ADMIN_EMAILS` members, letting any unauthenticated caller probe the operator list (`security_test.go:51` currently asserts this distinct 403).

**Fix:** return the same generic response used for existing users; log the reserved-email attempt server-side.

### AUTH-7 · LOW · `ADMIN_EMAILS` auto-promotion is one-way
`internal/auth/middleware.go:35-41` persists `SetRole(..., "ADMIN")`, but nothing demotes when the address is later removed from the env var.

**Fix:** document promotion as permanent, or record provenance (`role_source='config'`) and demote on the same middleware path.

### AUTH-8 · LOW · No startup validation that a signing secret exists
`main.go:38-42` starts with whatever `config.Load()` returns; `Secret()` may be `""`; `internal/crypto/crypto.go:30` still carries `// TODO: fail if the secret is empty`. The process runs "healthy" on `/healthz` while every auth request fails opaquely.

**Fix:** `log.Fatalf` in `main` when `cfg.Secret() == ""`; make `crypto.Key` reject empty input.

### AUTH-9 · LOW ✅ · Login conflates DB failure with invalid credentials
`internal/auth/handlers.go:104-107`: `if e != nil || ...` maps connectivity errors to 401. Signup correctly branches on `errors.Is(e, db.ErrNotFound)` (handlers.go:60-63); login should too — outages currently masquerade as auth failures in monitoring.

### AUTH-10 · LOW · Single secret, no key separation, weak KDF construction
`internal/crypto/crypto.go:31-34`: the AES-GCM key is raw `sha256.Sum256(secret)` of the same secret used as the HS256 HMAC key. One compromise breaks both session integrity and IMAP-password confidentiality. The byte format must stay compatible (AGENTS.md), so: for any future re-encryption migration, derive separate keys via HKDF with distinct `info` labels.

### AUTH-11 · NIT ✅ · Login Origin check mis-detects scheme behind TLS-terminating proxies
`handlers.go:91-95` uses `r.TLS != nil`; behind a proxy `r.TLS` is nil, so same-host HTTPS origins never match and login works only via the `NEXTAUTH_URL` branch. Fails closed, but breaks silently if `NEXTAUTH_URL` is unset. Honor `X-Forwarded-Proto` or require `NEXTAUTH_URL` in production.

### AUTH-12 · NIT · Assorted
- `config.Load()` re-reads/re-parses env on every request (`middleware.go:24`, `handlers.go:90,109,118`); `NewServer` receives a `*config.Config` that auth ignores. Inject it (also removes the `os.Setenv` data race the tests rely on).
- `main.go:59` ignores `srv.Shutdown` errors — a failed graceful drain exits 0.
- No minimum password length (see TEST-3).
- `db.FromContext` returns `nil` silently (`db.go:44`); a route mounted without `withDB` nil-panics at first query instead of failing cleanly.
- `HandleAuth` switches on path only, not method (handlers.go:115-138); harmless today because of the route registrations.

---

## 2. API handlers & integrations (Go)

**No SQL injection found.** Every value is a bind parameter; identifiers passed to `db.Quote` originate exclusively from server-side literals; all `ORDER BY` clauses are static. **No cross-user read/write/delete hole found:** every mutation funnels through `db.Update`/`remove`/`unshare`, which embed the owner column in `WHERE`, and 404s don't leak existence.

### API-1 · MEDIUM ✅ · Blind SSRF via CardDAV sync — no internal-IP filtering
`internal/handlers/misc.go:92-116`: `validURL` only checks scheme/host/userinfo, then a plain `http.Client` issues the `REPORT`:
```go
c := http.Client{Timeout: 20 * time.Second, CheckRedirect: ...ErrUseLastResponse}}
res, e := c.Do(req)
```
Any authenticated user can make the server request arbitrary internal hosts (`http://db:5432`, `http://169.254.169.254/…`). This is exactly what `preview.publicDial` defends against (preview.go:24-25), but this endpoint doesn't use it. Differing errors ("Could not connect" vs "rejected the CardDAV request", lines 118/123) give a port-open oracle; a 200/207 XML response is parsed into `Contact` rows the attacker can read back (limited exfiltration). Redirects are correctly not followed, and `target.Host != base.Host` (line 102) blocks host-swapping — those parts are good.

**Fix:** build the client with `Transport: &http.Transport{DialContext: preview.PublicDial}` (export and reuse it).

### API-2 · MEDIUM ✅ · IMAP host/port is an unfiltered dial primitive; `HandleImapCheck` fans out unbounded connections
- `internal/signals/signals.go:34`: `net.Dialer` dials user-supplied `host:port` with no internal-IP filtering; `tls:false` is accepted (plaintext IMAP, clear-text credentials). `HandleImapTest` (misc.go:239-263) gives any user a connect-anywhere timing oracle, and the 5-minute scheduler keeps reconnecting to whatever host is saved.
- `internal/handlers/misc.go:265-289` ✅: the watched-bookmarks query has **no LIMIT**, and `signals.Search` opens a **fresh TCP+TLS+LOGIN IMAP connection per bookmark** (signals.go:58-59) — N sequential mailbox logins inside one HTTP request with no deadline (slow-request DoS + hammering the mail provider).
- Empty `imapQuery` builds search criteria with **no conditions** (signals.go:74-80), so a legacy watch without a query matches the whole folder (capped at newest 50, signals.go:85-87); `syncLegacyWatch` (bookmarks.go:283-290) can create such a source.

**Fix:** apply `publicDial`-style private-range rejection to resolved IMAP hosts (or an operator allowlist); cap the query with `LIMIT`; reuse one IMAP connection per request; wrap in `context.WithTimeout`; skip sources with both `from` and `subject` empty.

### API-3 · LOW · Tag list leaks share metadata and cross-user usage counts to recipients ✅
`internal/handlers/tags.go:12`: the `WHERE` admits share recipients, but the `_count` and `sharedWith` subqueries are unscoped — a recipient learns every other user the tag is shared with (userIds, createdAt) and the total bookmark count across *all* users.

**Fix:** compute `sharedWith`/owner-wide `_count` only when `t."ownerId"=$1` (`CASE WHEN ...`); for recipients count only their own visible bookmarks.

### API-4 · LOW · Attaching a *shared* tag to your own bookmark exposes it to the whole share group
`internal/handlers/bookmarks.go:146-150` accepts any tag shared *with* you; `db.BookmarkVisible` (db/bookmarks.go:10-12) then makes your bookmark visible to **every** recipient of that tag. If Alice shares tag T with Bob and Carol, Bob tagging his private bookmark with T exposes it to Carol too.

**Fix:** decide the semantics explicitly. If shares are owner→recipient only, restrict the visibility clause to bookmarks owned by the tag's owner; otherwise document/surface group semantics in the UI.

### API-5 · LOW ✅ · Public bookmark projection includes `notes` and `dueDate` — deliberate, but under-documented
`internal/handlers/bookmarks.go:19-25` includes `notes`/`dueDate` in the unauthenticated projection, and the share page renders both (`web/app/share/bookmark/[id]/client.tsx:56,65`) — so this is a **feature**, not an accident. But `public.go` claims "private metadata is never loaded," and users toggling `isPublic` likely don't expect their free-text notes to become world-readable.

**Fix:** state exactly what will be published in the share/public-toggle UI; align the `public.go` comment; consider an explicit "include notes" toggle.

### API-6 · LOW · User search discloses every user's email to any authenticated user
`internal/handlers/misc.go:25-33`: two-character queries + `email ILIKE` let any user enumerate the full user base including emails. The share picker only needs `id` + display name.

**Fix:** match on `name` only (or exact email); return masked emails (`a***@domain`).

### API-7 · LOW · Pages list returns *all* pages with full markdown content, unbounded
`internal/handlers/pages.go:15-18`: `to_jsonb(p)` includes `content`; every sidebar/workspace load transfers the entire corpus (bookmarks, by contrast, are capped at 200 with keyset pagination). Also unbounded: `HandleAlertsCheck` (misc.go:18), `HandleContactsList` (misc.go:71), `HandleTagsList` (tags.go:12).

**Fix:** list projection without `content` (`to_jsonb(p)-'content'` + truncated excerpt); add pagination if corpora grow.

### API-8 · LOW · Date filtering depends on an unpinned Postgres session timezone
`internal/handlers/common.go:112-129` parses offset-less inputs (`"2006-01-02T15:04"`) as UTC into `timestamptz`; the list filter casts `b."dueDate"::date=$n::date` (bookmarks.go:75), which Postgres evaluates in the **session TimeZone** — nothing pins `TZ` in app or compose, so "due on date X" silently shifts for non-UTC deployments.

**Fix:** require an offset (or document UTC-only), and use `(b."dueDate" AT TIME ZONE 'UTC')::date` or pin the session timezone on connect.

### API-9 · LOW · Resource handling
- `internal/preview/preview.go:50-58`: a per-call `http.Transport` with zero-value `IdleConnTimeout: 0` leaks an idle socket per preview until GC — FD accumulation under load. **Fix:** package-level shared `Transport` (keeping `publicDial`), or `DisableKeepAlives: true`.
- `internal/signals/signals.go:38` ✅: `conn.SetDeadline(now+15s)` is never cleared, silently bounding *all* later commands (Select/Search/Fetch) to 15s from dial — shorter than the scheduler's 45s budget — and the ignored error hides failures. **Fix:** clear after login (`SetDeadline(time.Time{})`), check the error, rely on the context + per-command deadlines.
- `internal/preview/preview.go:36`: `publicDial` misses CGNAT `100.64.0.0/10` (Tailscale/GKE/some Docker setups) and `198.18.0.0/15`. **Fix:** explicit `netip.Prefix` rejections. (Check-then-dial uses the same resolved IP — no rebinding TOCTOU; redirects re-dial through `publicDial`. Those are right.)

### API-10 · LOW · Kanban column delete is O(n) round trips under a user-row lock
`internal/handlers/kanban.go:123-134`: loads **every** bookmark of the user as full JSON (notes, OG metadata, all columns), then issues one `UPDATE` per match — inside a transaction holding the `FOR UPDATE` user lock.

**Fix:** select only `id,"kanbanColumnId","kanbanStatus"`; better, push `ResolveColumnID`'s mapping ("explicit column → status-keyed column → first column") into a single set-based `UPDATE`.

### API-11 · NIT · Assorted
- `internal/handlers/bookmarks.go:32,78,93`: dead always-true predicate `$1::text IS NULL` with a hard-coded `nil` parameter — confusing port artifact; remove and renumber.
- `internal/kanban/kanban.go:11-28`: `GetColumns` takes a `FOR UPDATE` user-row lock on **every** columns GET, serializing concurrent board loads. Fast-path a count outside the transaction; lock-and-seed only when zero.
- `internal/handlers/misc.go:222-227,249-254`: any DB error (or decrypt failure after secret rotation) is masked as 400 "Password required" and never logged. Branch on `db.ErrNotFound`; pass the rest through as logged 500s.
- `internal/handlers/pages.go:108`: `PageBookmark` position insert (`max(position)+1`) lacks the `lockUser` taken everywhere else (pages.go:56) — concurrent adds can duplicate positions (display-order only).
- `internal/oidc/provider.go:26-42`: `HandleStatus` bypasses the shared JSON writer — no `Cache-Control: no-store` on an unauthenticated endpoint. (The 501 login/callback stubs correctly preserve the client-owned boundary.)
- `internal/signals/signals.go:139`: `RunSignalChecks` re-runs `LoadMailbox` (DB + AES-GCM decrypt) per source inside the loop — hoist it. (No scheduler race found: single loop goroutine, dropped ticks, per-user 45s contexts with `cancel`, `ON CONFLICT` dedup.)
- Seed script (`internal/scripts/seed.go:13-36`) reviewed: safe — never mutates existing accounts, admin only via `ADMIN_EMAILS` match, bcrypt 12.

---

## 3. Frontend (Next.js)

**Security architecture verified sound:** the browser never supplies trusted identity (`lib/api.ts` signs `uid` from the server-side session only); the proxy allowlists forwarded response headers and pins `no-store`; the 60s assertion never reaches the browser; no `dangerouslySetInnerHTML`/`innerHTML`/`eval` anywhere; every `<a href={b.url}>` is backed by Go's `validURL` (http/https only); `ReactMarkdown` runs without `rehype-raw` and sanitizes link URLs; `auth.ts:37-42` explicitly rejects the `authelia` provider, honoring the OIDC stub boundary.

### WEB-1 · MEDIUM ✅ · SSR output of every app screen is thrown away
`components/providers.tsx:10` creates `<SessionProvider>` with no `session` prop, so during SSR `useSession()` has no data and `components/app-shell.tsx:120` (`if (!session?.user) return null`) discards the server-rendered `children`. Every `(app)` screen is effectively client-only with a blank flash, then waits for `/api/auth/session` + `/api/me` client fetches.

**Fix:** `const session = await auth()` in `app/(app)/layout.tsx` → pass into `<SessionProvider session={session}>` (through `AppShellWrapper`/`Providers`), or render `children` unconditionally and gate only the chrome.

### WEB-2 · MEDIUM ✅ · Bookmark search: full paginated double-fetch per keystroke, no abort or ordering guard
`app/(app)/bookmarks/client.tsx:23-38`: `setTimeout(load, 0)` is not a debounce — every keystroke re-runs `load`, which crawls **all** cursor pages twice (`fetchBookmarks` loops until `X-Next-Cursor` is exhausted, `lib/bookmarks.ts:8-18`) including a `shared=true` fetch that doesn't even depend on `search`. Nothing aborts or sequence-guards in-flight requests, so a slow response for `"re"` can overwrite the results for `"red"`. The pages screen solves exactly this with `loadSequence`; this screen doesn't.

**Fix:** debounce ~250 ms, pass an `AbortSignal` (the `signal` parameter already exists and is unused here), add a request-sequence guard, load `shared` once.

### WEB-3 · MEDIUM ✅ · Settings actions: network failure permanently sticks spinners; unhandled rejections
`app/(app)/settings/client.tsx:26-66`: `saveImap`, `testImap`, `syncContacts` have no `try/catch/finally` — a rejected `fetch` (or `await res.json()` on a non-JSON body) leaves `savingImap`/`testingImap`/`syncingContacts` true forever (permanent button spinners) plus an unhandled rejection.

**Fix:** `try { … } catch { toast.error(…) } finally { setSaving(false) }`; guard `res.json()`.

### WEB-4 · MEDIUM · Same missing-error-path class elsewhere
- `app/(app)/contacts/client.tsx:29-34`: rejected `fetch` skips `setLoading(false)` → infinite skeleton.
- `app/login/client.tsx:21-31`: `signIn` **rejects** on network failure (verified in `next-auth/react.js`) — no `try/catch`, so the login form sticks in loading with no toast.
- `components/sidebar.tsx:68-75` (`createPage`), `app/(app)/calendar/client.tsx:134`, `tags/client.tsx:113` (per-keystroke user search, no debounce), `app-shell.tsx:67-70,115-118` (`exitViewAs`, `dismissAlert` — which optimistically removes the alert even when the DELETE fails).

**Fix:** a small shared `safeFetch` helper or `try/catch` + toast at each site; `try/finally` around loading flags; debounce user-search inputs (the pages screen's 250 ms pattern is the model).

### WEB-5 · MEDIUM ✅ · Public share page: SSR'd date without `timeZone` → hydration mismatch + off-by-one day
`app/share/bookmark/[id]/client.tsx:64-66`: `new Date(bookmark.dueDate).toLocaleDateString('en-US', {...})` with no `timeZone`. This page is server-rendered **with data**, and Go stores UTC (`common.go:123-126`), so `2026-10-08T00:00:00Z` renders "October 8" on the server and "October 7" in any UTC-negative browser — a React hydration error *and* a wrong date. The repo's own `safe-format.tsx` exists for this, but `eslint.ssr.config.mjs`'s `no-bare-locale-format` rule only flags zero-argument calls, so it slips lint.

**Fix:** `<SafeDate …/>` or `{ timeZone: 'UTC' }`; extend the lint rule to catch locale options without `timeZone`. (Client-fetch screens have the same pattern but only render post-fetch — still worth `timeZone: 'UTC'` for date-only values.)

### WEB-6 · MEDIUM · Kanban drag-and-drop has no keyboard path
`app/(app)/kanban/client.tsx:155-160`: cards are `draggable` `<div>`s; HTML5 DnD is mouse-only, so keyboard/AT users cannot move a card at all. Same clickable-`div`-without-`role`/`tabIndex` pattern on calendar day cells (`calendar/client.tsx:102-104`), contact rows (`contacts/client.tsx:71-73`), tag rows (`tags/client.tsx:80-85`), bookmark table rows (`bookmarks/client.tsx:109`).

**Fix:** add a "Move to →" action per card (or Arrow-key handlers with `role="button"`/`tabIndex=0`); convert rows/cells to `<button>` or add role + tabIndex + key handlers.

### WEB-7 · LOW · Due-alert notifications re-fire every 5-minute poll until dismissed
`components/app-shell.tsx:73-93`: Go's `HandleAlertsCheck` returns every alert with `NOT "alertSent"`, and `alertSent` is only set by explicit dismiss — so the interval re-notifies the same alerts indefinitely while the tab is open.

**Fix:** track notified IDs in a `useRef<Set>` and skip already-notified ones per session.

### WEB-8 · LOW · Delete reports success unconditionally
`app/(app)/bookmarks/client.tsx:134`: `await fetch(DELETE); toast.success('Deleted')` regardless of 403/404/502; the row silently reappears after reload.

**Fix:** check `res.ok` first (pattern already in `bookmark-detail-sheet.tsx:114-116`).

### WEB-9 · LOW · Dead `BookmarkDetailSheet` mounted in the app shell
`components/app-shell.tsx:53,145-149`: `setSelectedBookmark` is only ever called with `null` (verified by grep), yet the sheet — which fires 4-6 fetches when `bookmark` changes — is mounted on every screen alongside each screen's own sheet.

**Fix:** delete the shell-level sheet and its state.

### WEB-10 · LOW · Settings page derives the Authelia redirect URI from client-controllable headers
`app/(app)/settings/page.tsx:9-11` ✅ builds `callbackUrl` from `x-forwarded-host`/`x-forwarded-proto`. If the deployment's proxy doesn't strip/override these, header poisoning shows an admin an attacker-hosted redirect URI to paste into Authelia (token-theft setup). React escapes the text — no XSS; this is trusted-configuration display only.

**Fix:** derive from `NEXTAUTH_URL`/`AUTH_URL` (what `auth.ts` trusts); headers only as dev fallback.

### WEB-11 · LOW · Proxy origin check: skipped without `Origin`; fragile env comparison
`app/api/[...path]/route.ts:10-15` ✅: mutating requests with no `Origin` header pass unchecked (SameSite=Lax is the real barrier — this is defense-in-depth), and `process.env.NEXTAUTH_URL` is compared as a raw string (breaks with trailing slash/path; NextAuth v5 prefers `AUTH_URL`).

**Fix:** also reject `Sec-Fetch-Site: cross-site`; compare `new URL(envUrl).origin`.

### WEB-12 · LOW · `authorize()` dereferences `credentials` unguarded
`web/auth.ts:26-29` ✅: an empty POST to `/api/auth/callback/credentials` can invoke `authorize` with `credentials === undefined` → `TypeError` → 500 on an auth endpoint.

**Fix:** `if (!credentials?.email || !credentials?.password) return null`.

### WEB-13 · LOW · Detail sheet fires 6 requests per open; screens re-crawl everything
`bookmark-detail-sheet.tsx:46-54` + `bookmark-extras.tsx:17-24`: `/api/me` (already fetched by `AppShell`), columns, contacts, tags, the full bookmark (caller already has it), and signals — none aborted on rapid open/close. And every screen aggregates the *entire* collection 200-at-a-time per load (`lib/bookmarks.ts` — correct for the contract, heavy at thousands of items).

**Fix:** hoist shared reference data into a cache (React Query is already a dependency) or pass it down; consider server-side aggregate endpoints if collections grow.

### WEB-14 · NIT · Assorted
- **Inverted secret precedence** ✅: `auth.ts:9` reads `AUTH_SECRET || NEXTAUTH_SECRET`; `lib/api.ts:14` reads `NEXTAUTH_SECRET || AUTH_SECRET`. Harmless unless both are set *differently* — then assertions fail Go validation with no obvious symptom. Pick one order.
- **`HEAD` handled but not exported** ✅: `route.ts:10,19` treat `HEAD` as bodyless, but line 33 doesn't export it → Next's 405. Export `proxy as HEAD` or drop the dead branches.
- **Go's OIDC endpoints are unreachable from the browser**: NextAuth's catch-all owns `/api/auth/*`, so `server.go:49-52`'s stubs can never be hit from the frontend — latent 404 for whoever implements Authelia. Re-route (e.g. `/api/oidc/*`) or document.
- **Large final edit can be lost on tab close**: `pagehide` flush is fire-and-forget and PATCH sets `keepalive` only under 60 000 bytes (`pages/client.tsx:46`) — bigger bodies are cancelled mid-flight (sessionStorage mitigates same-tab only).
- **Hardcoded `INITIAL_DATE = new Date(2026, 9, 1)`** (`calendar/client.tsx:14-18`): hydration-safe but first paint shows Oct 2026 with Oct 1 as "today"; `today` goes stale across midnight. Render via `ClientOnly` + set `new Date()` post-mount.
- **No security headers** (`next.config.js`): no CSP/`frame-ancestors`/`referrer-policy`/`nosniff` despite rendering user-supplied URLs and remote `ogImage`/`favicon`. Cheap win via `headers()`.
- **"Stop sharing with 3f2a9c1b…"** (`tags/client.tsx:115`): Go returns raw `TagShare` rows, so the UI can only show an ID prefix. Join `name`/`email` like `shared-dates` does.
- **Dead weight in `web/package.json`**: unused `mapbox-gl`, `plotly.js`, `react-plotly.js`, `chart.js`, `formik`, `yup`, `swr`, `zustand`, `jotai`, `react-hot-toast`, …; `@hello-pangea/dnd: ^18.0.1` is the only floating range in an exactly-pinned manifest; dead `components/layouts/*` (name-colliding with the real `app-shell.tsx`), `hooks/use-toast.ts`, `ui/toast.tsx`/`toaster.tsx` (app uses Sonner). (Prisma deps: see INFRA-3.)

### Frontend verified non-issues (worth recording)
- No memory leaks: every listener/interval inspected is cleaned up (`app-shell.tsx:92`, `top-bar.tsx:32`, `sidebar.tsx:65`, `pages/client.tsx:60-64` incl. flush-on-unmount, all screen effects, `chunk-load-error-handler.tsx:46`).
- No `window` access during render; `client-only.tsx`'s `useSyncExternalStore` shim is correct; `suppressHydrationWarning` used appropriately in settings.
- API contract matches verified end-to-end: `_count.bookmarks`, `X-Next-Cursor` pagination, `/api/me` shape, `shareUserIds`, `tagIds: undefined` omission semantics, IMAP `••••••••` sentinel, `admin/page.tsx` using `getRealUser()` for the role gate (proper view-as semantics).

---

## 4. Database & migrations

**Consistency checks that PASSED (verified):**
- **No `schema.sql` ↔ `upgrade.sql` drift**: the only post-init change added `Bookmark_ownerId_updatedAt_id_idx` to *both* files; upgrade.sql's three operations all correspond to objects in schema.sql.
- **`upgrade.sql` is genuinely idempotent and atomic**: `BEGIN`/`COMMIT` + `ON_ERROR_STOP=1` (compose:29) means mid-file failure rolls back completely; the `pg_constraint` guard, `NOT EXISTS` + `ON CONFLICT DO NOTHING`, and `CREATE INDEX IF NOT EXISTS` are each correct; `migration_test.go:21-25` executes the file **twice** and asserts no duplication. On migrate failure, `service_completed_successfully` (compose:36) prevents the app from starting.
- **Init ordering is guaranteed**: the db healthcheck (`pg_isready -h 127.0.0.1`) cannot pass during initdb (temporary server listens on the socket only), so `schema.sql` always applies before `migrate`.
- **e2e cleanup cascade is complete**: every FK referencing `"User"` is `ON DELETE CASCADE`; the two `SET NULL` references (`contactId`, `kanbanColumnId`) null out safely — one delete removes all QA data.

### DB-1 · LOW · Missing composite/partial indexes for three verified query patterns
- `SharedDate` join in `db/bookmarks.go:12` (`creatorId`+`recipientId`+`date`) runs per bookmark row; only single-column indexes exist (schema.sql:242-244). Add `CREATE INDEX ON "SharedDate"("creatorId","recipientId",date)`.
- Alerts check (misc.go:19-21) filters `ownerId + alertAt<= + NOT alertSent`; add partial index `ON "Bookmark"("ownerId","alertAt") WHERE NOT "alertSent"`.
- `ILIKE '%q%'` in user search can't use B-trees; fine at family scale, `pg_trgm` if it grows.

(Otherwise index coverage is good: owner, `dueDate`, `kanbanColumnId`, and the keyset `(ownerId, updatedAt DESC, id DESC)` pagination index all exist.)

### DB-2 · NIT · Assorted
- `schema.sql:200`: `SignalSource.config json` should be `jsonb` (Prisma's `Json` maps to `jsonb`); additive `ALTER … TYPE jsonb USING config::jsonb` in upgrade.sql.
- `schema.sql:226`: `UNIQUE("sourceId","externalId")` — NULLs are distinct, so rows with NULL `sourceId` never dedup. Safe today (only writer sets both, signals.go:155); consider `NULLS NOT DISTINCT` (PG15+) if manual signals land without a source.
- `schema.sql:33`: `Account."expires_at" int` is int4 (2038). Matches upstream NextAuth adapter — leave, but don't port forward blindly.
- `upgrade.sql:5`: `ADD CONSTRAINT … FOREIGN KEY` validates under `SHARE ROW EXCLUSIVE` (brief write block). Fine at this scale; `NOT VALID` + `VALIDATE` for large tables.
- `tags.go:21`: names are trimmed but not case-folded, so `UNIQUE(name,"ownerId")` permits "Work" and "work". If case-insensitive tags are intended, normalize in the handler + `lower(name)` unique index in upgrade.sql.

---

## 5. Infrastructure, build & tests

### INFRA-1 · MEDIUM ✅ · `make test` silently skips the entire integration suite
`Makefile:14-15` runs `go test -race ./internal/...`, but every DB test skips without `TEST_DATABASE_URL` (`integration_test.go:26-28` ✅). A green `make test` can hide total integration breakage. Meanwhile the compose `tests` service drops `-race` (`docker-compose.yml:91`) — the two "full" paths each test something the other doesn't.

**Fix:** add `-race` to the tests-service command; make `make test` warn loudly (or fail) when `TEST_DATABASE_URL` is unset.

### INFRA-2 · MEDIUM · Weak default database password in four places
`docker-compose.yml:9,26,38,88`: `${POSTGRES_PASSWORD:-computer}`. Without `.env`, the stack comes up fully functional with password `computer` (only `AUTH_SECRET` has a `:?` guard). Localhost-only port bindings limit exposure to the compose network + host.

**Fix:** `${POSTGRES_PASSWORD:?}` like AUTH_SECRET, or a prominent README quickstart warning. Related: `${AUTH_SECRET:?…}` interpolation also blocks db-only commands (`make db-up`/`migrate`/`db-shell`) on a fresh clone — document that `.env` must exist first.

### INFRA-3 · MEDIUM ✅ · Stale Prisma artifacts and a misleading `web/.env.example`
`web/package.json:26,35,43` still depend on `prisma`, `@auth/prisma-adapter`, `@prisma/client`; `web/prisma/schema.prisma` survives; `web/.env.example` instructs `DATABASE_URL="postgresql://…/linkos…"`. Verified: **zero** Prisma/`DATABASE_URL` references in `web/app`, `web/lib`, `web/auth.ts`, `web/components` — Go owns persistence. An operator following `web/.env.example` wires a phantom database, and the Docker `deps` stage installs three unused heavy packages.

**Fix:** delete `web/prisma/`, drop the three deps, rewrite `web/.env.example` to only `AUTH_SECRET`/`NEXTAUTH_URL`/`API_BASE_URL`/`OIDC_*` (or delete it and point to the root `.env.example`).

### INFRA-4 · LOW · Real `.env` is mounted into the tests container ✅
`docker-compose.yml:85` (`.:/src:ro`) exposes the production `AUTH_SECRET` and `POSTGRES_PASSWORD` to every `go test` run; tests set their own secret via `t.Setenv`, so the mount is unnecessary exposure.

**Fix:** mount only what's needed (`./internal`, `./db`, `go.*`, `main.go`) or exclude `.env*`.

### INFRA-5 · LOW · Unpinned images and package manager
- Base images float at minor/major tags: `golang:1.23-alpine`, `node:22-alpine` ×3, `postgres:16-alpine` ×2 — builds aren't reproducible and `postgres:16-alpine` can change minor version under a live volume. Pin at least patch level or digests (Go tags in `Dockerfile:1` and the tests service currently match — keep them in lockstep).
- `web/package.json` has no `packageManager` field, so `web/Dockerfile:4` (`yarn install --frozen-lockfile`) resolves corepack's default (yarn 1.22.x) while `web/.yarnrc.yml` is Berry-format config yarn 1 ignores — works by accident. Add `"packageManager": "yarn@1.22.22"` (or migrate to Berry + `--immutable`) and delete or honor `.yarnrc.yml`.

### INFRA-6 · LOW · Test-tooling robustness
- `scripts/e2e.mjs:82-85`: an assertion/`execFileSync` failure **inside `finally`** masks the original test error and skips cleanup. Wrap cleanup in try/catch, log failures to stderr, re-throw only if the try block succeeded. (The `/^[a-z0-9]+$/i` injection guard before the `DELETE` is excellent — keep it.)
- `scripts/regressions.mjs:64-68`: free-port probe is TOCTOU-racy (port can be grabbed between close and `next start`). Retry the spawn on `EADDRINUSE`.
- `Makefile:18-19`: `make e2e` doesn't ensure `scripts/node_modules` — add a `[ -d scripts/node_modules ] || npm --prefix scripts ci` guard.

### INFRA-7 · LOW · Dead `DEMO_PASSWORD` config ✅
`internal/config/config.go:28,42` loads it; nothing reads `DEMODemoPassword`; `main.go:23` still advertises it. Dead env plumbing invites an operator to set a demo backdoor password that silently does nothing — or gets wired up later without review. Delete the field and update the comment.

### INFRA-8 · NIT · Assorted
- `.dockerignore` omits `scripts/` (14 MB of `node_modules` uploaded to the daemon per build), `*.md`, `Makefile`, `docker-compose*` — context-transfer waste, not a layer leak (Dockerfile COPYs only `go.mod`, `go.sum`, `main.go`, `internal`).
- `.env.example` doesn't document `PORT` (server.go:150), `DB_URL` fallback (config.go:14), or e2e's `BASE_URL`/`E2E_KEEP_DATA`.
- `web` service has no healthcheck while `app` does (operator visibility only).
- `docker-compose.full.yml` is a 2-line `include:` alias that will silently diverge if someone edits only one file — consider deleting it.

### TEST-1 · MEDIUM · Seven routes have zero test coverage at any level
Verified by grepping every route path across all 9 `_test.go` files:
- `GET /api/preview` — only `ParseHTML`/the SSRF dialer are unit-tested; the handler's auth/ownership/query plumbing isn't.
- `GET /api/bookmarks/{id}/markdown` — e2e asserts the link *element* exists, never fetches it.
- `GET /api/users/search` — untested, including the 2-rune guard.
- `PATCH /api/kanban/columns/{id}` — untested.
- `PUT /api/kanban/columns` success path — only the 400 case is tested.
- `PUT /api/pages` reorder — untested.
- `DELETE /api/tags/{id}` and `DELETE /api/signal-sources/{id}` owner-success — only the cross-user 404 is tested.

These are mutating/ownership-sensitive endpoints in the exact class where prior bugs lived (sharing revocation has its own test file precisely for that reason). **Fix:** ~15 lines in `TestApplicationFlows`/`sharing_test.go` covering tag delete (+ `BookmarkTag` cascade), column patch/reorder happy paths, pages reorder, users/search, and both markdown endpoints' content.

### TEST-2 · MEDIUM · Background signal scheduler has no test
No `_test.go` in `internal/signals/`. The legacy-watch backfill and the manual `action=check` path are tested, but `RunLoop` (started unconditionally in `main.go:53`) — interval, dedup under concurrency, error handling — is never exercised. **Fix:** refactor `RunLoop` to accept a tick function/interval; drive one tick against the in-memory IMAP server from `integrations_test.go`.

### TEST-3 · MEDIUM ✅ · No minimum password length, and boundary rules are untested
`internal/auth/handlers.go:40-51` rejects only `""` and `>72` bytes — a 1-character password is accepted. No test exercises the 72-byte cap or `mail.ParseAddress` rejection. **Fix:** minimum length ≥8 + a table test for 400 cases.

### TEST-4 · MEDIUM · No test for the Bearer-assertion path — the primary production auth channel
No test ever sets an `Authorization` header; nothing asserts that a 30-day cookie token is/isn't accepted as Bearer (AUTH-3), and `Secure`-flag behavior (AUTH-4) is untested. Given the Next↔Go bridge is how every browser request authenticates, it deserves first-class coverage.

---

## 6. Verified strengths (keep doing these)

1. **JWT handling is properly hardened** — HS256 pinned via `WithValidMethods`, issuer + expiry-required, empty-secret rejection on sign *and* verify, algorithm-confusion tested; the `role` claim is never trusted (DB reload per request), so demotions/deletions take effect immediately.
2. **Signup races handled at both layers** — unique-violation `23505` → 409 backstop, and first-admin election serialized with `pg_advisory_xact_lock` inside a transaction.
3. **Login CSRF defenses** — exact `application/json` requirement (415), Origin validation failing closed, assertions on both status codes *and* absence of `Set-Cookie` in `security_test.go`.
4. **Owner-scoping discipline** — every mutation embeds the owner column in `WHERE`; "not yours" and "doesn't exist" are indistinguishable 404s; identifiers reach `db.Quote` only from server-side literals.
5. **Bookmark pagination is textbook** — hard 200 cap, keyset cursor on `(updatedAt, id)`, fetch-one-extra for `X-Next-Cursor`, matching composite index; the client follows cursors with dedup + loop guard.
6. **`preview.publicDial` anti-SSRF design** — resolve once, dial the checked IP (no rebinding TOCTOU), redirects re-dial per hop, ≤5 redirects, 8s timeout, 1MB body cap. (Now extend the same treatment to CardDAV/IMAP — API-1/API-2.)
7. **Page autosave race discipline** (`lib/page-autosave.ts` + pages client) — revision counter with serial persist loop makes late-response clobbering provably impossible; `flush()` coalesces callers; sessionStorage drafts survive failed saves; sequence guards on load/select. The best-engineered part of the frontend — the model WEB-2 should copy.
8. **IMAP secret hygiene** — AES-GCM at rest, server-side password stripping (`-'password'`), `••••••••` sentinel, TLS 1.2 minimum with `ServerName` verification.
9. **Container hardening** — `FROM scratch` + numeric non-root user + CA certs/tzdata; `read_only`, `cap_drop: ALL`, `no-new-privileges`; localhost-only port binds; healthcheck reusing the app binary (scratch has no shell).
10. **Test-data isolation** — random `test_<id>` schemas created/dropped per test with production tables untouched; e2e deletes only regex-validated QA account IDs it created; regressions run against in-process fixtures with zero DB access.
11. **SSR-safety tooling** — `client-only.tsx`, `safe-format.tsx`, and a custom `eslint.ssr.config.mjs` with `noInlineConfig` (rules can't be suppressed) show a deliberate, self-enforced strategy; WEB-5 is a gap in the rule's coverage, not the approach.

---

## 7. Suggested action plan

**Immediate (security-relevant, small diffs):**
1. AUTH-1 + AUTH-2: rate-limit middleware on `/api/auth/*` + `/api/signup`; dummy-bcrypt on unknown-email login.
2. AUTH-3: `typ` claim binding assertion vs session tokens + reject `exp-iat > 120s` on Bearer + tests (closes TEST-4 simultaneously).
3. API-1/API-2: route CardDAV and IMAP dials through exported `preview.PublicDial` (plus CGNAT ranges, API-9); `LIMIT` + shared IMAP connection in `HandleImapCheck`.
4. AUTH-8/INFRA-7: fail fast on empty secret; delete `DEMO_PASSWORD`.
5. WEB-12: guard `authorize()` credentials.

**Short-term (correctness/UX):**
6. WEB-1: pass the server session into `SessionProvider`.
7. WEB-5: `timeZone: 'UTC'` (or `SafeDate`) on the share page + extend the SSR lint rule.
8. WEB-2/WEB-3/WEB-4: debounce + abort + sequence guard on bookmark search; `try/catch/finally` sweep across settings/contacts/login/sidebar/calendar/tags.
9. API-3/API-6: scope tag `sharedWith`/`_count` to owners; mask emails in user search.
10. INFRA-1/INFRA-3: `-race` in the tests service + loud skip warning; delete Prisma artifacts and fix `web/.env.example`.

**Medium-term (hardening & polish):**
11. AUTH-4/AUTH-5: `Secure` derived from `NEXTAUTH_URL` via `config.Config`; `tokenVersion` for revocable sessions.
12. TEST-1/TEST-2/TEST-3: close the seven-route coverage gap; scheduler tick test; password minimum + table tests.
13. WEB-6/WEB-7/WEB-8: keyboard-accessible kanban moves; per-session alert-notification dedup; `res.ok` checks before success toasts.
14. API-8/DB-1: pin UTC date casts; add the three composite/partial indexes (additive in `upgrade.sql`).
15. INFRA-5/INFRA-6: pin image tags + `packageManager`; harden e2e cleanup and port probing.
16. WEB-14 hygiene sweep: security headers, secret-precedence alignment, `HEAD` export, dead deps/components removal.

**Design decisions to make explicitly (no code until decided):**
- API-4: tag-share visibility semantics (group-wide vs owner→recipient).
- API-5: what the public toggle actually publishes (notes/dueDate) — surface it in the UI.

---

## 8. Remediation log

### 2026-10-08 — Immediate tier implemented

Fixed (all tiers verified: `go build`, `go vet`, `go test -race`, full
`make integration` suite, web `eslint` ×2 configs, `next build`, regressions,
stack rebuild + `make e2e` — all green):

- **AUTH-1** ✅ Rate limiting added (`internal/auth/ratelimit.go`): fixed-window
  per-IP caps (login 20/min, signup 10/hour) plus consecutive-failure lockout
  (5 failures per IP+email → 15 min), wired in `server.go`. Loopback peers are
  exempt by design (`:8080` binds to 127.0.0.1; all remote traffic arrives via
  the web container) — documented in code and AGENTS.md. 429s carry
  `Retry-After`. Unit-tested (`ratelimit_test.go`).
- **AUTH-2** ✅ Unknown/password-less accounts now pay a dummy cost-12 bcrypt
  comparison (`dummyPasswordHash` in `auth/handlers.go`), closing the
  enumeration timing oracle.
- **AUTH-3 / TEST-4** ✅ Channel binding: `SessionClaims.Type` (`typ`) —
  `SignSession` issues `typ=session`; `web/lib/api.ts` signs
  `typ=assertion`; `ReadSession` accepts assertions **only** on Bearer with
  `exp-iat ≤ 120s`, and session/legacy (no `typ`) tokens **only** on the
  cookie. Legacy 30-day cookies without `typ` keep working on their own
  channel. Covered by `TestTokenChannelBinding`; the Bearer path now also has
  live coverage through `make e2e`.
- **AUTH-8** ✅ `main.go` fails fast when no signing secret is configured;
  `crypto.Key` panics on empty secret (TODO resolved; callers already guard).
- **AUTH-9** ✅ Login now distinguishes `db.ErrNotFound` (401) from real DB
  failures (500 "Database unavailable").
- **API-1 / API-2 / API-9** ✅ New `internal/netguard` package centralizes the
  resolve→check→dial guard (same lookup feeds check and dial — no rebinding
  window) and additionally blocks CGNAT `100.64.0.0/10`, `198.18.0.0/15`,
  `192.0.0.0/24` and IPv4-mapped private addresses. `preview.publicDial`
  delegates to it; the CardDAV client now uses a shared netguard transport;
  IMAP `Connect` dials through it. `INTEGRATIONS_ALLOW_PRIVATE=1` is the
  documented opt-out for test fakes and deliberate LAN deployments (set in
  `testServerDB` only; production compose never sets it).
  `HandleImapCheck`: watched-bookmark query capped (`LIMIT 50`), single shared
  IMAP connection for the whole pass, 45s context deadline, empty queries
  skipped. `signals.SearchConn` refuses empty criteria (would otherwise match
  the entire mailbox). `Connect`'s 15s deadline is cleared after login and its
  error checked. `RunSignalChecks` loads the mailbox once and shares one
  connection across a user's sources.
- **INFRA-7** ✅ Dead `DEMO_PASSWORD` removed from `config.Config` and
  `main.go` docs.
- **WEB-12** ✅ `auth.ts` `authorize()` returns `null` on missing credentials
  instead of throwing a 500.
- Also fixed en passant: `main.go` logs `srv.Shutdown` errors (AUTH-12 nit).

**Behavioral note:** deployments must ship Go and web together — an old web
(assertions without `typ`) against a new Go rejects all Bearer auth. Compose
rebuilds both, so `docker compose up -d --build` is sufficient.

Remaining from the immediate tier: none.

### 2026-10-08 — Short-term tier implemented

All verified: `go build`/`vet`/`test -race`, full `make integration` (now
with `-race`), web `eslint` ×2 configs (0 errors), `next build` + TypeScript,
all 5 regressions, stack rebuild + `make e2e` — all green.

- **WEB-1** ✅ `(app)/layout.tsx` passes the server session through
  `AppShellWrapper` into a nested `<SessionProvider session={...}>`, so the
  signed-in shell and screens are server-rendered instead of discarded.
- **WEB-5** ✅ `timeZone: 'UTC'` added on the public share page, bookmark
  card, and bookmarks table; `no-bare-locale-format` in `eslint.ssr.config.mjs`
  now flags any `toLocaleDateString`/`toLocaleTimeString` call whose options
  lack `timeZone` (`toLocaleString` exempt — Numbers accept it without
  timeZone).
- **WEB-2** ✅ Bookmark search rewritten: 250 ms debounce, `AbortController`
  per load (using `fetchBookmarks`' previously unused `signal` param),
  sequence guard so stale responses can't clobber newer results, shared-tab
  list loaded once instead of per keystroke. **WEB-8** fixed en passant:
  delete checks `res.ok` before toasting success.
- **WEB-3/WEB-4** ✅ `try/catch/finally` sweep: settings IMAP save/test and
  Nextcloud sync (spinners can no longer stick; non-JSON bodies tolerated),
  contacts loader, login `signIn` rejection, sidebar `createPage`, app-shell
  `exitViewAs`/`dismissAlert` (alert now removed only on success), pages
  link-search effect; calendar/tags user search debounced (250 ms) with
  error-tolerant fetches.
- **API-3** ✅ `HandleTagsList`: `sharedWith` and the owner-wide `_count` are
  now owner-only (`CASE WHEN t."ownerId"=$1`); recipients see their own usage
  count and an empty share list.
- **API-6** ✅ `HandleUsersSearch`: emails must match exactly (no substring
  enumeration), names still `ILIKE`; returned emails are masked
  (`a***@domain`).
- **INFRA-1** ✅ Compose `tests` service switched to `golang:1.23` (Debian —
  alpine lacks gcc for `-race`) and runs `go test -race`; `make test` prints a
  loud skip warning when `TEST_DATABASE_URL` is unset.
- **INFRA-3** ✅ `web/prisma/` deleted; `prisma`, `@prisma/client`,
  `@auth/prisma-adapter` removed from `web/package.json` and `yarn.lock`
  regenerated (0 prisma entries); `web/.env.example` rewritten to the real
  web-app variables (no phantom `DATABASE_URL`); `web/README.md` setup and
  layout sections corrected.

E2e script hardened during verification: the save-then-navigate step raced
the create POST (Go fetches the preview before inserting, so a cancelled
request context aborts creation); the script now awaits the 201 response
before navigating.

### 2026-10-09 — Medium-term tier implemented

All verified: `go build`/`vet`/`test -race`, full `make integration`
(including the new coverage/revocation/scheduler tests), web `eslint` ×2
(0 errors; warnings 28→26 after dead-file removal), `next build` +
TypeScript with the pruned dependency set, all 5 regressions, stack rebuild
on pinned images + `make e2e` — all green.

- **AUTH-4** ✅ `Config.CookieSecure()`: explicit `COOKIE_SECURE` wins,
  otherwise an `https` `NEXTAUTH_URL` implies Secure. Both cookie writers
  (session, view-as) now use it; unit-tested (`config_test.go`).
- **AUTH-5** ✅ Revocable sessions: `"User".tokenVersion` (additive in
  `upgrade.sql` + `schema.sql`, default 1) is embedded as a `ver` claim in
  cookie JWTs; middleware and `/api/auth/session` reject stale versions;
  signout bumps the version (sign-out-everywhere semantics). Assertions and
  legacy typ-less cookies carry no `ver` and skip the check. Covered by
  `TestSignoutRevokesIssuedTokens` (replays the pre-signout cookie).
- **TEST-1** ✅ `coverage_test.go` closes all seven route gaps: preview
  handler (auth + parse), bookmark markdown content, users/search (name
  match, exact-email match, no email-substring enumeration, masked output,
  2-rune guard), kanban column PATCH + PUT reorder happy paths, pages
  reorder, tag delete with BookmarkTag cascade, signal-source delete.
- **TEST-2** ✅ `signals.RunOnce` extracted from `RunLoop`;
  `TestSignalSchedulerRunOnce` drives one tick against the in-memory IMAP
  server and asserts signal creation, `lastStatus="Checked"`, and dedup on a
  second tick.
- **TEST-3** ✅ Signup requires ≥8-character passwords (UI hint updated to
  match); `TestSignupValidation` table-tests all 400 paths without a DB.
- **WEB-6** ✅ Kanban cards: `role=button`, `tabIndex`, Enter/Space opens,
  Arrow Left/Right moves to the adjacent column (documented in the screen
  subtitle). Calendar day cells, in-cell bookmark chips, contact rows, tag
  rows and bookmark table rows got keyboard activation + focus rings.
- **WEB-7** ✅ Due-alert browser notifications fire once per alert per tab
  (`notifiedAlerts` ref) instead of every 5-minute poll.
- **API-8** ✅ Date filtering pinned to UTC on both sides
  (`AT TIME ZONE 'UTC'`) in the bookmarks list filter and the `SharedDate`
  visibility clause.
- **DB-1** ✅ `SharedDate(creatorId,recipientId,date)` composite and partial
  `Bookmark(ownerId,alertAt) WHERE NOT alertSent` indexes — additive in
  `upgrade.sql`, mirrored in `schema.sql`.
- **INFRA-5** ✅ Images pinned to patch level (verified tags exist):
  `golang:1.23.12-alpine3.22` (app), `golang:1.23.12` (tests),
  `node:22.23.3-alpine3.24` (web ×3), `postgres:16.15-alpine` (db+migrate).
  `packageManager: yarn@1.22.22` added; inert Berry-format `.yarnrc.yml`
  deleted.
- **INFRA-6** ✅ e2e cleanup wrapped in try/catch (cleanup failures no
  longer mask the test error; regex guard aborts the delete instead of the
  process); regressions retry up to 3 free ports on `EADDRINUSE`;
  `make e2e` auto-installs `scripts/node_modules` when missing.
- **WEB-14** ✅ Security headers (`nosniff`, `X-Frame-Options: DENY`,
  `Referrer-Policy`, `Permissions-Policy`) via `next.config.js` `headers()`
  — full CSP deferred (needs nonces for Next inline runtime). Secret
  precedence aligned to Go's order (`NEXTAUTH_SECRET || AUTH_SECRET`) in
  `auth.ts`. Proxy exports `HEAD`. Tag "Stop sharing" now shows the
  recipient's name/email (Go joins the user; F24). Dead code removed after
  import audits: `components/layouts/*` (STYLE_GUIDE.md updated),
  `hooks/use-toast.ts`, `ui/{toast,toaster,use-toast}`, and 34 unused
  packages (aws/azure SDKs, plotly, chart.js, mapbox, formik, yup, swr,
  zustand, jotai, react-hot-toast, @hello-pangea/dnd, open-graph-scraper,
  webpack, lodash, dayjs, dotenv, zod, react-is, @radix-ui/react-toast, …);
  `react-markdown`/`remark-gfm` pinned exactly; `yarn.lock` regenerated.

### 2026-10-09 — Low/nit backlog implemented

All verified: `go build`/`vet`/`test -race`, full `make integration`
(115s, race-enabled, restricted tests mount), web `eslint` ×2 (0 errors),
`next build` + TypeScript, all 5 regressions, stack rebuild + `make e2e`
(web now reports healthy via its new healthcheck) — all green.

- **AUTH-6** ✅ Signup against an `ADMIN_EMAILS` address now returns the
  same 409 "User already exists" as a duplicate (no probing oracle) and is
  logged server-side; `security_test.go` updated to assert 409.
- **AUTH-7** ✅ Documented as intended behavior (comment in
  `middleware.go`): promotion is persistent; removal from `ADMIN_EMAILS`
  does not demote — revocation is an explicit Admin-screen action.
- **AUTH-11** ✅ Login Origin scheme detection honors `X-Forwarded-Proto`
  behind TLS-terminating proxies (host must still match, so the header
  grants a spoofer nothing end-to-end).
- **API-10** ✅ Kanban column delete: loads only
  `id/kanbanColumnId/kanbanStatus` (was full-row JSON incl. notes/OG
  metadata), moves cards with one `UPDATE ... WHERE id=ANY(...)`, and
  repositions remaining columns with a single `unnest ... WITH ORDINALITY`
  statement — O(round trips) no longer scales with bookmark count while
  holding the user-row lock.
- **API-11 nits** ✅ Dead `$1::text IS NULL` predicate explained in place
  (renumbering would touch `db.BookmarkVisible`'s fixed `$2` convention
  everywhere); `kanban.GetColumns` fast-paths the common read outside any
  transaction (lock+seed only when count is zero, re-checked inside);
  IMAP save/test distinguish `db.ErrNotFound` (400 "Password required")
  from real DB/decrypt failures (logged 500); `PageBookmark` position
  insert now takes `lockUser` like every other position-assigning write;
  OIDC status/login/callback set `Cache-Control: no-store`.
- **AUTH-12 (partial)** ✅ `db.FromContext` panics with a clear
  "route missing withDB middleware" message instead of returning nil;
  `srv.Shutdown` error logging (previous batch). Per-request `config.Load`
  injection deferred (stylistic).
- **DB N1** ✅ `SignalSource.config` is `jsonb` (schema.sql for new
  installs; idempotent `ALTER ... TYPE jsonb USING config::jsonb` in
  upgrade.sql — migration_test runs it twice).
- **DB N2** ✅ Verified no writer can produce NULL `sourceId`
  (`RunSignalChecks` always sets it), so `UNIQUE(sourceId,externalId)`
  dedup is safe as-is; no change needed.
- **WEB-9** ✅ Dead shell-level `BookmarkDetailSheet` and its unreachable
  state removed from `app-shell.tsx` (screens mount their own sheets).
- **WEB-10** ✅ Settings Authelia redirect URI derives from
  `NEXTAUTH_URL`/`AUTH_URL` (same value `auth.ts` trusts); forwarded-header
  inference only as a dev fallback (`NODE_ENV !== 'production'`).
- **WEB-11** ✅ Proxy origin check parses the env URL (trailing
  slash/path can no longer break the match, `AUTH_URL` honored) and
  rejects `Sec-Fetch-Site: cross-site` when Origin is absent — while still
  permitting an explicitly allowlisted cross-origin.
- **WEB-13** ✅ New `lib/reference-data.ts`: 30s-TTL, in-flight-deduped,
  never-rejecting cache for the detail sheet's four reference fetches
  (`me`/columns/contacts/tags); failures aren't cached. Invalidated at
  every mutation site (tags screen reloads, dialog tag creation, settings
  contacts sync, all four kanban column mutations). Sheet opens now fire
  ~0-2 requests instead of 6.
- **L1** ✅ Compose `tests` service mounts only `go.mod`, `go.sum`,
  `internal/`, `db/` — the real `.env` (AUTH_SECRET, POSTGRES_PASSWORD) no
  longer reaches the test process.
- **L3** ✅ `POSTGRES_PASSWORD` now uses fail-fast `:?` interpolation like
  `AUTH_SECRET` (no silent `computer` default); `.env` was already
  mandatory for compose because of the AUTH_SECRET guard, so no workflow
  changes. `.env.example` documents it.
- **L7** ✅ `.dockerignore` excludes `scripts/` (14MB node_modules),
  `*.md`, `Makefile`, compose files from the Go build context.
- **N6** ✅ `.env.example` documents `PORT`, `DB_URL`,
  `TEST_DATABASE_URL`, `BASE_URL`, `CHROMIUM_PATH`, `E2E_KEEP_DATA`,
  `INTEGRATIONS_ALLOW_PRIVATE` (with a never-in-production warning) and
  the compose fail-fast interpolation behavior.
- **N9** ✅ `web` service healthcheck (`wget` on `/login`, start_period
  20s); `docker compose ps` now shows web health.

**Deferred with rationale:** AUTH-10 key separation (only meaningful
during a re-encryption migration), N1 config injection (stylistic),
N2 HandleAuth method switch (harmless), WEB-14 keepalive >60KB (drafts +
beforeunload guard mitigate), WEB-15 calendar INITIAL_DATE (hydration-safe;
mount effect corrects immediately), DB N3 int4 expires_at (matches
upstream adapter schema), DB N4 constraint validation lock (fine at this
scale), DB N5 tag case-folding (product decision), N8 compose.full.yml
(harmless documented alias), users/search pg_trgm (fine at family scale).

**Still open — needs product decisions:** API-4 (tag-share visibility:
group-wide vs owner→recipient) and API-5 (whether the public toggle should
keep publishing `notes`/`dueDate`, and how the UI should say so).
