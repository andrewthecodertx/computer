'use client'

import { useState, useEffect, useCallback } from 'react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import { BookmarkCard } from '@/components/bookmark-card'
import { BookmarkDetailSheet } from '@/components/bookmark-detail-sheet'
import { Plus, Tags, Trash2, Edit, Check, X } from 'lucide-react'
import { toast } from 'sonner'
import type { Bookmark, TagWithCount } from '@/components/app-shell'
import { fetchBookmarks } from '@/lib/bookmarks'

export function TagsClient() {
  const [tags, setTags] = useState<TagWithCount[]>([])
  const [selectedTag, setSelectedTag] = useState<string | null>(null)
  const [bookmarks, setBookmarks] = useState<Bookmark[]>([])
  const [selectedBookmark, setSelectedBookmark] = useState<Bookmark | null>(null)
  const [newName, setNewName] = useState('')
  const [newColor, setNewColor] = useState('#6366f1')
  const [editingId, setEditingId] = useState<string | null>(null)
  const [editName, setEditName] = useState('')
  const [editColor, setEditColor] = useState('#6366f1')
  const [loading, setLoading] = useState(true)
  const [effectiveId, setEffectiveId] = useState('')
  const [shareQuery, setShareQuery] = useState('')
  const [shareUsers, setShareUsers] = useState<any[]>([])
  useEffect(() => { fetch('/api/me').then(r => r.json()).then(me => setEffectiveId(me.effectiveUser?.id || '')).catch(() => {}) }, [])

  // Debounced user search; failures keep the previous results silently.
  useEffect(() => {
    const q = shareQuery.trim()
    const t = setTimeout(async () => {
      if (q.length < 2) { setShareUsers([]); return }
      try {
        const res = await fetch(`/api/users/search?q=${encodeURIComponent(q)}`)
        if (res.ok) setShareUsers(await res.json())
      } catch { /* transient; retried on next keystroke */ }
    }, q.length < 2 ? 0 : 250)
    return () => clearTimeout(t)
  }, [shareQuery])

  const loadTags = useCallback(async () => {
    try {
      const res = await fetch('/api/tags')
      if (res.ok) setTags(await res.json())
    } catch { toast.error('Could not load tags') }
  }, [])

  const loadBookmarks = useCallback(async (tagId: string) => {
    setLoading(true)
    try { setBookmarks(await fetchBookmarks(new URLSearchParams({ tagId }))) }
    catch { toast.error('Could not load bookmarks') }
    finally { setLoading(false) }
  }, [])

  useEffect(() => { const timer = setTimeout(loadTags, 0); return () => clearTimeout(timer) }, [loadTags])
  useEffect(() => { const timer = setTimeout(() => { if (selectedTag) loadBookmarks(selectedTag) }, 0); return () => clearTimeout(timer) }, [selectedTag, loadBookmarks])
  useEffect(() => { const reload = () => { loadTags(); if (selectedTag) loadBookmarks(selectedTag) }; window.addEventListener('computer:bookmarks-changed', reload); return () => window.removeEventListener('computer:bookmarks-changed', reload) }, [loadTags, loadBookmarks, selectedTag])

  const createTag = async () => {
    if (!newName) return
    const res = await fetch('/api/tags', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ name: newName, color: newColor }) })
    if (res.ok) { toast.success('Tag created'); setNewName(''); loadTags() }
    else toast.error('Failed')
  }

  const updateTag = async (id: string) => {
    const response = await fetch(`/api/tags/${id}`, { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ name: editName, color: editColor }) })
    if (!response.ok) { toast.error('Could not update tag'); return }
    setEditingId(null)
    loadTags()
  }

  const deleteTag = async (id: string) => {
    if (!confirm('Delete this tag?')) return
    const response = await fetch(`/api/tags/${id}`, { method: 'DELETE' })
    if (!response.ok) { toast.error('Could not delete tag'); return }
    toast.success('Tag deleted')
    if (selectedTag === id) setSelectedTag(null)
    loadTags()
  }

  return (
    <div className="flex gap-6 h-[calc(100vh-120px)]">
      <div className="w-64 shrink-0 space-y-4 overflow-y-auto">
        <h1 className="font-display text-2xl font-bold tracking-tight">Tags</h1>
        <div className="flex gap-1.5">
          <Input placeholder="New tag" value={newName} onChange={(e: React.ChangeEvent<HTMLInputElement>) => setNewName(e.target.value)} className="flex-1" />
          <input type="color" value={newColor} onChange={(e: React.ChangeEvent<HTMLInputElement>) => setNewColor(e.target.value)} className="h-9 w-9 rounded border cursor-pointer" />
          <Button size="icon-sm" onClick={createTag}><Plus className="h-4 w-4" /></Button>
        </div>
        <div className="space-y-1">
          {tags.map((t: TagWithCount) => (
            <div
              key={t.id}
              className={`flex items-center gap-2 rounded-lg px-3 py-2 cursor-pointer transition-colors ${
                selectedTag === t.id ? 'bg-accent' : 'hover:bg-muted'
              }`}
              onClick={() => setSelectedTag(t.id)}
            >
              {editingId === t.id ? (
                <>
                  <input type="color" value={editColor} onChange={(e: React.ChangeEvent<HTMLInputElement>) => setEditColor(e.target.value)} className="h-5 w-5 rounded cursor-pointer" />
                  <Input value={editName} onChange={(e: React.ChangeEvent<HTMLInputElement>) => setEditName(e.target.value)} className="h-7 text-xs flex-1" />
                  <button onClick={() => updateTag(t.id)}><Check className="h-3 w-3 text-green-500" /></button>
                  <button onClick={() => setEditingId(null)}><X className="h-3 w-3" /></button>
                </>
              ) : (
                <>
                  <div className="h-3 w-3 rounded-full shrink-0" style={{ backgroundColor: t.color }} />
                  <span className="text-sm font-medium flex-1 truncate">{t.name}</span>
                  <span className="text-xs text-muted-foreground">{t._count?.bookmarks ?? 0}</span>
                  {t.ownerId === effectiveId ? <><button aria-label="Edit tag" onClick={(e: React.MouseEvent) => { e.stopPropagation(); setEditingId(t.id); setEditName(t.name); setEditColor(t.color) }}><Edit className="h-3 w-3 text-muted-foreground" /></button>
                  <button aria-label="Delete tag" onClick={(e: React.MouseEvent) => { e.stopPropagation(); deleteTag(t.id) }}><Trash2 className="h-3 w-3 text-muted-foreground hover:text-destructive" /></button></> : <span className="text-xs text-muted-foreground">Shared</span>}
                </>
              )}
            </div>
          ))}
        </div>
      </div>

      <div className="flex-1 overflow-y-auto">
        {selectedTag ? (
          <div className="space-y-4">
            <h2 className="font-medium">Bookmarks tagged: <Badge style={{ backgroundColor: `${tags.find(t => t.id === selectedTag)?.color ?? '#6366f1'}20`, color: tags.find(t => t.id === selectedTag)?.color }}>{tags.find(t => t.id === selectedTag)?.name}</Badge></h2>
            {tags.find(t => t.id === selectedTag)?.ownerId === effectiveId && <div className="space-y-2 rounded border p-3">
              <Input placeholder="Share this tag with a user…" value={shareQuery} onChange={e => setShareQuery(e.target.value)} />
              {shareUsers.map(u => <Button key={u.id} size="sm" variant="outline" onClick={async () => { const res = await fetch(`/api/tags/${selectedTag}`, { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ shareUserIds: [u.id] }) }); if (!res.ok) { toast.error('Could not share tag'); return } toast.success('Tag shared'); setShareUsers([]); setShareQuery(''); loadTags() }}>Share with {u.name || u.email}</Button>)}
              {(tags.find(t => t.id === selectedTag)?.sharedWith || []).map(share => <Button key={share.userId} size="xs" variant="ghost" onClick={async () => { const res = await fetch(`/api/tags/${selectedTag}/share/${share.userId}`, { method: 'DELETE' }); if (!res.ok) { toast.error('Could not stop sharing'); return } loadTags() }}>Stop sharing with {share.userId.slice(0, 8)}…</Button>)}
            </div>}
            {loading ? (
              <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
                {Array.from({ length: 6 }).map((_, i) => <div key={i} className="h-32 rounded-lg bg-muted animate-pulse" />)}
              </div>
            ) : bookmarks.length === 0 ? (
              <p className="text-sm text-muted-foreground py-8 text-center">No bookmarks with this tag</p>
            ) : (
              <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
                {bookmarks.map((b: Bookmark) => <BookmarkCard key={b.id} bookmark={b} compact onClick={() => setSelectedBookmark(b)} />)}
              </div>
            )}
          </div>
        ) : (
          <div className="flex flex-col items-center justify-center h-full text-center">
            <Tags className="h-12 w-12 text-muted-foreground/50 mb-4" />
            <p className="text-muted-foreground">Select a tag to view its bookmarks</p>
          </div>
        )}
      </div>

      <BookmarkDetailSheet bookmark={selectedBookmark} onClose={() => setSelectedBookmark(null)} onUpdate={() => { if (selectedTag) loadBookmarks(selectedTag) }} />
    </div>
  )
}
