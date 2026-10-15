# Computer — What It Does and Where It's Going

Computer is a self-hosted bookmark workspace for teams and individuals.
Think of it as "every link you save, plus everything around it": notes,
previews, due dates, projects, boards, calendars, contacts, and even email
watchers — all in one place you control.

It runs entirely on your own hardware with Docker: a Go API, a Next.js web
app, and PostgreSQL. Nothing is sent to a third party.

---

## What you can do today

### Save and organize links

- Every bookmark holds a URL, a title, Markdown notes, a due date, and tags.
- When you paste a URL, Computer fetches a **live preview** — page title,
  description, and favicon — so lists are easy to scan.
- Find things fast: full-text search plus filters by tag, contact, board
  column, date, or share status. Long lists page smoothly, no matter how
  many bookmarks you have.
- Export any bookmark as a Markdown file.

### Write pages

Pages are Markdown documents for the thinking *around* your links — project
notes, meeting prep, reading lists.

- A Notion-style `/` menu inserts headings, to-dos, lists, quotes, and code
  blocks; typing `[]` + space starts a checkbox.
- Paste a bare URL and Computer offers to turn it into a nicely titled link
  (it asks first — it never silently rewrites what you typed).
- **Your work is protected.** Changes autosave, but if two people (or two
  tabs) edit the same page, Computer pauses and asks what to do —
  *Overwrite*, *Save as copy*, or *Discard*. It will never quietly throw
  away writing.
- Pin pages, reorder them, rename inline, link bookmarks into them, and
  export to Markdown.

### Build projects with Collections

Collections are like Notion databases, with one important twist: the rows
are **your existing bookmarks**, not copies. The same link can live in two
projects, each with its own properties.

- Add typed columns: text, number, single/multi select, date, checkbox, URL.
- Save **views** — a table with filters and sorts, or a board grouped by a
  select column. Dragging a card on the board is just editing a value; the
  table updates instantly.
- Filtering and sorting happen on the server, so views stay fast with lots
  of items.
- Collections are private to their owner for now, and they never leak
  access: if someone can't see the underlying bookmark, they can't see it
  through a collection either.

### Track work visually

- A **kanban board** over all your bookmarks, with custom columns. Deleting
  a column moves its cards — nothing is ever lost.
- A **calendar** showing bookmarks by due date, including dates shared with
  you by teammates.

### Share deliberately

- Share a single bookmark, a whole tag (everyone with the tag sees its
  bookmarks — the UI says so up front), or a calendar date.
- **Public links** publish a bookmark to anyone with the URL. The share
  panel tells you exactly what becomes public before you flip the switch,
  and public pages never reveal who owns them.
- Shared readers always see a trimmed-down version — your contacts, alerts,
  and email watchers stay private.

### Watch your inbox

This is Computer's most unusual feature. Connect an IMAP mailbox and attach
**email watchers** to bookmarks: "tell me when mail from this sender, or
matching this subject, arrives."

- Matching emails become **signals** on the bookmark, deduplicated
  automatically.
- Checking is incremental and gentle on your mail server — it remembers
  where it left off, works in bounded batches, and retries safely after
  failures.
- Mailbox passwords are encrypted at rest; connections require proper TLS.
- **Due-date alerts** surface bookmarks coming due in the next 15 minutes.

### Keep contacts in sync

Sync contacts from Nextcloud (or any CardDAV server) and attach them to
bookmarks — useful for "the link about the invoice from Acme." Credentials
are never stored, and sync requires HTTPS.

### Move fast in the UI

- **Ctrl/Cmd+K** opens a quick-find palette: jump to any page, bookmark,
  tag, or view, or run commands like "new page" and "add bookmark."
- Collapsible sidebar, breadcrumbs, light/dark themes.

### Administer a multi-user install

- Two roles: USER and ADMIN. Admins can **view as** another user to
  troubleshoot — strictly read-only, time-boxed to 8 hours, audit-logged,
  and ended by signing out.
- Safety rails: you can't demote yourself, and you can't demote the last
  admin. Role changes are logged.
- The first signup becomes admin (one-shot, so an empty install can't be
  hijacked later); registration can be disabled entirely, and admin
  addresses can be reserved for operator provisioning.

### Security you don't have to think about

A few things baked in, in plain terms:

- **Sessions are short-lived and revocable.** Signing out really signs you
  out everywhere.
- **Login is rate-limited per person**, not per server — attackers can't
  guess passwords at speed, and one bad actor can't lock out your whole
  team through the proxy.
- **The server doesn't fetch arbitrary internal addresses** when generating
  previews or talking to mail/contact servers, closing a common
  self-hosted-app attack (SSRF).
- **Database changes are checksummed and atomic** — upgrades can't leave a
  half-applied schema, and fresh installs provably match upgraded ones.
- The nginx deployment template ships with HSTS and a Content-Security-Policy.

---

## Where we're going next

### The next 3 months: polish and hardening

A recent independent code review rated the app free of critical and high
issues; the remaining items are hardening and cleanup:

- **Tighten the outbound-fetch guard** against exotic IPv6 forms and
  restrict which ports previews and IMAP may use.
- **Cap input sizes** (names, titles, notes, URLs) and improve small
  parsers: Markdown export escaping, contact (vCard) edge cases.
- **Speed:** an index-backed page search that treats `%` and `_` literally,
  a leaner kanban column delete, and memory hygiene in the email scheduler.
- **Mailbox checks that fail partway** should return what they found
  instead of nothing.
- **Testing and CI:** make database tests fail loudly in CI instead of
  skipping, add unit tests for the Markdown/kanban/vCard code, and run
  vulnerability scanning on every release check.
- **Dependencies:** move the IMAP library to its maintained v2 line and pin
  NextAuth to stable once it leaves beta.

### 3–6 months: connected pages and trust

The product roadmap ships in phases, and each phase only starts once the
previous one has passed its exit tests.

**Connected pages**

- Nested pages (a real hierarchy, with loops rejected).
- Internal links and **backlinks** — see which pages mention this one —
  without ever revealing titles you're not allowed to see.
- Embed a live collection view inside a page. Embedding follows the same
  permission rules as everything else: it displays, never publishes.

**Trust features**

- Server-side **revision history** for pages, with restore.
- **Trash and restore** — deleted things come back safely, and restoring
  never resurrects an expired share.
- Structured export (beyond per-item Markdown).
- **Collection sharing** — explicit, disclosed, and revocable. This is the
  big one: it's deliberately last because sharing must never become a way
  to widen access by accident. Once it lands, cross-collection **relation
  properties** can follow.

**Nicer views and notifications**

- Real calendar and list renderers for collection views (today they're
  stored but displayed as tables).
- Sorting by all stored sort keys, not just the first.
- Alerts delivered to you (email or webhook) instead of waiting for you to
  look at the app.
- OIDC/Authelia single sign-on for client installs that need it.

### Explicitly not happening (for now)

These are decisions, not oversights:

- **No block editor.** Markdown-first until block storage, undo, export,
  and concurrency can all be done properly.
- **No real-time collaboration, formulas, rollups, plugins, or AI-written
  changes.**
- **No automatic cross-workspace collections.** Membership stays explicit —
  a bookmark is in a collection because someone put it there.
