export const dynamic = 'force-dynamic'
import { NextRequest, NextResponse } from 'next/server'
import { prisma } from '@/lib/prisma'
import { getAuthUser } from '@/lib/auth-guard'

export async function GET(req: NextRequest) {
  const user = await getAuthUser()
  if (!user) return NextResponse.json({ error: 'Unauthorized' }, { status: 401 })

  const tags = await prisma.tag.findMany({
    where: {
      OR: [{ ownerId: user.id }, { sharedWith: { some: { userId: user.id } } }],
    },
    include: { _count: { select: { bookmarks: true } }, sharedWith: { include: { user: { select: { id: true, name: true } } } } },
    orderBy: { name: 'asc' },
  })

  return NextResponse.json(tags)
}

export async function POST(req: NextRequest) {
  const user = await getAuthUser()
  if (!user) return NextResponse.json({ error: 'Unauthorized' }, { status: 401 })

  const { name, color } = await req.json()
  if (!name) return NextResponse.json({ error: 'Name required' }, { status: 400 })

  const tag = await prisma.tag.create({
    data: { name, color: color ?? '#6366f1', ownerId: user.id },
  })

  return NextResponse.json(tag, { status: 201 })
}
