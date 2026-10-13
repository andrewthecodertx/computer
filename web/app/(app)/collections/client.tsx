'use client'

import { useCallback, useEffect, useState } from 'react'
import Link from 'next/link'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import { Checkbox } from '@/components/ui/checkbox'
import { Label } from '@/components/ui/label'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { CalendarDays, Columns3, Filter, Kanban, Layers, List, Plus, Table2, Trash2, X } from 'lucide-react'
import { toast } from 'sonner'
import type { Bookmark } from '@/components/app-shell'
import { cn } from '@/lib/utils'

// Collections workspace (Notion-like upgrades phase 2). Contextual workflow
// state over existing bookmarks: the same link can be "Done" in one
// collection and "Unread" in another, and removing a membership never
// deletes the bookmark. Every mutating PATCH carries the expected version;
// a 409 reloads instead of clobbering (the DATA-05 contract).

type PropertyOption = { id: string; label: string; color?: string }
type CollectionProperty = {
  id: string; name: string
  type: 'TEXT' | 'NUMBER' | 'SELECT' | 'MULTI_SELECT' | 'DATE' | 'CHECKBOX' | 'URL'
  position: number; config: { options?: PropertyOption[]; mode?: 'date' | 'datetime' }; version: number
}
type FilterClause = { propertyId: string; op: string; value?: unknown }
type CollectionView = {
  id: string; name: string; type: 'TABLE' | 'BOARD' | 'CALENDAR' | 'LIST'; position: number
  filters: { operator: 'AND' | 'OR'; clauses: FilterClause[] }
  sorts: { field: string; dir: string }[]
  groupByPropertyId: string | null; displaySettings: Record<string, unknown>; version: number
}
type CollectionItem = {
  id: string; collectionId: string; bookmarkId: string
  propertyValues: Record<string, unknown>; position: number; version: number
  addedAt: string; updatedAt: string; bookmark: Bookmark
}
type CollectionSummary = {
  id: string; name: string; description: string | null; icon: string | null
  version: number; createdAt: string; updatedAt: string
  _count: { items: number; properties: number; views: number }
}
type CollectionDetail = CollectionSummary & { properties: CollectionProperty[]; views: CollectionView[] }

const VIEW_ICONS = { TABLE: Table2, BOARD: Columns3, CALENDAR: CalendarDays, LIST: List } as const
const OP_LABELS: Record<string, string> = {
  eq: 'is', ne: 'is not', contains: 'contains', gt: 'after/above', gte: '≥', lt: 'before/below', lte: '≤',
  empty: 'is empty', notEmpty: 'is not empty',
}
// Mirrors the server allowlists in internal/handlers/collections.go.
const FILTER_OPS: Record<string, string[]> = {
  TEXT: ['eq', 'ne', 'contains', 'empty', 'notEmpty'],
  URL: ['eq', 'ne', 'contains', 'empty', 'notEmpty'],
  NUMBER: ['eq', 'ne', 'gt', 'gte', 'lt', 'lte', 'empty', 'notEmpty'],
  DATE: ['eq', 'ne', 'gt', 'gte', 'lt', 'lte', 'empty', 'notEmpty'],
  SELECT: ['eq', 'ne', 'empty', 'notEmpty'],
  CHECKBOX: ['eq', 'ne', 'empty', 'notEmpty'],
  MULTI_SELECT: ['contains', 'empty', 'notEmpty'],
}
const PROPERTY_TYPES = ['TEXT', 'NUMBER', 'SELECT', 'MULTI_SELECT', 'DATE', 'CHECKBOX', 'URL'] as const
const NONE = '__none__'

async function json(res: Response): Promise<any> { return res.json().catch(() => null) }

