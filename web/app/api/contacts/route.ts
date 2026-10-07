export const dynamic = 'force-dynamic'
import { NextResponse } from 'next/server'
import { prisma } from '@/lib/prisma'
import { getAuthUser } from '@/lib/auth-guard'

export async function GET() {
  const user = await getAuthUser()
  if (!user) return NextResponse.json({ error: 'Unauthorized' }, { status: 401 })

  const contacts = await prisma.contact.findMany({
    where: { userId: user.id },
    include: { _count: { select: { bookmarks: true } } },
    orderBy: { displayName: 'asc' },
  })

  return NextResponse.json(contacts)
}
