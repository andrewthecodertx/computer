import NextAuth from 'next-auth'
import CredentialsProvider from 'next-auth/providers/credentials'
import jwt from 'jsonwebtoken'
import { createHmac } from 'node:crypto'
import { getOidcStatus } from '@/lib/oidc-config'

// Review SEC-2: browser IP attestation for Go's rate limiting (see lib/api.ts
// for the identical semantics used by the data proxy). Duplicated here to keep
// auth.ts import-free of lib/api.ts (avoiding a lib/api.ts → @/auth → lib/api.ts
// cycle); both helpers must stay in sync.
const observedClientIP = (headers: Headers): string | null => {
  if (process.env.TRUSTED_PROXY !== '1') return null
  const ip = headers.get('x-real-ip')
  return ip && ip.trim() ? ip.trim() : null
}
const signClientIP = (secret: string, ip: string): { ts: string; sig: string } => {
  const ts = String(Math.floor(Date.now() / 1000))
  return { ts, sig: createHmac('sha256', secret).update(`${ip}|${ts}`).digest('hex') }
}

// NextAuth owns the browser session and the eventual client-managed OIDC flow.
// Password verification and data ownership live in Go, not Prisma.
export const { handlers, auth, signIn, signOut } = NextAuth({
  trustHost: true,
  // Same precedence as Go (config.Secret) and lib/api.ts, so a deployment
  // with both variables set can never sign and verify with different values.
  secret: process.env.NEXTAUTH_SECRET || process.env.AUTH_SECRET,
  session: { strategy: 'jwt' },
  pages: { signIn: '/login' },
  providers: [
    ...(getOidcStatus().enabled ? [{
      id: 'authelia', name: 'Authelia', type: 'oidc',
      issuer: process.env.OIDC_ISSUER,
      clientId: process.env.OIDC_CLIENT_ID,
      clientSecret: process.env.OIDC_CLIENT_SECRET,
      authorization: { params: { scope: 'openid profile email' } },
      profile(profile: any) {
        return { id: profile.sub, name: profile.name ?? profile.preferred_username ?? profile.sub,
          email: profile.email, image: profile.picture ?? null }
      },
    } as any] : []),
    CredentialsProvider({
      credentials: { email: { label: 'Email', type: 'email' }, password: { label: 'Password', type: 'password' } },
      async authorize(credentials, request) {
        if (!credentials?.email || !credentials?.password) return null
        // Review SEC-1: browser logins arrive through this NextAuth path, not
        // the data proxy — without attested headers Go buckets every user on
        // this server's address (Compose) or exempted them entirely
        // (loopback, same-host setups), and one attacker could lock out any
        // account. NextAuth v5 passes the credentials Request as the second
        // argument; sign the observed browser IP the same way lib/api.ts does.
        const secret = process.env.NEXTAUTH_SECRET || process.env.AUTH_SECRET
        const headers: Record<string, string> = { 'Content-Type': 'application/json' }
        const ip = observedClientIP(request?.headers ?? new Headers())
        if (ip && secret) {
          const { ts, sig } = signClientIP(secret, ip)
          headers['x-computer-client-ip'] = ip
          headers['x-computer-client-ts'] = ts
          headers['x-computer-client-sig'] = sig
        }
        const response = await fetch(`${process.env.API_BASE_URL || 'http://localhost:8080'}/api/auth/login`, {
          method: 'POST', headers, cache: 'no-store',
          body: JSON.stringify({ email: credentials.email, password: credentials.password }),
        })
        if (!response.ok) {
          // Surface Go's 429 so callers can distinguish throttling from bad
          // credentials (the custom login route maps everything else to 401).
          if (response.status === 429) {
            const retry = response.headers.get('retry-after')
            const err = new Error(response.status === 429 ? 'Throttled' : 'Unauthorized') as Error & { status?: number; retryAfter?: string }
            err.status = 429
            if (retry) err.retryAfter = retry
            throw err
          }
          return null
        }
        const data = await response.json()
        // Carry Go's tokenVersion into the NextAuth JWT so every assertion
        // this session signs is revocable by a version bump ("sign out
        // everywhere") — review SEC-02.
        return { ...data.user, tokenVersion: data.tokenVersion }
      },
    }),
  ],
  callbacks: {
    async signIn({ account }) {
      // Client integration point: exchange a verified Authelia identity for a
      // local Go User/Account before enabling OIDC. No subject is trusted as a
      // database user ID; the backend exchange stays an explicit 501 stub.
      return account?.provider !== 'authelia'
    },
    async jwt({ token, user }) { if (user) { token.id = user.id; token.ver = user.tokenVersion } return token },
    async session({ session, token }) {
      if (session.user) { session.user.id = token.id as string; session.user.ver = token.ver }
      return session
    },
  },
  events: {
    // Review AUTH-LIFECYCLE: a browser sign-out must reach Go's revocation.
    // Without this the tokenVersion never bumps on NextAuth sign-out:
    // previously minted assertions stay valid for the rest of their (≤120s)
    // lifetime and the user's Go cookie sessions survive the logout. Go's
    // /api/auth/signout means "sign out everywhere" — deliberately the same
    // semantics here. Fail-open: if Go is unreachable the local sign-out
    // still completes, degrading to the previous behavior. Review CR-02:
    // this event is the backstop — the UI's explicit /api/session/revoke
    // call is what makes revocation failure user-visible; here the outcome
    // is inspected, deadline-bound, and logged without secrets.
    async signOut(message) {
      const token = 'token' in message ? message.token : null
      const secret = process.env.NEXTAUTH_SECRET || process.env.AUTH_SECRET
      const id = typeof token?.id === 'string' ? token.id : ''
      const ver = typeof token?.ver === 'number' ? token.ver : 0
      if (!secret || !id || ver < 1) return
      const controller = new AbortController()
      const timer = setTimeout(() => controller.abort(), 5000)
      try {
        const response = await fetch(`${process.env.API_BASE_URL || 'http://localhost:8080'}/api/auth/signout`, {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            // Same short-lived assertion the data proxy signs (lib/api.ts).
            Authorization: `Bearer ${jwt.sign({ uid: id, ver, typ: 'assertion' }, secret, { algorithm: 'HS256', issuer: 'computer', expiresIn: '60s' })}`,
          },
          cache: 'no-store',
          signal: controller.signal,
        })
        if (response.status === 401) {
          // The assertion was rejected — overwhelmingly because an explicit
          // revoke already bumped the tokenVersion (the revocation this call
          // exists to guarantee has then already happened). Nothing to retry.
          console.warn(`signout event: revocation assertion rejected (401) for user ${id}; session is revoked or expired`)
        } else if (!response.ok) {
          // Local sign-out still proceeds, but global revocation did NOT
          // happen: other sessions/assertions survive. Make it visible.
          console.error(`signout event: Go revocation failed with ${response.status} for user ${id}; other sessions may remain active`)
        }
      } catch (error) {
        const kind = error instanceof Error ? error.name : 'Error'
        console.error(`signout event: Go revocation unreachable for user ${id} (${kind}); other sessions may remain active`)
      } finally {
        clearTimeout(timer)
      }
    },
  },
})
