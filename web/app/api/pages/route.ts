export const dynamic = 'force-dynamic'
import { NextRequest, NextResponse } from 'next/server'
import { prisma } from '@/lib/prisma'
import { getAuthUser } from '@/lib/auth-guard'

// GET ?search= lists pages (pinned first, then by position).
export async function GET(req: NextRequest) {
  const user = await getAuthUser()
  if (!user) return NextResponse.json({ error: 'Unauthorized' }, { status: 401 })
  const search = req.nextUrl.searchParams.get('search')?.trim()
  const pages = await prisma.page.findMany({
    where: {
      ownerId: user.id,
      ...(search && { OR: [{ title: { contains: search, mode: 'insensitive' } }, { content: { contains: search, mode: 'insensitive' } }] }),
    },
    select: { id: true, title: true, icon: true, pinned: true, position: true, updatedAt: true, _count: { select: { bookmarks: true } } },
    orderBy: [{ pinned: 'desc' }, { position: 'asc' }, { createdAt: 'asc' }],
  })
  return NextResponse.json(pages)
}

export async function POST(req: NextRequest) {
  const user = await getAuthUser()
  if (!user) return NextResponse.json({ error: 'Unauthorized' }, { status: 401 })
  const { title, content } = await req.json().catch(() => ({}))
  const count = await prisma.page.count({ where: { ownerId: user.id } })
  const page = await prisma.page.create({
    data: { title: (title?.trim() || 'Untitled page').slice(0, 120), content: content ?? '', position: count, ownerId: user.id },
  })
  return NextResponse.json(page, { status: 201 })
}

// PUT { order: string[] } reorders pages.
export async function PUT(req: NextRequest) {
  const user = await getAuthUser()
  if (!user) return NextResponse.json({ error: 'Unauthorized' }, { status: 401 })
  const { order } = await req.json()
  if (!Array.isArray(order)) return NextResponse.json({ error: 'order required' }, { status: 400 })
  const owned = new Set((await prisma.page.findMany({ where: { ownerId: user.id }, select: { id: true } })).map((p) => p.id))
  await prisma.$transaction(
    (order as string[]).filter((id) => owned.has(id)).map((id, i) => prisma.page.update({ where: { id }, data: { position: i } })),
  )
  return NextResponse.json({ ok: true })
}
