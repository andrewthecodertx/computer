# Connecting computer to Authelia (OIDC)

computer includes an **Authelia OpenID Connect provider scaffold**, but
end-to-end sign-in is intentionally **stubbed**. Real credentials register
the NextAuth provider; the login button and sign-in callback still block it
until a developer implements mapping to local Go users.

This guide covers configuration you can prepare now and the developer work
needed to finish the connection. Email/password login remains available.
The client-managed OIDC flow runs on the trusted Next.js server; Go owns
local users, accounts, and data authorization.

---

## 1. Current implementation

Paths in this table are relative to the repository root.

| Piece | File | Current behavior |
|---|---|---|
| Config detection | `web/lib/oidc-config.ts` | `enabled` means all three OIDC variables contain non-placeholder values; it does not verify connectivity or a working login |
| Auth provider | `web/auth.ts` | Registers provider `authelia` when configured, but `callbacks.signIn` rejects every Authelia sign-in |
| Login page | `web/app/login/client.tsx` | `oidcReady = false` keeps the button disabled even with real credentials |
| Settings → OIDC | `web/app/(app)/settings/client.tsx` | Shows configuration status, issuer, and callback URL; explicitly says integration is pending |
| Go OIDC handlers | `internal/oidc/provider.go` | Status reports configuration; login and callback return **501** |

A value counts as "not set" if it is empty or looks like a placeholder. These
are matched case-insensitively: `REPLACE_ME`, `changeme`, `placeholder`,
anything containing `example.com`/`.org`/`.net`, `<...>`, values starting with
`your_`/`your-`, or `todo`.

**All three** variables must hold real values before the NextAuth provider is
registered. The readiness gate and identity mapping are separate requirements.

The root `.env.example` supplies:

```env
OIDC_ISSUER=REPLACE_ME
OIDC_CLIENT_ID=REPLACE_ME
OIDC_CLIENT_SECRET=REPLACE_ME
```

Setting these values prepares the configuration. Complete section 4 before
expecting users to sign in through Authelia.

---

## 2. What to configure in Authelia

