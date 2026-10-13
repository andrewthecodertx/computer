# Web application

- Next.js App Router UI; Go owns persistence. Data requests go through
  `app/api/[...path]/route.ts` and `lib/api.ts`, not local Prisma routes.
- Keep NextAuth `/api/auth/*` local. Credentials call Go; the server proxy
  signs short-lived assertions for the authenticated session's ID. Never pass
  a browser-supplied user ID as trusted identity.
- `API_BASE_URL` is server-only Docker DNS (`http://app:8080`); the browser
  uses relative `/api/*` URLs. Shared dates and tags grant read-only access.
- Product semantics (decided, do not "fix" silently): tag sharing is
  group-wide — every recipient sees all bookmarks under a shared tag,
  including ones other recipients add; public bookmark links intentionally
  publish notes and dueDate but carry NO owner attribution (the README
  promises owner identity stays private). Keep the tags share panel and the
  detail sheet's public-toggle disclosure in sync with these rules.
- Bookmark API responses are permission-specific: shared readers receive a
  narrowed projection without contact, recipient list, alerts, mailbox
  watches or board placement — UI code must keep treating those fields as
  optional (see the `Bookmark` type in `components/app-shell.tsx`).
- Collections workspace (`app/(app)/collections/client.tsx`): membership is
  contextual and never grants access — revoked bookmarks vanish from item
  surfaces, so never assume an item implies visibility. Every mutating PATCH
  sends the expected `version`; a 409 reloads the server state instead of
  clobbering. The filter editor mirrors the server's per-type operator
  allowlists (`FILTER_OPS`); keep them in sync with
  `internal/handlers/collections.go`. Board views group by one select
  property; table/board edit the same membership rows, never copies.
- Page autosave (`lib/page-autosave.ts`): a version conflict is a distinct
  paused state. Recovery drafts persist their original base version, and
  drafts whose base is unknown (legacy storage) pause for review as well;
  typing never resolves a conflict, and only the banner actions (overwrite /
  save-as-copy / discard) may touch remote content. Successful saves refresh
  the stored envelope so recovery never sees a stale base. Content saves
  must carry `version` (the Go API rejects content PATCHes without it).
  Remote preview/favicon images always render with
  `referrerPolicy="no-referrer"`.
- Client-owned Authelia integration remains stubbed. `auth.ts` rejects OIDC
  sign-in until verified identities are mapped to local Go users. See
  `docs/AUTHELIA.md`; do not assume an OIDC subject is a database user ID.
- From this directory: `yarn lint`, `yarn build` (includes type checking),
  `yarn eslint -c eslint.ssr.config.mjs .`. Fix hydration failures rather than
  weakening the SSR config. `components/client-only.tsx` and
  `components/safe-format.tsx` provide safe patterns.
- Follow `STYLE_GUIDE.md` for design tokens and reusable UI components.
- Root layout keeps the providers, Sonner and `ChunkLoadErrorHandler`.
- Standalone Docker runtime needs `.next/static` and `public` alongside
  `server.js`; do not copy host `node_modules`, build output or `.env` files.
