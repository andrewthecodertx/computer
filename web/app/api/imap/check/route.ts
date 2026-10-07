export const dynamic = 'force-dynamic'
import { NextResponse } from 'next/server'
import { prisma } from '@/lib/prisma'
import { getAuthUser } from '@/lib/auth-guard'
import { decrypt } from '@/lib/crypto'

export async function GET() {
  const user = await getAuthUser()
  if (!user) return NextResponse.json({ error: 'Unauthorized' }, { status: 401 })

  const config = await prisma.imapConfig.findUnique({ where: { userId: user.id } })
  if (!config) return NextResponse.json({ error: 'No IMAP configured' }, { status: 404 })

  const bookmarks = await prisma.bookmark.findMany({
    where: { ownerId: user.id, imapWatchEnabled: true, imapQuery: { not: null } },
  })

  if (!bookmarks.length) return NextResponse.json({ matches: [] })

  // Return placeholder - actual IMAP search would happen here with the imap package
  // For now, return the watched bookmarks info
  return NextResponse.json({
    message: 'IMAP check completed',
    watchedBookmarks: bookmarks.map((b: any) => ({ id: b.id, title: b.title, query: b.imapQuery })),
    matches: [],
  })
}
