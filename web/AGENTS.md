# AGENTS.md

Next.js 16 (App Router) app — "computer", a link OS. TypeScript, Prisma 6 + PostgreSQL, Tailwind v3, shadcn-style UI.

## Commands

- `yarn dev` — dev server on `http://localhost:3000`
- `yarn build && yarn start` — production build + serve (self-host behind a reverse proxy)
- `yarn lint` — `eslint .` (flat config; `eslint.config.mjs`)
- `yarn eslint -c eslint.ssr.config.mjs .` — SSR/hydration-safety lint, run automatically after each build. Do not delete/weaken it; fix flagged code instead.
- `yarn prisma generate` — regenerate client (required after schema changes)
- `yarn prisma db push` — apply schema to DB (non-destructive)
- `yarn prisma db seed` — runs `scripts/safe-seed.ts`, which guards `scripts/seed.ts` against `prisma.delete*` calls, then executes it. Use `SEED_EMAIL=... SEED_PASSWORD=... yarn prisma db seed` to create an initial user.
- No test framework exists (no test scripts, no CI config).

## Environment & setup

- Copy `.env.example` → `.env`. `DATABASE_URL`, `NEXTAUTH_SECRET` (== `AUTH_SECRET`, stable — changing it invalidates stored IMAP passwords), `NEXTAUTH_URL`.
- Node >= 20.9, Yarn Corepack (`corepack enable`), PostgreSQL 13+.
- `@/` is the path alias for the repo root (tsconfig paths).
- Two Prisma clients exist in `lib/`: `lib/prisma.ts` is the live one (29 imports). `lib/db.ts` is a duplicate — never edit it, never import it.
- Authelia OIDC is stubbed: provider only registers when `OIDC_ISSUER`, `OIDC_CLIENT_ID`, `OIDC_CLIENT_SECRET` are all real (non-placeholder). See `docs/AUTHELIA.md`.

## Key architecture / gotchas

- `app/layout.tsx` sets `export const dynamic = 'force-dynamic'` — every page is dynamic by default.
- Root layout is the single place for global infra: `ThemeProvider`, `Toaster`, `ChunkLoadErrorHandler`. Don't remove `ChunkLoadErrorHandler` — it handles a known Next.js dev chunk-load race.
- `next.config.js`: `typescript.ignoreBuildErrors: true` (type errors don't fail builds), `images.unoptimized: true`, dev `allowedDevOrigins` enumerated (no wildcard — adding a preview host requires an edit).
- `next.config.user.json` is an optional override file; only `skipTrailingSlashRedirect` / `trailingSlash` (booleans) are allowed.
- IMAP passwords are AES-256-GCM encrypted in `lib/crypto.ts`, keyed off `NEXTAUTH_SECRET`.
- Admins: first signup becomes admin, or list emails in `ADMIN_EMAILS`. Admins can view-as any user via `app/api/admin/view-as/route.ts` (sets `VIEW_AS_COOKIE`, 8h, httpOnly).
- `lib/auth-guard.ts` exposes `getRealUser` and `VIEW_AS_COOKIE`.

## SSR / hydration rules (enforced by `eslint.ssr.config.mjs`)

- No bare `window`/`document`/`localStorage`/`navigator`/`sessionStorage` at module scope — crashes or mismatches during SSR. Move into `useEffect`/handlers or wrap in `<ClientOnly>` (`components/client-only.tsx`).
- No `Date.now()` / `Math.random()` / `new Date()` in render or in `useState`/`useReducer`/`useMemo` initializers — hydration mismatch. Compute in `useEffect` or use `<SafeDate>`/`<SafeTime>`/`<SafeNumber>` (`components/safe-format.tsx`), which format with explicit `en-US` + `timeZone: 'UTC'`.
- No bare `toLocaleDateString()`/`toLocaleTimeString()`/`toLocaleString()` — pass explicit locale/timeZone.

## Style

- Follow `STYLE_GUIDE.md` (design tokens, layout components, component inventory). Tailwind font classes: `font-sans` (DM Sans), `font-display` (Plus Jakarta Sans), `font-mono` (JetBrains Mono).
- `package.json` has no Prettier config; formatting is governed by ESLint's `eslint-plugin-prettier`.

## Notes

- No `.gitignore`, no git history, no CI, no pre-commit hooks in this repo.
- `yarn.lock` is Yarn Berry-style (`nodeLinker: node-modules` in `.yarnrc.yml`).