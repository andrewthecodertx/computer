import 'server-only'
import jwt from 'jsonwebtoken'
import { createHmac } from 'node:crypto'
import { auth } from '@/auth'
import { cookies } from 'next/headers'

export const apiBase = () => process.env.API_BASE_URL || 'http://localhost:8080'

// Review SEC-2: the browser IP is forwarded to Go only when an operator has
// pinned the trusted proxy boundary (TRUSTED_PROXY=1) that overwrites
// X-Real-IP — the shipped nginx template does. X-Forwarded-For is never
// used: its leftmost entry is client-supplied. The HMAC covers ip|unix_ts
// and Go rejects timestamps older than ~60s, bounding header replay.
export const observedClientIP = (headers: Headers): string | null => {
  if (process.env.TRUSTED_PROXY !== '1') return null
  const ip = headers.get('x-real-ip')
  return ip && ip.trim() ? ip.trim() : null
}

export const signClientIP = (ip: string): { ts: string; sig: string } => {
  const secret = process.env.NEXTAUTH_SECRET || process.env.AUTH_SECRET
  if (!secret) throw new Error('Missing authentication secret')
  const ts = String(Math.floor(Date.now() / 1000))
  return { ts, sig: createHmac('sha256', secret).update(`${ip}|${ts}`).digest('hex') }
}

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
  // Review AUTH-RATE/SEC-2: attest the browser IP for Go's login/signup
  // throttling so proxied users do not all share this server's peer-IP
  // bucket. The timestamped HMAC keeps the header unforgeable and
  // short-lived; Go falls back to the direct peer when it is missing or
  // invalid.
  const clientIP = headers.get('x-computer-client-ip')
  if (clientIP && secret) {
    const { ts, sig } = signClientIP(clientIP)
    headers.set('x-computer-client-ts', ts)
    headers.set('x-computer-client-sig', sig)
  }
  const viewAs = (await cookies()).get('computer_view_as')?.value
  if (viewAs) headers.set('Cookie', `computer_view_as=${encodeURIComponent(viewAs)}`)
  return fetch(`${apiBase()}${path}`, { ...init, headers, cache: 'no-store' })
}
