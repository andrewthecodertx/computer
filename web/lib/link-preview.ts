import { useEffect, useState } from 'react'

// Shared client-side layer over `GET /api/preview?url=`. The Go endpoint
// dials the target site (netguard-protected, 8s bound), so results are
// memoized per URL and concurrent requests are deduplicated. This is what
// makes inline link cards in Markdown pages cheap to render.

export type LinkPreviewData = {
  title: string | null
  description: string | null
  favicon: string | null
  ogImage: string | null
  ogTitle: string | null
  ogDescription: string | null
}

type Entry = { value: LinkPreviewData | null; at: number }

const cache = new Map<string, Entry>()
const inflight = new Map<string, Promise<LinkPreviewData | null>>()

// Failures (offline target, blocked host, endpoint down) are transient, so
// negative results expire quickly instead of sticking for the session.
const NEGATIVE_TTL_MS = 60_000

export function cachedLinkPreview(url: string): LinkPreviewData | null | undefined {
  const hit = cache.get(url)
  if (!hit) return undefined
  if (hit.value === null && Date.now() - hit.at > NEGATIVE_TTL_MS) {
    cache.delete(url)
    return undefined
  }
  return hit.value
}

export async function fetchLinkPreview(url: string): Promise<LinkPreviewData | null> {
  const hit = cachedLinkPreview(url)
  if (hit !== undefined) return hit
  const pending = inflight.get(url)
  if (pending) return pending
  const run = (async () => {
    try {
      const res = await fetch(`/api/preview?url=${encodeURIComponent(url)}`)
      if (!res.ok) return null
      const data = await res.json()
      if (!data || typeof data !== 'object') return null
      const str = (v: unknown) => (typeof v === 'string' && v.trim() ? v : null)
      const value: LinkPreviewData = {
        title: str(data.title),
        description: str(data.description),
        favicon: str(data.favicon),
        ogImage: str(data.ogImage),
        ogTitle: str(data.ogTitle),
        ogDescription: str(data.ogDescription),
      }
      // An object with nothing displayable counts as a miss.
      return value.title || value.description || value.ogImage ? value : null
    } catch {
      return null
    } finally {
      inflight.delete(url)
    }
  })()
  inflight.set(url, run)
  const value = await run
  cache.set(url, { value, at: Date.now() })
  return value
}

// undefined = still loading, null = nothing resolved. The value is derived
// from the module cache during render (identical on server and client first
// paint — no hydration mismatch); the effect only kicks off the fetch and
// requests a re-render once the cache is populated.
export function useLinkPreview(url: string | null): LinkPreviewData | null | undefined {
  const [, bump] = useState(0)
  useEffect(() => {
    if (!url || cachedLinkPreview(url) !== undefined) return
    let active = true
    void fetchLinkPreview(url).then(() => { if (active) bump((n) => n + 1) })
    return () => { active = false }
  }, [url])
  return url ? cachedLinkPreview(url) : undefined
}
