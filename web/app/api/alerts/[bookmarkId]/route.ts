export const dynamic = 'force-dynamic'
import { NextRequest, NextResponse } from 'next/server'
import { prisma } from '@/lib/prisma'
import { getAuthUser } from '@/lib/auth-guard'

export async function DELETE(req: NextRequest, { params }: { params: Promise<{ bookmarkId: string }> }) {
  const user = await getAuthUser()
  if (!user) return NextResponse.json({ error: 'Unauthorized' }, { status: 401 })
  const { bookmarkId } = await params

  await prisma.bookmark.updateMany({
    where: { id: bookmarkId, ownerId: user.id },
    data: { alertSent: true },
  })

  return NextResponse.json({ ok: true })
}
