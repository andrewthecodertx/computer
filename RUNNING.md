# Running computer

This guide takes you from cloning the repository to signing in on your own
computer. Docker runs the web app, Go API, and PostgreSQL database together.
You do not need to install Go, Node.js, PostgreSQL, Make, or OpenSSL separately.

## Before you start

- **Docker with Docker Compose v2 or newer.** Docker Desktop includes Compose
  on Windows and macOS. On Linux, install Docker Engine and the Compose plugin.
- **Git** to clone the repository. Alternatively, download and extract the
  repository ZIP from GitHub.
- An internet connection for the first build to download images and packages.
- Local ports **3000** and **8080** available for the app.

Start Docker Desktop, or make sure the Docker service is running on Linux.
Open a terminal (PowerShell on Windows) and check:

```sh
docker info
docker compose version
```

Both commands should succeed. On Windows, Docker must be using Linux containers.

## 1. Download the app

```sh
git clone https://github.com/andrewthecodertx/computer.git
cd computer
```

If you downloaded a ZIP, open a terminal in the extracted folder instead.
Run the remaining commands from the folder containing `docker-compose.yml`.

## 2. Create your configuration

Copy the supplied example to a file named `.env` in the repository root.
Do this once on a new installation; keep an existing `.env` when updating.

**macOS / Linux:**

```sh
cp .env.example .env
```

**Windows PowerShell:**

```powershell
Copy-Item .env.example .env
```

Generate an authentication secret using Docker. This command works in either
terminal:

```sh
docker run --rm node:22-alpine node -e "console.log(require('node:crypto').randomBytes(32).toString('hex'))"
```

The final line of output is a 64-character random value. Open `.env` in a text
editor and replace `replace-with-a-random-secret` on the `AUTH_SECRET` line
with that value. Save the file.

For a new local installation, keep these other settings:

```dotenv
NEXTAUTH_URL=http://localhost:3000
COOKIE_SECURE=0
ADMIN_EMAILS=
POSTGRES_VOLUME=computer_pgdata
POSTGRES_VOLUME_EXTERNAL=false
```

The supplied `POSTGRES_PASSWORD=computer` works with the bundled database.
If you choose a different password, set it before the first startup; use a
random hexadecimal value to avoid special characters in the connection URL.
Keep the `OIDC_*` placeholders as supplied: use email/password login for now.

Keep `.env` for future starts and upgrades. It is ignored by Git. **Keep
`AUTH_SECRET` unchanged:** changing it invalidates sessions, and — unless a
separate `ENCRYPTION_KEY` is set — makes saved IMAP passwords unreadable.
Setting `ENCRYPTION_KEY` (a random hex string, backed up with the database)
decouples integration-credential encryption from session signing: you can
then rotate `AUTH_SECRET` without losing stored mailbox passwords, and rows
encrypted under the old derivation are re-encrypted transparently on first
use. Back up `.env` with your database backups: without the secrets it holds,
encrypted integration data in a database backup cannot be decrypted.

Mailbox connections always use verified implicit TLS (port 993 by default);
plaintext IMAP logins are rejected.

## 3. Build and start

```sh
docker compose up -d --build
```

The first build can take several minutes. Docker automatically:

1. Starts PostgreSQL and initializes a new database.
2. Applies any pending additive database migrations.
3. Starts the Go API.
4. Starts the Next.js web app.

Check startup status:

```sh
docker compose ps -a
```

Expected results:

| Service | Expected state |
|---|---|
| `db` | Running, healthy |
| `migrate` | Exited with code **0** — normal; it runs once per startup |
| `app` | Running, healthy |
| `web` | Running |

If `db` or `app` still says `health: starting`, wait a few seconds and check
again. The `tests` service is optional and does not start during normal use.

## 4. Open the app and create your account

Open **http://localhost:3000** in your browser.

1. Choose **Sign up** and create an account with your email and a password.
2. Sign in using those credentials.
3. Add a bookmark to check that saving works.

