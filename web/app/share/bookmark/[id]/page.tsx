import { apiBase } from '@/lib/api'
import { notFound } from 'next/navigation'
import { PublicBookmarkView } from './client'

export default async function PublicSharePage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params

  const response = await fetch(`${apiBase()}/api/public/bookmarks/${encodeURIComponent(id)}`, { cache: 'no-store' })
  if (!response.ok) notFound()
  const bookmark = await response.json()

  return <PublicBookmarkView bookmark={JSON.parse(JSON.stringify(bookmark))} />
}
