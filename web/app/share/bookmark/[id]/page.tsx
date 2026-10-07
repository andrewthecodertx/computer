import { prisma } from '@/lib/prisma'
import { notFound } from 'next/navigation'
import { PublicBookmarkView } from './client'

export default async function PublicSharePage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params

  const bookmark = await prisma.bookmark.findFirst({
    where: { id, isPublic: true },
    include: { tags: { include: { tag: true } }, contact: true, owner: { select: { name: true } } },
  })

  if (!bookmark) notFound()

  return <PublicBookmarkView bookmark={JSON.parse(JSON.stringify(bookmark))} />
}
