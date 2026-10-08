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
`AUTH_SECRET` unchanged:** changing it invalidates sessions and makes saved
IMAP passwords unreadable. Back up `.env` with your database backups.

## 3. Build and start

```sh
docker compose up -d --build
```

The first build can take several minutes. Docker automatically:

1. Starts PostgreSQL and initializes a new database.
2. Applies the additive database migration.
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

The first account whose email does not end in `@example.com` becomes the
administrator. Create your own account first. Later accounts are regular
users; an administrator can manage their roles from the Admin screen.

The default setup is accessible only from this computer. Public bookmark
links are also local until you configure externally accessible hosting.

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
`docker compose up -d --build`.

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
Docker. This guide's first-run steps cover local use; remote hosting also
requires the server's networking and HTTPS setup.
