'use client'

import { useState, useEffect, useCallback, useRef } from 'react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import { BookmarkDetailSheet } from '@/components/bookmark-detail-sheet'
import { ExternalLink, Search, Trash2, ChevronUp, ChevronDown, Inbox, Share2 } from 'lucide-react'
import { toast } from 'sonner'
import type { Bookmark } from '@/components/app-shell'
import { fetchBookmarks } from '@/lib/bookmarks'

export function BookmarksClient() {
  const [bookmarks, setBookmarks] = useState<Bookmark[]>([])
  const [sharedBookmarks, setSharedBookmarks] = useState<Bookmark[]>([])
  const [loading, setLoading] = useState(true)
  const [search, setSearch] = useState('')
  const [selected, setSelected] = useState<Bookmark | null>(null)
  const [tab, setTab] = useState<'mine' | 'shared'>('mine')
  const [sortField, setSortField] = useState<'title' | 'updatedAt' | 'kanbanStatus'>('updatedAt')
  const [sortDir, setSortDir] = useState<'asc' | 'desc'>('desc')

  // Debounced, abortable, sequence-guarded load: a slow response for an older
  // query can never overwrite newer results (same discipline as pages client).
  const loadSeq = useRef(0)
  const abortRef = useRef<AbortController | null>(null)
  const load = useCallback(() => {
    abortRef.current?.abort()
    const ac = new AbortController()
    abortRef.current = ac
    const seq = ++loadSeq.current
    setLoading(true)
    const params = new URLSearchParams()
    if (search) params.set('search', search)
    ;(async () => {
      try {
        const mine = await fetchBookmarks(params, ac.signal)
        if (seq !== loadSeq.current) return
        setBookmarks(mine ?? [])
      } catch {
        if (seq === loadSeq.current && !ac.signal.aborted) toast.error('Could not load bookmarks')
      } finally {
        if (seq === loadSeq.current) setLoading(false)
      }
    })()
  }, [search])

  // The shared list is not search-filtered; load it once and on changes only.
  const loadShared = useCallback(async () => {
    try { setSharedBookmarks(await fetchBookmarks(new URLSearchParams({ shared: 'true' })) ?? []) }
    catch { /* the mine-tab load surfaces errors */ }
  }, [])

  useEffect(() => {
    const timer = setTimeout(load, search ? 250 : 0)
    const reload = () => { load(); void loadShared() }
    window.addEventListener('computer:bookmarks-changed', reload)
    return () => { clearTimeout(timer); window.removeEventListener('computer:bookmarks-changed', reload); abortRef.current?.abort() }
  }, [load, loadShared, search])
  useEffect(() => { const timer = setTimeout(loadShared, 0); return () => clearTimeout(timer) }, [loadShared])
  // Top-bar search lands here (this view replaced the old dashboard).
  useEffect(() => {
    const timer = setTimeout(() => setSearch(new URLSearchParams(window.location.search).get('search') || ''), 0)
    const handler = (event: Event) => setSearch((event as CustomEvent<string>).detail)
    window.addEventListener('computer:search', handler)
    return () => { clearTimeout(timer); window.removeEventListener('computer:search', handler) }
  }, [])

  const displayBookmarks = tab === 'mine' ? bookmarks : sharedBookmarks
  const sorted = [...(displayBookmarks ?? [])].sort((a: any, b: any) => {
    const va = a?.[sortField] ?? ''
    const vb = b?.[sortField] ?? ''
    return sortDir === 'asc' ? String(va).localeCompare(String(vb)) : String(vb).localeCompare(String(va))
  })

  const toggleSort = (field: typeof sortField) => {
    if (sortField === field) setSortDir(d => d === 'asc' ? 'desc' : 'asc')
    else { setSortField(field); setSortDir('asc') }
  }

  const sortIcon = (field: typeof sortField) => {
    if (sortField !== field) return null
    return sortDir === 'asc' ? <ChevronUp className="h-3 w-3" /> : <ChevronDown className="h-3 w-3" />
  }

  return (
    <div className="space-y-6">
      <div>
        <h1 className="font-display text-2xl font-bold tracking-tight">All Bookmarks</h1>
        <p className="text-sm text-muted-foreground mt-1">Every saved link. Use the sidebar to filter by calendar, tags, board, or contacts.</p>
      </div>
      <div className="flex items-center gap-3">
        <div className="relative flex-1 max-w-sm">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
          <Input placeholder="Filter..." value={search} onChange={(e: React.ChangeEvent<HTMLInputElement>) => setSearch(e.target.value)} className="pl-9" />
        </div>
        <div className="flex rounded-lg border overflow-hidden">
          <button onClick={() => setTab('mine')} className={`flex items-center gap-1.5 px-3 py-1.5 text-sm font-medium transition-colors ${tab === 'mine' ? 'bg-primary text-primary-foreground' : 'bg-card hover:bg-muted'}`}>
            <Inbox className="h-3.5 w-3.5" /> Mine
          </button>
          <button onClick={() => setTab('shared')} className={`flex items-center gap-1.5 px-3 py-1.5 text-sm font-medium transition-colors ${tab === 'shared' ? 'bg-primary text-primary-foreground' : 'bg-card hover:bg-muted'}`}>
            <Share2 className="h-3.5 w-3.5" /> Shared
          </button>
        </div>
      </div>
      <div className="rounded-lg border overflow-hidden">
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="bg-muted/50 text-left">
                <th className="px-4 py-3 font-medium cursor-pointer" onClick={() => toggleSort('title')}>
                  <span className="flex items-center gap-1">Title {sortIcon('title')}</span>
                </th>
                <th className="px-4 py-3 font-medium">URL</th>
                <th className="px-4 py-3 font-medium">Tags</th>
                <th className="px-4 py-3 font-medium cursor-pointer" onClick={() => toggleSort('kanbanStatus')}>
                  <span className="flex items-center gap-1">Status {sortIcon('kanbanStatus')}</span>
                </th>
                <th className="px-4 py-3 font-medium cursor-pointer" onClick={() => toggleSort('updatedAt')}>
                  <span className="flex items-center gap-1">Updated {sortIcon('updatedAt')}</span>
                </th>
                <th className="px-4 py-3 font-medium w-20">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y">
              {loading ? (
                <tr><td colSpan={6} className="px-4 py-8 text-center text-muted-foreground">Loading...</td></tr>
              ) : sorted.length === 0 ? (
                <tr><td colSpan={6} className="px-4 py-8 text-center text-muted-foreground">No bookmarks found</td></tr>
              ) : sorted.map((b: Bookmark) => (
                <tr key={b.id} className="hover:bg-muted/30 cursor-pointer" onClick={() => setSelected(b)}>
                  <td className="px-4 py-3">
                    <div className="flex items-center gap-2">
                      {b.favicon && <img src={b.favicon} alt="" className="h-4 w-4 rounded" onError={(e: any) => { e.target.style.display = 'none' }} />}
                      <span className="truncate max-w-[200px]">{b.title ?? 'Untitled'}</span>
                    </div>
                  </td>
                  <td className="px-4 py-3">
                    <a href={b.url} target="_blank" rel="noopener noreferrer" onClick={(e: React.MouseEvent) => e.stopPropagation()} className="text-primary hover:underline truncate max-w-[200px] block">
                      {(() => { try { return new URL(b.url).hostname } catch { return b.url } })()}
                    </a>
                  </td>
                  <td className="px-4 py-3">
                    <div className="flex gap-1">
                      {(b.tags ?? []).slice(0, 2).map((bt: any) => (
                        <Badge key={bt?.tag?.id} variant="secondary" className="text-[10px] px-1.5 py-0" style={{ backgroundColor: `${bt?.tag?.color}20`, color: bt?.tag?.color }}>{bt?.tag?.name}</Badge>
                      ))}
                    </div>
                  </td>
                  <td className="px-4 py-3"><Badge variant="outline" className="text-[10px]">{b.kanbanStatus?.replace('_', ' ')}</Badge></td>
                  <td className="px-4 py-3 text-xs text-muted-foreground">{new Date(b.updatedAt).toLocaleDateString('en-US', { month: 'short', day: 'numeric', timeZone: 'UTC' })}</td>
                  <td className="px-4 py-3">
                    <div className="flex gap-1" onClick={(e: React.MouseEvent) => e.stopPropagation()}>
                      <a href={b.url} target="_blank" rel="noopener noreferrer"><ExternalLink className="h-4 w-4 text-muted-foreground hover:text-primary" /></a>
                      {tab === 'mine' && (
                        <button onClick={async () => { if (!confirm('Delete?')) return; try { const res = await fetch(`/api/bookmarks/${b.id}`, { method: 'DELETE' }); if (res.ok) { toast.success('Deleted'); load() } else toast.error('Could not delete bookmark') } catch { toast.error('Could not delete bookmark') } }}><Trash2 className="h-4 w-4 text-muted-foreground hover:text-destructive" /></button>
                      )}
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
      <BookmarkDetailSheet bookmark={selected} onClose={() => setSelected(null)} onUpdate={load} />
    </div>
  )
}
