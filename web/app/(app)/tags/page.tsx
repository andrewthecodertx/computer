import { TagsClient } from './client'

// Quick Find deep-links to a tag via /tags?tag=<id>.
export default async function TagsPage({ searchParams }: { searchParams: Promise<{ tag?: string }> }) {
  const { tag } = await searchParams
  return <TagsClient initialTag={tag ?? null} />
}
