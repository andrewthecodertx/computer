export const dynamic = 'force-dynamic'
import { NextResponse } from 'next/server'
import { prisma } from '@/lib/prisma'
import { getAuthUser } from '@/lib/auth-guard'

export async function GET() {
  const user = await getAuthUser()
  if (!user) return NextResponse.json({ error: 'Unauthorized' }, { status: 401 })

  const now = new Date()
  const soon = new Date(now.getTime() + 15 * 60 * 1000)

  const alerts = await prisma.bookmark.findMany({
    where: {
      ownerId: user.id,
      alertAt: { lte: soon },
      alertSent: false,
    },
    select: { id: true, title: true, url: true, alertAt: true },
    orderBy: { alertAt: 'asc' },
  })

  return NextResponse.json(alerts)
}
