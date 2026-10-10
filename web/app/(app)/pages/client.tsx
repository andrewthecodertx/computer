'use client'

import { useCallback, useEffect, useRef, useState } from 'react'
import { useRouter } from 'next/navigation'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { toast } from 'sonner'
import {
  ChevronLeft, ChevronRight, Code, Download, Eye, FileText, Heading1, Heading2, Heading3,
  Link2, List, ListOrdered, ListTodo, Minus, Pencil, Pin, PinOff, Plus, Quote, Search, Trash2, Type, X,
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { cn } from '@/lib/utils'
import { PageAutosave, PageConflictError, type PageSaveState } from '@/lib/page-autosave'
import { PAGES_CHANGED, nextUntitledTitle, notifyActivePage, notifyPagesChanged, type PageSummary } from '@/lib/pages'

export { PAGES_CHANGED }

// Notion-style slash menu: typing "/" at the start of a line offers blocks
// that insert their Markdown equivalent. The textarea stays the source of
// truth, so autosave and conflict handling are untouched.
const SLASH_BLOCKS = [
  { key: 'text', label: 'Text', desc: 'Plain paragraph', icon: Type, prefix: '' },
  { key: 'h1', label: 'Heading 1', desc: 'Large section heading', icon: Heading1, prefix: '# ' },
  { key: 'h2', label: 'Heading 2', desc: 'Medium section heading', icon: Heading2, prefix: '## ' },
  { key: 'h3', label: 'Heading 3', desc: 'Small section heading', icon: Heading3, prefix: '### ' },
  { key: 'todo', label: 'To-do list', desc: 'Track tasks with a checkbox', icon: ListTodo, prefix: '- [ ] ' },
  { key: 'bullet', label: 'Bulleted list', desc: 'Simple bulleted list', icon: List, prefix: '- ' },
  { key: 'num', label: 'Numbered list', desc: 'List with numbering', icon: ListOrdered, prefix: '1. ' },
  { key: 'quote', label: 'Quote', desc: 'Capture a quote', icon: Quote, prefix: '> ' },
  { key: 'div', label: 'Divider', desc: 'Visual section break', icon: Minus, prefix: '---\n' },
  { key: 'code', label: 'Code', desc: 'Code snippet block', icon: Code, prefix: '```\n' },
] as const

type LinkedBookmark = { id: string; url: string; title: string | null; favicon: string | null; ogImage: string | null; ogTitle: string | null; ogDescription: string | null; description: string | null }
type PageDetail = PageSummary & { content: string; version: number; bookmarks: { bookmark: LinkedBookmark }[] }

const notify = notifyPagesChanged
const host = (u: string) => { try { return new URL(u).hostname.replace(/^www\./, '') } catch { return u } }

export function PagesClient({ initialId }: { initialId: string | null }) {
  const router = useRouter()
  const [pages, setPages] = useState<PageSummary[]>([])
  const [activeId, setActiveId] = useState<string | null>(initialId)
  const [page, setPage] = useState<PageDetail | null>(null)
  const [filter, setFilter] = useState('')
  const [mode, setMode] = useState<'edit' | 'preview'>('preview')
  const [title, setTitle] = useState('')
  const [content, setContent] = useState('')
  const [saveStates, setSaveStates] = useState<Record<string, PageSaveState>>({})
  const [linkQuery, setLinkQuery] = useState('')
  const [results, setResults] = useState<LinkedBookmark[]>([])
  const [linkOpen, setLinkOpen] = useState(false)
  const [slash, setSlash] = useState<{ query: string; start: number } | null>(null)
  const [slashIndex, setSlashIndex] = useState(0)
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  const pendingCaret = useRef<number | null>(null)
  const activePage = useRef(initialId)
  const loadSequence = useRef(0)
  const selectionSequence = useRef(0)
  // Optimistic concurrency (review DATA-01): PageAutosave tracks the server
  // revision per page and sends it with content saves, so a newer save from
  // another tab or device answers 409 instead of being silently overwritten.
  const persistContent = useCallback(async (id: string, content: string, version: number | undefined) => {
    const body = JSON.stringify(typeof version === 'number' ? { content, version } : { content })
    let res: Response
    try {
      res = await fetch(`/api/pages/${id}`, {
        method: 'PATCH', headers: { 'Content-Type': 'application/json' }, body,
        keepalive: new TextEncoder().encode(body).length < 60000,
      })
    } catch (error) {
      toast.error('Could not save page. Your draft is kept in this tab; retry saving.')
      throw error
    }
    if (res.status === 409) {
      // Another session saved first: re-base on the server revision and keep
      // the local draft. The user is told the next save wins over the newer
      // version, and autosave retries against the fresh base.
      let serverVersion: number | undefined
      try {
        const latest = await fetch(`/api/pages/${id}`, { cache: 'no-store' }).then(r => (r.ok ? r.json() : null))
        if (latest && typeof latest.version === 'number') serverVersion = latest.version
      } catch { /* keep the stale base; the next save surfaces the conflict again */ }
      toast.error('This page was saved in another tab or device. Your text is kept here; saving again overwrites that newer version.')
      throw new PageConflictError(serverVersion)
    }
    if (!res.ok) {
      toast.error('Could not save page. Your draft is kept in this tab; retry saving.')
      throw new Error('Save failed')
    }
    const saved = await res.json().catch(() => null)
    return saved && typeof saved.version === 'number' ? saved.version : undefined
  }, [])
  const [autosave] = useState(() => new PageAutosave(persistContent, (id, state) => setSaveStates(previous => ({ ...previous, [id]: state }))))
  const saveState = activeId ? saveStates[activeId] ?? 'saved' : 'saved'

  useEffect(() => {
    const beforeUnload = (event: BeforeUnloadEvent) => {
      if (autosave.hasUnsaved()) { event.preventDefault(); event.returnValue = '' }
    }
    const flush = () => { void autosave.flushAll() }
    window.addEventListener('beforeunload', beforeUnload)
    window.addEventListener('pagehide', flush)
    return () => {
      window.removeEventListener('beforeunload', beforeUnload)
      window.removeEventListener('pagehide', flush)
      flush()
    }
  }, [autosave])

  const loadPages = useCallback(async () => {
    const res = await fetch('/api/pages')
    if (!res.ok) return [] as PageSummary[]
    const data: PageSummary[] = await res.json()
    setPages(data)
    return data
  }, [])

  const loadPage = useCallback(async (id: string) => {
    const sequence = ++loadSequence.current
    const res = await fetch(`/api/pages/${id}`)
    if (sequence !== loadSequence.current || activePage.current !== id) return
    if (!res.ok) { setPage(null); notifyActivePage(null); return }
    const data: PageDetail = await res.json()
    if (sequence !== loadSequence.current || activePage.current !== id) return
    if (typeof data.version === 'number') autosave.setVersion(data.id, data.version)
    const draft = autosave.draft(id)
    setPage(data)
    setTitle(data.title)
    setContent(draft ?? data.content)
    notifyActivePage(data.title)
    if (draft !== undefined || !data.content) setMode('edit')
  }, [autosave])

  useEffect(() => {
    const timer = setTimeout(() => loadPages().then((data) => {
      if (!activePage.current && data.length) { activePage.current = data[0].id; setActiveId(data[0].id) }
    }).catch((e) => console.error('Failed to load pages', e)), 0)
    return () => clearTimeout(timer)
  }, [loadPages])

  useEffect(() => {
    const timer = setTimeout(() => {
      if (activeId) loadPage(activeId).catch((e) => console.error('Failed to load page', e))
      else setPage(null)
    }, 0)
    return () => clearTimeout(timer)
  }, [activeId, loadPage])

  const select = async (id: string) => {
    const sequence = ++selectionSequence.current
    if (activePage.current && !await autosave.flush(activePage.current)) return
    if (sequence !== selectionSequence.current) return
    activePage.current = id
    setActiveId(id)
    router.replace(`/pages?id=${id}`)
  }

  // The sidebar can create or open pages while this workspace stays mounted.
  useEffect(() => {
    if (!initialId || initialId === activePage.current) return
    void loadPages()
    void select(initialId)
  }, [initialId])

  // Sidebar mutations (rename, pin, delete) announce themselves; keep the
  // page list in step and drop the detail view when the open page is gone.
  useEffect(() => {
    const reload = () => {
      void loadPages().then((data) => {
        if (activePage.current && !data.some((p) => p.id === activePage.current)) {
          activePage.current = null
          setActiveId(null)
          setPage(null)
          notifyActivePage(null)
          router.replace('/pages')
        }
      })
    }
    window.addEventListener(PAGES_CHANGED, reload)
    return () => window.removeEventListener(PAGES_CHANGED, reload)
  }, [loadPages, router])

  // Leaving the pages workspace clears the breadcrumb title.
  useEffect(() => () => { notifyActivePage(null) }, [])

  const patch = async (body: Record<string, unknown>) => {
    if (!activeId || page?.id !== activeId) return null
    const res = await fetch(`/api/pages/${page.id}`, { method: 'PATCH', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) })
    if (!res.ok) { toast.error('Could not save page'); return null }
    const saved = await res.json().catch(() => null)
    if (saved && typeof saved.version === 'number') autosave.setVersion(page.id, saved.version)
    return res
  }

  const createPage = async () => {
    if (activePage.current && !await autosave.flush(activePage.current)) return
    const res = await fetch('/api/pages', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ title: nextUntitledTitle(pages.map((p) => p.title)) }) })
    if (!res.ok) { toast.error('Could not create page'); return }
    const p = await res.json()
    await loadPages()
    notify()
    setMode('edit')
    select(p.id)
  }

  const onContentChange = (v: string) => {
    if (!page || page.id !== activeId) return
    setContent(v)
    autosave.edit(page.id, v)
  }

  // Show the block menu only while the caret sits on a bare "/token" line.
  const updateSlash = (text: string, caret: number) => {
    const lineStart = text.lastIndexOf('\n', caret - 1) + 1
    const line = text.slice(lineStart, caret)
    const m = /^\/([\w-]*)$/.exec(line)
    if (m) { setSlash({ query: m[1].toLowerCase(), start: lineStart }); setSlashIndex(0) }
    else setSlash(null)
  }

  const slashFiltered = SLASH_BLOCKS.filter((b) => !slash?.query || b.key.includes(slash.query) || b.label.toLowerCase().includes(slash.query))

  const applyBlock = (block: (typeof SLASH_BLOCKS)[number]) => {
    const ta = textareaRef.current
    if (!slash || !ta) return
    const caret = ta.selectionStart
    const next = content.slice(0, slash.start) + block.prefix + content.slice(caret)
    pendingCaret.current = slash.start + block.prefix.length
    onContentChange(next)
    setSlash(null)
  }

  // Restore the caret after a programmatic content rewrite (slash insert,
  // "[] " shortcut). Runs once the new content has reached the textarea.
  useEffect(() => {
    if (pendingCaret.current === null || mode !== 'edit') return
    const ta = textareaRef.current
    if (!ta) return
    const c = pendingCaret.current
    pendingCaret.current = null
    ta.focus()
    ta.setSelectionRange(c, c)
  }, [content, mode])

  const saveTitle = async () => {
    if (!page) return
    const t = title.trim()
    // Keep the generated "Untitled N" name until the user gives a real one.
    if (!t) { setTitle(page.title); return }
    if (t === page.title) return
    if (await patch({ title: t })) {
      setPage(current => current?.id === page.id ? { ...current, title: t } : current)
      notifyActivePage(t)
      await loadPages(); notify()
    }
  }

  const togglePin = async () => {
    if (!page) return
    if (await patch({ pinned: !page.pinned })) {
      setPage(current => current?.id === page.id ? { ...current, pinned: !page.pinned } : current)
      await loadPages(); notify()
    }
  }

  const move = async (dir: -1 | 1) => {
    const idx = pages.findIndex((p) => p.id === activeId)
    const j = idx + dir
    if (idx < 0 || j < 0 || j >= pages.length) return
    const next = [...pages]
    ;[next[idx], next[j]] = [next[j], next[idx]]
    setPages(next)
    await fetch('/api/pages', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ order: next.map((p) => p.id) }) })
    await loadPages(); notify()
  }

  const download = async () => {
    if (!activeId) return
    if (!await autosave.flush(activeId)) return
    const a = document.createElement('a')
    a.href = `/api/pages/${activeId}/markdown`
    a.download = ''
    document.body.appendChild(a); a.click(); a.remove()
  }

  const remove = async () => {
    if (!page || !confirm(`Delete page "${page.title}"? Bookmarks are kept.`)) return
    if (!await autosave.flush(page.id)) return
    const res = await fetch(`/api/pages/${page.id}`, { method: 'DELETE' })
    if (!res.ok) { toast.error('Could not delete page'); return }
    autosave.forget(page.id)
    const data = await loadPages(); notify()
    if (data.length) select(data[0].id)
    else { activePage.current = null; setActiveId(null); router.replace('/pages') }
  }

  useEffect(() => {
    if (!linkOpen) return
    const q = linkQuery.trim()
    const t = setTimeout(async () => {
      try {
        const params = new URLSearchParams({ limit: '8' })
        if (q) params.set('search', q)
        const res = await fetch(`/api/bookmarks?${params}`)
        if (res.ok) setResults((await res.json()).slice(0, 8))
      } catch { /* transient; retried on next keystroke */ }
    }, 250)
    return () => clearTimeout(t)
  }, [linkQuery, linkOpen])

  const addBookmark = async (bookmarkId: string) => {
    if (activeId && !await autosave.flush(activeId)) return
    if (await patch({ addBookmarkId: bookmarkId })) {
      setLinkOpen(false); setLinkQuery('')
      if (activeId) await loadPage(activeId)
      await loadPages()
    }
  }

  const addUrl = async () => {
    let url = linkQuery.trim()
    if (!url) return
    if (!/^https?:\/\//i.test(url)) url = `https://${url}`
    const res = await fetch('/api/bookmarks', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ url }) })
    if (!res.ok) { toast.error('Could not save URL'); return }
    const b = await res.json()
    await addBookmark(b.id)
    toast.success('Link added')
  }

  const removeBookmark = async (bookmarkId: string) => {
    if (await patch({ removeBookmarkId: bookmarkId })) {
      if (page) setPage({ ...page, bookmarks: page.bookmarks.filter((x) => x.bookmark.id !== bookmarkId) })
      await loadPages()
    }
  }

  const visible = pages.filter((p) => p.title.toLowerCase().includes(filter.toLowerCase()))
  const looksLikeUrl = /^(https?:\/\/)?[\w-]+(\.[\w-]+)+/i.test(linkQuery.trim())

  return (
    <div className="flex h-full flex-col">
      <div className="flex items-center gap-2 border-b border-border/60 px-4 py-2">
        <div className="relative w-40 shrink-0">
          <Search className="absolute left-2 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
          <Input value={filter} onChange={(e) => setFilter(e.target.value)} placeholder="Filter pages" className="h-8 pl-7 text-sm" />
        </div>
        <div className="flex flex-1 items-center gap-1 overflow-x-auto">
          {visible.map((p) => (
            <button key={p.id} onClick={() => select(p.id)} className={cn('flex shrink-0 items-center gap-1.5 rounded-md px-3 py-1.5 text-sm transition-colors', p.id === activeId ? 'bg-primary/10 font-medium text-primary' : 'text-muted-foreground hover:bg-muted hover:text-foreground')}>
              {p.pinned ? <Pin className="h-3 w-3" /> : <FileText className="h-3 w-3" />}
              <span className="max-w-[10rem] truncate">{p.title}</span>
            </button>
          ))}
        </div>
        <Button size="sm" onClick={createPage}><Plus className="mr-1 h-4 w-4" />New page</Button>
      </div>

      {!page || page.id !== activeId ? (
        <div className="flex flex-1 flex-col items-center justify-center gap-3 p-10 text-center">
          <FileText className="h-10 w-10 text-muted-foreground" />
          <h2 className="font-display text-xl font-semibold">{pages.length ? 'Select a page' : 'No pages yet'}</h2>
          <p className="max-w-sm text-sm text-muted-foreground">Pages are Markdown notes that collect links into a workspace. A bookmark can live on many pages.</p>
          <Button onClick={createPage}><Plus className="mr-1 h-4 w-4" />Create a page</Button>
        </div>
      ) : (
        <div className="mx-auto w-full max-w-4xl flex-1 overflow-y-auto px-6 py-8">
          <div className="mb-6 flex flex-wrap items-center gap-2">
            <input value={title} onChange={(e) => setTitle(e.target.value)} onBlur={saveTitle} onKeyDown={(e) => e.key === 'Enter' && (e.target as HTMLInputElement).blur()} className="min-w-0 flex-1 bg-transparent font-display text-3xl font-bold outline-none" aria-label="Page title" />
            <span className="text-xs text-muted-foreground">{saveState === 'saving' ? 'Saving…' : saveState === 'dirty' ? 'Unsaved' : 'Saved'}</span>
            {saveState === 'dirty' && <Button size="sm" variant="outline" onClick={() => autosave.flush(page.id)}>Save now</Button>}
            <Button variant="ghost" size="icon-sm" title={mode === 'edit' ? 'Preview' : 'Edit'} onClick={() => { setMode(mode === 'edit' ? 'preview' : 'edit'); setSlash(null) }}>{mode === 'edit' ? <Eye className="h-4 w-4" /> : <Pencil className="h-4 w-4" />}</Button>
            <Button variant="ghost" size="icon-sm" title={page.pinned ? 'Unpin' : 'Pin to sidebar'} onClick={togglePin}>{page.pinned ? <PinOff className="h-4 w-4" /> : <Pin className="h-4 w-4" />}</Button>
            <Button variant="ghost" size="icon-sm" title="Move tab left" onClick={() => move(-1)}><ChevronLeft className="h-4 w-4" /></Button>
            <Button variant="ghost" size="icon-sm" title="Move tab right" onClick={() => move(1)}><ChevronRight className="h-4 w-4" /></Button>
            <Button variant="ghost" size="icon-sm" title="Download .md" onClick={download}><Download className="h-4 w-4" /></Button>
            <Button variant="ghost" size="icon-sm" title="Delete page" onClick={remove}><Trash2 className="h-4 w-4 text-destructive" /></Button>
          </div>

          {mode === 'edit' ? (
            <div className="relative">
              <Textarea
                ref={textareaRef}
                value={content}
                onChange={(e) => { onContentChange(e.target.value); updateSlash(e.target.value, e.target.selectionStart) }}
                onClick={(e) => updateSlash(e.currentTarget.value, e.currentTarget.selectionStart)}
                onBlur={() => setSlash(null)}
                onKeyDown={(e) => {
                  // While the slash menu is open, arrows/Enter/Tab/Escape drive it.
                  if (slash && slashFiltered.length > 0) {
                    if (e.key === 'ArrowDown') { e.preventDefault(); setSlashIndex((i) => (i + 1) % slashFiltered.length); return }
                    if (e.key === 'ArrowUp') { e.preventDefault(); setSlashIndex((i) => (i - 1 + slashFiltered.length) % slashFiltered.length); return }
                    if (e.key === 'Enter' || e.key === 'Tab') { e.preventDefault(); applyBlock(slashFiltered[Math.min(slashIndex, slashFiltered.length - 1)]); return }
                    if (e.key === 'Escape') { e.preventDefault(); setSlash(null); return }
                  }
                  // Notion Markdown shortcut: "[]" + space becomes a to-do item.
                  if (e.key === ' ') {
                    const ta = e.currentTarget
                    const caret = ta.selectionStart
                    const lineStart = ta.value.lastIndexOf('\n', caret - 1) + 1
                    if (ta.value.slice(lineStart, caret) === '[]') {
                      e.preventDefault()
                      const next = ta.value.slice(0, lineStart) + '- [ ] ' + ta.value.slice(caret)
                      pendingCaret.current = lineStart + '- [ ] '.length
                      onContentChange(next)
                    }
                  }
                }}
                placeholder="Write Markdown notes… type / for blocks, paste links, lists, thoughts."
                className="min-h-[260px] font-mono text-sm"
              />
              {slash && slashFiltered.length > 0 && (
                <div
                  onMouseDown={(e) => e.preventDefault()}
                  role="listbox"
                  aria-label="Blocks to insert"
                  className="absolute left-2 top-full z-20 mt-1 w-72 rounded-lg border bg-popover p-1 shadow-lg"
                >
                  <p className="px-2 py-1 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">Basic blocks</p>
                  <div className="max-h-64 overflow-y-auto">
                    {slashFiltered.map((b, i) => (
                      <button
                        key={b.key}
                        type="button"
                        role="option"
                        aria-selected={i === slashIndex}
                        onMouseEnter={() => setSlashIndex(i)}
                        onClick={() => applyBlock(b)}
                        className={cn('flex w-full items-center gap-2.5 rounded-md px-2 py-1.5 text-left', i === slashIndex ? 'bg-accent text-accent-foreground' : 'hover:bg-accent/50')}
                      >
                        <span className="flex h-7 w-7 shrink-0 items-center justify-center rounded border bg-card">
                          <b.icon className="h-3.5 w-3.5 text-muted-foreground" />
                        </span>
                        <span className="min-w-0">
                          <span className="block text-sm font-medium">{b.label}</span>
                          <span className="block truncate text-xs text-muted-foreground">{b.desc}</span>
                        </span>
                      </button>
                    ))}
                  </div>
                </div>
              )}
            </div>
          ) : (
            <div className="prose prose-sm max-w-none dark:prose-invert min-h-[80px] cursor-text" onDoubleClick={() => setMode('edit')}>
              {content ? (
                <ReactMarkdown remarkPlugins={[remarkGfm]} components={{ a: ({ node, ...props }) => <a {...props} target="_blank" rel="noopener noreferrer" /> }}>{content}</ReactMarkdown>
              ) : (
                <p className="text-muted-foreground">No notes yet. Double-click or press the pencil to write.</p>
              )}
            </div>
          )}

          <div className="mt-10 mb-3 flex items-center justify-between">
            <h3 className="font-display text-lg font-semibold">Links on this page <span className="text-sm font-normal text-muted-foreground">({page.bookmarks.length})</span></h3>
            <Popover open={linkOpen} onOpenChange={setLinkOpen}>
              <PopoverTrigger asChild><Button size="sm" variant="outline"><Link2 className="mr-1 h-4 w-4" />Add link</Button></PopoverTrigger>
              <PopoverContent align="end" className="w-80 p-2">
                <Input autoFocus value={linkQuery} onChange={(e) => setLinkQuery(e.target.value)} onKeyDown={(e) => e.key === 'Enter' && looksLikeUrl && addUrl()} placeholder="Search bookmarks or paste a URL" className="h-8 text-sm" />
                <div className="mt-2 max-h-64 overflow-y-auto">
                  {looksLikeUrl && (
                    <button onClick={addUrl} className="flex w-full items-center gap-2 rounded px-2 py-1.5 text-left text-sm hover:bg-muted"><Plus className="h-3.5 w-3.5" />Save &amp; add “{linkQuery.trim()}”</button>
                  )}
                  {results.filter((r) => !page.bookmarks.some((x) => x.bookmark.id === r.id)).map((r) => (
                    <button key={r.id} onClick={() => addBookmark(r.id)} className="flex w-full flex-col rounded px-2 py-1.5 text-left hover:bg-muted">
                      <span className="truncate text-sm">{r.title || r.ogTitle || r.url}</span>
                      <span className="truncate text-xs text-muted-foreground">{host(r.url)}</span>
                    </button>
                  ))}
                  {!looksLikeUrl && results.length === 0 && <p className="px-2 py-3 text-center text-xs text-muted-foreground">No matching bookmarks</p>}
                </div>
              </PopoverContent>
            </Popover>
          </div>

          {page.bookmarks.length === 0 ? (
            <div className="rounded-lg bg-muted/40 p-8 text-center text-sm text-muted-foreground">No links yet. Add existing bookmarks or paste a new URL.</div>
          ) : (
            <div className="grid gap-3 sm:grid-cols-2">
              {page.bookmarks.map(({ bookmark: b }) => (
                <div key={b.id} className="group relative flex gap-3 rounded-lg bg-card p-3 shadow-sm ring-1 ring-border/50 transition-shadow hover:shadow-md">
                  {b.ogImage ? (
                    // eslint-disable-next-line @next/next/no-img-element
                    <img src={b.ogImage} alt={`Preview of ${b.title || host(b.url)}`} className="h-16 w-24 shrink-0 rounded object-cover bg-muted" onError={(e) => { (e.target as HTMLImageElement).style.display = 'none' }} />
                  ) : null}
                  <a href={b.url} target="_blank" rel="noopener noreferrer" className="min-w-0 flex-1">
                    <div className="flex items-center gap-1.5">
                      {b.favicon && (
                        // eslint-disable-next-line @next/next/no-img-element
                        <img src={b.favicon} alt="" className="h-3.5 w-3.5" onError={(e) => { (e.target as HTMLImageElement).style.display = 'none' }} />
                      )}
                      <span className="truncate text-sm font-medium">{b.title || b.ogTitle || b.url}</span>
                    </div>
                    <p className="mt-0.5 line-clamp-2 text-xs text-muted-foreground">{b.description || b.ogDescription || ''}</p>
                    <p className="mt-1 text-[11px] text-muted-foreground/80">{host(b.url)}</p>
                  </a>
                  <button onClick={() => removeBookmark(b.id)} title="Remove from page" className="absolute right-1.5 top-1.5 rounded p-1 text-muted-foreground opacity-0 transition-opacity hover:bg-muted group-hover:opacity-100"><X className="h-3.5 w-3.5" /></button>
                </div>
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  )
}
