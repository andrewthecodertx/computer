'use client'

import Link from 'next/link'
import { useRouter, useSearchParams } from 'next/navigation'
import { Suspense, useCallback, useEffect, useState, useSyncExternalStore } from 'react'
import { cn } from '@/lib/utils'
import {
  Calendar, Tags, Users, Kanban, Link2, Settings, ChevronLeft, ChevronRight, ChevronDown,
  FileText, Pin, PinOff, Plus, Shield, Search, MoreHorizontal, Trash2, Pencil,
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { toast } from 'sonner'
import { PAGES_CHANGED, nextUntitledTitle, notifyPagesChanged, type PageSummary } from '@/lib/pages'

// Screens below are filters/views over the same bookmark collection, not
// separate data types. Pages are the primary interface objects.
const viewItems = [
  { href: '/bookmarks', label: 'All Bookmarks', icon: Link2 },
  { href: '/calendar', label: 'Calendar', icon: Calendar },
  { href: '/tags', label: 'Tags', icon: Tags },
  { href: '/kanban', label: 'Kanban', icon: Kanban },
  { href: '/contacts', label: 'Contacts', icon: Users },
]

// Section collapse state persists per browser. useSyncExternalStore keeps it
// hydration-safe (server snapshot uses the default; the client snapshot reads
// localStorage after mount) without setState inside an effect.
const SECTION_STORE_EVENT = 'computer:sidebar-sections'

function readSection(key: string, fallback: boolean): boolean {
  try {
    const stored = window.localStorage.getItem(key)
    return stored === '0' || stored === '1' ? stored === '1' : fallback
  } catch { return fallback }
}

function usePersistentState(key: string, initial: boolean) {
  const subscribe = useCallback((onChange: () => void) => {
    window.addEventListener('storage', onChange)
    window.addEventListener(SECTION_STORE_EVENT, onChange)
    return () => {
      window.removeEventListener('storage', onChange)
      window.removeEventListener(SECTION_STORE_EVENT, onChange)
    }
  }, [])
  const getSnapshot = useCallback(() => readSection(key, initial), [key, initial])
  const getServerSnapshot = useCallback(() => initial, [initial])
  const value = useSyncExternalStore(subscribe, getSnapshot, getServerSnapshot)
  const update = useCallback((next: boolean) => {
    try { window.localStorage.setItem(key, next ? '1' : '0') } catch { /* non-fatal */ }
    window.dispatchEvent(new Event(SECTION_STORE_EVENT))
  }, [key])
  return [value, update] as const
}

interface SidebarProps {
  open: boolean
  onToggle: () => void
  onQuickFind: () => void
  pathname: string
  isAdmin?: boolean
}

export function Sidebar({ open, onToggle, onQuickFind, pathname, isAdmin = false }: SidebarProps) {
  return (
    <aside
      className={cn(
        'flex flex-col border-r bg-[hsl(var(--sidebar))] text-[hsl(var(--sidebar-foreground))] transition-all duration-300 ease-in-out',
        open ? 'w-60' : 'w-16'
      )}
    >
      <div className="flex items-center gap-2 px-3 py-3">
        <Link href="/pages" className="flex min-w-0 items-center gap-2">
          <div className="flex h-6 w-6 shrink-0 items-center justify-center rounded bg-primary text-primary-foreground">
            <Link2 className="h-3.5 w-3.5" />
          </div>
          {open && <span className="truncate font-display text-sm font-bold tracking-tight">computer</span>}
        </Link>
      </div>
      <Suspense fallback={<div className="flex-1" />}>
        <SidebarNav open={open} pathname={pathname} isAdmin={isAdmin} />
      </Suspense>
      <div className="space-y-0.5 border-t p-2">
        <button
          onClick={onQuickFind}
          title="Quick find (Ctrl/⌘ K)"
          className="flex w-full items-center gap-2 rounded-lg px-3 py-1.5 text-sm text-muted-foreground transition-colors hover:bg-accent/50 hover:text-foreground"
        >
          <Search className="h-4 w-4 shrink-0" />
          {open && <span className="flex-1 text-left">Quick find</span>}
          {open && <kbd className="rounded border bg-background px-1 text-[10px] font-medium text-muted-foreground">⌘K</kbd>}
        </button>
        <Link
          href="/settings"
          className={cn(
            'flex w-full items-center gap-2 rounded-lg px-3 py-1.5 text-sm transition-colors',
            pathname === '/settings' ? 'bg-accent font-medium text-accent-foreground' : 'text-muted-foreground hover:bg-accent/50 hover:text-foreground'
          )}
        >
          <Settings className="h-4 w-4 shrink-0" />
          {open && <span>Settings</span>}
        </Link>
        <Button variant="ghost" size="icon-sm" onClick={onToggle} className="w-full" aria-label={open ? 'Collapse sidebar' : 'Expand sidebar'}>
          {open ? <ChevronLeft className="h-4 w-4" /> : <ChevronRight className="h-4 w-4" />}
        </Button>
      </div>
    </aside>
  )
}

function SectionHeader({ label, open, expanded, onToggle, action }: {
  label: string
  open: boolean
  expanded: boolean
  onToggle: () => void
  action?: { label: string; onClick: () => void }
}) {
  if (!open) return null
  return (
    <div className="group/header flex items-center gap-0.5 rounded-md pr-1 hover:bg-accent/40">
      <button onClick={onToggle} className="flex flex-1 items-center gap-1 px-2 py-1 text-left" aria-expanded={expanded}>
        <ChevronDown className={cn('h-3 w-3 shrink-0 text-muted-foreground transition-transform', !expanded && '-rotate-90')} />
        <span className="text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">{label}</span>
      </button>
      {action && (
        <button
          onClick={action.onClick}
          title={action.label}
          aria-label={action.label}
          className="shrink-0 rounded p-1 text-muted-foreground opacity-0 transition-opacity hover:bg-accent hover:text-foreground group-hover/header:opacity-100 focus-visible:opacity-100"
        >
          <Plus className="h-3.5 w-3.5" />
        </button>
      )}
    </div>
  )
}

function SidebarNav({ open, pathname, isAdmin = false }: { open: boolean; pathname: string; isAdmin?: boolean }) {
  const router = useRouter()
  const searchParams = useSearchParams()
  const [pages, setPages] = useState<PageSummary[]>([])
  const [pagesExpanded, setPagesExpanded] = usePersistentState('computer:sidebar:pages', true)
  const [viewsExpanded, setViewsExpanded] = usePersistentState('computer:sidebar:views', true)
  const [renamingId, setRenamingId] = useState<string | null>(null)
  const [renameValue, setRenameValue] = useState('')
  const activePageId = pathname === '/pages' ? searchParams.get('id') : null

  const load = useCallback(() => {
    fetch('/api/pages')
      .then((r) => (r.ok ? r.json() : []))
      .then((d: PageSummary[]) => setPages(d ?? []))
      .catch((e) => console.error('Failed to load pages', e))
  }, [])

  useEffect(() => {
    load()
    window.addEventListener(PAGES_CHANGED, load)
    return () => window.removeEventListener(PAGES_CHANGED, load)
  }, [load])

  const createPage = async () => {
    const title = nextUntitledTitle(pages.map((p) => p.title))
    try {
      const res = await fetch('/api/pages', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ title }) })
      if (!res.ok) { toast.error('Could not create page'); return }
      const p = await res.json()
      notifyPagesChanged()
      router.push(`/pages?id=${p.id}`)
    } catch { toast.error('Could not create page') }
  }

  const patchPage = async (id: string, body: Record<string, unknown>) => {
    try {
      const res = await fetch(`/api/pages/${id}`, { method: 'PATCH', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) })
      if (!res.ok) { toast.error('Could not update page'); return }
      load()
      notifyPagesChanged()
    } catch { toast.error('Could not update page') }
  }

  const startRename = (p: PageSummary) => {
    setRenamingId(p.id)
    setRenameValue(p.title)
  }

  const commitRename = async (p: PageSummary) => {
    const t = renameValue.trim()
    setRenamingId(null)
    if (!t || t === p.title) return
    await patchPage(p.id, { title: t })
  }

  const removePage = async (p: PageSummary) => {
    if (!confirm(`Delete page "${p.title}"? Bookmarks are kept.`)) return
    try {
      const res = await fetch(`/api/pages/${p.id}`, { method: 'DELETE' })
      if (!res.ok) { toast.error('Could not delete page'); return }
      load()
      notifyPagesChanged()
    } catch { toast.error('Could not delete page') }
  }

  const viewEntries = isAdmin ? [...viewItems, { href: '/admin', label: 'Admin', icon: Shield }] : viewItems

  return (
    <nav className="flex-1 space-y-0.5 overflow-y-auto p-2">
      <SectionHeader label="Pages" open={open} expanded={pagesExpanded} onToggle={() => setPagesExpanded(!pagesExpanded)} action={{ label: 'New page', onClick: createPage }} />
      {(pagesExpanded || !open) && pages.map((p) => {
        const isActive = pathname === '/pages' && activePageId === p.id
        if (renamingId === p.id) {
          return (
            <div key={p.id} className="px-1 py-0.5">
              <Input
                autoFocus
                value={renameValue}
                onChange={(e) => setRenameValue(e.target.value)}
                onBlur={() => commitRename(p)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter') { e.preventDefault(); (e.target as HTMLInputElement).blur() }
                  if (e.key === 'Escape') setRenamingId(null)
                }}
                className="h-7 text-sm"
                aria-label={`Rename page ${p.title}`}
              />
            </div>
          )
        }
        return (
          <div
            key={p.id}
            className={cn(
              'group/row flex items-center rounded-lg pr-1 transition-colors',
              isActive ? 'bg-accent text-accent-foreground' : 'hover:bg-accent/50'
            )}
          >
            <Link href={`/pages?id=${p.id}`} className="flex min-w-0 flex-1 items-center gap-2 px-2 py-1.5 text-sm">
              {p.pinned ? <Pin className="h-3.5 w-3.5 shrink-0 text-muted-foreground" /> : <FileText className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />}
              {(open || isActive) && <span className={cn('truncate', isActive ? 'font-medium' : 'text-muted-foreground')}>{p.title}</span>}
            </Link>
            {open && (
              <div className="flex shrink-0 items-center gap-0.5 opacity-0 transition-opacity focus-within:opacity-100 group-hover/row:opacity-100">
                <button
                  onClick={createPage}
                  title="New page"
                  aria-label="New page"
                  className="rounded p-0.5 text-muted-foreground hover:bg-accent hover:text-foreground"
                >
                  <Plus className="h-3.5 w-3.5" />
                </button>
                <DropdownMenu>
                  <DropdownMenuTrigger asChild>
                    <button title="Page actions" aria-label={`Actions for ${p.title}`} className="rounded p-0.5 text-muted-foreground hover:bg-accent hover:text-foreground">
                      <MoreHorizontal className="h-3.5 w-3.5" />
                    </button>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="start" className="w-44">
                    <DropdownMenuItem onClick={() => patchPage(p.id, { pinned: !p.pinned })}>
                      {p.pinned ? <PinOff className="mr-2 h-3.5 w-3.5" /> : <Pin className="mr-2 h-3.5 w-3.5" />}
                      {p.pinned ? 'Unpin' : 'Pin to sidebar'}
                    </DropdownMenuItem>
                    <DropdownMenuItem onClick={() => startRename(p)}>
                      <Pencil className="mr-2 h-3.5 w-3.5" />
                      Rename
                    </DropdownMenuItem>
                    <DropdownMenuSeparator />
                    <DropdownMenuItem className="text-destructive focus:text-destructive" onClick={() => removePage(p)}>
                      <Trash2 className="mr-2 h-3.5 w-3.5" />
                      Delete
                    </DropdownMenuItem>
                  </DropdownMenuContent>
                </DropdownMenu>
              </div>
            )}
          </div>
        )
      })}
      {open && pagesExpanded && pages.length === 0 && (
        <p className="px-3 py-1 text-xs text-muted-foreground/70">No pages yet</p>
      )}

      <div className="pt-2">
        <SectionHeader label="Views" open={open} expanded={viewsExpanded} onToggle={() => setViewsExpanded(!viewsExpanded)} />
        {(viewsExpanded || !open) && viewEntries.map((item) => {
          const isActive = pathname === item.href || pathname?.startsWith(item.href + '/')
          return (
            <Link
              key={item.href}
              href={item.href}
              className={cn(
                'flex items-center gap-2 rounded-lg px-2 py-1.5 text-sm transition-colors',
                isActive ? 'bg-accent font-medium text-accent-foreground' : 'text-muted-foreground hover:bg-accent/50 hover:text-foreground'
              )}
            >
              <item.icon className="h-4 w-4 shrink-0" />
              {open && <span className="truncate">{item.label}</span>}
            </Link>
          )
        })}
      </div>
    </nav>
  )
}
