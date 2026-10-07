'use client'

import { useState, useEffect, useCallback } from 'react'
import { Sheet, SheetContent, SheetHeader, SheetTitle } from '@/components/ui/sheet'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import { Textarea } from '@/components/ui/textarea'
import { ExternalLink, Edit, Save, X, Trash2, Share2, Globe, Users, Calendar, Clock, Mail, Tag } from 'lucide-react'
import { toast } from 'sonner'
import type { Bookmark } from '@/components/app-shell'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'

interface Props {
  bookmark: Bookmark | null
  onClose: () => void
  onUpdate: () => void
}

export function BookmarkDetailSheet({ bookmark, onClose, onUpdate }: Props) {
  const [editing, setEditing] = useState(false)
  const [notes, setNotes] = useState('')
  const [kanban, setKanban] = useState('INBOX')
  const [dueDate, setDueDate] = useState('')
  const [alertAt, setAlertAt] = useState('')
  const [isPublic, setIsPublic] = useState(false)
  const [imapEnabled, setImapEnabled] = useState(false)
  const [imapQuery, setImapQuery] = useState('')
  const [shareSearch, setShareSearch] = useState('')
  const [shareResults, setShareResults] = useState<any[]>([])
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    if (bookmark) {
      setNotes(bookmark?.notes ?? '')
      setKanban(bookmark?.kanbanStatus ?? 'INBOX')
      setDueDate(bookmark?.dueDate ? new Date(bookmark.dueDate).toISOString().split('T')[0] : '')
      setAlertAt(bookmark?.alertAt ? new Date(bookmark.alertAt).toISOString().slice(0, 16) : '')
      setIsPublic(bookmark?.isPublic ?? false)
      setImapEnabled(bookmark?.imapWatchEnabled ?? false)
      setImapQuery(bookmark?.imapQuery ?? '')
      setEditing(false)
    }
  }, [bookmark])

  const handleSave = useCallback(async () => {
    if (!bookmark) return
    setSaving(true)
    try {
      const res = await fetch(`/api/bookmarks/${bookmark.id}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          notes,
          kanbanStatus: kanban,
          dueDate: dueDate || null,
          alertAt: alertAt || null,
          isPublic,
          imapWatchEnabled: imapEnabled,
          imapQuery: imapQuery || null,
        }),
      })
      if (res.ok) {
        toast.success('Bookmark updated')
        setEditing(false)
        onUpdate()
      } else {
        toast.error('Failed to update')
      }
    } catch { toast.error('Error saving') }
    setSaving(false)
  }, [bookmark, notes, kanban, dueDate, alertAt, isPublic, imapEnabled, imapQuery, onUpdate])

  const handleDelete = useCallback(async () => {
    if (!bookmark || !confirm('Delete this bookmark?')) return
    await fetch(`/api/bookmarks/${bookmark.id}`, { method: 'DELETE' })
    toast.success('Bookmark deleted')
    onClose()
    onUpdate()
  }, [bookmark, onClose, onUpdate])

  const searchUsers = useCallback(async (q: string) => {
    setShareSearch(q)
    if (q.length < 2) { setShareResults([]); return }
    try {
      const res = await fetch(`/api/users/search?q=${encodeURIComponent(q)}`)
      if (res.ok) setShareResults(await res.json())
    } catch { /* ignore */ }
  }, [])

  const shareWith = useCallback(async (userId: string) => {
    if (!bookmark) return
    await fetch(`/api/bookmarks/${bookmark.id}/share`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ userIds: [userId] }),
    })
    toast.success('Shared!')
    setShareSearch('')
    setShareResults([])
    onUpdate()
  }, [bookmark, onUpdate])

  const togglePublic = useCallback(async () => {
    if (!bookmark) return
    await fetch(`/api/bookmarks/${bookmark.id}/share`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ isPublic: !isPublic }),
    })
    setIsPublic(!isPublic)
    toast.success(isPublic ? 'Made private' : 'Made public')
    onUpdate()
  }, [bookmark, isPublic, onUpdate])

  const domain = (() => { try { return bookmark ? new URL(bookmark.url).hostname : '' } catch { return '' } })()

  return (
    <Sheet open={!!bookmark} onOpenChange={() => onClose()}>
      <SheetContent className="w-full sm:max-w-lg overflow-y-auto">
        <SheetHeader>
          <SheetTitle className="flex items-center gap-2">
            {bookmark?.favicon && (
              <img src={bookmark.favicon} alt="" className="h-5 w-5 rounded" onError={(e: any) => { e.target.style.display = 'none' }} />
            )}
            <span className="truncate">{bookmark?.title ?? 'Untitled'}</span>
          </SheetTitle>
        </SheetHeader>

        <div className="mt-4 space-y-6">
          {/* URL */}
          <a
            href={bookmark?.url ?? '#'}
            target="_blank"
            rel="noopener noreferrer"
            className="flex items-center gap-2 rounded-lg bg-primary/10 px-4 py-3 text-sm font-medium text-primary hover:bg-primary/20 transition-colors"
          >
            <ExternalLink className="h-4 w-4 shrink-0" />
            <span className="truncate">{bookmark?.url ?? ''}</span>
          </a>

          {/* OG Preview */}
          {bookmark?.ogImage && (
            <div className="rounded-lg border overflow-hidden">
              <img src={bookmark.ogImage} alt="" className="w-full aspect-video object-cover" onError={(e: any) => { e.target.style.display = 'none' }} />
              <div className="p-3">
                <p className="text-sm font-medium">{bookmark?.ogTitle ?? bookmark?.title}</p>
                <p className="text-xs text-muted-foreground mt-1">{bookmark?.ogDescription}</p>
                <p className="text-xs text-muted-foreground mt-1">{domain}</p>
              </div>
            </div>
          )}

          {/* Tags */}
          <div>
            <p className="text-xs font-medium text-muted-foreground mb-1.5 flex items-center gap-1"><Tag className="h-3 w-3" /> Tags</p>
            <div className="flex flex-wrap gap-1.5">
              {(bookmark?.tags ?? []).map((bt: any) => (
                <Badge key={bt?.tag?.id} variant="secondary" style={{ backgroundColor: `${bt?.tag?.color ?? '#6366f1'}20`, color: bt?.tag?.color ?? '#6366f1' }}>
                  {bt?.tag?.name ?? ''}
                </Badge>
              ))}
              {(bookmark?.tags?.length ?? 0) === 0 && <span className="text-xs text-muted-foreground">No tags</span>}
            </div>
          </div>

          {/* Notes */}
          <div>
            <div className="flex items-center justify-between mb-1.5">
              <p className="text-xs font-medium text-muted-foreground">Notes</p>
              <Button variant="ghost" size="xs" onClick={() => setEditing(!editing)}>
                {editing ? <X className="h-3 w-3" /> : <Edit className="h-3 w-3" />}
              </Button>
            </div>
            {editing ? (
              <Textarea value={notes} onChange={(e: React.ChangeEvent<HTMLTextAreaElement>) => setNotes(e.target.value)} rows={6} placeholder="Markdown notes..." />
            ) : (
              <div className="prose prose-sm dark:prose-invert max-w-none rounded-lg bg-muted/50 p-3 min-h-[60px]">
                {notes ? <ReactMarkdown remarkPlugins={[remarkGfm]}>{notes}</ReactMarkdown> : <span className="text-muted-foreground text-xs">No notes</span>}
              </div>
            )}
          </div>

          {/* Kanban Status */}
          <div>
            <p className="text-xs font-medium text-muted-foreground mb-1.5">Status</p>
            <div className="flex gap-1.5">
              {['INBOX', 'TODO', 'IN_PROGRESS', 'DONE'].map((s: string) => (
                <Button
                  key={s}
                  variant={kanban === s ? 'default' : 'outline'}
                  size="xs"
                  onClick={() => setKanban(s)}
                >
                  {s.replace('_', ' ')}
                </Button>
              ))}
            </div>
          </div>

          {/* Due Date & Alert */}
          <div className="grid grid-cols-2 gap-3">
            <div>
              <p className="text-xs font-medium text-muted-foreground mb-1.5 flex items-center gap-1"><Calendar className="h-3 w-3" /> Due Date</p>
              <Input type="date" value={dueDate} onChange={(e: React.ChangeEvent<HTMLInputElement>) => setDueDate(e.target.value)} />
            </div>
            <div>
              <p className="text-xs font-medium text-muted-foreground mb-1.5 flex items-center gap-1"><Clock className="h-3 w-3" /> Alert</p>
              <Input type="datetime-local" value={alertAt} onChange={(e: React.ChangeEvent<HTMLInputElement>) => setAlertAt(e.target.value)} />
            </div>
          </div>

          {/* Contact */}
          {bookmark?.contact && (
            <div>
              <p className="text-xs font-medium text-muted-foreground mb-1.5 flex items-center gap-1"><Users className="h-3 w-3" /> Contact</p>
              <div className="flex items-center gap-2 rounded-lg bg-muted/50 px-3 py-2">
                <div className="h-8 w-8 rounded-full bg-primary/20 flex items-center justify-center text-xs font-bold text-primary">
                  {(bookmark.contact?.displayName ?? '?')[0]?.toUpperCase()}
                </div>
                <div>
                  <p className="text-sm font-medium">{bookmark.contact?.displayName}</p>
                  <p className="text-xs text-muted-foreground">{bookmark.contact?.email}</p>
                </div>
              </div>
            </div>
          )}

          {/* IMAP Watch */}
          <div>
            <p className="text-xs font-medium text-muted-foreground mb-1.5 flex items-center gap-1"><Mail className="h-3 w-3" /> Mailbox Watch</p>
            <div className="flex items-center gap-2">
              <label className="flex items-center gap-2 text-sm">
                <input type="checkbox" checked={imapEnabled} onChange={() => setImapEnabled(!imapEnabled)} className="rounded" />
                Enabled
              </label>
              {imapEnabled && (
                <Input placeholder="Search query (subject/from)" value={imapQuery} onChange={(e: React.ChangeEvent<HTMLInputElement>) => setImapQuery(e.target.value)} className="flex-1" />
              )}
            </div>
          </div>

          {/* Sharing */}
          <div>
            <p className="text-xs font-medium text-muted-foreground mb-1.5 flex items-center gap-1"><Share2 className="h-3 w-3" /> Sharing</p>
            <div className="space-y-2">
              <Button variant={isPublic ? 'default' : 'outline'} size="sm" onClick={togglePublic} className="gap-1.5">
                <Globe className="h-3 w-3" />
                {isPublic ? 'Public' : 'Private'}
              </Button>
              {isPublic && (
                <p className="text-xs text-muted-foreground">
                  Public link: <code className="bg-muted px-1 rounded">/share/bookmark/{bookmark?.id}</code>
                </p>
              )}
              <Input placeholder="Search users to share with..." value={shareSearch} onChange={(e: React.ChangeEvent<HTMLInputElement>) => searchUsers(e.target.value)} />
              {shareResults.map((u: any) => (
                <button key={u?.id} onClick={() => shareWith(u?.id)} className="flex items-center gap-2 w-full rounded-md px-2 py-1.5 hover:bg-muted text-sm">
                  <span className="h-6 w-6 rounded-full bg-primary/20 flex items-center justify-center text-xs font-bold text-primary">
                    {(u?.name ?? u?.email ?? '?')[0]?.toUpperCase()}
                  </span>
                  {u?.name ?? u?.email}
                </button>
              ))}
            </div>
          </div>

          {/* Actions */}
          <div className="flex items-center gap-2 pt-2 border-t">
            <Button onClick={handleSave} loading={saving} className="flex-1 gap-1.5">
              <Save className="h-4 w-4" /> Save Changes
            </Button>
            <Button variant="destructive" size="icon" onClick={handleDelete}>
              <Trash2 className="h-4 w-4" />
            </Button>
          </div>
        </div>
      </SheetContent>
    </Sheet>
  )
}