The example uses **Authelia 4.39 syntax**. Authelia must have its OIDC provider
configured, be served over valid HTTPS, and be reachable from both the user's
browser and the Next.js container. See the upstream
[OIDC provider](https://www.authelia.com/configuration/identity-providers/openid-connect/provider/)
and [client configuration](https://www.authelia.com/configuration/identity-providers/openid-connect/clients/)
documentation for your installed version.

### 2.1 Generate a client secret

```bash
docker run --rm authelia/authelia:4.39 authelia crypto hash generate pbkdf2 \
  --variant sha512 --random --random.length 72 --random.charset rfc3986
```

This prints:
- **Random Password**: the *plain* secret. It goes into computer as `OIDC_CLIENT_SECRET`.
- **Digest**: the *hashed* secret. It goes into Authelia's `client_secret`.

### 2.2 Register the computer client (`configuration.yml`)

Merge this example into the existing OIDC provider configuration. Replace the
client secret digest and public app hostname; keep your existing provider
signing keys, HMAC secret, and other clients.

```yaml
identity_providers:
  oidc:
    # jwks / hmac_secret must already be configured for OIDC to work
    claims_policies:
      computer:
        # Include these identity claims in the ID token.
        id_token: ['email', 'email_verified', 'name', 'preferred_username']
    clients:
      - client_id: 'computer'
        client_name: 'computer'
        client_secret: '$pbkdf2-sha512$310000$...'   # Digest from 2.1
        public: false
        authorization_policy: 'two_factor'          # or 'one_factor'
        consent_mode: 'explicit'
        claims_policy: 'computer'
        redirect_uris:
          - 'https://<COMPUTER_HOST>/api/auth/callback/authelia'
        scopes: ['openid', 'profile', 'email']
        grant_types: ['authorization_code']
        response_types: ['code']
        token_endpoint_auth_method: 'client_secret_basic'
```

Validate the configuration and restart your Authelia deployment after editing.

**Redirect URI rule:** it must exactly match
`<NEXTAUTH_URL>/api/auth/callback/authelia`, including scheme, hostname, port
if present, and path. For an HTTPS deployment, use
`https://<COMPUTER_HOST>/api/auth/callback/authelia`.

The callback is handled by NextAuth on the **web app**, not the Go API.
Forward `/api/auth/*` to Next.js in your reverse proxy. The identically named
direct-Go callback on port 8080 is a deliberate 501 stub.

### 2.3 Users

Provide email and profile claims for Authelia users. Automatic local account
creation and linking are not implemented yet. The developer integration must
define account provisioning and verify ownership before linking an existing
email/password account; matching an email string alone is not enough.

---

## 3. What to configure in computer

Set these environment variables in the **root** `.env` or hosting environment.
Compose passes the OIDC variables to the API and web containers. Replace the
sample hostnames with your actual hosts.

| Variable | Example | Notes |
|---|---|---|
| `NEXTAUTH_URL` | `https://computer.company.tld` | Public web app URL; determines the callback |
| `COOKIE_SECURE` | `1` | For an HTTPS deployment |
| `OIDC_ISSUER` | `https://auth.company.tld` | Exact issuer advertised in Authelia's discovery document |
| `OIDC_CLIENT_ID` | `computer` | Must match `client_id` in Authelia |
| `OIDC_CLIENT_SECRET` | *(plain secret from 2.1)* | The **plain** value, not the digest |

Keep the existing `AUTH_SECRET`: changing it makes stored IMAP passwords
unreadable. The client secret is a separate value and stays server-side.

The default Compose ports bind to localhost. Configure an HTTPS reverse proxy
for the public web URL before using these HTTPS settings; see
[RUNNING.md](../../RUNNING.md#running-on-a-remote-server).

Apply environment changes from the repository root:

```sh
docker compose up -d
```

Open **Settings → OIDC** to confirm the three variables are set. With real
values it shows **Configured … Client integration is still pending**.
This is configuration confirmation, not a successful connection test.

---

## 4. Developer handoff: complete local identity mapping

NextAuth handles OIDC discovery, authorization, token exchange, and token
validation. The remaining integration must connect that verified identity to
the PostgreSQL users that Go authorizes.

1. **Add trusted Go-backed identity lookup/provisioning.** No provisioning
   endpoint is supplied yet. Any new endpoint must authenticate calls from
   the trusted Next server, and accept only identities obtained after OIDC
   verification. Define first-login account creation and an explicit policy
   for linking existing accounts.
2. **Persist the external identity link.** Use the `User` and `Account` tables
   in `db/schema.sql` to associate provider `authelia` and its verified subject
   with a local `User.id`. Validate the configured issuer as part of the flow.
   Returning users must resolve to the same local user.
3. **Update `web/auth.ts`.** Replace the unconditional Authelia rejection in
   `callbacks.signIn` with the completed lookup/provisioning flow. Ensure the
   JWT/session callbacks store the resolved local `User.id`, rather than the
   OIDC profile's `sub`. `web/lib/api.ts` signs API assertions for the session
   ID, and Go expects that ID to identify a real local user.
4. **Enable the login readiness gate.** Update `oidcReady` in
   `web/app/login/client.tsx` only after identity mapping works. Retain the
   configuration check for the three OIDC variables.
5. **Rebuild and verify.** Run `docker compose up -d --build`, then perform
   the checks in section 5.

Keep the authorization-code callback on Next.js. Completing this integration
does not require turning Go's placeholder login/callback routes into another
OIDC client. Simply enabling the button or accepting `sub` as the local user
ID will not make authenticated Go data access work.

## 5. Verify the completed integration

Before implementing the handoff, you can verify configuration and discovery:

1. Open `<OIDC_ISSUER>/.well-known/openid-configuration`. It should return
   JSON with the expected issuer and endpoints. The Next container must also
   be able to reach those HTTPS endpoints.
2. Check **Settings → OIDC** for the configured issuer and exact callback URL.
3. Confirm email/password login remains usable while integration is pending.

After the developer integration is complete:

1. Sign out, click **Sign in with Authelia**, and authenticate with Authelia.
2. Confirm the browser returns through the NextAuth callback to `/pages`.
3. Check `/api/me` through the web app and save a bookmark to confirm the
   session resolves to the intended local user and Go accepts its identity.
4. Sign out and back in to verify the same user and data are retained.
5. Test a new Authelia user, an explicitly linked existing account, and two
   different users to verify provisioning, account linking, and data isolation.

## 6. Troubleshooting

| Symptom | Likely cause |
|---|---|
| Variables are set, but button is disabled | Expected in the current build: `oidcReady` remains false until section 4 is implemented |
| Config still says Not connected | One of the variables is empty or a placeholder, or containers were not recreated after editing `.env` |
| NextAuth returns AccessDenied | The current `callbacks.signIn` deliberately rejects Authelia; after integration, investigate identity mapping |
| Go callback returns 501 | Expected stub; the browser callback must reach Next.js, not Go |
| Authelia shows "invalid redirect_uri" | Callback URL not listed exactly in `redirect_uris` |
| Error page `?error=Configuration` after login | Wrong client secret (digest used instead of plain), or the issuer is unreachable or has an invalid TLS certificate |
| Required identity claims are missing | Check Authelia's claims policy and user profile |
| Browser has a session, but Go returns 401 | Check that the session ID is the resolved local `User.id`, not the OIDC subject |

## 7. Optional: Authelia-only login

Once the completed integration has been verified, decide whether to keep
email/password login as a fallback. An Authelia-only deployment requires
disabling the credentials provider, local login/signup screens, and local
signup/login endpoints in both Next.js and Go, while preserving the trusted
OIDC identity-provisioning path. Hiding the email form alone does not disable
the backend password login routes.
