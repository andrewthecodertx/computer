export const dynamic = 'force-dynamic'
import { NextRequest, NextResponse } from 'next/server'
import { prisma } from '@/lib/prisma'
import { getRealUser } from '@/lib/auth-guard'

export async function GET() {
  const me = await getRealUser()
  if (!me) return NextResponse.json({ error: 'Unauthorized' }, { status: 401 })
  if (me.role !== 'ADMIN') return NextResponse.json({ error: 'Forbidden' }, { status: 403 })
  const users = await prisma.user.findMany({
    where: { NOT: { email: { endsWith: '@example.com' } } },
    select: { id: true, name: true, email: true, role: true, createdAt: true, _count: { select: { bookmarks: true, pages: true, tags: true } } },
    orderBy: { createdAt: 'asc' },
  })
  return NextResponse.json(users)
}

export async function PATCH(req: NextRequest) {
  const me = await getRealUser()
  if (!me) return NextResponse.json({ error: 'Unauthorized' }, { status: 401 })
  if (me.role !== 'ADMIN') return NextResponse.json({ error: 'Forbidden' }, { status: 403 })
  const { userId, role } = await req.json()
  if (!userId || !['USER', 'ADMIN'].includes(role)) return NextResponse.json({ error: 'userId and role (USER|ADMIN) required' }, { status: 400 })
  if (userId === me.id && role !== 'ADMIN') return NextResponse.json({ error: 'You cannot remove your own admin role' }, { status: 400 })
  const user = await prisma.user.update({ where: { id: userId }, data: { role }, select: { id: true, role: true } })
  return NextResponse.json(user)
}