// datetime-local inputs need local "YYYY-MM-DDTHH:mm"; the server stores a
// fixed-width UTC RFC3339 string, so convert through Date on both ends.
function toLocalInput(stored: unknown): string {
  if (typeof stored !== 'string' || !stored) return ''
  const d = new Date(stored)
  if (Number.isNaN(d.getTime())) return ''
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`
}

function BookmarkCell({ bookmark }: { bookmark: Bookmark }) {
  const host = (() => { try { return new URL(bookmark.url).hostname } catch { return bookmark.url } })()
  return (
    <Link href={`/bookmarks?open=${bookmark.id}`} className="flex min-w-0 items-center gap-2 hover:underline">
      {bookmark.favicon
        // eslint-disable-next-line @next/next/no-img-element -- remote favicons stay no-referrer (web/AGENTS.md)
        ? <img src={bookmark.favicon} alt="" referrerPolicy="no-referrer" className="h-4 w-4 shrink-0 rounded-sm" />
        : <Layers className="h-4 w-4 shrink-0 text-muted-foreground" />}
      <span className="min-w-0">
        <span className="block max-w-[16rem] truncate text-sm">{bookmark.title || bookmark.url}</span>
        <span className="block max-w-[16rem] truncate text-xs text-muted-foreground">{host}</span>
      </span>
    </Link>
  )
}

const asText = (v: unknown) => (typeof v === 'string' || typeof v === 'number' ? String(v) : '')

// Per-type inline value editor. Saves immediately on commit gestures
// (change/blur/Enter); failures toast and the prop value re-renders the
// server state, so a rejected edit never sticks locally.
function ValueEditor({ prop, value, onSave }: { prop: CollectionProperty; value: unknown; onSave: (v: unknown) => Promise<void> }) {
  const [text, setText] = useState(() => asText(value))
  const [seen, setSeen] = useState(value)
  // Adjusting state when a prop changes: the React-documented render-time
  // pattern (guarded, converges immediately) instead of a cascading effect.
  if (value !== seen) {
    setSeen(value)
    setText(asText(value))
  }
  const [open, setOpen] = useState(false)
  const commitText = async () => {
    const next = text.trim()
    const current = asText(value)
    if (next === current) return
    if (next === '') { await onSave(null); return }
    await onSave(prop.type === 'NUMBER' ? Number(next) : next)
  }

  switch (prop.type) {
    case 'CHECKBOX':
      return <Checkbox aria-label={prop.name} checked={value === true} onCheckedChange={(c) => { void onSave(c === true) }} />
    case 'SELECT': {
      const options = prop.config.options ?? []
      return (
        <Select
          value={typeof value === 'string' && value ? value : NONE}
          onValueChange={(id) => { void onSave(id === NONE ? null : id) }}
        >
          <SelectTrigger className="h-8 w-[9.5rem]" aria-label={prop.name}><SelectValue /></SelectTrigger>
          <SelectContent>
            <SelectItem value={NONE}>—</SelectItem>
            {options.map((o) => (
              <SelectItem key={o.id} value={o.id}>
                <span className="flex items-center gap-1.5">
                  {o.color && <span className="h-2 w-2 rounded-full" style={{ backgroundColor: o.color }} />}
                  {o.label}
                </span>
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      )
    }
    case 'MULTI_SELECT': {
      const options = prop.config.options ?? []
      const selected = new Set(Array.isArray(value) ? (value as string[]) : [])
      const toggle = async (id: string) => {
        const next = new Set(selected)
        if (next.has(id)) next.delete(id); else next.add(id)
        await onSave([...next])
      }
      return (
        <Popover open={open} onOpenChange={setOpen}>
          <PopoverTrigger asChild>
            <button type="button" aria-label={`Edit ${prop.name}`} className="flex min-h-8 w-[9.5rem] flex-wrap items-center gap-1 rounded-md border border-input px-2 py-1 text-left hover:bg-muted">
              {selected.size === 0 && <span className="text-xs text-muted-foreground">—</span>}
              {options.filter((o) => selected.has(o.id)).map((o) => (
                <Badge key={o.id} variant="secondary" className="gap-1 text-xs" style={o.color ? { borderColor: o.color } : undefined}>{o.label}</Badge>
              ))}
            </button>
          </PopoverTrigger>
          <PopoverContent className="w-52 space-y-2" align="start">
            {options.map((o) => (
              <label key={o.id} className="flex items-center gap-2 text-sm">
                <Checkbox checked={selected.has(o.id)} onCheckedChange={() => { void toggle(o.id) }} />
                {o.color && <span className="h-2 w-2 rounded-full" style={{ backgroundColor: o.color }} />}
                {o.label}
              </label>
            ))}
            {options.length === 0 && <p className="text-xs text-muted-foreground">No options yet</p>}
          </PopoverContent>
        </Popover>
      )
    }
    case 'DATE':
      return prop.config.mode === 'datetime'
        ? <Input type="datetime-local" aria-label={prop.name} className="h-8 w-[13rem]" value={toLocalInput(value)} onChange={(e) => { const v = e.target.value; void onSave(v ? new Date(v).toISOString() : null) }} />
        : <Input type="date" aria-label={prop.name} className="h-8 w-[9.5rem]" value={typeof value === 'string' ? value : ''} onChange={(e) => { void onSave(e.target.value || null) }} />
    case 'NUMBER':
    case 'TEXT':
    case 'URL':
    default:
      return (
        <Input
          aria-label={prop.name}
          className="h-8 w-[9.5rem]"
          type={prop.type === 'NUMBER' ? 'number' : 'text'}
          value={text}
          onChange={(e) => setText(e.target.value)}
          onBlur={() => { void commitText() }}
          onKeyDown={(e) => { if (e.key === 'Enter') (e.target as HTMLInputElement).blur() }}
        />
      )
  }
}

export function CollectionsClient() {
  const [collections, setCollections] = useState<CollectionSummary[]>([])
  const [activeId, setActiveId] = useState<string | null>(null)
  const [detail, setDetail] = useState<CollectionDetail | null>(null)
  const [items, setItems] = useState<CollectionItem[]>([])
  const [cursor, setCursor] = useState<string | null>(null)
  const [viewId, setViewId] = useState<string | null>(null)
  const [newName, setNewName] = useState('')
  const [nameDraft, setNameDraft] = useState('')
  const [addOpen, setAddOpen] = useState(false)
  const [addQuery, setAddQuery] = useState('')
  const [addResults, setAddResults] = useState<Bookmark[]>([])
  const [propOpen, setPropOpen] = useState(false)
  const [propName, setPropName] = useState('')
  const [propType, setPropType] = useState<(typeof PROPERTY_TYPES)[number]>('SELECT')
  const [propOptions, setPropOptions] = useState<{ label: string; color: string }[]>([])
  const [propMode, setPropMode] = useState<'date' | 'datetime'>('date')
  const [viewOpen, setViewOpen] = useState(false)
  const [viewName, setViewName] = useState('')
  const [viewType, setViewType] = useState<'TABLE' | 'BOARD' | 'CALENDAR' | 'LIST'>('TABLE')
  const [filterOpen, setFilterOpen] = useState(false)
  const [fProp, setFProp] = useState('')
  const [fOp, setFOp] = useState('eq')
  const [fValue, setFValue] = useState('')

  const loadCollections = useCallback(async () => {
    try {
      const res = await fetch('/api/collections')
      if (res.ok) setCollections(await res.json())
    } catch { toast.error('Could not load collections') }
  }, [])

  const loadDetail = useCallback(async (id: string) => {
    try {
      const res = await fetch(`/api/collections/${id}`)
      if (!res.ok) { setDetail(null); return }
      const data: CollectionDetail = await res.json()
      setDetail(data)
      setNameDraft(data.name)
      setViewId((current) => (current && data.views.some((v) => v.id === current) ? current : data.views[0]?.id ?? null))
    } catch { toast.error('Could not load collection') }
  }, [])

  const loadItems = useCallback(async (id: string, view: string | null, append?: string | null) => {
    try {
      const params = new URLSearchParams({ limit: '200' })
      if (view) params.set('viewId', view)
      if (append) params.set('cursor', append)
      const res = await fetch(`/api/collections/${id}/items?${params}`)
      if (!res.ok) return
      const rows: CollectionItem[] = await res.json()
      setItems((current) => (append ? [...current, ...rows] : rows))
      setCursor(res.headers.get('X-Next-Cursor'))
    } catch { toast.error('Could not load items') }
  }, [])

  useEffect(() => { const t = setTimeout(loadCollections, 0); return () => clearTimeout(t) }, [loadCollections])
  useEffect(() => {
    const t = setTimeout(() => {
      if (!activeId && collections.length) setActiveId(collections[0].id)
    }, 0)
    return () => clearTimeout(t)
  }, [collections, activeId])
  useEffect(() => {
    const t = setTimeout(() => {
      if (!activeId) { setDetail(null); return }
      loadDetail(activeId)
    }, 0)
    return () => clearTimeout(t)
  }, [activeId, loadDetail])
  useEffect(() => {
    const t = setTimeout(() => {
      if (!activeId) { setItems([]); return }
      loadItems(activeId, viewId)
    }, 0)
    return () => clearTimeout(t)
  }, [activeId, viewId, loadItems])

  // Versioned mutation helper: 409 means another session moved first — the
  // server state wins, we reload, and nothing is clobbered (DATA-05 lesson).
  const mutate = async (res: Response, reload: () => void, okMessage?: string): Promise<boolean> => {
    if (res.ok) { if (okMessage) toast.success(okMessage); return true }
    if (res.status === 409) {
      const body = await json(res)
      toast.error(body?.error || 'Changed elsewhere; reloaded the latest version')
      reload()
      return false
    }
    const body = await json(res)
    toast.error(body?.error || 'Request failed')
    return false
  }
  const patch = (url: string, body: Record<string, unknown>) =>
    fetch(url, { method: 'PATCH', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) })

  const activeView = detail?.views.find((v) => v.id === viewId) ?? null
  const groupProp = activeView?.groupByPropertyId ? detail?.properties.find((p) => p.id === activeView.groupByPropertyId) : null

  const createCollection = async () => {
    const name = newName.trim()
    if (!name) return
    const res = await fetch('/api/collections', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ name }) })
    if (!res.ok) { await mutate(res, loadCollections); return }
    const created = await res.json().catch(() => null)
    toast.success('Collection created')
    setNewName('')
    await loadCollections()
    if (created?.id) setActiveId(created.id)
  }

  const saveName = async () => {
    if (!detail) return
    const name = nameDraft.trim()
    if (!name || name === detail.name) { setNameDraft(detail.name); return }
    const res = await patch(`/api/collections/${detail.id}`, { version: detail.version, name })
    if (await mutate(res, () => loadDetail(detail.id), 'Renamed')) {
      setDetail((d) => (d ? { ...d, name, version: d.version + 1 } : d))
      loadCollections()
    } else setNameDraft(detail.name)
  }

  const deleteCollection = async () => {
    if (!detail || !confirm(`Delete collection "${detail.name}"? Bookmarks are kept.`)) return
    const res = await fetch(`/api/collections/${detail.id}`, { method: 'DELETE' })
    if (await mutate(res, loadCollections, 'Collection deleted')) {
      setActiveId(null)
      setDetail(null)
      loadCollections()
    }
  }

  const createProperty = async () => {
    if (!detail) return
    const name = propName.trim()
    if (!name) return
    const config: Record<string, unknown> = {}
    if (propType === 'SELECT' || propType === 'MULTI_SELECT') {
      config.options = propOptions.filter((o) => o.label.trim()).map((o) => ({ label: o.label.trim(), ...(o.color ? { color: o.color } : {}) }))
    }
    if (propType === 'DATE') config.mode = propMode
    const res = await fetch(`/api/collections/${detail.id}/properties`, {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name, type: propType, config }),
    })
    if (await mutate(res, () => loadDetail(detail.id), 'Property added')) {
      setPropOpen(false); setPropName(''); setPropOptions([])
      loadDetail(detail.id)
    }
  }

  const deleteProperty = async (prop: CollectionProperty) => {
    if (!detail || !confirm(`Delete property "${prop.name}"? Its values are removed from every item.`)) return
    const res = await fetch(`/api/collections/${detail.id}/properties/${prop.id}`, { method: 'DELETE' })
    if (await mutate(res, () => loadDetail(detail.id), 'Property deleted')) {
      loadDetail(detail.id)
      loadItems(detail.id, viewId)
    }
  }

  const createView = async () => {
    if (!detail) return
    const name = viewName.trim()
    if (!name) return
    const res = await fetch(`/api/collections/${detail.id}/views`, {
      method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ name, type: viewType }),
    })
    if (!res.ok) { await mutate(res, () => loadDetail(detail.id)); return }
    const created = await res.json().catch(() => null)
    toast.success('View added')
    setViewOpen(false); setViewName('')
    await loadDetail(detail.id)
    if (created?.id) setViewId(created.id)
  }

  const deleteView = async () => {
    if (!detail || !activeView || !confirm(`Delete view "${activeView.name}"?`)) return
    const res = await fetch(`/api/collections/${detail.id}/views/${activeView.id}`, { method: 'DELETE' })
    if (await mutate(res, () => loadDetail(detail.id), 'View deleted')) {
      setViewId(null)
      loadDetail(detail.id)
    }
  }

  const patchView = async (changes: Record<string, unknown>) => {
    if (!detail || !activeView) return
    const res = await patch(`/api/collections/${detail.id}/views/${activeView.id}`, { version: activeView.version, ...changes })
    if (res.ok) {
      const updated: CollectionView = await res.json()
      setDetail((d) => (d ? { ...d, views: d.views.map((v) => (v.id === updated.id ? updated : v)) } : d))
      loadItems(detail.id, updated.id)
    } else await mutate(res, () => loadDetail(detail.id))
  }

  const addItem = async (bookmarkId: string) => {
    if (!detail) return
    const res = await fetch(`/api/collections/${detail.id}/items`, {
      method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ bookmarkId }),
    })
    if (res.ok) {
      loadItems(detail.id, viewId)
      loadCollections()
    } else if (res.status === 409) toast.error('Already in this collection')
    else { const body = await json(res); toast.error(body?.error || 'Could not add bookmark') }
  }

  const removeItem = async (item: CollectionItem) => {
    if (!detail) return
    const res = await fetch(`/api/collections/${detail.id}/items/${item.id}`, { method: 'DELETE' })
    if (await mutate(res, () => loadItems(detail.id, viewId))) {
      setItems((current) => current.filter((i) => i.id !== item.id))
      loadCollections()
    }
  }

  const saveValue = async (item: CollectionItem, propId: string, value: unknown) => {
    if (!detail) return
    const res = await patch(`/api/collections/${detail.id}/items/${item.id}`, {
      version: item.version, propertyValues: { [propId]: value },
    })
    if (res.ok) {
      const updated: CollectionItem = await res.json()
      setItems((current) => current.map((i) => (i.id === updated.id ? updated : i)))
    } else await mutate(res, () => loadItems(detail.id, viewId))
  }

  // Debounced bookmark search for the add dialog.
  useEffect(() => {
    if (!addOpen) return
    const q = addQuery.trim()
    const t = setTimeout(async () => {
      try {
        const params = new URLSearchParams({ limit: '8' })
        if (q) params.set('search', q)
        const res = await fetch(`/api/bookmarks?${params}`)
        if (res.ok) setAddResults((await res.json()).slice(0, 8))
      } catch { /* transient; retried on next keystroke */ }
    }, 250)
    return () => clearTimeout(t)
  }, [addQuery, addOpen])

  const filterClauses = activeView?.filters?.clauses ?? []
  const setClause = (clauses: FilterClause[]) => { void patchView({ filters: { operator: activeView?.filters?.operator ?? 'AND', clauses } }) }
  const addClause = () => {
    if (!fProp || !fOp) return
    const prop = detail?.properties.find((p) => p.id === fProp)
    if (!prop) return
    let value: unknown
    if (!['empty', 'notEmpty'].includes(fOp)) {
      if (prop.type === 'NUMBER') value = Number(fValue)
      else if (prop.type === 'CHECKBOX') value = fValue === 'true'
      else value = fValue
      if (fValue === '' || (prop.type === 'NUMBER' && Number.isNaN(value))) { toast.error('Filter value required'); return }
    }
    setClause([...filterClauses, value === undefined ? { propertyId: fProp, op: fOp } : { propertyId: fProp, op: fOp, value }])
    setFilterOpen(false); setFValue('')
  }
  const clauseLabel = (c: FilterClause) => {
    const prop = detail?.properties.find((p) => p.id === c.propertyId)
    if (!prop) return 'unknown'
    let shown = c.value
    if (prop.type === 'SELECT' && typeof c.value === 'string') shown = prop.config.options?.find((o) => o.id === c.value)?.label ?? c.value
    if (prop.type === 'CHECKBOX') shown = c.value === true ? 'checked' : 'unchecked'
    return `${prop.name} ${OP_LABELS[c.op] ?? c.op}${shown !== undefined ? ` ${shown}` : ''}`
  }

  const renderTable = () => (
    <div className="overflow-x-auto rounded-lg border border-border/60">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead className="min-w-[16rem]">Bookmark</TableHead>
            {(detail?.properties ?? []).map((p) => (
              <TableHead key={p.id} className="min-w-[11rem]">
                <span className="flex items-center gap-1.5">
                  {p.name}
                  <button type="button" aria-label={`Delete property ${p.name}`} className="text-muted-foreground opacity-0 transition-opacity hover:text-destructive group-hover:opacity-100" onClick={() => deleteProperty(p)}>
                    <Trash2 className="h-3 w-3" />
                  </button>
                </span>
              </TableHead>
            ))}
            <TableHead className="w-10" />
          </TableRow>
        </TableHeader>
        <TableBody>
          {items.map((item) => (
            <TableRow key={item.id} className="group">
              <TableCell><BookmarkCell bookmark={item.bookmark} /></TableCell>
              {(detail?.properties ?? []).map((p) => (
                <TableCell key={p.id}><ValueEditor prop={p} value={item.propertyValues?.[p.id]} onSave={(v) => saveValue(item, p.id, v)} /></TableCell>
              ))}
              <TableCell>
                <button type="button" aria-label="Remove from collection" className="text-muted-foreground hover:text-destructive" onClick={() => removeItem(item)}>
                  <Trash2 className="h-4 w-4" />
                </button>
              </TableCell>
            </TableRow>
          ))}
          {items.length === 0 && (
            <TableRow>
              <TableCell colSpan={(detail?.properties.length ?? 0) + 2} className="py-10 text-center text-sm text-muted-foreground">
                No items{filterClauses.length ? " match this view's filters" : ''}. Add existing bookmarks — nothing is duplicated.
              </TableCell>
            </TableRow>
          )}
        </TableBody>
      </Table>
    </div>
  )

  const renderBoard = () => {
    if (!groupProp || groupProp.type !== 'SELECT') {
      return (
        <div className="rounded-lg border border-dashed border-border p-8 text-center">
          <Kanban className="mx-auto mb-2 h-8 w-8 text-muted-foreground" />
          <p className="text-sm text-muted-foreground">Board views group by a select property.</p>
          <div className="mx-auto mt-3 w-56">
            <Select
              value={NONE}
              onValueChange={(id) => { if (id !== NONE) patchView({ groupByPropertyId: id }) }}
            >
              <SelectTrigger aria-label="Group by property"><SelectValue placeholder="Choose a select property…" /></SelectTrigger>
              <SelectContent>
                {(detail?.properties ?? []).filter((p) => p.type === 'SELECT').map((p) => (
                  <SelectItem key={p.id} value={p.id}>{p.name}</SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        </div>
      )
    }
    const options = groupProp.config.options ?? []
    const columns = [...options.map((o) => ({ key: o.id, label: o.label, color: o.color })), { key: NONE, label: 'No value', color: undefined }]
    return (
      <div className="flex gap-3 overflow-x-auto pb-2">
        {columns.map((col) => {
          const colItems = items.filter((i) => {
            const v = i.propertyValues?.[groupProp.id]
            return col.key === NONE ? (v === undefined || v === null) : v === col.key
          })
          return (
            <div key={col.key} className="w-64 shrink-0 rounded-lg border border-border/60 bg-muted/30 p-2">
              <div className="mb-2 flex items-center gap-1.5 px-1 text-sm font-medium">
                {col.color && <span className="h-2 w-2 rounded-full" style={{ backgroundColor: col.color }} />}
                {col.label}
                <span className="ml-auto text-xs text-muted-foreground">{colItems.length}</span>
              </div>
              <div className="space-y-2">
                {colItems.map((item) => (
                  <div key={item.id} className="rounded-md border border-border bg-card p-2 shadow-sm">
                    <BookmarkCell bookmark={item.bookmark} />
                    <div className="mt-1.5 flex items-center justify-between gap-1">
                      <Select
                        value={typeof item.propertyValues?.[groupProp.id] === 'string' ? (item.propertyValues[groupProp.id] as string) : NONE}
                        onValueChange={(id) => saveValue(item, groupProp.id, id === NONE ? null : id)}
                      >
                        <SelectTrigger className="h-7 w-28 text-xs" aria-label={`Move ${item.bookmark.title ?? 'card'}`}><SelectValue /></SelectTrigger>
                        <SelectContent>
                          <SelectItem value={NONE}>—</SelectItem>
                          {options.map((o) => <SelectItem key={o.id} value={o.id}>{o.label}</SelectItem>)}
                        </SelectContent>
                      </Select>
                      <button type="button" aria-label="Remove from collection" className="text-muted-foreground hover:text-destructive" onClick={() => removeItem(item)}>
                        <Trash2 className="h-3.5 w-3.5" />
                      </button>
                    </div>
                  </div>
                ))}
                {colItems.length === 0 && <p className="px-1 py-3 text-center text-xs text-muted-foreground">Empty</p>}
              </div>
            </div>
          )
        })}
      </div>
    )
  }

  return (
    <div className="flex h-full flex-col">
      <div className="flex flex-1 overflow-hidden">
        {/* Collection list */}
        <div className="flex w-60 shrink-0 flex-col border-r border-border/60 p-3">
          <h1 className="mb-3 flex items-center gap-2 font-display text-lg font-semibold tracking-tight">
            <Layers className="h-4 w-4" /> Collections
          </h1>
          <div className="mb-3 flex gap-1.5">
            <Input value={newName} onChange={(e) => setNewName(e.target.value)} onKeyDown={(e) => { if (e.key === 'Enter') createCollection() }} placeholder="New collection" className="h-8 text-sm" aria-label="New collection name" />
            <Button size="sm" variant="outline" className="h-8 shrink-0 px-2" onClick={createCollection} aria-label="Create collection"><Plus className="h-4 w-4" /></Button>
          </div>
          <div className="flex-1 space-y-1 overflow-y-auto">
            {collections.map((c) => (
              <button
                key={c.id}
                onClick={() => { setActiveId(c.id); setViewId(null) }}
                className={cn('flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm transition-colors', c.id === activeId ? 'bg-primary/10 font-medium text-primary' : 'text-muted-foreground hover:bg-muted hover:text-foreground')}
              >
                <Layers className="h-3.5 w-3.5 shrink-0" />
                <span className="min-w-0 flex-1 truncate">{c.name}</span>
                <span className="shrink-0 text-xs text-muted-foreground">{c._count.items}</span>
              </button>
            ))}
            {collections.length === 0 && (
              <p className="px-2 py-6 text-center text-xs text-muted-foreground">
                Collections hold workflow state per project. The same bookmark can live in many — nothing is duplicated.
              </p>
            )}
          </div>
        </div>

        {/* Detail */}
        <div className="flex-1 overflow-y-auto px-6 py-5">
          {!detail ? (
            <div className="flex h-full flex-col items-center justify-center gap-3 text-center">
              <Layers className="h-10 w-10 text-muted-foreground" />
              <h2 className="font-display text-xl font-semibold">{collections.length ? 'Select a collection' : 'No collections yet'}</h2>
              <p className="max-w-sm text-sm text-muted-foreground">
                A collection is a saved workflow over existing bookmarks: per-collection status, priority, dates — independent everywhere the link appears.
              </p>
            </div>
          ) : (
            <>
              <div className="mb-4 flex flex-wrap items-center gap-2">
                <Input value={nameDraft} onChange={(e) => setNameDraft(e.target.value)} onBlur={saveName} onKeyDown={(e) => { if (e.key === 'Enter') (e.target as HTMLInputElement).blur() }} aria-label="Collection name" className="w-72 bg-transparent font-display text-2xl font-bold tracking-tight" />
                <span className="text-xs text-muted-foreground">{detail._count.items} item{detail._count.items === 1 ? '' : 's'}</span>
                <div className="ml-auto flex items-center gap-1.5">
                  <Button size="sm" variant="outline" onClick={() => setAddOpen(true)}><Plus className="mr-1 h-4 w-4" />Add bookmarks</Button>
                  <Button size="sm" variant="outline" onClick={() => setPropOpen(true)}><Plus className="mr-1 h-4 w-4" />Property</Button>
                  <Button size="sm" variant="ghost" onClick={deleteCollection} aria-label="Delete collection" title="Delete collection"><Trash2 className="h-4 w-4 text-destructive" /></Button>
                </div>
              </div>

              {/* View tabs */}
              <div className="mb-3 flex flex-wrap items-center gap-1.5">
                {detail.views.map((v) => {
                  const Icon = VIEW_ICONS[v.type] ?? Table2
                  return (
                    <button
                      key={v.id}
                      onClick={() => setViewId(v.id)}
                      className={cn('flex items-center gap-1.5 rounded-md px-2.5 py-1 text-sm transition-colors', v.id === viewId ? 'bg-primary/10 font-medium text-primary' : 'text-muted-foreground hover:bg-muted hover:text-foreground')}
                    >
                      <Icon className="h-3.5 w-3.5" /> {v.name}
                    </button>
                  )
                })}
                <Button size="sm" variant="ghost" className="h-7 px-2" onClick={() => setViewOpen(true)} aria-label="Add view"><Plus className="h-4 w-4" /></Button>
                {activeView && (
                  <div className="ml-auto flex items-center gap-1.5">
                    <Popover open={filterOpen} onOpenChange={(o) => { setFilterOpen(o); if (o) { setFProp(detail.properties[0]?.id ?? ''); setFOp(FILTER_OPS[detail.properties[0]?.type ?? 'TEXT']?.[0] ?? 'eq') } }}>
                      <PopoverTrigger asChild>
                        <Button size="sm" variant="ghost" className="h-7 gap-1 px-2 text-muted-foreground"><Filter className="h-3.5 w-3.5" />Filter{filterClauses.length > 0 && ` (${filterClauses.length})`}</Button>
                      </PopoverTrigger>
                      <PopoverContent className="w-80 space-y-2" align="end">
                        <Label className="text-xs uppercase tracking-wider text-muted-foreground">Add a filter clause</Label>
                        <Select value={fProp || undefined} onValueChange={(id) => { setFProp(id); const p = detail.properties.find((x) => x.id === id); setFOp(FILTER_OPS[p?.type ?? 'TEXT']?.[0] ?? 'eq'); setFValue('') }}>
                          <SelectTrigger aria-label="Filter property"><SelectValue placeholder="Property" /></SelectTrigger>
                          <SelectContent>
                            {detail.properties.filter((p) => FILTER_OPS[p.type]).map((p) => <SelectItem key={p.id} value={p.id}>{p.name}</SelectItem>)}
                          </SelectContent>
                        </Select>
                        <Select value={fOp} onValueChange={setFOp}>
                          <SelectTrigger aria-label="Filter operator"><SelectValue /></SelectTrigger>
                          <SelectContent>
                            {(FILTER_OPS[detail.properties.find((p) => p.id === fProp)?.type ?? 'TEXT'] ?? []).map((op) => <SelectItem key={op} value={op}>{OP_LABELS[op] ?? op}</SelectItem>)}
                          </SelectContent>
                        </Select>
                        {!['empty', 'notEmpty'].includes(fOp) && (() => {
                          const p = detail.properties.find((x) => x.id === fProp)
                          if (p?.type === 'SELECT') return (
                            <Select value={fValue || undefined} onValueChange={setFValue}>
                              <SelectTrigger aria-label="Filter value"><SelectValue placeholder="Value" /></SelectTrigger>
                              <SelectContent>{(p.config.options ?? []).map((o) => <SelectItem key={o.id} value={o.id}>{o.label}</SelectItem>)}</SelectContent>
                            </Select>
                          )
                          if (p?.type === 'CHECKBOX') return (
                            <Select value={fValue || undefined} onValueChange={setFValue}>
                              <SelectTrigger aria-label="Filter value"><SelectValue placeholder="Value" /></SelectTrigger>
                              <SelectContent><SelectItem value="true">checked</SelectItem><SelectItem value="false">unchecked</SelectItem></SelectContent>
                            </Select>
                          )
                          return <Input value={fValue} onChange={(e) => setFValue(e.target.value)} placeholder="Value" aria-label="Filter value" className="h-8 text-sm" />
                        })()}
                        <Button size="sm" className="w-full" onClick={addClause}>Apply filter</Button>
                      </PopoverContent>
                    </Popover>
                    <Button size="sm" variant="ghost" className="h-7 px-2 text-muted-foreground" onClick={deleteView} aria-label="Delete view" title="Delete view"><Trash2 className="h-3.5 w-3.5" /></Button>
                  </div>
                )}
              </div>

              {/* Active filter chips */}
              {filterClauses.length > 0 && (
                <div className="mb-3 flex flex-wrap items-center gap-1.5">
                  <span className="text-xs text-muted-foreground">{activeView?.filters?.operator ?? 'AND'}:</span>
                  {filterClauses.map((c, i) => (
                    <Badge key={i} variant="secondary" className="gap-1">
                      {clauseLabel(c)}
                      <button type="button" aria-label="Remove filter" onClick={() => setClause(filterClauses.filter((_, j) => j !== i))}><X className="h-3 w-3" /></button>
                    </Badge>
                  ))}
                </div>
              )}

              {activeView?.type === 'BOARD' ? renderBoard() : renderTable()}
              {activeView && (activeView.type === 'CALENDAR' || activeView.type === 'LIST') && (
                <p className="mt-2 text-xs text-muted-foreground">Calendar and list layouts render as a table in this phase.</p>
              )}
              {cursor && (
                <Button size="sm" variant="outline" className="mt-3" onClick={() => loadItems(detail.id, viewId, cursor)}>Load more</Button>
              )}
            </>
          )}
        </div>
      </div>

      {/* Add bookmarks dialog */}
      <Dialog open={addOpen} onOpenChange={(o) => { setAddOpen(o); if (!o) setAddQuery('') }}>
        <DialogContent className="max-w-lg">
          <DialogHeader><DialogTitle>Add existing bookmarks</DialogTitle></DialogHeader>
          <p className="text-xs text-muted-foreground">Membership is contextual — adding here never duplicates or re-owns the bookmark.</p>
          <Input autoFocus value={addQuery} onChange={(e) => setAddQuery(e.target.value)} placeholder="Search bookmarks…" aria-label="Search bookmarks" />
          <div className="max-h-72 space-y-1 overflow-y-auto">
            {addResults.map((b) => {
              const member = items.some((i) => i.bookmarkId === b.id)
              return (
                <div key={b.id} className="flex items-center gap-2 rounded-md border border-border/60 px-2 py-1.5">
                  <div className="min-w-0 flex-1">
                    <p className="truncate text-sm">{b.title || b.url}</p>
                    <p className="truncate text-xs text-muted-foreground">{b.url}</p>
                  </div>
                  <Button size="sm" variant={member ? 'ghost' : 'outline'} disabled={member} onClick={() => addItem(b.id)}>
                    {member ? 'Added' : 'Add'}
                  </Button>
                </div>
              )
            })}
            {addResults.length === 0 && <p className="py-6 text-center text-sm text-muted-foreground">No bookmarks found</p>}
          </div>
        </DialogContent>
      </Dialog>

      {/* New property dialog */}
      <Dialog open={propOpen} onOpenChange={setPropOpen}>
        <DialogContent className="max-w-md">
          <DialogHeader><DialogTitle>New property</DialogTitle></DialogHeader>
          <div className="space-y-3">
            <Input value={propName} onChange={(e) => setPropName(e.target.value)} placeholder="Name (e.g. Status)" aria-label="Property name" />
            <Select value={propType} onValueChange={(t) => setPropType(t as (typeof PROPERTY_TYPES)[number])}>
              <SelectTrigger aria-label="Property type"><SelectValue /></SelectTrigger>
              <SelectContent>
                {PROPERTY_TYPES.map((t) => <SelectItem key={t} value={t}>{t.replace('_', ' ').toLowerCase().replace(/\b\w/g, (m) => m.toUpperCase())}</SelectItem>)}
              </SelectContent>
            </Select>
            <p className="text-xs text-muted-foreground">Type is permanent — conversions that could destroy values are refused by design.</p>
            {(propType === 'SELECT' || propType === 'MULTI_SELECT') && (
              <div className="space-y-2">
                <Label className="text-xs uppercase tracking-wider text-muted-foreground">Options</Label>
                {propOptions.map((o, i) => (
                  <div key={i} className="flex items-center gap-2">
                    <Input type="color" aria-label="Option color" value={o.color || '#6366f1'} onChange={(e) => setPropOptions(propOptions.map((x, j) => (j === i ? { ...x, color: e.target.value } : x)))} className="h-8 w-10 shrink-0 p-1" />
                    <Input value={o.label} onChange={(e) => setPropOptions(propOptions.map((x, j) => (j === i ? { ...x, label: e.target.value } : x)))} placeholder="Option label" aria-label="Option label" className="h-8 text-sm" />
                    <Button size="sm" variant="ghost" className="h-8 px-2" aria-label="Remove option" onClick={() => setPropOptions(propOptions.filter((_, j) => j !== i))}><X className="h-4 w-4" /></Button>
                  </div>
                ))}
                <Button size="sm" variant="outline" onClick={() => setPropOptions([...propOptions, { label: '', color: '#6366f1' }])}><Plus className="mr-1 h-4 w-4" />Option</Button>
              </div>
            )}
            {propType === 'DATE' && (
              <Select value={propMode} onValueChange={(m) => setPropMode(m as 'date' | 'datetime')}>
                <SelectTrigger aria-label="Date mode"><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="date">Date only (never shifts across timezones)</SelectItem>
                  <SelectItem value="datetime">Date &amp; time (stored UTC)</SelectItem>
                </SelectContent>
              </Select>
            )}
            <Button className="w-full" onClick={createProperty}>Create property</Button>
          </div>
        </DialogContent>
      </Dialog>

      {/* New view dialog */}
      <Dialog open={viewOpen} onOpenChange={setViewOpen}>
        <DialogContent className="max-w-sm">
          <DialogHeader><DialogTitle>New view</DialogTitle></DialogHeader>
          <div className="space-y-3">
            <Input autoFocus value={viewName} onChange={(e) => setViewName(e.target.value)} placeholder="View name" aria-label="View name" />
            <Select value={viewType} onValueChange={(t) => setViewType(t as typeof viewType)}>
              <SelectTrigger aria-label="View type"><SelectValue /></SelectTrigger>
              <SelectContent>
                <SelectItem value="TABLE">Table</SelectItem>
                <SelectItem value="BOARD">Board</SelectItem>
                <SelectItem value="CALENDAR">Calendar (renders as table for now)</SelectItem>
                <SelectItem value="LIST">List (renders as table for now)</SelectItem>
              </SelectContent>
            </Select>
            <p className="text-xs text-muted-foreground">A view is a saved query — filters and grouping never copy records.</p>
            <Button className="w-full" onClick={createView}>Create view</Button>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  )
}
