export type PageSaveState = 'saved' | 'saving' | 'dirty'
type Draft = {
  content: string
  revision: number
  savedRevision: number
  timer: ReturnType<typeof setTimeout> | null
  saving: Promise<boolean> | null
}

// Each page owns its debounce and serial save queue. A late response cannot
// overwrite a newer edit, and tab-local drafts survive a failed save/navigation.
export class PageAutosave {
  private drafts = new Map<string, Draft>()
  constructor(
    private save: (id: string, content: string) => Promise<void>,
    private state: (id: string, state: PageSaveState) => void,
  ) {}

  private key(id: string) { return `computer:page-draft:${id}` }
  private store(id: string, content: string | null) {
    try {
      if (content === null) sessionStorage.removeItem(this.key(id))
      else sessionStorage.setItem(this.key(id), JSON.stringify(content))
    } catch { /* Saving to the API still works if browser storage is unavailable. */ }
  }

  draft(id: string): string | undefined {
    const existing = this.drafts.get(id)
    if (existing) return existing.revision > existing.savedRevision ? existing.content : undefined
    try {
      const raw = sessionStorage.getItem(this.key(id))
      if (raw !== null) {
        const content: unknown = JSON.parse(raw)
        if (typeof content === 'string') {
          this.drafts.set(id, { content, revision: 1, savedRevision: 0, timer: null, saving: null })
          this.state(id, 'dirty')
          return content
        }
      }
    } catch { /* Ignore unavailable storage or malformed drafts. */ }
    return undefined
  }

  edit(id: string, content: string) {
    const draft = this.drafts.get(id) ?? { content, revision: 0, savedRevision: 0, timer: null, saving: null }
    draft.content = content
    draft.revision++
    if (draft.timer) clearTimeout(draft.timer)
    this.drafts.set(id, draft)
    this.store(id, content)
    this.state(id, 'dirty')
    draft.timer = setTimeout(() => { void this.flush(id) }, 800)
  }

  async flush(id: string): Promise<boolean> {
    const draft = this.drafts.get(id)
    if (!draft) return true
    if (draft.timer) { clearTimeout(draft.timer); draft.timer = null }
    if (draft.saving) return draft.saving
    if (draft.revision === draft.savedRevision) return true
    draft.saving = this.persist(id, draft)
    try { return await draft.saving }
    finally { draft.saving = null }
  }

  private async persist(id: string, draft: Draft): Promise<boolean> {
    while (draft.savedRevision < draft.revision) {
      const revision = draft.revision
      const content = draft.content
      this.state(id, 'saving')
      try { await this.save(id, content) }
      catch { this.state(id, 'dirty'); return false }
      draft.savedRevision = revision
    }
    if (draft.timer) { clearTimeout(draft.timer); draft.timer = null }
    this.store(id, null)
    this.state(id, 'saved')
    return true
  }

  hasUnsaved() { return [...this.drafts.values()].some(draft => draft.revision > draft.savedRevision) }
  async flushAll() { return (await Promise.all([...this.drafts.keys()].map(id => this.flush(id)))).every(Boolean) }
  forget(id: string) {
    const draft = this.drafts.get(id)
    if (draft?.timer) clearTimeout(draft.timer)
    this.drafts.delete(id)
    this.store(id, null)
  }
}
