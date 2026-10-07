export const dynamic = 'force-dynamic'
import { NextRequest, NextResponse } from 'next/server'
import { prisma } from '@/lib/prisma'
import { getAuthUser } from '@/lib/auth-guard'
import { markdownResponse, pageToMarkdown, slugify } from '@/lib/markdown-export'

export async function GET(req: NextRequest, { params }: { params: Promise<{ id: string }> }) {
  const user = await getAuthUser()
  if (!user) return NextResponse.json({ error: 'Unauthorized' }, { status: 401 })
  const { id } = await params
  const page = await prisma.page.findFirst({
    where: { id, ownerId: user.id },
    include: { bookmarks: { orderBy: { position: 'asc' }, include: { bookmark: { select: { url: true, title: true, description: true, ogDescription: true } } } } },
  })
  if (!page) return NextResponse.json({ error: 'Not found' }, { status: 404 })
  return markdownResponse(pageToMarkdown(page), slugify(page.title))
}
