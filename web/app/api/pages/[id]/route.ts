export const dynamic = 'force-dynamic'
import { NextRequest, NextResponse } from 'next/server'
import { prisma } from '@/lib/prisma'
import { getAuthUser } from '@/lib/auth-guard'

const bookmarkSelect = {
  id: true, url: true, title: true, favicon: true, ogImage: true, ogTitle: true, ogDescription: true, description: true,
}

export async function GET(req: NextRequest, { params }: { params: Promise<{ id: string }> }) {
  const user = await getAuthUser()
  if (!user) return NextResponse.json({ error: 'Unauthorized' }, { status: 401 })
  const { id } = await params
  const page = await prisma.page.findFirst({
    where: { id, ownerId: user.id },
    include: { bookmarks: { orderBy: { position: 'asc' }, include: { bookmark: { select: bookmarkSelect } } } },
  })
  if (!page) return NextResponse.json({ error: 'Not found' }, { status: 404 })
  return NextResponse.json(page)
}

// PATCH { title?, content?, pinned?, icon?, addBookmarkId?, removeBookmarkId? }
export async function PATCH(req: NextRequest, { params }: { params: Promise<{ id: string }> }) {
  const user = await getAuthUser()
  if (!user) return NextResponse.json({ error: 'Unauthorized' }, { status: 401 })
  const { id } = await params
  const page = await prisma.page.findFirst({ where: { id, ownerId: user.id }, select: { id: true } })
  if (!page) return NextResponse.json({ error: 'Not found' }, { status: 404 })
  const body = await req.json()

  if (body.addBookmarkId) {
    const bm = await prisma.bookmark.findFirst({
      where: { id: body.addBookmarkId, OR: [{ ownerId: user.id }, { sharedWith: { some: { userId: user.id } } }] },
      select: { id: true },
    })
    if (!bm) return NextResponse.json({ error: 'Bookmark not found' }, { status: 404 })
    const count = await prisma.pageBookmark.count({ where: { pageId: id } })
    await prisma.pageBookmark.upsert({
      where: { pageId_bookmarkId: { pageId: id, bookmarkId: bm.id } },
      update: {},
      create: { pageId: id, bookmarkId: bm.id, position: count },
    })
  }
  if (body.removeBookmarkId) {
    await prisma.pageBookmark.deleteMany({ where: { pageId: id, bookmarkId: body.removeBookmarkId } })
  }

  const updated = await prisma.page.update({
    where: { id },
    data: {
      ...(typeof body.title === 'string' && { title: body.title.trim().slice(0, 120) || 'Untitled page' }),
      ...(typeof body.content === 'string' && { content: body.content }),
      ...(typeof body.pinned === 'boolean' && { pinned: body.pinned }),
      ...(body.icon !== undefined && { icon: body.icon || null }),
    },
    include: { bookmarks: { orderBy: { position: 'asc' }, include: { bookmark: { select: bookmarkSelect } } } },
  })
  return NextResponse.json(updated)
}

export async function DELETE(req: NextRequest, { params }: { params: Promise<{ id: string }> }) {
  const user = await getAuthUser()
  if (!user) return NextResponse.json({ error: 'Unauthorized' }, { status: 401 })
  const { id } = await params
  const page = await prisma.page.findFirst({ where: { id, ownerId: user.id }, select: { id: true } })
  if (!page) return NextResponse.json({ error: 'Not found' }, { status: 404 })
  await prisma.page.delete({ where: { id } })
  return NextResponse.json({ ok: true })
}
