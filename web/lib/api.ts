import 'server-only'
import jwt from 'jsonwebtoken'
import { createHmac } from 'node:crypto'
import { auth } from '@/auth'
import { cookies } from 'next/headers'

export const apiBase = () => process.env.API_BASE_URL || 'http://localhost:8080'

// Only this trusted Next server signs assertions. The browser never chooses a
// user ID, and Go still re-reads roles/ownership on every request. The typ
// claim binds the token to the Bearer channel; Go rejects it as a cookie and
// rejects long-lived session tokens presented as assertions. The ver claim
// carries the login-time tokenVersion (review SEC-02): Go rejects assertions
// whose version predates a revocation bump, so "sign out everywhere" also
// kills NextAuth-backed browser sessions on their next backend call. Sessions
// predating ver are rejected and must sign in again.
export async function backendFetch(path: string, init: RequestInit = {}) {
  const session = await auth()
  const headers = new Headers(init.headers)
  const secret = process.env.NEXTAUTH_SECRET || process.env.AUTH_SECRET
  if (session?.user?.id) {
    if (!secret) throw new Error('Missing authentication secret')
    headers.set('Authorization', `Bearer ${jwt.sign({ uid: session.user.id, ver: session.user.ver, typ: 'assertion' }, secret, {
      algorithm: 'HS256', issuer: 'computer', expiresIn: '60s',
    })}`)
  }
  // Review AUTH-RATE: attest the browser IP for Go's login/signup throttling
  // so proxied users do not all share this server's peer-IP bucket. The
  // HMAC signature keeps the header unforgeable by browsers; Go falls back
  // to the direct peer when it is missing or invalid.
  const clientIP = headers.get('x-computer-client-ip')
  if (clientIP && secret) {
    headers.set('x-computer-client-sig', createHmac('sha256', secret).update(clientIP).digest('hex'))
  }
  const viewAs = (await cookies()).get('computer_view_as')?.value
  if (viewAs) headers.set('Cookie', `computer_view_as=${encodeURIComponent(viewAs)}`)
  return fetch(`${apiBase()}${path}`, { ...init, headers, cache: 'no-store' })
}
