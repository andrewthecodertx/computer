import 'server-only'
import jwt from 'jsonwebtoken'
import { auth } from '@/auth'
import { cookies } from 'next/headers'

export const apiBase = () => process.env.API_BASE_URL || 'http://localhost:8080'

// Only this trusted Next server signs assertions. The browser never chooses a
// user ID, and Go still re-reads roles/ownership on every request. The typ
// claim binds the token to the Bearer channel; Go rejects it as a cookie and
// rejects long-lived session tokens presented as assertions.
export async function backendFetch(path: string, init: RequestInit = {}) {
  const session = await auth()
  const headers = new Headers(init.headers)
  if (session?.user?.id) {
    const secret = process.env.NEXTAUTH_SECRET || process.env.AUTH_SECRET
    if (!secret) throw new Error('Missing authentication secret')
    headers.set('Authorization', `Bearer ${jwt.sign({ uid: session.user.id, typ: 'assertion' }, secret, {
      algorithm: 'HS256', issuer: 'computer', expiresIn: '60s',
    })}`)
  }
  const viewAs = (await cookies()).get('computer_view_as')?.value
  if (viewAs) headers.set('Cookie', `computer_view_as=${encodeURIComponent(viewAs)}`)
  return fetch(`${apiBase()}${path}`, { ...init, headers, cache: 'no-store' })
}
