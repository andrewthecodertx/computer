export type PageSaveState = 'saved' | 'saving' | 'dirty' | 'conflict'

type Draft = {
  content: string
  revision: number
  savedRevision: number
  // The server version this content was edited from (review DATA-03). It is
  // persisted with the draft so a recovery after reload cannot silently
  // re-base onto a newer remote version.
  baseVersion: number | undefined
  // Server version reported by a 409 or detected during recovery, while
  // autosave is paused (review DATA-04). Typing never resolves a conflict.
  conflict: number | null
  timer: ReturnType<typeof setTimeout> | null
  saving: Promise<boolean> | null
}

// Storage envelope: content plus the server version it was based on. Plain
// string drafts written by older builds are still recovered (base unknown).
type Envelope = { content: string; baseVersion?: number }

// Thrown by the save callback when the server rejects a stale base version
// (optimistic concurrency, review DATA-01). serverVersion is the revision the
// server reported; the queue enters a paused conflict state instead of
// adopting it — overwriting requires an explicit user decision (DATA-04).
export class PageConflictError extends Error {
  constructor(readonly serverVersion?: number) { super('Page was modified elsewhere') }
}

// Each page owns its debounce and serial save queue. A late response cannot
// overwrite a newer edit, and tab-local drafts survive a failed save/navigation.
export class PageAutosave {
  private drafts = new Map<string, Draft>()
  // Server revision per page: sent with content saves and refreshed from save
  // responses, so concurrent tabs/devices get 409s instead of silent overwrites.
  private versions = new Map<string, number>()
  constructor(
    // Resolves with the server's new version when the response carries one.
    private save: (id: string, content: string, version: number | undefined) => Promise<number | void>,
    private state: (id: string, state: PageSaveState) => void,
  ) {}

  setVersion(id: string, version: number) { this.versions.set(id, version) }

  private key(id: string) { return `computer:page-draft:${id}` }
  private store(id: string, draft: Draft | null) {
    try {
      if (draft === null) sessionStorage.removeItem(this.key(id))
      else {
        const envelope: Envelope = { content: draft.content }
        if (draft.baseVersion !== undefined) envelope.baseVersion = draft.baseVersion
        sessionStorage.setItem(this.key(id), JSON.stringify(envelope))
      }
    } catch { /* Saving to the API still works if browser storage is unavailable. */ }
  }

  // restore recovers an unsaved draft for id, comparing the version it was
  // based on with the freshly fetched server version (review DATA-03). A
  // mismatch means another session saved in between: the draft enters the
  // paused conflict state instead of being rebased onto the newer version.
  restore(id: string, serverVersion?: number): string | undefined {
    let draft = this.drafts.get(id)
    if (draft && draft.revision <= draft.savedRevision) draft = undefined
    if (!draft) {
      try {
        const raw = sessionStorage.getItem(this.key(id))
        if (raw !== null) {
          const parsed: unknown = JSON.parse(raw)
          let content: string | undefined
          let baseVersion: number | undefined
          if (typeof parsed === 'string') content = parsed // legacy plain-string draft
          else if (parsed && typeof parsed === 'object') {
            const envelope = parsed as Envelope
            if (typeof envelope.content === 'string') content = envelope.content
            if (typeof envelope.baseVersion === 'number') baseVersion = envelope.baseVersion
          }
          if (content !== undefined) {
            draft = { content, revision: 1, savedRevision: 0, baseVersion, conflict: null, timer: null, saving: null }
            this.drafts.set(id, draft)
          }
        }
      } catch { /* Ignore unavailable storage or malformed drafts. */ }
    }
    if (!draft) return undefined
    if (draft.conflict === null && draft.baseVersion !== undefined &&
        typeof serverVersion === 'number' && draft.baseVersion !== serverVersion) {
      draft.conflict = serverVersion
    }
    this.state(id, draft.conflict !== null ? 'conflict' : 'dirty')
    return draft.content
  }

  edit(id: string, content: string) {
    const draft = this.drafts.get(id) ?? {
      content, revision: 0, savedRevision: 0,
      baseVersion: this.versions.get(id), conflict: null, timer: null, saving: null,
    }
    draft.content = content
    draft.revision++
    if (draft.timer) { clearTimeout(draft.timer); draft.timer = null }
    this.drafts.set(id, draft)
    this.store(id, draft)
    // Review DATA-04: while a conflict is unresolved, typing keeps the local
    // draft but never schedules a save — keystrokes are not consent to
    // overwrite another session's work.
    if (draft.conflict !== null) { this.state(id, 'conflict'); return }
    this.state(id, 'dirty')
    draft.timer = setTimeout(() => { void this.flush(id) }, 800)
  }

  async flush(id: string): Promise<boolean> {
    const draft = this.drafts.get(id)
    if (!draft) return true
    if (draft.timer) { clearTimeout(draft.timer); draft.timer = null }
    if (draft.saving) return draft.saving
    if (draft.conflict !== null) return false
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
      try {
        const version = await this.save(id, content, this.versions.get(id))
        if (typeof version === 'number') this.versions.set(id, version)
      } catch (error) {
        if (error instanceof PageConflictError) {
          // Pause on the reported remote version without adopting it as the
          // save base; resolveOverwrite is the only path that rebases.
          draft.conflict = typeof error.serverVersion === 'number'
            ? error.serverVersion
            : (this.versions.get(id) ?? 0)
          this.state(id, 'conflict')
          return false
        }
        this.state(id, 'dirty')
        return false
      }
      draft.savedRevision = revision
      if (draft.baseVersion !== undefined) {
        const current = this.versions.get(id)
        if (current !== undefined) draft.baseVersion = current
      }
    }
    if (draft.timer) { clearTimeout(draft.timer); draft.timer = null }
    this.store(id, null)
    this.state(id, 'saved')
    return true
  }

  // conflictVersion reports the remote version a paused conflict was detected
  // against, or null when the page is not in the conflict state.
  conflictVersion(id: string): number | null {
    return this.drafts.get(id)?.conflict ?? null
  }

  // resolveOverwrite is the explicit user decision to replace the newer
  // remote content with the local draft (review DATA-04).
  async resolveOverwrite(id: string): Promise<boolean> {
    const draft = this.drafts.get(id)
    if (!draft || draft.conflict === null) return true
    this.versions.set(id, draft.conflict)
    draft.baseVersion = draft.conflict
    draft.conflict = null
    this.state(id, 'dirty')
    return this.flush(id)
  }

  // discard drops the local draft after an explicit user decision; the caller
  // reloads the page so the editor shows the remote content again.
  discard(id: string) {
    const draft = this.drafts.get(id)
    if (draft?.timer) clearTimeout(draft.timer)
    this.drafts.delete(id)
    this.store(id, null)
    this.state(id, 'saved')
  }

  hasUnsaved() { return [...this.drafts.values()].some(draft => draft.revision > draft.savedRevision) }
  async flushAll() { return (await Promise.all([...this.drafts.keys()].map(id => this.flush(id)))).every(Boolean) }
  forget(id: string) {
    const draft = this.drafts.get(id)
    if (draft?.timer) clearTimeout(draft.timer)
    this.drafts.delete(id)
    this.versions.delete(id)
    this.store(id, null)
  }
}
