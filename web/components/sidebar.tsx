'use client'

import Link from 'next/link'
import { useRouter, useSearchParams } from 'next/navigation'
import { Suspense, useCallback, useEffect, useState } from 'react'
import { cn } from '@/lib/utils'
import { Calendar, Tags, Users, Kanban, Link2, Settings, ChevronLeft, ChevronRight, FileText, Pin, Plus, Shield } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { PAGES_CHANGED, nextUntitledTitle, notifyPagesChanged, type PageSummary } from '@/lib/pages'

// Screens below are filters/views over the same bookmark collection, not
// separate data types. Pages are the primary interface objects.
const filterItems = [
  { href: '/bookmarks', label: 'All Bookmarks', icon: Link2 },
  { href: '/calendar', label: 'Calendar', icon: Calendar },
  { href: '/tags', label: 'Tags', icon: Tags },
  { href: '/kanban', label: 'Kanban', icon: Kanban },
  { href: '/contacts', label: 'Contacts', icon: Users },
]

export function Sidebar({ open, onToggle, pathname, isAdmin = false }: { open: boolean; onToggle: () => void; pathname: string; isAdmin?: boolean }) {
  return (
    <aside
      className={cn(
        'flex flex-col border-r bg-[hsl(var(--sidebar))] text-[hsl(var(--sidebar-foreground))] transition-all duration-300 ease-in-out',
        open ? 'w-56' : 'w-16'
      )}
    >
      <div className="flex items-center gap-2 border-b px-3 py-4">
        <Link href="/pages" className="flex items-center gap-2">
          <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-primary text-primary-foreground">
            <Link2 className="h-4 w-4" />
          </div>
          {open && <span className="font-display text-lg font-bold tracking-tight">computer</span>}
        </Link>
      </div>
      <Suspense fallback={<div className="flex-1" />}>
        <SidebarNav open={open} pathname={pathname} isAdmin={isAdmin} />
      </Suspense>
      <div className="border-t p-2">
        <Button variant="ghost" size="icon-sm" onClick={onToggle} className="w-full">
          {open ? <ChevronLeft className="h-4 w-4" /> : <ChevronRight className="h-4 w-4" />}
        </Button>
      </div>
    </aside>
  )
}

function SidebarNav({ open, pathname, isAdmin = false }: { open: boolean; pathname: string; isAdmin?: boolean }) {
  const router = useRouter()
  const searchParams = useSearchParams()
  const [pages, setPages] = useState<PageSummary[]>([])
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
    const res = await fetch('/api/pages', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ title }) })
    if (!res.ok) return
    const p = await res.json()
    notifyPagesChanged()
    router.push(`/pages?id=${p.id}`)
  }

  const filterEntries = isAdmin
    ? [...filterItems, { href: '/settings', label: 'Settings', icon: Settings }, { href: '/admin', label: 'Admin', icon: Shield }]
    : [...filterItems, { href: '/settings', label: 'Settings', icon: Settings }]

  return (
    <nav className="flex-1 space-y-1 overflow-y-auto p-2">
      <div className="flex items-center justify-between px-3 pb-1 pt-1">
        {open && <p className="text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">Pages</p>}
        <Button variant="ghost" size="icon-sm" onClick={createPage} title="New page" aria-label="New page" className="shrink-0">
          <Plus className="h-4 w-4" />
        </Button>
      </div>
      {pages.map((p) => {
        const isActive = pathname === '/pages' && activePageId === p.id
        return (
          <Link
            key={p.id}
            href={`/pages?id=${p.id}`}
            className={cn(
              'flex items-center gap-2 rounded-lg px-3 py-1.5 text-sm transition-colors',
              isActive ? 'bg-accent font-medium text-accent-foreground' : 'text-muted-foreground hover:bg-accent/50 hover:text-foreground'
            )}
          >
            {p.pinned ? <Pin className="h-3.5 w-3.5 shrink-0" /> : <FileText className="h-3.5 w-3.5 shrink-0" />}
            {open && <span className="truncate">{p.title}</span>}
          </Link>
        )
      })}
      {open && pages.length === 0 && (
        <p className="px-3 py-1.5 text-xs text-muted-foreground/70">No pages yet</p>
      )}
      {open && (
        <p className="px-3 pb-1 pt-4 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">Filters</p>
      )}
      {filterEntries.map((item) => {
        const isActive = pathname === item.href || pathname?.startsWith(item.href + '/')
        return (
          <Link
            key={item.href}
            href={item.href}
            className={cn(
              'flex items-center gap-3 rounded-lg px-3 py-2 text-sm font-medium transition-colors',
              isActive ? 'bg-accent text-accent-foreground' : 'text-muted-foreground hover:bg-accent/50 hover:text-foreground'
            )}
          >
            <item.icon className="h-4 w-4 shrink-0" />
            {open && <span>{item.label}</span>}
          </Link>
        )
      })}
    </nav>
  )
}
