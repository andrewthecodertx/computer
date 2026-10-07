'use client'

import { useState, useRef, useEffect } from 'react'
import { signOut } from 'next-auth/react'
import { Search, Bell, Plus, Menu, LogOut, Sun, Moon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import { useTheme } from 'next-themes'
import { cn } from '@/lib/utils'

interface TopBarProps {
  user: { id: string; name?: string | null; email?: string | null; image?: string | null }
  onSearch: (q: string) => void
  searchQuery: string
  alerts: any[]
  onDismissAlert: (id: string) => void
  onAddBookmark: () => void
  onToggleSidebar: () => void
}

export function TopBar({ user, onSearch, searchQuery, alerts, onDismissAlert, onAddBookmark, onToggleSidebar }: TopBarProps) {
  const [showAlerts, setShowAlerts] = useState(false)
  const alertRef = useRef<HTMLDivElement>(null)
  const { theme, setTheme } = useTheme()

  useEffect(() => {
    const handler = (e: MouseEvent) => {
      if (alertRef.current && !alertRef.current.contains(e.target as Node)) setShowAlerts(false)
    }
    document.addEventListener('mousedown', handler)
    return () => document.removeEventListener('mousedown', handler)
  }, [])

  return (
    <header className="flex items-center gap-4 border-b bg-card px-4 py-3">
      <Button variant="ghost" size="icon-sm" onClick={onToggleSidebar} className="lg:hidden">
        <Menu className="h-4 w-4" />
      </Button>

      <div className="relative flex-1 max-w-md">
        <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
        <Input
          placeholder="Search bookmarks, tags, notes..."
          value={searchQuery}
          onChange={(e: React.ChangeEvent<HTMLInputElement>) => onSearch(e.target.value)}
          className="pl-9 bg-muted/50"
        />
      </div>

      <Button onClick={onAddBookmark} size="sm" className="gap-1.5">
        <Plus className="h-4 w-4" />
        <span className="hidden sm:inline">Add Bookmark</span>
      </Button>

      <Button variant="ghost" size="icon-sm" onClick={() => setTheme(theme === 'dark' ? 'light' : 'dark')}>
        <Sun className="h-4 w-4 rotate-0 scale-100 transition-all dark:-rotate-90 dark:scale-0" />
        <Moon className="absolute h-4 w-4 rotate-90 scale-0 transition-all dark:rotate-0 dark:scale-100" />
      </Button>

      <div className="relative" ref={alertRef}>
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

      <div className="flex items-center gap-2">
        <div className="flex h-8 w-8 items-center justify-center rounded-full bg-primary text-primary-foreground text-xs font-bold">
          {(user?.name ?? user?.email ?? '?')[0]?.toUpperCase()}
        </div>
        <span className="hidden sm:inline text-sm font-medium">{user?.name ?? user?.email ?? 'User'}</span>
        <Button variant="ghost" size="icon-sm" onClick={() => signOut({ redirectTo: '/login' })}>
          <LogOut className="h-4 w-4" />
        </Button>
      </div>
    </header>
  )
}
