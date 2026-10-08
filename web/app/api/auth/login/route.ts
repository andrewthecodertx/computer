export const dynamic = 'force-dynamic'
import { NextResponse } from 'next/server'
import { signIn } from '@/auth'

export async function POST(req: Request) {
  const expectedOrigin = new URL(process.env.NEXTAUTH_URL || req.url).origin
  if (req.headers.get('origin') !== expectedOrigin) {
    return NextResponse.json({ error: 'Invalid request origin' }, { status: 403 })
  }
  if (req.headers.get('content-type')?.split(';')[0].trim().toLowerCase() !== 'application/json') {
    return NextResponse.json({ error: 'Content-Type must be application/json' }, { status: 415 })
  }
  try {
    const body = await req.json()
    const result = await signIn('credentials', {
      email: body.email,
      password: body.password,
      redirect: false,
    })
    return NextResponse.json({ ok: true, url: result })
  } catch {
    return NextResponse.json({ error: 'Invalid credentials' }, { status: 401 })
  }
}
