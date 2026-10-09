'use client'

import { useState, useEffect, useCallback, useRef } from 'react'
import { useSession } from 'next-auth/react'
import { usePathname, useRouter } from 'next/navigation'
import { Sidebar } from '@/components/sidebar'
import { TopBar } from '@/components/top-bar'
import { AddBookmarkDialog } from '@/components/add-bookmark-dialog'

export type Bookmark = {
  id: string
  url: string
  title: string | null
  description: string | null
  favicon: string | null
  ogImage: string | null
  ogTitle: string | null
  ogDescription: string | null
  notes: string | null
  dueDate: string | null
  alertAt: string | null
  alertSent: boolean
  kanbanStatus: string
  kanbanColumnId?: string | null
  imapWatchEnabled: boolean
  imapQuery: string | null
  isPublic: boolean
  ownerId: string
  contactId: string | null
  contact: any | null
  tags: { tag: { id: string; name: string; color: string } }[]
  sharedWith: any[]
  owner?: any
  createdAt: string
  updatedAt: string
}

export type TagWithCount = {
  id: string
  name: string
  color: string
  ownerId: string
  _count: { bookmarks: number }
  sharedWith?: any[]
}

export function AppShell({ children }: { children: React.ReactNode }) {
  const { data: session } = useSession()
  const router = useRouter()
  const pathname = usePathname()
  const [sidebarOpen, setSidebarOpen] = useState(true)
  const [addDialogOpen, setAddDialogOpen] = useState(false)
  const [searchQuery, setSearchQuery] = useState('')
  const [alerts, setAlerts] = useState<any[]>([])
  const [me, setMe] = useState<{ isAdmin: boolean; viewingAs: { name: string | null; email: string } | null } | null>(null)

  useEffect(() => {
    if (!session?.user) return
    fetch('/api/me')
      .then((r) => (r.ok ? r.json() : null))
      .then(setMe)
      .catch((e) => console.error('Failed to load account info', e))
  }, [session?.user])

  const exitViewAs = useCallback(async () => {
    try {
      await fetch('/api/admin/view-as', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ userId: null }) })
    } catch { /* navigating to /admin surfaces any persistent failure */ }
    window.location.href = '/admin'
  }, [])

  // Poll alerts. The API returns every un-dismissed alert on each poll, so
  // track already-notified IDs per tab to avoid re-firing every 5 minutes.
  const notifiedAlerts = useRef<Set<string>>(new Set())
  useEffect(() => {
    if (!session?.user) return
    const check = async () => {
      try {
        const res = await fetch('/api/alerts/check')
        if (res.ok) {
          const data = await res.json()
          setAlerts(data ?? [])
          if (data?.length && typeof Notification !== 'undefined' && Notification.permission === 'granted') {
            for (const a of data) {
              const id = String(a?.id ?? a?.bookmarkId ?? '')
              if (!id || notifiedAlerts.current.has(id)) continue
              notifiedAlerts.current.add(id)
              new Notification('computer alert', { body: `${a?.title ?? 'Bookmark'} is due!`, icon: '/favicon.svg' })
            }
          }
        }
      } catch { /* ignore */ }
    }
    check()
    const interval = setInterval(check, 5 * 60 * 1000)
    return () => clearInterval(interval)
  }, [session?.user])

  // Request notification permission
  useEffect(() => {
    if (typeof Notification !== 'undefined' && Notification.permission === 'default') {
      Notification.requestPermission()
    }
  }, [])

  const refreshPage = useCallback(() => {
    window.dispatchEvent(new Event('computer:bookmarks-changed'))
    router.refresh()
  }, [router])

  const onSearch = useCallback((q: string) => {
    setSearchQuery(q)
    window.dispatchEvent(new CustomEvent('computer:search', { detail: q }))
    if (pathname !== '/bookmarks') {
          router.push(`/bookmarks?search=${encodeURIComponent(q)}`)
    }
  }, [pathname, router])

  const dismissAlert = useCallback(async (bookmarkId: string) => {
    try {
      const res = await fetch(`/api/alerts/${bookmarkId}`, { method: 'DELETE' })
      if (!res.ok) return
    } catch { return }
    setAlerts((prev: any[]) => (prev ?? []).filter((a: any) => a?.id !== bookmarkId))
  }, [])

  if (!session?.user) return null

  return (
    <div className="flex h-screen overflow-hidden bg-background">
      <Sidebar open={sidebarOpen} onToggle={() => setSidebarOpen(!sidebarOpen)} pathname={pathname} isAdmin={!!me?.isAdmin} />
      <div className="flex flex-1 flex-col overflow-hidden">
        {me?.viewingAs && (
          <div className="flex items-center justify-center gap-3 bg-amber-500 px-4 py-1.5 text-sm font-medium text-amber-950">
            <span>Admin view — viewing as {me.viewingAs.name || me.viewingAs.email}</span>
            <button onClick={exitViewAs} className="rounded bg-amber-950/15 px-2 py-0.5 text-xs font-semibold hover:bg-amber-950/25">Exit</button>
          </div>
        )}
        <TopBar
          user={session.user as any}
          onSearch={onSearch}
          searchQuery={searchQuery}
          alerts={alerts}
          onDismissAlert={dismissAlert}
          onAddBookmark={() => setAddDialogOpen(true)}
          onToggleSidebar={() => setSidebarOpen(!sidebarOpen)}
        />
        <main className="flex-1 overflow-auto p-6">
          {children}
        </main>
      </div>
      <AddBookmarkDialog
        open={addDialogOpen}
        onClose={() => setAddDialogOpen(false)}
        onCreated={() => { setAddDialogOpen(false); refreshPage() }}
      />
    </div>
  )
}
