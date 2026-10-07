export const dynamic = 'force-dynamic'
import { NextRequest, NextResponse } from 'next/server'
import { prisma } from '@/lib/prisma'
import { getAuthUser } from '@/lib/auth-guard'

export async function GET(req: NextRequest) {
  const user = await getAuthUser()
  if (!user) return NextResponse.json({ error: 'Unauthorized' }, { status: 401 })

  const sp = req.nextUrl.searchParams
  const search = sp.get('search') ?? undefined
  const tagId = sp.get('tagId') ?? undefined
  const contactId = sp.get('contactId') ?? undefined
  const kanban = sp.get('kanban') ?? undefined
  const date = sp.get('date') ?? undefined
  const shared = sp.get('shared') === 'true'

  const where: any = {}

  if (shared) {
    where.sharedWith = { some: { userId: user.id } }
  } else {
    where.ownerId = user.id
  }

  if (search) {
    where.OR = [
      { title: { contains: search, mode: 'insensitive' } },
      { url: { contains: search, mode: 'insensitive' } },
      { notes: { contains: search, mode: 'insensitive' } },
      { tags: { some: { tag: { name: { contains: search, mode: 'insensitive' } } } } },
    ]
  }
  if (tagId) where.tags = { some: { tagId } }
  if (contactId) where.contactId = contactId
  if (kanban) where.kanbanStatus = kanban
  if (date) {
    const d = new Date(date)
    const next = new Date(d)
    next.setDate(next.getDate() + 1)
    where.dueDate = { gte: d, lt: next }
  }

  const bookmarks = await prisma.bookmark.findMany({
    where,
    include: { tags: { include: { tag: true } }, contact: true, sharedWith: true, owner: { select: { id: true, name: true, email: true, image: true } } },
    orderBy: { updatedAt: 'desc' },
    take: 200,
  })

  return NextResponse.json(bookmarks)
}

export async function POST(req: NextRequest) {
  const user = await getAuthUser()
  if (!user) return NextResponse.json({ error: 'Unauthorized' }, { status: 401 })

  const body = await req.json()
  const { url, title, description, notes, dueDate, alertAt, kanbanStatus, contactId, tagIds, isPublic, imapWatchEnabled, imapQuery } = body

  if (!url) return NextResponse.json({ error: 'URL required' }, { status: 400 })

  // Fetch OG preview
  let ogData: any = {}
  try {
    const ogs = (await import('open-graph-scraper')).default
    const { result } = await ogs({ url, timeout: 8000, fetchOptions: { headers: { 'User-Agent': 'Mozilla/5.0 (compatible; computer/1.0)' } } })
    let favicon = result?.favicon ?? null
    if (favicon && !favicon.startsWith('http')) {
      const parsed = new URL(url)
      favicon = favicon.startsWith('/') ? `${parsed.origin}${favicon}` : `${parsed.origin}/${favicon}`
    }
    if (!favicon) try { favicon = `${new URL(url).origin}/favicon.ico` } catch { /* noop */ }
    ogData = {
      ogTitle: result?.ogTitle ?? null,
      ogDescription: result?.ogDescription ?? null,
      ogImage: result?.ogImage?.[0]?.url ?? null,
      favicon,
    }
  } catch { /* ignore */ }

  const bookmark = await prisma.bookmark.create({
    data: {
      url,
      title: title ?? ogData.ogTitle ?? url,
      description: description ?? ogData.ogDescription ?? null,
      favicon: ogData.favicon ?? null,
      ogImage: ogData.ogImage ?? null,
      ogTitle: ogData.ogTitle ?? null,
      ogDescription: ogData.ogDescription ?? null,
      notes: notes ?? null,
      dueDate: dueDate ? new Date(dueDate) : null,
      alertAt: alertAt ? new Date(alertAt) : null,
      kanbanStatus: kanbanStatus ?? 'INBOX',
      contactId: contactId ?? null,
      isPublic: isPublic ?? false,
      imapWatchEnabled: imapWatchEnabled ?? false,
      imapQuery: imapQuery ?? null,
      ownerId: user.id,
      tags: tagIds?.length ? { create: (tagIds as string[]).map((tagId: string) => ({ tagId })) } : undefined,
    },
    include: { tags: { include: { tag: true } }, contact: true, sharedWith: true },
  })

  return NextResponse.json(bookmark, { status: 201 })
}
