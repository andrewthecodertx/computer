export const dynamic = 'force-dynamic'
import { NextRequest, NextResponse } from 'next/server'
import { prisma } from '@/lib/prisma'
import { getAuthUser } from '@/lib/auth-guard'

export async function GET(req: NextRequest, { params }: { params: Promise<{ id: string }> }) {
  const user = await getAuthUser()
  if (!user) return NextResponse.json({ error: 'Unauthorized' }, { status: 401 })
  const { id } = await params

  const bookmark = await prisma.bookmark.findFirst({
    where: {
      id,
      OR: [{ ownerId: user.id }, { sharedWith: { some: { userId: user.id } } }, { isPublic: true }],
    },
    include: { tags: { include: { tag: true } }, contact: true, sharedWith: { include: { user: { select: { id: true, name: true, email: true } } } }, owner: { select: { id: true, name: true, email: true, image: true } }, pageLinks: { where: { page: { ownerId: user.id } }, include: { page: { select: { id: true, title: true } } } } },
  })

  if (!bookmark) return NextResponse.json({ error: 'Not found' }, { status: 404 })
  return NextResponse.json(bookmark)
}

export async function PUT(req: NextRequest, { params }: { params: Promise<{ id: string }> }) {
  const user = await getAuthUser()
  if (!user) return NextResponse.json({ error: 'Unauthorized' }, { status: 401 })
  const { id } = await params

  const existing = await prisma.bookmark.findFirst({ where: { id, ownerId: user.id } })
  if (!existing) return NextResponse.json({ error: 'Not found' }, { status: 404 })

  const body = await req.json()
  const { url, title, description, notes, dueDate, alertAt, kanbanStatus, kanbanColumnId, contactId, tagIds, isPublic, imapWatchEnabled, imapQuery } = body

  if (kanbanColumnId) {
    const col = await prisma.kanbanColumn.findFirst({ where: { id: kanbanColumnId, userId: user.id }, select: { id: true } })
    if (!col) return NextResponse.json({ error: 'Invalid column' }, { status: 400 })
  }

  // Re-fetch OG if URL changed
  let ogData: any = {}
  if (url && url !== existing.url) {
    try {
      const ogs = (await import('open-graph-scraper')).default
      const { result } = await ogs({ url, timeout: 8000, fetchOptions: { headers: { 'User-Agent': 'Mozilla/5.0 (compatible; computer/1.0)' } } })
      let favicon = result?.favicon ?? null
      if (favicon && !favicon.startsWith('http')) {
        const parsed = new URL(url)
        favicon = favicon.startsWith('/') ? `${parsed.origin}${favicon}` : `${parsed.origin}/${favicon}`
      }
      if (!favicon) try { favicon = `${new URL(url).origin}/favicon.ico` } catch { /* noop */ }
      ogData = { ogTitle: result?.ogTitle, ogDescription: result?.ogDescription, ogImage: result?.ogImage?.[0]?.url, favicon }
    } catch { /* ignore */ }
  }

  if (tagIds !== undefined) {
    await prisma.bookmarkTag.deleteMany({ where: { bookmarkId: id } })
    if (tagIds?.length) {
      await prisma.bookmarkTag.createMany({ data: (tagIds as string[]).map((tagId: string) => ({ bookmarkId: id, tagId })) })
    }
  }

  const bookmark = await prisma.bookmark.update({
    where: { id },
    data: {
      ...(url !== undefined && { url }),
      ...(title !== undefined && { title }),
      ...(description !== undefined && { description }),
      ...(notes !== undefined && { notes }),
      ...(dueDate !== undefined && { dueDate: dueDate ? new Date(dueDate) : null }),
      ...(alertAt !== undefined && { alertAt: alertAt ? new Date(alertAt) : null, alertSent: false }),
      ...(kanbanStatus !== undefined && { kanbanStatus }),
      ...(kanbanColumnId !== undefined && { kanbanColumnId: kanbanColumnId || null }),
      ...(contactId !== undefined && { contactId: contactId || null }),
      ...(isPublic !== undefined && { isPublic }),
      ...(imapWatchEnabled !== undefined && { imapWatchEnabled }),
      ...(imapQuery !== undefined && { imapQuery }),
      ...(ogData.ogTitle && { ogTitle: ogData.ogTitle, ogDescription: ogData.ogDescription, ogImage: ogData.ogImage, favicon: ogData.favicon }),
    },
    include: { tags: { include: { tag: true } }, contact: true, sharedWith: true },
  })

  return NextResponse.json(bookmark)
}

export async function DELETE(req: NextRequest, { params }: { params: Promise<{ id: string }> }) {
  const user = await getAuthUser()
  if (!user) return NextResponse.json({ error: 'Unauthorized' }, { status: 401 })
  const { id } = await params

  const existing = await prisma.bookmark.findFirst({ where: { id, ownerId: user.id } })
  if (!existing) return NextResponse.json({ error: 'Not found' }, { status: 404 })

  await prisma.bookmark.delete({ where: { id } })
  return NextResponse.json({ ok: true })
}
