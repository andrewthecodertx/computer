# Connecting computer to Authelia (OIDC)

computer supports single sign-on through **Authelia** using OpenID Connect.
Out of the box this integration is **stubbed out**. The code is in place, but
it stays inactive until real credentials are supplied. Until then, users sign
in with email and password.

---

## 1. How the stub works

| Piece | File | Behaviour while NOT connected | Behaviour once connected |
|---|---|---|---|
| Config detection | `lib/oidc-config.ts` | `getOidcStatus().enabled === false` | `enabled === true` |
| Auth provider | `auth.ts` | Authelia provider is **not registered**. Only the email/password provider exists. | Authelia OIDC provider registered with id `authelia` |
| Login page | `app/login/` | "Sign in with Authelia" button is shown **disabled** with a "not connected yet" note | Button is active and redirects to Authelia |
| Settings → OIDC tab | `app/(app)/settings/` | Shows **Not connected** and lists which variables are missing | Shows **Connected**, the issuer URL and the callback URL |

A value counts as "not set" if it is empty or looks like a placeholder. These
are matched case-insensitively: `REPLACE_ME`, `changeme`, `placeholder`,
anything containing `example.com`/`.org`/`.net`, `<...>`, values starting with
`your_`/`your-`, or `todo`.

**All three** variables must hold real values before Authelia is enabled. A
partially filled config stays disabled. It never crashes the app.

The current `.env` ships with:

```env
OIDC_ISSUER=REPLACE_ME
OIDC_CLIENT_ID=REPLACE_ME
OIDC_CLIENT_SECRET=REPLACE_ME
```

No code changes are needed to connect. Set the three variables and restart or
redeploy.

---

## 2. What to configure in Authelia

Requires Authelia **v4.38 or later** (examples use the 4.39 syntax), served over
**valid HTTPS** and reachable from the server that runs computer.

### 2.1 Generate a client secret

```bash
docker run --rm authelia/authelia:latest authelia crypto hash generate pbkdf2 \
  --variant sha512 --random --random.length 72 --random.charset rfc3986
```

This prints:
- **Random Password**: the *plain* secret. It goes into computer as `OIDC_CLIENT_SECRET`.
- **Digest**: the *hashed* secret. It goes into Authelia's `client_secret`.

### 2.2 Register the computer client (`configuration.yml`)

```yaml
identity_providers:
  oidc:
    # jwks / hmac_secret must already be configured for OIDC to work
    claims_policies:
      computer:
        # REQUIRED on 4.39+: puts email/name into the ID token
        id_token: ['email', 'email_verified', 'name', 'preferred_username']
    clients:
      - client_id: 'computer'
        client_name: 'computer'
        client_secret: '$pbkdf2-sha512$310000$...'   # Digest from 2.1
        public: false
        authorization_policy: 'two_factor'          # or 'one_factor'
        consent_mode: 'implicit'
        claims_policy: 'computer'
        redirect_uris:
          - 'https://<COMPUTER_HOST>/api/auth/callback/authelia'
        scopes: ['openid', 'profile', 'email']
        grant_types: ['authorization_code']
        response_types: ['code']
        token_endpoint_auth_method: 'client_secret_basic'
```

Restart Authelia after editing.

**Redirect URI rule:** it must be exactly
`https://<COMPUTER_HOST>/api/auth/callback/authelia`. Use one entry per hostname
computer is served on (preview, production, custom domain).

### 2.3 Users

Every Authelia user **must have an email address**. computer uses the email to
create the account on first login. It also uses it to link an Authelia login to
an existing email/password account with the same address.

---

## 3. What to configure in computer

Set these environment variables (in `.env` or the hosting environment):

| Variable | Example | Notes |
|---|---|---|
| `OIDC_ISSUER` | `https://auth.yourdomain.com` | Authelia base URL, no trailing slash, no path |
| `OIDC_CLIENT_ID` | `computer` | Must match `client_id` in Authelia |
| `OIDC_CLIENT_SECRET` | *(plain secret from 2.1)* | The **plain** value, not the digest |

Then restart or redeploy. Open **Settings → OIDC** to confirm it shows **Connected**.

---

## 4. Verifying

1. Open `https://auth.yourdomain.com/.well-known/openid-configuration` in a browser.
   It must return JSON. If it doesn't, computer cannot connect either.
2. In computer, open **Settings → OIDC**. It should show *Connected*.
3. Log out, then click **Sign in with Authelia** on the login page.
4. After you authenticate in Authelia you land on `/dashboard`. On first login,
   the user record is created automatically.

## 5. Troubleshooting

| Symptom | Likely cause |
|---|---|
| Button still disabled | One of the three variables is empty or looks like a placeholder, or the app was not restarted |
| Authelia shows "invalid redirect_uri" | Callback URL not listed exactly in `redirect_uris` |
| Error page `?error=Configuration` after login | Wrong client secret (digest used instead of plain), or the issuer is unreachable or has an invalid TLS certificate |
| Login succeeds, but the account has no email or fails | `claims_policy` missing (Authelia 4.39+), or the user has no email in Authelia |

## 6. Optional: Authelia-only login

Email/password login stays available alongside Authelia. To make Authelia the
only method, remove the `CredentialsProvider` from `auth.ts`, the email form in
`app/login/client.tsx`, and the `/signup` page and `/api/signup` route.
