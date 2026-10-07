export const dynamic = 'force-dynamic'
import { NextRequest, NextResponse } from 'next/server'
import { prisma } from '@/lib/prisma'
import { getAuthUser } from '@/lib/auth-guard'

export async function POST(req: NextRequest, { params }: { params: Promise<{ id: string }> }) {
  const user = await getAuthUser()
  if (!user) return NextResponse.json({ error: 'Unauthorized' }, { status: 401 })
  const { id } = await params

  const existing = await prisma.bookmark.findFirst({ where: { id, ownerId: user.id } })
  if (!existing) return NextResponse.json({ error: 'Not found' }, { status: 404 })

  const { userIds, isPublic } = await req.json()

  if (isPublic !== undefined) {
    await prisma.bookmark.update({ where: { id }, data: { isPublic } })
  }

  if (userIds?.length) {
    for (const uid of userIds as string[]) {
      await prisma.bookmarkShare.upsert({
        where: { bookmarkId_userId: { bookmarkId: id, userId: uid } },
        create: { bookmarkId: id, userId: uid },
        update: {},
      })
    }
  }

  const bookmark = await prisma.bookmark.findUnique({
    where: { id },
    include: { sharedWith: { include: { user: { select: { id: true, name: true, email: true } } } } },
  })

  return NextResponse.json(bookmark)
}
