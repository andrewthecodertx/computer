'use client'

import { useState, useCallback, useEffect } from 'react'
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Badge } from '@/components/ui/badge'
import { Link2, Loader2, Plus, X } from 'lucide-react'
import { toast } from 'sonner'

interface Props {
  open: boolean
  onClose: () => void
  onCreated: () => void
}

export function AddBookmarkDialog({ open, onClose, onCreated }: Props) {
  const [url, setUrl] = useState('')
  const [title, setTitle] = useState('')
  const [description, setDescription] = useState('')
  const [notes, setNotes] = useState('')
  const [dueDate, setDueDate] = useState('')
  const [kanban, setKanban] = useState('INBOX')
  const [loading, setLoading] = useState(false)
  const [fetching, setFetching] = useState(false)
  const [tags, setTags] = useState<{ id: string; name: string; color: string }[]>([])
  const [allTags, setAllTags] = useState<any[]>([])
  const [newTagName, setNewTagName] = useState('')

  useEffect(() => {
    if (open) {
      fetch('/api/tags').then(r => r.json()).then(setAllTags).catch(() => {})
      setUrl(''); setTitle(''); setDescription(''); setNotes(''); setDueDate(''); setKanban('INBOX'); setTags([])
    }
  }, [open])

  const fetchPreview = useCallback(async () => {
    if (!url) return
    setFetching(true)
    try {
      const res = await fetch(`/api/preview?url=${encodeURIComponent(url)}`)
      if (res.ok) {
        const data = await res.json()
        if (data?.ogTitle && !title) setTitle(data.ogTitle)
        if (data?.ogDescription && !description) setDescription(data.ogDescription)
      }
    } catch { /* ignore */ }
    setFetching(false)
  }, [url, title, description])

  const handleSubmit = useCallback(async () => {
    if (!url) { toast.error('URL is required'); return }
    setLoading(true)
    try {
      const res = await fetch('/api/bookmarks', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          url,
          title: title || undefined,
          description: description || undefined,
          notes: notes || undefined,
          dueDate: dueDate || undefined,
          kanbanStatus: kanban,
          tagIds: tags.map(t => t.id),
        }),
      })
      if (res.ok) {
        toast.success('Bookmark created')
        onCreated()
      } else {
        const data = await res.json().catch(() => ({}))
        toast.error(data?.error ?? 'Failed')
      }
    } catch { toast.error('Error creating bookmark') }
    setLoading(false)
  }, [url, title, description, notes, dueDate, kanban, tags, onCreated])

  const createTag = useCallback(async () => {
    if (!newTagName) return
    try {
      const res = await fetch('/api/tags', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name: newTagName }),
      })
      if (res.ok) {
        const tag = await res.json()
        setTags(prev => [...prev, { id: tag.id, name: tag.name, color: tag.color }])
        setAllTags(prev => [...prev, tag])
        setNewTagName('')
      }
    } catch { /* ignore */ }
  }, [newTagName])

  return (
    <Dialog open={open} onOpenChange={() => onClose()}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2"><Link2 className="h-5 w-5" /> Add Bookmark</DialogTitle>
        </DialogHeader>
        <div className="space-y-4">
          <div>
            <label className="text-xs font-medium text-muted-foreground">URL *</label>
            <div className="relative">
              <Input
                placeholder="https://..."
                value={url}
                onChange={(e: React.ChangeEvent<HTMLInputElement>) => setUrl(e.target.value)}
                onBlur={fetchPreview}
              />
              {fetching && <Loader2 className="absolute right-3 top-1/2 -translate-y-1/2 h-4 w-4 animate-spin text-muted-foreground" />}
            </div>
          </div>
          <div>
            <label className="text-xs font-medium text-muted-foreground">Title</label>
            <Input value={title} onChange={(e: React.ChangeEvent<HTMLInputElement>) => setTitle(e.target.value)} placeholder="Auto-filled from URL" />
          </div>
          <div>
            <label className="text-xs font-medium text-muted-foreground">Description</label>
            <Input value={description} onChange={(e: React.ChangeEvent<HTMLInputElement>) => setDescription(e.target.value)} />
          </div>
          <div>
            <label className="text-xs font-medium text-muted-foreground">Notes (Markdown)</label>
            <Textarea value={notes} onChange={(e: React.ChangeEvent<HTMLTextAreaElement>) => setNotes(e.target.value)} rows={3} />
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="text-xs font-medium text-muted-foreground">Due Date</label>
              <Input type="date" value={dueDate} onChange={(e: React.ChangeEvent<HTMLInputElement>) => setDueDate(e.target.value)} />
            </div>
            <div>
              <label className="text-xs font-medium text-muted-foreground">Status</label>
              <select
                value={kanban}
                onChange={(e: React.ChangeEvent<HTMLSelectElement>) => setKanban(e.target.value)}
                className="w-full rounded-md border bg-background px-3 py-2 text-sm"
              >
                <option value="INBOX">Inbox</option>
                <option value="TODO">To Do</option>
                <option value="IN_PROGRESS">In Progress</option>
                <option value="DONE">Done</option>
              </select>
            </div>
          </div>
          <div>
            <label className="text-xs font-medium text-muted-foreground">Tags</label>
            <div className="flex flex-wrap gap-1.5 mb-2">
              {tags.map(t => (
                <Badge key={t.id} variant="secondary" className="gap-1" style={{ backgroundColor: `${t.color}20`, color: t.color }}>
                  {t.name}
                  <button onClick={() => setTags(prev => prev.filter(x => x.id !== t.id))}><X className="h-3 w-3" /></button>
                </Badge>
              ))}
            </div>
            <div className="flex gap-1.5">
              <select
                onChange={(e: React.ChangeEvent<HTMLSelectElement>) => {
                  const tag = allTags.find((t: any) => t.id === e.target.value)
                  if (tag && !tags.find(t => t.id === tag.id)) setTags(prev => [...prev, { id: tag.id, name: tag.name, color: tag.color }])
                  e.target.value = ''
                }}
                className="flex-1 rounded-md border bg-background px-3 py-1.5 text-sm"
                defaultValue=""
              >
                <option value="" disabled>Select tag...</option>
                {allTags.map((t: any) => <option key={t.id} value={t.id}>{t.name}</option>)}
              </select>
              <div className="flex gap-1">
                <Input placeholder="New tag" value={newTagName} onChange={(e: React.ChangeEvent<HTMLInputElement>) => setNewTagName(e.target.value)} className="w-24" />
                <Button variant="outline" size="icon-sm" onClick={createTag}><Plus className="h-3 w-3" /></Button>
              </div>
            </div>
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>Cancel</Button>
          <Button onClick={handleSubmit} loading={loading}>Save Bookmark</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
