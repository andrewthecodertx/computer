import { BookmarksClient } from './client'

// Quick Find deep-links (?open=<id>, ?search=<q>) arrive through searchParams
// so the client reacts to query changes without remounting.
export default async function BookmarksPage({ searchParams }: { searchParams: Promise<{ search?: string; open?: string }> }) {
  const { search, open } = await searchParams
  return <BookmarksClient initialSearch={search ?? null} initialOpen={open ?? null} />
}
