export const dynamic = 'force-dynamic'
import { NextRequest, NextResponse } from 'next/server'
import { prisma } from '@/lib/prisma'
import { getAuthUser } from '@/lib/auth-guard'
import { getColumns } from '@/lib/kanban'

export async function GET() {
  const user = await getAuthUser()
  if (!user) return NextResponse.json({ error: 'Unauthorized' }, { status: 401 })
  return NextResponse.json(await getColumns(user.id))
}

export async function POST(req: NextRequest) {
  const user = await getAuthUser()
  if (!user) return NextResponse.json({ error: 'Unauthorized' }, { status: 401 })
  const { label, color } = await req.json()
  if (!label?.trim()) return NextResponse.json({ error: 'Label required' }, { status: 400 })
  const cols = await getColumns(user.id)
  const col = await prisma.kanbanColumn.create({
    data: { label: label.trim().slice(0, 40), color: color ?? '#8b5cf6', position: cols.length, userId: user.id },
  })
  return NextResponse.json(col, { status: 201 })
}

// PUT { order: string[] } reorders columns.
export async function PUT(req: NextRequest) {
  const user = await getAuthUser()
  if (!user) return NextResponse.json({ error: 'Unauthorized' }, { status: 401 })
  const { order } = await req.json()
  if (!Array.isArray(order)) return NextResponse.json({ error: 'order required' }, { status: 400 })
  const cols = await getColumns(user.id)
  const owned = new Set(cols.map((c) => c.id))
  await prisma.$transaction(
    (order as string[]).filter((id) => owned.has(id)).map((id, i) => prisma.kanbanColumn.update({ where: { id }, data: { position: i } })),
  )
  return NextResponse.json(await getColumns(user.id))
}
