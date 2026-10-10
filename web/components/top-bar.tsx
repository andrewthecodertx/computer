'use client'

import { useEffect, useState } from 'react'
import { signOut } from 'next-auth/react'
import { usePathname } from 'next/navigation'
import Link from 'next/link'
import { Search, Bell, Plus, Menu, LogOut, Sun, Moon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from '@/components/ui/breadcrumb'
import { useTheme } from 'next-themes'
import { useMounted } from '@/components/client-only'
import { ACTIVE_PAGE_CHANGED } from '@/lib/pages'

// Notion keeps the top bar minimal: a breadcrumb path on the left, a subtle
// Quick Find pill, and ghost icon buttons on the right.
const SECTION_LABELS: Record<string, string> = {
  '/bookmarks': 'All Bookmarks',
  '/calendar': 'Calendar',
  '/tags': 'Tags',
  '/kanban': 'Kanban',
  '/contacts': 'Contacts',
  '/settings': 'Settings',
  '/admin': 'Admin',
  '/pages': 'Pages',
}

interface TopBarProps {
  user: { id: string; name?: string | null; email?: string | null; image?: string | null }
  alerts: any[]
  onDismissAlert: (id: string) => void
  onAddBookmark: () => void
  onToggleSidebar: () => void
  onQuickFind: () => void
}

export function TopBar({ user, alerts, onDismissAlert, onAddBookmark, onToggleSidebar, onQuickFind }: TopBarProps) {
  const [showAlerts, setShowAlerts] = useState(false)
  const pathname = usePathname()
  const mounted = useMounted()
  const { theme, setTheme } = useTheme()
  const [activePageTitle, setActivePageTitle] = useState<string | null>(null)

  // The pages workspace announces the open page so breadcrumbs can show it.
  // The title only renders on /pages, so route changes need no reset effect.
  useEffect(() => {
    const handler = (e: Event) => setActivePageTitle((e as CustomEvent<string | null>).detail)
    window.addEventListener(ACTIVE_PAGE_CHANGED, handler)
    return () => window.removeEventListener(ACTIVE_PAGE_CHANGED, handler)
  }, [])
  const pageTitle = pathname === '/pages' ? activePageTitle : null

  useEffect(() => {
    const handler = (e: MouseEvent) => {
      const el = document.getElementById('topbar-alerts')
      if (el && !el.contains(e.target as Node)) setShowAlerts(false)
    }
    document.addEventListener('mousedown', handler)
    return () => document.removeEventListener('mousedown', handler)
  }, [])

  const section = SECTION_LABELS[pathname] ?? 'computer'
  const isMac = mounted && typeof navigator !== 'undefined' && /mac|iphone|ipad/i.test(navigator.userAgent)
  const quickFindHint = !mounted ? '⌘K' : isMac ? '⌘K' : 'Ctrl K'

  return (
    <header className="flex items-center gap-2 border-b bg-card px-3 py-2">
      <Button variant="ghost" size="icon-sm" onClick={onToggleSidebar} className="lg:hidden">
        <Menu className="h-4 w-4" />
      </Button>

      <Breadcrumb className="min-w-0 flex-1">
        <BreadcrumbList className="flex-nowrap">
          <BreadcrumbItem>
            <BreadcrumbLink asChild className="font-medium">
              <Link href="/pages">computer</Link>
            </BreadcrumbLink>
          </BreadcrumbItem>
          <BreadcrumbSeparator />
          <BreadcrumbItem className="min-w-0">
            {pageTitle ? (
              <>
                <BreadcrumbLink asChild>
                  <Link href="/pages">Pages</Link>
                </BreadcrumbLink>
                <BreadcrumbSeparator />
                <BreadcrumbPage className="truncate font-normal text-foreground">{pageTitle}</BreadcrumbPage>
              </>
            ) : (
              <BreadcrumbPage className="truncate font-normal text-foreground">{section}</BreadcrumbPage>
            )}
          </BreadcrumbItem>
        </BreadcrumbList>
      </Breadcrumb>

      <button
        onClick={onQuickFind}
        aria-label="Quick find"
        className="flex shrink-0 items-center gap-2 rounded-md border border-border/60 bg-muted/40 px-2.5 py-1.5 text-sm text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
      >
        <Search className="h-3.5 w-3.5" />
        <span className="hidden md:inline">Quick find</span>
        <kbd className="pointer-events-none hidden select-none rounded border bg-background px-1.5 text-[10px] font-medium md:inline-flex">{quickFindHint}</kbd>
      </button>

      <Button variant="ghost" size="sm" onClick={onAddBookmark} className="shrink-0 gap-1.5">
        <Plus className="h-4 w-4" />
        <span className="hidden sm:inline">New</span>
      </Button>

      <Button variant="ghost" size="icon-sm" className="shrink-0" onClick={() => setTheme(theme === 'dark' ? 'light' : 'dark')}>
        <Sun className="h-4 w-4 rotate-0 scale-100 transition-all dark:-rotate-90 dark:scale-0" />
        <Moon className="absolute h-4 w-4 rotate-90 scale-0 transition-all dark:rotate-0 dark:scale-100" />
      </Button>

      <div className="relative shrink-0" id="topbar-alerts">
        <Button variant="ghost" size="icon-sm" onClick={() => setShowAlerts(!showAlerts)}>
          <Bell className="h-4 w-4" />
          {(alerts?.length ?? 0) > 0 && (
            <span className="absolute -top-0.5 -right-0.5 flex h-4 w-4 items-center justify-center rounded-full bg-destructive text-[10px] text-destructive-foreground">
              {alerts?.length ?? 0}
            </span>
          )}
        </Button>
        {showAlerts && (
          <div className="absolute right-0 top-full mt-2 w-80 rounded-lg border bg-card p-2 shadow-lg z-50">
            <p className="px-2 py-1 text-xs font-medium text-muted-foreground">Alerts</p>
            {(alerts?.length ?? 0) === 0 && (
              <p className="px-2 py-4 text-sm text-center text-muted-foreground">No pending alerts</p>
            )}
            {(alerts ?? []).map((a: any) => (
              <div key={a?.id} className="flex items-center gap-2 rounded-md px-2 py-1.5 hover:bg-muted">
                <div className="flex-1 min-w-0">
                  <p className="text-sm font-medium truncate">{a?.title ?? 'Untitled'}</p>
                  <p className="text-xs text-muted-foreground">Alert due</p>
                </div>
                <Button variant="ghost" size="xs" onClick={() => onDismissAlert(a?.id)}>Dismiss</Button>
              </div>
            ))}
          </div>
        )}
      </div>

      <div className="flex shrink-0 items-center gap-2 pl-1">
        <div className="flex h-7 w-7 items-center justify-center rounded-full bg-primary text-primary-foreground text-xs font-bold">
          {(user?.name ?? user?.email ?? '?')[0]?.toUpperCase()}
        </div>
        <span className="hidden sm:inline text-sm font-medium">{user?.name ?? user?.email ?? 'User'}</span>
        <Button variant="ghost" size="icon-sm" onClick={() => signOut({ redirectTo: '/login' })} aria-label="Sign out">
          <LogOut className="h-4 w-4" />
        </Button>
      </div>
    </header>
  )
}
