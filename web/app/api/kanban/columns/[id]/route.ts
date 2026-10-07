export const dynamic = 'force-dynamic'
import { NextRequest, NextResponse } from 'next/server'
import { prisma } from '@/lib/prisma'
import { getAuthUser } from '@/lib/auth-guard'
import { getColumns, resolveColumnId } from '@/lib/kanban'

export async function PATCH(req: NextRequest, { params }: { params: Promise<{ id: string }> }) {
  const user = await getAuthUser()
  if (!user) return NextResponse.json({ error: 'Unauthorized' }, { status: 401 })
  const { id } = await params
  const col = await prisma.kanbanColumn.findFirst({ where: { id, userId: user.id } })
  if (!col) return NextResponse.json({ error: 'Not found' }, { status: 404 })
  const { label, color } = await req.json()
  const updated = await prisma.kanbanColumn.update({
    where: { id },
    data: { ...(label?.trim() && { label: label.trim().slice(0, 40) }), ...(color && { color }) },
  })
  return NextResponse.json(updated)
}

// Deleting a column never deletes bookmarks: they move to the first remaining column.
export async function DELETE(req: NextRequest, { params }: { params: Promise<{ id: string }> }) {
  const user = await getAuthUser()
  if (!user) return NextResponse.json({ error: 'Unauthorized' }, { status: 401 })
  const { id } = await params
  const cols = await getColumns(user.id)
  if (!cols.some((c) => c.id === id)) return NextResponse.json({ error: 'Not found' }, { status: 404 })
  const remaining = cols.filter((c) => c.id !== id)
  if (remaining.length === 0) return NextResponse.json({ error: 'A board needs at least one column' }, { status: 400 })
  const target = remaining[0]

  const bookmarks = await prisma.bookmark.findMany({ where: { ownerId: user.id }, select: { id: true, kanbanColumnId: true, kanbanStatus: true } })
  const moving = bookmarks.filter((b) => resolveColumnId(b, cols) === id).map((b) => b.id)
  await prisma.$transaction([
    prisma.bookmark.updateMany({ where: { id: { in: moving } }, data: { kanbanColumnId: target.id } }),
    prisma.kanbanColumn.delete({ where: { id } }),
    ...remaining.map((c, i) => prisma.kanbanColumn.update({ where: { id: c.id }, data: { position: i } })),
  ])
  return NextResponse.json({ ok: true, movedTo: target.id, moved: moving.length })
}
