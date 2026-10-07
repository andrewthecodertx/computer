export const dynamic = 'force-dynamic'
import { NextRequest, NextResponse } from 'next/server'
import { prisma } from '@/lib/prisma'
import { getAuthUser } from '@/lib/auth-guard'

export async function GET() {
  const user = await getAuthUser()
  if (!user) return NextResponse.json({ error: 'Unauthorized' }, { status: 401 })

  const dates = await prisma.sharedDate.findMany({
    where: {
      OR: [{ recipientId: user.id }, { creatorId: user.id }],
    },
    include: {
      creator: { select: { id: true, name: true, email: true } },
      recipient: { select: { id: true, name: true, email: true } },
    },
    orderBy: { date: 'asc' },
  })

  return NextResponse.json(dates)
}

export async function POST(req: NextRequest) {
  const user = await getAuthUser()
  if (!user) return NextResponse.json({ error: 'Unauthorized' }, { status: 401 })

  const { date, note, recipientIds } = await req.json()
  if (!date || !recipientIds?.length) {
    return NextResponse.json({ error: 'Date and recipients required' }, { status: 400 })
  }

  const created = []
  for (const rid of recipientIds as string[]) {
    const sd = await prisma.sharedDate.create({
      data: { date: new Date(date), note: note ?? null, creatorId: user.id, recipientId: rid },
    })
    created.push(sd)
  }

  return NextResponse.json(created, { status: 201 })
}
