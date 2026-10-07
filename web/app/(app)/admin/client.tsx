'use client'

import { useEffect, useState } from 'react'
import { toast } from 'sonner'
import { Eye, Shield } from 'lucide-react'
import { Button } from '@/components/ui/button'

type U = { id: string; name: string | null; email: string; role: string; _count?: { bookmarks: number; pages: number; tags: number } }

export function AdminClient({ currentUserId }: { currentUserId: string }) {
  const [users, setUsers] = useState<U[]>([])

  const load = () =>
    fetch('/api/admin/users')
      .then((r) => (r.ok ? r.json() : []))
      .then((d) => setUsers(Array.isArray(d) ? d : d?.users ?? []))
      .catch((e) => console.error('Failed to load users', e))

  useEffect(() => { load() }, [])

  const viewAs = async (userId: string) => {
    const res = await fetch('/api/admin/view-as', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ userId }) })
    if (!res.ok) return toast.error('Could not switch account')
    window.location.href = '/dashboard'
  }

  const toggleRole = async (u: U) => {
    const res = await fetch('/api/admin/users', { method: 'PATCH', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ userId: u.id, role: u.role === 'ADMIN' ? 'USER' : 'ADMIN' }) })
    if (!res.ok) return toast.error((await res.json().catch(() => null))?.error ?? 'Could not change role')
    load()
  }

  return (
    <div className="mx-auto max-w-4xl">
      <h1 className="font-display text-2xl font-bold">Admin</h1>
      <p className="mb-6 text-sm text-muted-foreground">All accounts in the group. “View as” opens an account with a visible banner; exit any time.</p>
      <div className="divide-y divide-border/60 rounded-xl bg-card shadow-sm ring-1 ring-border/50">
        {users.map((u) => (
          <div key={u.id} className="flex flex-wrap items-center gap-3 p-4">
            <div className="min-w-0 flex-1">
              <p className="truncate font-medium">{u.name || u.email} {u.role === 'ADMIN' && <Shield className="ml-1 inline h-3.5 w-3.5 text-primary" />}</p>
              <p className="truncate text-xs text-muted-foreground">{u.email} · {u._count?.bookmarks ?? 0} bookmarks · {u._count?.pages ?? 0} pages</p>
            </div>
            <Button size="sm" variant="outline" disabled={u.id === currentUserId} onClick={() => toggleRole(u)}>{u.role === 'ADMIN' ? 'Make user' : 'Make admin'}</Button>
            <Button size="sm" disabled={u.id === currentUserId} onClick={() => viewAs(u.id)}><Eye className="mr-1 h-4 w-4" />View as</Button>
          </div>
        ))}
        {users.length === 0 && <p className="p-6 text-center text-sm text-muted-foreground">No users found.</p>}
      </div>
    </div>
  )
}