With the default empty `ADMIN_EMAILS`, the first account whose email does not
end in `@example.com` becomes the administrator. Create your own account first.
Later accounts are regular users; an administrator can manage their roles
from the Admin screen.

Addresses listed in `ADMIN_EMAILS` are reserved and cannot be claimed through
public signup. To create an account for one of those addresses, use the
operator-provisioning command in [README.md](README.md#developer-commands).

The default setup is accessible only from this computer. Public bookmark
links are also local until you configure externally accessible hosting.

### What to test

- Create, edit, and delete a bookmark, including its URL, description, and tags.
- Organize bookmarks using tags, calendar dates, contacts, and kanban columns.
- Create a collection, add an existing bookmark to it (nothing is duplicated),
  add a select property such as Status, and switch between the table and a
  board view over the same membership.
- Write a Markdown page, switch between pages, and download its Markdown export.
- Share a bookmark, tag, or date with a second account and revoke the share.
- Open a public bookmark link in a signed-out browser window.

Pages autosave after a short pause and finish saving before switching page
tabs. If saving fails, the page remains **Unsaved** and a draft is kept in the
current browser tab. Reconnect and click **Save now**; tab-local drafts are
not a replacement for database backups. If another tab or device saved the
same page first, autosave pauses with a conflict banner — choose explicitly
between overwriting, saving a copy, or discarding; typing alone never
overwrites the newer version.

## Everyday commands

Run these from the repository root:

| Task | Command |
|---|---|
| Stop the app and keep your data | `docker compose down` |
| Start it again | `docker compose up -d` |
| Restart the web app and API | `docker compose restart app web` |
| Check service status | `docker compose ps -a` |
| Follow application logs | `docker compose logs -f app web` |

Press **Ctrl+C** to stop following logs; the app keeps running.

After changing `.env`, use `docker compose up -d` to recreate affected
containers with the new settings. A simple `restart` does not load changed
environment variables.

### Where your data lives

Bookmarks, accounts, pages, and settings live in the Docker PostgreSQL volume
named `computer_pgdata` by default. They survive normal stops, container
recreation, and image rebuilds.

**Do not use `docker compose down -v` or delete that volume if you want to
keep your data.** The volume is not a backup; back up the database separately.

### Backups and restore

Take dumps with the database running (review OPS-03: a dump is only a backup
once a restore from it has been proven):

```sh
docker compose exec -T db pg_dump -U computer -d computer | gzip > computer-$(date -u +%Y%m%d).sql.gz
cp .env env-$(date -u +%Y%m%d).bak   # AUTH_SECRET / ENCRYPTION_KEY / POSTGRES_PASSWORD
```

Keep the dump **and** the `.env` snapshot together and copy both off-host:
stored IMAP passwords are AES-GCM ciphertext, and without the secret they
were encrypted under, a restored database cannot decrypt them. The
server deploy script (`scripts/server-deploy.sh`) does this automatically
before every deploy into `/var/backups/computer` (0600 dumps, 15 retained);
it starts a stopped database rather than skipping the backup silently.

Restore into an isolated environment first to verify a dump before trusting
it:

```sh
docker compose up -d db                       # fresh volume = empty database
gunzip -c computer-YYYYMMDD.sql.gz | docker compose exec -T db psql -U computer -d computer
```

Then restore the matching `.env` so the secrets can decrypt integration
data, and start the stack. Migrations are additive, so any current or newer
application release reads an older dump; reverting to an older data state
means restoring that dump, not reverting SQL.

### Updating the app

Back up your database and `.env` before updating. Then run:

```sh
git pull --ff-only
docker compose up -d --build
docker compose ps -a
```

Keep your existing `.env` and database volume. Database migrations run
automatically during startup.

## Optional integrations

The core app works without any external services.

- **IMAP email:** In **Settings → IMAP**, enter your mail server, port, TLS
  setting, username, password, and folder. Test the connection, then save.
  Add an email watcher from a bookmark's Signals panel. Enabled watchers are
  checked every five minutes while the API is running.
- **Nextcloud contacts:** In **Settings → Nextcloud**, enter your Nextcloud
  URL, username, and app password, then sync contacts. You can associate a
  synced contact with a bookmark.
- **Browser alerts:** Allow notifications when prompted. Alert checks run
  while the app is open in your browser.
- **Authelia:** This integration is intentionally unfinished. Setting OIDC
  environment variables alone does not enable sign-in. See
  [web/docs/AUTHELIA.md](web/docs/AUTHELIA.md) for integration details.

## Troubleshooting

### Docker cannot connect to its daemon

Start Docker Desktop or the Linux Docker service, then retry `docker info`.
On Linux, ensure your account has permission to use Docker.

### `docker compose` is not recognized

Install the Docker Compose plugin or update Docker Desktop. These instructions
use `docker compose` with a space, rather than the older `docker-compose`.

### `Set AUTH_SECRET in .env`

Confirm `.env` exists beside `docker-compose.yml`, has a nonempty `AUTH_SECRET`,
and was not saved as `.env.txt`. Follow step 2 to generate the value.

### Port 3000 or 8080 is already in use

Stop the other application or Docker stack using that port, then retry
`docker compose up -d --build`. Alternatively move this stack's host ports in
`.env` (`WEB_HOST_PORT`, `APP_HOST_PORT`). When you shift `WEB_HOST_PORT`,
`NEXTAUTH_URL` must be updated to the same browser-facing origin (for example
`http://localhost:3100`): the API proxy rejects requests whose Origin matches
neither, so sign-in works but saving fails with "Invalid request origin".
Recreate the stack with `docker compose up -d` after changing `.env`.

### The app does not open, or sign-in fails

Use **http://localhost:3000** consistently; keep `NEXTAUTH_URL` set to that
address and `COOKIE_SECURE=0` for local HTTP. Then check:

```sh
docker compose ps -a
docker compose logs --tail=100 db migrate app web
```

The API's database readiness check is available in your browser at
**http://localhost:8080/readyz**. A ready API returns `{"status":"ready"}`.

If PostgreSQL reports a password mismatch on an existing volume, restore
the original `POSTGRES_PASSWORD` in `.env`. Changing that variable does not
change the password of a database that was already initialized.

When asking for help, include service status and relevant error messages.
Remove passwords, secrets, and personal data from anything you share.

## Running on a remote server

The default Compose ports bind to `127.0.0.1`. For remote access, configure an
HTTPS reverse proxy to the web app on port 3000, set `NEXTAUTH_URL` to your
public HTTPS URL, and set `COOKIE_SECURE=1`. The database remains internal to
Docker.

### Bundled deployment script

The repository ships a target-agnostic deployment for exactly this setup —
no host or domain is hardcoded. Prerequisites: a Debian/Ubuntu server with
ssh access (an ssh config alias works well), DNS for your domain already
pointing at it, and `rsync` + `git` on your workstation. Then, from the
repository root:

```sh
HOST=app-server DOMAIN=computer.example.tld CERTBOT_EMAIL=ops@example.tld ./scripts/deploy.sh
```

The script deploys a **known git revision** (dirty working trees are refused
unless `ALLOW_DIRTY=1`) and on the server provisions Docker/nginx/certbot,
writes `.env` on first deploy, takes a pre-deploy database dump plus `.env`
snapshot into `/var/backups/computer` (0600, 15 retained), builds
revision-tagged images, renders the nginx + Let's Encrypt proxy from
`deploy/nginx/computer.conf.template`, and verifies `https://DOMAIN/login`
end to end.

Rollback: re-deploy an older revision (`git checkout <rev> && ./scripts/deploy.sh`),
or restore one of the pre-deploy dumps together with its `.env` snapshot.
Database migrations are additive, so an older application release keeps
working against a migrated database; rolling the data itself back means the
dump, not reverting SQL.
