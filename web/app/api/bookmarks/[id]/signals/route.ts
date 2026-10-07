export const dynamic = 'force-dynamic'
import { NextRequest, NextResponse } from 'next/server'
import { prisma } from '@/lib/prisma'
import { getAuthUser } from '@/lib/auth-guard'
import { SIGNAL_SOURCES, publicSourceTypes, runSignalChecks } from '@/lib/signals'

// GET: watchers + signals for a bookmark, plus available source types.
export async function GET(req: NextRequest, { params }: { params: Promise<{ id: string }> }) {
  const user = await getAuthUser()
  if (!user) return NextResponse.json({ error: 'Unauthorized' }, { status: 401 })
  const { id } = await params
  const bm = await prisma.bookmark.findFirst({ where: { id, ownerId: user.id }, select: { id: true } })
  if (!bm) return NextResponse.json({ sources: [], signals: [], types: publicSourceTypes() })
  const [sources, signals] = await Promise.all([
    prisma.signalSource.findMany({ where: { bookmarkId: id }, orderBy: { createdAt: 'asc' } }),
    prisma.signal.findMany({ where: { bookmarkId: id }, orderBy: { occurredAt: 'desc' }, take: 50 }),
  ])
  return NextResponse.json({ sources, signals, types: publicSourceTypes() })
}

// POST { type, config } adds a watcher.  POST { action: 'check' } runs this bookmark's watchers now.
export async function POST(req: NextRequest, { params }: { params: Promise<{ id: string }> }) {
  const user = await getAuthUser()
  if (!user) return NextResponse.json({ error: 'Unauthorized' }, { status: 401 })
  const { id } = await params
  const bm = await prisma.bookmark.findFirst({ where: { id, ownerId: user.id }, select: { id: true } })
  if (!bm) return NextResponse.json({ error: 'Not found' }, { status: 404 })
  const body = await req.json()
  if (body?.action === 'check') return NextResponse.json(await runSignalChecks(user.id, id))

  const def = SIGNAL_SOURCES[body?.type]
  if (!def) return NextResponse.json({ error: 'Unknown source type' }, { status: 400 })
  const config: Record<string, string> = {}
  for (const f of def.configFields) {
    const v = body?.config?.[f.key]
    if (typeof v === 'string' && v.trim()) config[f.key] = v.trim().slice(0, 200)
  }
  const source = await prisma.signalSource.create({ data: { type: def.type, config, bookmarkId: id, ownerId: user.id } })
  return NextResponse.json(source, { status: 201 })
}
