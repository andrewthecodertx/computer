'use client'

import { useState, useEffect, useCallback } from 'react'
import { BookmarkDetailSheet } from '@/components/bookmark-detail-sheet'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { ExternalLink, Calendar, GripVertical, Plus, MoreHorizontal, Pencil, Trash2, ArrowLeft, ArrowRight, Check, X } from 'lucide-react'
import { toast } from 'sonner'
import type { Bookmark } from '@/components/app-shell'

type Column = { id: string; label: string; color: string; position: number; key: string | null }
const PALETTE = ['#64748b', '#3b82f6', '#f59e0b', '#22c55e', '#8b5cf6', '#ec4899', '#ef4444', '#14b8a6']

function columnFor(b: Bookmark, cols: Column[]) {
  const explicit = (b as any).kanbanColumnId as string | null
  if (explicit && cols.some(c => c.id === explicit)) return explicit
  return cols.find(c => c.key === b.kanbanStatus)?.id ?? cols[0]?.id
}

export function KanbanClient() {
  const [bookmarks, setBookmarks] = useState<Bookmark[]>([])
  const [columns, setColumns] = useState<Column[]>([])
  const [loading, setLoading] = useState(true)
  const [selectedBookmark, setSelectedBookmark] = useState<Bookmark | null>(null)
  const [draggedId, setDraggedId] = useState<string | null>(null)
  const [editingId, setEditingId] = useState<string | null>(null)
  const [editLabel, setEditLabel] = useState('')
  const [adding, setAdding] = useState(false)
  const [newLabel, setNewLabel] = useState('')

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const [bRes, cRes] = await Promise.all([fetch('/api/bookmarks'), fetch('/api/kanban/columns')])
      if (bRes.ok) setBookmarks(await bRes.json())
      if (cRes.ok) setColumns(await cRes.json())
    } catch (e) { console.error('Kanban load failed', e) }
    setLoading(false)
  }, [])

  useEffect(() => { const timer = setTimeout(load, 0); window.addEventListener('computer:bookmarks-changed', load); return () => { clearTimeout(timer); window.removeEventListener('computer:bookmarks-changed', load) } }, [load])

  const moveToColumn = useCallback(async (bookmarkId: string, columnId: string) => {
    setBookmarks(prev => prev.map(b => b.id === bookmarkId ? ({ ...b, kanbanColumnId: columnId } as any) : b))
    try {
      const res = await fetch(`/api/bookmarks/${bookmarkId}`, {
        method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ kanbanColumnId: columnId }),
      })
      if (!res.ok) throw new Error(await res.text())
    } catch (e) { console.error(e); toast.error('Failed to move'); load() }
  }, [load])

  const addColumn = async () => {
    if (!newLabel.trim()) return
    const res = await fetch('/api/kanban/columns', {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ label: newLabel, color: PALETTE[columns.length % PALETTE.length] }),
    })
    if (res.ok) { const col = await res.json(); setColumns(c => [...c, col]); setNewLabel(''); setAdding(false) }
    else toast.error('Could not add column')
  }

  const saveColumn = async (id: string, data: Partial<Column>) => {
    const res = await fetch(`/api/kanban/columns/${id}`, { method: 'PATCH', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(data) })
    if (res.ok) { const col = await res.json(); setColumns(cs => cs.map(c => c.id === id ? col : c)) }
    else toast.error('Could not update column')
    setEditingId(null)
  }

  const deleteColumn = async (col: Column) => {
    if (columns.length <= 1) { toast.error('A board needs at least one column'); return }
    const dest = columns.find(c => c.id !== col.id)
    if (!confirm(`Delete "${col.label}"? Its bookmarks will move to "${dest?.label}".`)) return
    const res = await fetch(`/api/kanban/columns/${col.id}`, { method: 'DELETE' })
    if (res.ok) { toast.success('Column deleted'); load() } else toast.error((await res.json())?.error ?? 'Could not delete')
  }

  const moveColumn = async (index: number, dir: -1 | 1) => {
    const next = [...columns]
    const j = index + dir
    if (j < 0 || j >= next.length) return
    ;[next[index], next[j]] = [next[j], next[index]]
    setColumns(next)
    const res = await fetch('/api/kanban/columns', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ order: next.map(c => c.id) }) })
    if (!res.ok) { toast.error('Could not reorder'); load() }
  }

  return (
    <div className="space-y-6">
      <div className="flex items-end justify-between gap-4">
        <div>
          <h1 className="font-display text-2xl font-bold tracking-tight">Kanban Board</h1>
          <p className="text-sm text-muted-foreground mt-1">Drag bookmarks between columns. Use a column’s menu to rename, recolor, reorder or delete it.</p>
        </div>
        {!adding ? (
          <Button size="sm" variant="outline" className="gap-1.5" onClick={() => setAdding(true)}><Plus className="h-4 w-4" /> Add column</Button>
        ) : (
          <div className="flex items-center gap-1.5">
            <Input autoFocus value={newLabel} onChange={e => setNewLabel(e.target.value)} onKeyDown={e => { if (e.key === 'Enter') addColumn(); if (e.key === 'Escape') setAdding(false) }} placeholder="Column name" className="h-8 w-40" />
            <Button size="icon-sm" onClick={addColumn}><Check className="h-4 w-4" /></Button>
            <Button size="icon-sm" variant="ghost" onClick={() => setAdding(false)}><X className="h-4 w-4" /></Button>
          </div>
        )}
      </div>

      <div className="flex gap-4 overflow-x-auto pb-2 h-[calc(100vh-180px)]">
        {columns.map((col, idx) => {
          const colBookmarks = bookmarks.filter(b => columnFor(b, columns) === col.id)
          return (
            <div
              key={col.id}
              className="rounded-lg border bg-muted/30 flex flex-col w-72 shrink-0"
              style={{ borderTopColor: col.color, borderTopWidth: 3 }}
              onDragOver={e => { e.preventDefault(); e.dataTransfer.dropEffect = 'move' }}
              onDrop={e => { e.preventDefault(); if (draggedId) { moveToColumn(draggedId, col.id); setDraggedId(null) } }}
            >
              <div className="px-3 py-2 border-b flex items-center justify-between gap-2">
                {editingId === col.id ? (
                  <Input autoFocus value={editLabel} onChange={e => setEditLabel(e.target.value)} onBlur={() => saveColumn(col.id, { label: editLabel })}
                    onKeyDown={e => { if (e.key === 'Enter') saveColumn(col.id, { label: editLabel }); if (e.key === 'Escape') setEditingId(null) }} className="h-7 text-sm" />
                ) : (
                  <h3 className="text-sm font-medium truncate flex items-center gap-2">
                    <span className="h-2 w-2 rounded-full shrink-0" style={{ backgroundColor: col.color }} />{col.label}
                  </h3>
                )}
                <div className="flex items-center gap-1 shrink-0">
                  <span className="text-xs text-muted-foreground bg-muted rounded-full px-2 py-0.5">{colBookmarks.length}</span>
                  <DropdownMenu>
                    <DropdownMenuTrigger asChild>
                      <Button size="icon-sm" variant="ghost" aria-label={`Column ${col.label} options`}><MoreHorizontal className="h-4 w-4" /></Button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end">
                      <DropdownMenuItem onClick={() => { setEditingId(col.id); setEditLabel(col.label) }}><Pencil className="h-3.5 w-3.5 mr-2" />Rename</DropdownMenuItem>
                      <DropdownMenuItem disabled={idx === 0} onClick={() => moveColumn(idx, -1)}><ArrowLeft className="h-3.5 w-3.5 mr-2" />Move left</DropdownMenuItem>
                      <DropdownMenuItem disabled={idx === columns.length - 1} onClick={() => moveColumn(idx, 1)}><ArrowRight className="h-3.5 w-3.5 mr-2" />Move right</DropdownMenuItem>
                      <DropdownMenuSeparator />
                      <div className="flex gap-1.5 px-2 py-1.5">
                        {PALETTE.map(c => (
                          <button key={c} aria-label={`Color ${c}`} onClick={() => saveColumn(col.id, { color: c })} className="h-4 w-4 rounded-full ring-offset-1 hover:ring-2 ring-ring" style={{ backgroundColor: c }} />
                        ))}
                      </div>
                      <DropdownMenuSeparator />
                      <DropdownMenuItem className="text-destructive" onClick={() => deleteColumn(col)}><Trash2 className="h-3.5 w-3.5 mr-2" />Delete column</DropdownMenuItem>
                    </DropdownMenuContent>
                  </DropdownMenu>
                </div>
              </div>
              <div className="flex-1 overflow-y-auto p-2 space-y-2">
                {loading ? (
                  Array.from({ length: 3 }).map((_, i) => <div key={i} className="h-20 rounded-lg bg-muted/50 animate-pulse" />)
                ) : colBookmarks.map(b => (
                  <div
                    key={b.id}
                    draggable
                    onDragStart={e => { setDraggedId(b.id); e.dataTransfer.effectAllowed = 'move' }}
                    onClick={() => setSelectedBookmark(b)}
                    className="rounded-lg border bg-card p-3 cursor-grab active:cursor-grabbing shadow-sm hover:shadow-md transition-shadow group"
                  >
                    <div className="flex items-start gap-2">
                      <GripVertical className="h-4 w-4 text-muted-foreground/50 mt-0.5 shrink-0" />
                      <div className="flex-1 min-w-0">
                        <div className="flex items-center gap-1.5">
                          {b.favicon && <img src={b.favicon} alt="" className="h-3.5 w-3.5 rounded" onError={(e: any) => { e.target.style.display = 'none' }} />}
                          <p className="text-sm font-medium truncate">{b.title ?? 'Untitled'}</p>
                        </div>
                        <div className="flex flex-wrap gap-1 mt-1.5">
                          {(b.tags ?? []).slice(0, 2).map((bt: any) => (
                            <Badge key={bt?.tag?.id} variant="secondary" className="text-[9px] px-1 py-0" style={{ backgroundColor: `${bt?.tag?.color}20`, color: bt?.tag?.color }}>{bt?.tag?.name}</Badge>
                          ))}
                        </div>
                        {b.dueDate && (
                          <p className="text-[10px] text-muted-foreground mt-1 flex items-center gap-1">
                            <Calendar className="h-3 w-3" />
                            {new Date(b.dueDate).toLocaleDateString('en-US', { month: 'short', day: 'numeric', timeZone: 'UTC' })}
                          </p>
                        )}
                      </div>
                      <a href={b.url} target="_blank" rel="noopener noreferrer" onClick={e => e.stopPropagation()} className="opacity-0 group-hover:opacity-100" aria-label="Open link">
                        <ExternalLink className="h-3.5 w-3.5 text-muted-foreground hover:text-primary" />
                      </a>
                    </div>
                  </div>
                ))}
              </div>
            </div>
          )
        })}
      </div>

      <BookmarkDetailSheet bookmark={selectedBookmark} onClose={() => setSelectedBookmark(null)} onUpdate={load} />
    </div>
  )
}
