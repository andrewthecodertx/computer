export const PAGES_CHANGED = 'computer:pages-changed'

export const notifyPagesChanged = () => window.dispatchEvent(new Event(PAGES_CHANGED))

export type PageSummary = { id: string; title: string; pinned: boolean; position: number; _count?: { bookmarks: number } }

// Pages start as "Untitled 1", "Untitled 2", ... until the user renames them.
// Pick the lowest positive integer not already used by an untitled page.
export function nextUntitledTitle(titles: string[]): string {
  const used = new Set<number>()
  for (const t of titles) {
    const m = /^Untitled (\d+)$/.exec(t.trim())
    if (m) used.add(parseInt(m[1], 10))
  }
  let n = 1
  while (used.has(n)) n++
  return `Untitled ${n}`
}
