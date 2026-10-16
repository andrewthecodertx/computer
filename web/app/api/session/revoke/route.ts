export const dynamic = 'force-dynamic'
import { NextResponse } from 'next/server'
import jwt from 'jsonwebtoken'
import { auth } from '@/auth'
import { apiBase } from '@/lib/api'

// Review CR-02: "sign out everywhere" is an explicit, observable action.
// The NextAuth signOut event is a fire-and-forget backstop; this route lets
// the UI establish whether Go's global revocation (tokenVersion bump, which
// kills Go cookie sessions and previously minted assertions) actually
// succeeded BEFORE the local session is discarded — so a backend outage
// degrades to an accurate "signed out here, other sessions may remain"
// outcome instead of a silent partial logout. The request is deadline-bound
// and logs without secrets.
export async function POST(req: Request) {
  const expectedOrigin = new URL(process.env.NEXTAUTH_URL || req.url).origin
  if (req.headers.get('origin') !== expectedOrigin) {
    return NextResponse.json({ error: 'Invalid request origin' }, { status: 403 })
  }
  const session = await auth()
  const secret = process.env.NEXTAUTH_SECRET || process.env.AUTH_SECRET
  const id = typeof session?.user?.id === 'string' ? session.user.id : ''
  const ver = typeof session?.user?.ver === 'number' ? session.user.ver : 0
  if (!secret || !id || ver < 1) {
    // No backend identity attached to this session — nothing to revoke.
    return NextResponse.json({ revoked: false, reason: 'no-backend-session' })
  }
  const controller = new AbortController()
  const timer = setTimeout(() => controller.abort(), 5000)
  try {
    const res = await fetch(`${apiBase()}/api/auth/signout`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        // Same short-lived assertion the data proxy signs (lib/api.ts).
        Authorization: `Bearer ${jwt.sign({ uid: id, ver, typ: 'assertion' }, secret, { algorithm: 'HS256', issuer: 'computer', expiresIn: '60s' })}`,
      },
      cache: 'no-store',
      signal: controller.signal,
    })
    if (!res.ok) {
      console.error(`session revoke: Go signout answered ${res.status} for user ${id}`)
      return NextResponse.json({ revoked: false, reason: 'backend-error' })
    }
    return NextResponse.json({ revoked: true })
  } catch (error) {
    const kind = error instanceof Error ? error.name : 'Error'
    console.error(`session revoke: Go signout unreachable for user ${id} (${kind})`)
    return NextResponse.json({ revoked: false, reason: 'unreachable' })
  } finally {
    clearTimeout(timer)
  }
}
