export const dynamic = 'force-dynamic'
import { NextRequest, NextResponse } from 'next/server'
import { prisma } from '@/lib/prisma'
import { getRealUser, VIEW_AS_COOKIE } from '@/lib/auth-guard'

// POST { userId } starts viewing as that user; POST { userId: null } returns to your own account.
export async function POST(req: NextRequest) {
  const me = await getRealUser()
  if (!me) return NextResponse.json({ error: 'Unauthorized' }, { status: 401 })
  if (me.role !== 'ADMIN') return NextResponse.json({ error: 'Forbidden' }, { status: 403 })
  const { userId } = await req.json()
  const res = NextResponse.json({ ok: true })
  if (!userId || userId === me.id) {
    res.cookies.set(VIEW_AS_COOKIE, '', { path: '/', maxAge: 0 })
    return res
  }
  const target = await prisma.user.findUnique({ where: { id: userId }, select: { id: true } })
  if (!target) return NextResponse.json({ error: 'User not found' }, { status: 404 })
  res.cookies.set(VIEW_AS_COOKIE, userId, { path: '/', httpOnly: true, sameSite: 'lax', secure: true, maxAge: 60 * 60 * 8 })
  return res
}
