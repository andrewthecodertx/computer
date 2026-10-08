import type { Bookmark } from '@/components/app-shell'

// The API keeps its array response; follow its cursor until the collection is complete.
export async function fetchBookmarks(query = new URLSearchParams(), signal?: AbortSignal): Promise<Bookmark[]> {
  const params = new URLSearchParams(query)
  const bookmarks = new Map<string, Bookmark>()
  const seen = new Set<string>()
  for (;;) {
    const response = await fetch(`/api/bookmarks?${params}`, { signal })
    if (!response.ok) throw new Error('Could not load bookmarks')
    const page: Bookmark[] = await response.json()
    for (const bookmark of page) bookmarks.set(bookmark.id, bookmark)
    const cursor = response.headers.get('X-Next-Cursor')
    if (!cursor) return [...bookmarks.values()]
    if (seen.has(cursor)) throw new Error('Invalid bookmark pagination')
    seen.add(cursor)
    params.set('cursor', cursor)
  }
}
