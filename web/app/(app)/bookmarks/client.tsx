'use client'

import { useState, useEffect, useCallback } from 'react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import { BookmarkDetailSheet } from '@/components/bookmark-detail-sheet'
import { ExternalLink, Search, Trash2, ChevronUp, ChevronDown } from 'lucide-react'
import { toast } from 'sonner'
import type { Bookmark } from '@/components/app-shell'

export function BookmarksClient() {
  const [bookmarks, setBookmarks] = useState<Bookmark[]>([])
  const [loading, setLoading] = useState(true)
  const [search, setSearch] = useState('')
  const [selected, setSelected] = useState<Bookmark | null>(null)
  const [sortField, setSortField] = useState<'title' | 'updatedAt' | 'kanbanStatus'>('updatedAt')
  const [sortDir, setSortDir] = useState<'asc' | 'desc'>('desc')

  const load = useCallback(async () => {
    setLoading(true)
    const params = new URLSearchParams()
    if (search) params.set('search', search)
    const res = await fetch(`/api/bookmarks?${params}`)
    if (res.ok) setBookmarks(await res.json())
    setLoading(false)
  }, [search])

  useEffect(() => { const timer = setTimeout(load, 0); window.addEventListener('computer:bookmarks-changed', load); return () => { clearTimeout(timer); window.removeEventListener('computer:bookmarks-changed', load) } }, [load])

  const sorted = [...(bookmarks ?? [])].sort((a: any, b: any) => {
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
        <p className="text-sm text-muted-foreground mt-1">Browse and manage your entire collection</p>
      </div>
      <div className="relative max-w-sm">
        <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
        <Input placeholder="Filter..." value={search} onChange={(e: React.ChangeEvent<HTMLInputElement>) => setSearch(e.target.value)} className="pl-9" />
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
                  <td className="px-4 py-3 text-xs text-muted-foreground">{new Date(b.updatedAt).toLocaleDateString('en-US', { month: 'short', day: 'numeric' })}</td>
                  <td className="px-4 py-3">
                    <div className="flex gap-1" onClick={(e: React.MouseEvent) => e.stopPropagation()}>
                      <a href={b.url} target="_blank" rel="noopener noreferrer"><ExternalLink className="h-4 w-4 text-muted-foreground hover:text-primary" /></a>
                      <button onClick={async () => { if (confirm('Delete?')) { await fetch(`/api/bookmarks/${b.id}`, { method: 'DELETE' }); toast.success('Deleted'); load() } }}><Trash2 className="h-4 w-4 text-muted-foreground hover:text-destructive" /></button>
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
