// Short-TTL cache with in-flight deduplication for the reference data the
// bookmark detail sheet needs on every open (identity, kanban columns,
// contacts, tags). Mutation sites call invalidateReference() so a just-created
// tag/column/contact shows up immediately instead of after the TTL.

export type ReferenceKind = 'me' | 'columns' | 'contacts' | 'tags'

const PATHS: Record<ReferenceKind, string> = {
  me: '/api/me',
  columns: '/api/kanban/columns',
  contacts: '/api/contacts',
  tags: '/api/tags',
}

const TTL_MS = 30_000
const cache = new Map<ReferenceKind, { at: number; promise: Promise<unknown> }>()

export function invalidateReference(kind: ReferenceKind) {
  cache.delete(kind)
}

// Resolves to null on any failure; failures are not kept cached.
export function fetchReference<T = unknown>(kind: ReferenceKind): Promise<T | null> {
  const hit = cache.get(kind)
  if (hit && Date.now() - hit.at < TTL_MS) return hit.promise as Promise<T | null>
  const promise = (async () => {
    try {
      const res = await fetch(PATHS[kind])
      return res.ok ? ((await res.json()) as T) : null
    } catch {
      return null
    }
  })()
  const entry = { at: Date.now(), promise }
  cache.set(kind, entry)
  void promise.then((v) => { if (v === null && cache.get(kind) === entry) cache.delete(kind) })
  return promise
}
