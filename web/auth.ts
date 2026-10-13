import NextAuth from 'next-auth'
import CredentialsProvider from 'next-auth/providers/credentials'
import jwt from 'jsonwebtoken'
import { getOidcStatus } from '@/lib/oidc-config'

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
      async authorize(credentials) {
        if (!credentials?.email || !credentials?.password) return null
        const response = await fetch(`${process.env.API_BASE_URL || 'http://localhost:8080'}/api/auth/login`, {
          method: 'POST', headers: { 'Content-Type': 'application/json' }, cache: 'no-store',
          body: JSON.stringify({ email: credentials.email, password: credentials.password }),
        })
        if (!response.ok) return null
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
    // still completes, degrading to the previous behavior.
    async signOut(message) {
      const token = 'token' in message ? message.token : null
      const secret = process.env.NEXTAUTH_SECRET || process.env.AUTH_SECRET
      const id = typeof token?.id === 'string' ? token.id : ''
      const ver = typeof token?.ver === 'number' ? token.ver : 0
      if (!secret || !id || ver < 1) return
      try {
        await fetch(`${process.env.API_BASE_URL || 'http://localhost:8080'}/api/auth/signout`, {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            // Same short-lived assertion the data proxy signs (lib/api.ts).
            Authorization: `Bearer ${jwt.sign({ uid: id, ver, typ: 'assertion' }, secret, { algorithm: 'HS256', issuer: 'computer', expiresIn: '60s' })}`,
          },
          cache: 'no-store',
        })
      } catch { /* local sign-out proceeds; the assertion channel dies with the NextAuth JWT */ }
    },
  },
})
