export const dynamic = 'force-dynamic'
import { NextRequest, NextResponse } from 'next/server'
import { prisma } from '@/lib/prisma'
import { getAuthUser } from '@/lib/auth-guard'

export async function PUT(req: NextRequest, { params }: { params: Promise<{ id: string }> }) {
  const user = await getAuthUser()
  if (!user) return NextResponse.json({ error: 'Unauthorized' }, { status: 401 })
  const { id } = await params

  const existing = await prisma.tag.findFirst({ where: { id, ownerId: user.id } })
  if (!existing) return NextResponse.json({ error: 'Not found' }, { status: 404 })

  const { name, color, shareUserIds } = await req.json()

  if (shareUserIds?.length) {
    for (const uid of shareUserIds as string[]) {
      await prisma.tagShare.upsert({
        where: { tagId_userId: { tagId: id, userId: uid } },
        create: { tagId: id, userId: uid },
        update: {},
      })
    }
  }

  const tag = await prisma.tag.update({
    where: { id },
    data: { ...(name && { name }), ...(color && { color }) },
    include: { _count: { select: { bookmarks: true } } },
  })

  return NextResponse.json(tag)
}

export async function DELETE(req: NextRequest, { params }: { params: Promise<{ id: string }> }) {
  const user = await getAuthUser()
  if (!user) return NextResponse.json({ error: 'Unauthorized' }, { status: 401 })
  const { id } = await params

  const existing = await prisma.tag.findFirst({ where: { id, ownerId: user.id } })
  if (!existing) return NextResponse.json({ error: 'Not found' }, { status: 404 })

  await prisma.tag.delete({ where: { id } })
  return NextResponse.json({ ok: true })
}
