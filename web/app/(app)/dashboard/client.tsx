'use client'

import { useState, useEffect, useCallback } from 'react'
import { BookmarkCard } from '@/components/bookmark-card'
import { BookmarkDetailSheet } from '@/components/bookmark-detail-sheet'
import { AddBookmarkDialog } from '@/components/add-bookmark-dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Plus, Search, Inbox, Share2 } from 'lucide-react'
import type { Bookmark } from '@/components/app-shell'
import { fetchBookmarks } from '@/lib/bookmarks'

export function DashboardClient() {
  const [bookmarks, setBookmarks] = useState<Bookmark[]>([])
  const [sharedBookmarks, setSharedBookmarks] = useState<Bookmark[]>([])
  const [loading, setLoading] = useState(true)
  const [search, setSearch] = useState('')
  const [selected, setSelected] = useState<Bookmark | null>(null)
  const [addOpen, setAddOpen] = useState(false)
  const [tab, setTab] = useState<'mine' | 'shared'>('mine')

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const params = new URLSearchParams()
      if (search) params.set('search', search)
      const [mine, shared] = await Promise.all([
        fetchBookmarks(params),
        fetchBookmarks(new URLSearchParams({ shared: 'true' })),
      ])
      setBookmarks(mine ?? [])
      setSharedBookmarks(shared ?? [])
    } catch { /* ignore */ }
    setLoading(false)
  }, [search])

  useEffect(() => { const timer = setTimeout(load, 0); window.addEventListener('computer:bookmarks-changed', load); return () => { clearTimeout(timer); window.removeEventListener('computer:bookmarks-changed', load) } }, [load])
  useEffect(() => {
    const timer = setTimeout(() => setSearch(new URLSearchParams(window.location.search).get('search') || ''), 0)
    const handler = (event: Event) => setSearch((event as CustomEvent<string>).detail)
    window.addEventListener('computer:search', handler)
    return () => { clearTimeout(timer); window.removeEventListener('computer:search', handler) }
  }, [])

  const displayBookmarks = tab === 'mine' ? bookmarks : sharedBookmarks

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="font-display text-2xl font-bold tracking-tight">Dashboard</h1>
          <p className="text-sm text-muted-foreground mt-1">Your recently saved bookmarks and shared links</p>
        </div>
        <Button onClick={() => setAddOpen(true)} className="gap-1.5">
          <Plus className="h-4 w-4" /> Add Bookmark
        </Button>
      </div>

      <div className="flex items-center gap-3">
        <div className="relative flex-1 max-w-sm">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
          <Input placeholder="Search..." value={search} onChange={(e: React.ChangeEvent<HTMLInputElement>) => setSearch(e.target.value)} className="pl-9" />
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

      {loading ? (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4">
          {Array.from({ length: 8 }).map((_, i) => <div key={i} className="h-48 rounded-lg bg-muted animate-pulse" />)}
        </div>
      ) : displayBookmarks.length === 0 ? (
        <div className="flex flex-col items-center justify-center py-20 text-center">
          <Inbox className="h-12 w-12 text-muted-foreground/50 mb-4" />
          <h3 className="text-lg font-medium">No bookmarks yet</h3>
          <p className="text-sm text-muted-foreground mt-1">Add your first bookmark to get started</p>
          <Button onClick={() => setAddOpen(true)} className="mt-4 gap-1.5"><Plus className="h-4 w-4" /> Add Bookmark</Button>
        </div>
      ) : (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4">
          {displayBookmarks.map((b: Bookmark) => <BookmarkCard key={b.id} bookmark={b} onClick={() => setSelected(b)} />)}
        </div>
      )}

      <BookmarkDetailSheet bookmark={selected} onClose={() => setSelected(null)} onUpdate={load} />
      <AddBookmarkDialog open={addOpen} onClose={() => setAddOpen(false)} onCreated={() => { setAddOpen(false); load() }} />
    </div>
  )
}
