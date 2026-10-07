export const dynamic = 'force-dynamic'
import { NextResponse } from 'next/server'
import { signIn } from '@/auth'

export async function POST(req: Request) {
  try {
    const body = await req.json()
    const result = await signIn('credentials', {
      email: body.email,
      password: body.password,
      redirect: false,
    })
    return NextResponse.json({ ok: true, url: result })
  } catch (error: any) {
    return NextResponse.json({ error: 'Invalid credentials' }, { status: 401 })
  }
}
