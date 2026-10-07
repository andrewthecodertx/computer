export const dynamic = 'force-dynamic'
import { NextRequest, NextResponse } from 'next/server'
import { prisma } from '@/lib/prisma'
import { getAuthUser } from '@/lib/auth-guard'

export async function PATCH(req: NextRequest, { params }: { params: Promise<{ id: string }> }) {
  const user = await getAuthUser()
  if (!user) return NextResponse.json({ error: 'Unauthorized' }, { status: 401 })
  const { id } = await params
  const s = await prisma.signalSource.findFirst({ where: { id, ownerId: user.id } })
  if (!s) return NextResponse.json({ error: 'Not found' }, { status: 404 })
  const { enabled } = await req.json()
  return NextResponse.json(await prisma.signalSource.update({ where: { id }, data: { enabled: !!enabled } }))
}

export async function DELETE(req: NextRequest, { params }: { params: Promise<{ id: string }> }) {
  const user = await getAuthUser()
  if (!user) return NextResponse.json({ error: 'Unauthorized' }, { status: 401 })
  const { id } = await params
  const s = await prisma.signalSource.findFirst({ where: { id, ownerId: user.id } })
  if (!s) return NextResponse.json({ error: 'Not found' }, { status: 404 })
  await prisma.signalSource.delete({ where: { id } })
  return NextResponse.json({ ok: true })
}
