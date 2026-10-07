# LinkOS: Requirements

## 1. Purpose

LinkOS is a web-based "computer": a personal, multi-user collection of URLs that
point to applications and sites. Users find links through several ways of
organizing them (calendar, tags, contacts, kanban). Clicking a URL opens the
target application or site.

These requirements come from the original project brief. Section 8 adds
decisions made later in the project, and Section 9 lists points the brief left
open.

**Priority key:** **Must** = stated in the brief. **Should** = needed for a
stated requirement to work in practice.

---

## 2. Bookmarks and organization

| ID | Requirement | Priority |
|---|---|---|
| BM-1 | A user can save (bookmark) a URL. | Must |
| BM-2 | Clicking a bookmarked URL takes the user to the target application or site. | Must |
| BM-3 | A user can edit and delete their own bookmarks. | Should |
| BM-4 | A bookmark can have a title. | Should |
| ORG-1 | **Calendar date:** a bookmark can be assigned to a calendar date. | Must |
| ORG-2 | A calendar view shows bookmarks on their assigned dates. | Must |
| ORG-3 | **Tag:** a bookmark can have one or more tags. | Must |
| ORG-4 | A user can browse bookmarks by tag. | Must |
| ORG-5 | **Contact:** a bookmark can be linked to a contact stored in Nextcloud. | Must |
| ORG-6 | Contacts are read from the user's Nextcloud account; LinkOS does not keep a separate address book. | Must |
| ORG-7 | A user can browse bookmarks by contact. | Must |
| ORG-8 | **Kanban:** bookmarks can be shown on a kanban board as cards in columns. | Must |
| ORG-9 | A user can move a bookmark between kanban columns. | Should |
| ORG-10 | One bookmark can be organized in several ways at once (date, tags, contact and kanban column). | Should |
| ORG-11 | A user can search their bookmarks. | Should |

## 3. Pages and markdown notes

| ID | Requirement | Priority |
|---|---|---|
| NOTE-1 | URLs can be embedded in a page together with notes. | Must |
| NOTE-2 | Notes are written in Markdown. | Must |
| NOTE-3 | Notes are "just markdown files": plain Markdown that can be read and moved outside the app. | Must |
| NOTE-4 | Markdown is displayed formatted (headings, lists, links, tables and so on). | Should |

## 4. Alerts and mailbox watching

| ID | Requirement | Priority |
|---|---|---|
| ALR-1 | A URL can have a date alert. | Must |
| ALR-2 | The user is notified when an alert comes due. | Must |
| ALR-3 | The user can dismiss an alert once handled. | Should |
| MAIL-1 | A URL can "watch" an IMAP mailbox for emails. | Must |
| MAIL-2 | The user can enter their IMAP connection details (server, port, security, username, password, folder). | Should |
| MAIL-3 | The user can say which emails a URL watches for (for example by sender or subject). | Should |
| MAIL-4 | Matching emails are shown with the URL they relate to. | Should |
| MAIL-5 | Mailbox passwords are stored encrypted. | Should |

## 5. Users and authentication

| ID | Requirement | Priority |
|---|---|---|
| AUTH-1 | The application is multi-user. Each user has their own bookmarks, tags, notes and settings. | Must |
| AUTH-2 | Users sign in through **Authelia** using OpenID Connect (OIDC). | Must |
| AUTH-3 | The first time a user signs in through Authelia, their account is created automatically. | Should |
| AUTH-4 | Users cannot see each other's data unless it has been shared with them (Section 7). | Must |

## 6. URL previews

| ID | Requirement | Priority |
|---|---|---|
| PREV-1 | Each URL shows a preview of the target. | Must |
| PREV-2 | The preview shows the page's title, description, image and site icon where available. | Should |
| PREV-3 | If a preview can't be fetched, the URL still works and shows a basic fallback. | Should |

## 7. Sharing

| ID | Requirement | Priority |
|---|---|---|
| SHR-1 | A URL can be shared. | Must |
| SHR-2 | A tag can be shared. Recipients see the URLs under that tag. | Must |
| SHR-3 | A calendar date can be shared. Recipients see that date's URLs. | Must |
| SHR-4 | Shared items are clearly marked as shared, with the owner shown. | Should |
| SHR-5 | The owner can stop sharing an item. | Should |

---

## 8. Decisions made later in the project

| ID | Decision |
|---|---|
| DEC-1 | The application database is the user's own PostgreSQL server at 172.234.23.227, in a database named `linkos`. |
| DEC-2 | Authelia sign-in is **stubbed** for now: the code is in place but stays inactive until real Authelia details are entered. The client will connect it, using `docs/AUTHELIA.md`. |
| DEC-3 | Until Authelia is connected, users sign in with email and password. |
| DEC-4 | Nextcloud contacts are read over CardDAV, using credentials each user enters (Nextcloud address, username, app password). |
| DEC-5 | Previews come from the target page's OpenGraph data (title, description, image), fetched by the server, not from screenshots. |
| DEC-6 | IMAP settings are per user, not one shared mailbox. |
| DEC-7 | Sharing works between users of the app. A bookmark can also be given a public read-only link. |
| DEC-8 | The full source code can be downloaded so the app can be hosted elsewhere. |

---

## 9. Open questions

The brief doesn't settle these. They should be confirmed with the client.

1. **What is a "page"?** Is it one URL with its notes, or a document that gathers several URLs together with notes (NOTE-1)?
2. **Markdown as files:** do notes have to be actual `.md` files (on disk, in Nextcloud, or exportable), or is plain-Markdown text stored by the app enough (NOTE-3)? *The current build stores note text in the database.*
3. **Mailbox watching:** what should happen when a matching email arrives: an in-app notice, a browser notification, or an email? Should the mailbox be checked automatically in the background, or when the user asks (MAIL-1)?
4. **Alert delivery:** in the browser only, or also by email or push notification (ALR-2)?
5. **Who can sharing reach?** Only users of this app, or also people outside it? Should recipients be able to edit, or only view (Section 7)?
6. **Kanban columns:** a fixed set (for example Inbox, To Do, In Progress, Done) or columns each user defines (ORG-8)?
7. **Authelia only:** once Authelia is connected, should email/password sign-in be turned off (DEC-3)?
8. **Authorization:** should Authelia groups control what users can do (for example admin rights), or is signing in enough?
