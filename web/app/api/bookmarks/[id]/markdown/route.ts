export const dynamic = 'force-dynamic'
import { NextRequest, NextResponse } from 'next/server'
import { prisma } from '@/lib/prisma'
import { getAuthUser } from '@/lib/auth-guard'
import { bookmarkToMarkdown, markdownResponse, slugify } from '@/lib/markdown-export'

export async function GET(req: NextRequest, { params }: { params: Promise<{ id: string }> }) {
  const user = await getAuthUser()
  if (!user) return NextResponse.json({ error: 'Unauthorized' }, { status: 401 })
  const { id } = await params
  const b = await prisma.bookmark.findFirst({
    where: { id, OR: [{ ownerId: user.id }, { sharedWith: { some: { userId: user.id } } }] },
    include: { tags: { include: { tag: { select: { name: true } } } } },
  })
  if (!b) return NextResponse.json({ error: 'Not found' }, { status: 404 })
  return markdownResponse(bookmarkToMarkdown(b), slugify(b.title ?? b.url))
}
