'use client'

import Link from 'next/link'
import { cn } from '@/lib/utils'
import { useEffect, useState } from 'react'
import { Home, Calendar, Tags, Users, Kanban, Link2, Settings, ChevronLeft, ChevronRight, FileText, Pin, Shield } from 'lucide-react'
import { Button } from '@/components/ui/button'

const navItems = [
  { href: '/dashboard', label: 'Dashboard', icon: Home },
  { href: '/pages', label: 'Pages', icon: FileText },
  { href: '/calendar', label: 'Calendar', icon: Calendar },
  { href: '/tags', label: 'Tags', icon: Tags },
  { href: '/contacts', label: 'Contacts', icon: Users },
  { href: '/kanban', label: 'Kanban', icon: Kanban },
  { href: '/bookmarks', label: 'All Bookmarks', icon: Link2 },
  { href: '/settings', label: 'Settings', icon: Settings },
]

export function Sidebar({ open, onToggle, pathname, isAdmin = false }: { open: boolean; onToggle: () => void; pathname: string; isAdmin?: boolean }) {
  const [pinned, setPinned] = useState<{ id: string; title: string }[]>([])

  useEffect(() => {
    const load = () =>
      fetch('/api/pages')
        .then((r) => (r.ok ? r.json() : []))
        .then((d: any[]) => setPinned((d ?? []).filter((p) => p.pinned)))
        .catch((e) => console.error('Failed to load pinned pages', e))
    load()
    window.addEventListener('computer:pages-changed', load)
    return () => window.removeEventListener('computer:pages-changed', load)
  }, [])

  const items = isAdmin ? [...navItems, { href: '/admin', label: 'Admin', icon: Shield }] : navItems

  return (
    <aside
      className={cn(
        'flex flex-col border-r bg-[hsl(var(--sidebar))] text-[hsl(var(--sidebar-foreground))] transition-all duration-300 ease-in-out',
        open ? 'w-56' : 'w-16'
      )}
    >
      <div className="flex items-center gap-2 border-b px-3 py-4">
        <Link href="/dashboard" className="flex items-center gap-2">
          <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-primary text-primary-foreground">
            <Link2 className="h-4 w-4" />
          </div>
          {open && <span className="font-display text-lg font-bold tracking-tight">computer</span>}
        </Link>
      </div>
      <nav className="flex-1 space-y-1 p-2">
        {items.map((item) => {
          const isActive = pathname === item.href || pathname?.startsWith(item.href + '/')
          return (
            <Link
              key={item.href}
              href={item.href}
              className={cn(
                'flex items-center gap-3 rounded-lg px-3 py-2.5 text-sm font-medium transition-colors',
                isActive
                  ? 'bg-accent text-accent-foreground'
                  : 'text-muted-foreground hover:bg-accent/50 hover:text-foreground'
              )}
            >
              <item.icon className="h-4 w-4 shrink-0" />
              {open && <span>{item.label}</span>}
            </Link>
          )
        })}
        {open && pinned.length > 0 && (
          <div className="pt-4">
            <p className="px-3 pb-1 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">Pinned pages</p>
            {pinned.map((p) => (
              <Link key={p.id} href={`/pages?id=${p.id}`} className="flex items-center gap-2 rounded-lg px-3 py-1.5 text-sm text-muted-foreground transition-colors hover:bg-accent/50 hover:text-foreground">
                <Pin className="h-3 w-3 shrink-0" />
                <span className="truncate">{p.title}</span>
              </Link>
            ))}
          </div>
        )}
      </nav>
      <div className="border-t p-2">
        <Button variant="ghost" size="icon-sm" onClick={onToggle} className="w-full">
          {open ? <ChevronLeft className="h-4 w-4" /> : <ChevronRight className="h-4 w-4" />}
        </Button>
      </div>
    </aside>
  )
}
