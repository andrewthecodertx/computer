'use client'

import { useCallback, useEffect, useState } from 'react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { toast } from 'sonner'
import type { Bookmark } from '@/components/app-shell'

export function BookmarkExtras({ bookmark, canEdit, onUpdate }: { bookmark: Bookmark; canEdit: boolean; onUpdate: () => void }) {
  const [sources, setSources] = useState<any[]>([])
  const [signals, setSignals] = useState<any[]>([])
  const [pages, setPages] = useState<any[]>([])
  const [from, setFrom] = useState('')
  const [subject, setSubject] = useState('')
  const [busy, setBusy] = useState(false)
  const [shares, setShares] = useState<any[]>([])
  const load = useCallback(async () => {
    const [signalResponse, bookmarkResponse] = await Promise.all([
      fetch(`/api/bookmarks/${bookmark.id}/signals`), fetch(`/api/bookmarks/${bookmark.id}`),
    ])
    if (signalResponse.ok) { const data = await signalResponse.json(); setSources(data.sources); setSignals(data.signals) }
    if (bookmarkResponse.ok) { const data = await bookmarkResponse.json(); setPages(data.pageLinks ?? []); setShares(data.sharedWith ?? []) }
  }, [bookmark.id])
  useEffect(() => { const timer = setTimeout(() => load().catch(() => toast.error('Could not load bookmark details')), 0); return () => clearTimeout(timer) }, [load])
  const change = async (url: string, method: string, body?: any) => {
    setBusy(true)
    try {
      const response = await fetch(url, { method, headers: { 'Content-Type': 'application/json' }, body: body ? JSON.stringify(body) : undefined })
      if (!response.ok) { toast.error((await response.json()).error || 'Request failed'); return }
      await load(); onUpdate()
    } catch { toast.error('Request failed') } finally { setBusy(false) }
  }
  return <div className="space-y-4 border-t pt-4">
    <a className="text-sm text-primary underline" href={`/api/bookmarks/${bookmark.id}/markdown`}>Download Markdown</a>
    {canEdit && shares.length > 0 && <div><h3 className="text-sm font-medium">Shared with</h3>{shares.map(share => <div key={share.userId} className="flex items-center justify-between text-xs"><span>{share.user?.name || share.user?.email}</span><Button size="xs" variant="ghost" onClick={() => change(`/api/bookmarks/${bookmark.id}/share/${share.userId}`, 'DELETE')}>Stop sharing</Button></div>)}</div>}
    <div><h3 className="text-sm font-medium">On pages</h3>
      {pages.length ? pages.map(link => <a key={link.pageId} className="block text-sm text-primary" href={`/pages?id=${link.pageId}`}>{link.page.title}</a>) : <p className="text-xs text-muted-foreground">Not linked to a page yet</p>}
    </div>
    <div className="space-y-2"><h3 className="text-sm font-medium">Signals</h3>
      {canEdit && <>
        <div className="flex gap-2"><Input placeholder="From address (optional)" value={from} onChange={e => setFrom(e.target.value)} /><Input placeholder="Subject contains" value={subject} onChange={e => setSubject(e.target.value)} /></div>
        <div className="flex gap-2"><Button size="sm" disabled={busy} onClick={() => change(`/api/bookmarks/${bookmark.id}/signals`, 'POST', { type: 'email', config: { from, subject } })}>Add email watcher</Button>
          <Button size="sm" variant="outline" disabled={busy} onClick={() => change(`/api/bookmarks/${bookmark.id}/signals`, 'POST', { action: 'check' })}>Check now</Button></div>
      </>}
      {sources.map(source => <div key={source.id} className="rounded border p-2 text-xs">
        <p>{source.type}: {source.config.from || 'any sender'} / {source.config.subject || 'any subject'}</p><p className="text-muted-foreground">{source.lastStatus || 'Not checked yet'}</p>
        {canEdit && <div className="flex gap-2"><Button size="xs" variant="ghost" onClick={() => change(`/api/signal-sources/${source.id}`, 'PATCH', { enabled: !source.enabled })}>{source.enabled ? 'Pause' : 'Enable'}</Button><Button size="xs" variant="ghost" onClick={() => change(`/api/signal-sources/${source.id}`, 'DELETE')}>Remove</Button></div>}
      </div>)}
      {signals.map(signal => <div key={signal.id} className="rounded bg-muted p-2 text-sm"><p>{signal.title}</p><p className="text-xs text-muted-foreground">{signal.summary}</p></div>)}
      {!signals.length && <p className="text-xs text-muted-foreground">No matching events yet. Configure a mailbox in Settings to check email watchers.</p>}
    </div>
  </div>
}
