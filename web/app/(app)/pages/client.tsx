'use client'

import { useCallback, useEffect, useRef, useState } from 'react'
import { useRouter } from 'next/navigation'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { toast } from 'sonner'
import { ChevronLeft, ChevronRight, Download, Eye, FileText, Link2, Pencil, Pin, PinOff, Plus, Search, Trash2, X } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { cn } from '@/lib/utils'
import { PageAutosave, type PageSaveState } from '@/lib/page-autosave'
import { PAGES_CHANGED, nextUntitledTitle, notifyPagesChanged, type PageSummary } from '@/lib/pages'

export { PAGES_CHANGED }

type LinkedBookmark = { id: string; url: string; title: string | null; favicon: string | null; ogImage: string | null; ogTitle: string | null; ogDescription: string | null; description: string | null }
type PageDetail = PageSummary & { content: string; bookmarks: { bookmark: LinkedBookmark }[] }

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
  const activePage = useRef(initialId)
  const loadSequence = useRef(0)
  const selectionSequence = useRef(0)
  const [autosave] = useState(() => new PageAutosave(async (id, content) => {
    const body = JSON.stringify({ content })
    try {
      const res = await fetch(`/api/pages/${id}`, {
        method: 'PATCH', headers: { 'Content-Type': 'application/json' }, body,
        keepalive: new TextEncoder().encode(body).length < 60000,
      })
      if (!res.ok) throw new Error('Save failed')
    } catch (error) { toast.error('Could not save page. Your draft is kept in this tab; retry saving.'); throw error }
  }, (id, state) => setSaveStates(previous => ({ ...previous, [id]: state }))))
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
    if (!res.ok) { setPage(null); return }
    const data: PageDetail = await res.json()
    if (sequence !== loadSequence.current || activePage.current !== id) return
    const draft = autosave.draft(id)
    setPage(data)
    setTitle(data.title)
    setContent(draft ?? data.content)
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

  const patch = async (body: Record<string, unknown>) => {
    if (!activeId || page?.id !== activeId) return null
    const res = await fetch(`/api/pages/${page.id}`, { method: 'PATCH', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) })
    if (!res.ok) { toast.error('Could not save page'); return null }
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

  const saveTitle = async () => {
    if (!page) return
    const t = title.trim()
    // Keep the generated "Untitled N" name until the user gives a real one.
    if (!t) { setTitle(page.title); return }
    if (t === page.title) return
    if (await patch({ title: t })) {
      setPage(current => current?.id === page.id ? { ...current, title: t } : current)
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
      const params = new URLSearchParams({ limit: '8' })
      if (q) params.set('search', q)
      const res = await fetch(`/api/bookmarks?${params}`)
      if (res.ok) setResults((await res.json()).slice(0, 8))
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
            <Button variant="ghost" size="icon-sm" title={mode === 'edit' ? 'Preview' : 'Edit'} onClick={() => setMode(mode === 'edit' ? 'preview' : 'edit')}>{mode === 'edit' ? <Eye className="h-4 w-4" /> : <Pencil className="h-4 w-4" />}</Button>
            <Button variant="ghost" size="icon-sm" title={page.pinned ? 'Unpin' : 'Pin to sidebar'} onClick={togglePin}>{page.pinned ? <PinOff className="h-4 w-4" /> : <Pin className="h-4 w-4" />}</Button>
            <Button variant="ghost" size="icon-sm" title="Move tab left" onClick={() => move(-1)}><ChevronLeft className="h-4 w-4" /></Button>
            <Button variant="ghost" size="icon-sm" title="Move tab right" onClick={() => move(1)}><ChevronRight className="h-4 w-4" /></Button>
            <Button variant="ghost" size="icon-sm" title="Download .md" onClick={download}><Download className="h-4 w-4" /></Button>
            <Button variant="ghost" size="icon-sm" title="Delete page" onClick={remove}><Trash2 className="h-4 w-4 text-destructive" /></Button>
          </div>

          {mode === 'edit' ? (
            <Textarea value={content} onChange={(e) => onContentChange(e.target.value)} placeholder="Write Markdown notes… paste links, lists, thoughts." className="min-h-[260px] font-mono text-sm" />
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
