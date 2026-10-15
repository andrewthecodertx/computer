# Computer — Where We Stand and What's Next

A plain-language look at the app: what it does today, what it doesn't do
yet, and what we can build over the next 3–6 months.

*Written October 2026. The big assumption in this plan: **Authelia** (a
self-hosted login server) becomes the way people sign in, and the app stops
keeping its own passwords.*

---

## What we have today

**Saving and finding links**

- Save any URL with a title, notes, a due date, and tags.
- Saved links show a preview (page title, description, small icon) so lists
  are easy to scan.
- Search and filters: by text, tag, contact, board column, date, or what's
  shared with you.
- Export any bookmark as a Markdown file.

**Writing**

- Pages: Markdown documents with a `/` menu (headings, to-do lists, quotes,
  code) and checkbox shortcuts.
- Autosave that is safe: if two edits collide, the app stops and asks what
  to do. It never silently throws away writing.
- Pin pages, reorder them, link bookmarks into them.

**Projects and views**

- Collections: Notion-style projects where the rows are your bookmarks.
  Add columns (text, number, select, date, checkbox, URL), save filtered
  table views, or use a drag-and-drop board view.
- A kanban board over all bookmarks, with custom columns.
- A calendar of due dates, including dates teammates shared with you.
- Ctrl/Cmd+K quick-find jumps to anything.

**Sharing**

- Share one bookmark, a whole tag, or a calendar date.
- Public links: publish a bookmark to anyone with the URL. The app tells you
  exactly what becomes public before you turn it on.
- Shared people see a trimmed version — your contacts, alerts, and mail
  settings stay private.

**Email and contacts**

- Email watchers: connect a mailbox and get a signal on a bookmark when
  mail arrives from a sender or matching a subject. Mail passwords are
  encrypted; connections must use proper TLS.
- Due-date alerts for bookmarks coming due in the next 15 minutes.
- Contact sync from Nextcloud (CardDAV); attach people to bookmarks.

**Admin and safety**

- Multi-user with two roles (USER, ADMIN).
- Admins can "view as" another user — read-only, time-limited, and logged.
- Guard rails: you can't demote yourself or the last admin; the first
  signup becomes admin once, and open registration can be turned off.
- Signing out really signs you out everywhere.
- Logins are rate-limited so attackers can't guess passwords at speed.
- The server refuses to fetch hidden internal network addresses (a common
  self-hosted app attack).
- Database upgrades are checksummed and atomic; a release gate runs real
  tests on every change.

---

## What is missing today

**Login and accounts**

- No "forgot password" — a locked-out user needs someone to edit the
  database by hand. This is the biggest gap.
- No two-factor authentication (2FA).
- No single sign-on (logging in with an existing account system).
- No email verification.

**Product features**

- Collections can't be shared — they're private to their owner.
- No page history (can't see or restore older versions).
- No trash — deletes are permanent.
- No nested pages or backlinks ("which pages mention this one?").
- Calendar and list views for collections are stored but still display as
  tables.
- Alerts only show inside the app — nothing is emailed or pushed to you.
- Email is the only watcher type (no RSS feeds, no webhooks).

**Technical debt** (from a recent independent review — nothing critical,
16 small/medium items left)

- The network-address blocker misses some rare IPv6 tricks, and outbound
  fetches aren't limited to normal ports.
- No size limits on text fields (a user could paste 1 MB into a title).
- Page search treats `%` and `_` as wildcards and has no index — slow and
  surprising on big installs.
- The email library is on an unmaintained version; the login library
  (NextAuth) is still a beta.
- Database tests silently skip when the test database isn't configured;
  the web (React) code never got a line-by-line review; web libraries have
  never been audited.
- Vulnerability scanning exists (`make audit`) but isn't run automatically.

**Operations**

- Backup and restore aren't documented or tested as a first-class thing.
- No version numbers or changelog — clients update by pulling the latest
  main branch.
- Only works as one server instance (fine for now, but it's an unstated
  assumption).

---

## What we can do over the next 3–6 months

### The next 3 months: switch the login, shore up the base

**1. Move sign-in to Authelia.** This one move fixes several gaps at once:

- No more stored passwords — so no password-reset problem to solve.
- 2FA (authenticator apps, security keys) comes free with Authelia.
- One login can work across several apps (single sign-on).
- A lot of our own login-protection code can be deleted.
- Before building, we need four decisions:
  - **Break-glass:** keep one special local admin for "Authelia is down"
    emergencies? (Strongly recommended: yes.)
  - **Lockout speed:** when someone is disabled in Authelia, how fast must
    the app kick them out? (Their existing session can outlive the
    disable unless we design for it.)
  - **Sharing:** today you can share with an account that exists; with
    Authelia, accounts appear on first login. Do we pre-create people by
    email, or only allow sharing with people who have logged in once?
  - **Roles:** should Authelia groups decide who is an admin, or keep
    roles managed inside the app? (Start with: inside the app.)

**2. Finish the review backlog** (about 2–3 weeks of work):

- Block the rare IPv6 tricks; limit outbound fetches to normal ports.
- Add size limits to text fields.
- Fix page search (escape wildcards, add an index).
- Small error-handling cleanups; make partial email-check results show up
  instead of nothing.

**3. Update the two aging libraries:**

- Email (IMAP) library to its maintained version.
- NextAuth to a stable release — this becomes urgent, since it will be the
  only front door.

**4. Make the safety net automatic:**

- Vulnerability scanning on every CI run.
- Database tests fail loudly in CI instead of skipping.
- A review pass over the web/React code (never reviewed line by line).

**5. Operations basics:**

- Document and test backup + restore.
- Start tagging releases with version numbers and a changelog.

### 3–6 months: grow the product

- **Page history and trash** — see old versions, restore them; deleted
  things are recoverable.
- **Nested pages and backlinks** — a real page tree, and "what links here"
  that never shows titles you're not allowed to see.
- **Collection sharing** — the big one. Explicit, shown in the UI, and
  revocable. Design the permission rules first (a project must never
  accidentally widen who can see a bookmark), then build it. Once it
  exists, cross-project "relation" columns can follow.
- **Alerts that reach you** — email or webhook when a due date hits or an
  email watcher matches. Turns the app from a dashboard into a notifier.
- **RSS watchers** — "tell me when this feed mentions X." Reuses everything
  the email-watcher system already does.
- **Real calendar and list views** for collections; sorting by more than
  one column.
- **Authelia single sign-on for the client install** (the OIDC stubs
  currently in the code get removed, not implemented — NextAuth handles it).

### Things we are choosing NOT to do

- No block editor — Markdown first.
- No real-time collaboration, formulas, rollups, plugins, or AI-made
  changes.
- No automatic/project-wide magic collections — membership stays explicit.
- No local passwords for regular users once Authelia ships (only the
  break-glass admin, if we keep one).
