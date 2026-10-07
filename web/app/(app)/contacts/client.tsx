'use client'

import { useState, useEffect, useCallback } from 'react'
import { Button } from '@/components/ui/button'
import { BookmarkCard } from '@/components/bookmark-card'
import { BookmarkDetailSheet } from '@/components/bookmark-detail-sheet'
import { Users, RefreshCw, Mail, Phone } from 'lucide-react'
import { toast } from 'sonner'
import type { Bookmark } from '@/components/app-shell'

interface Contact {
  id: string
  uid: string
  displayName: string
  email: string | null
  phone: string | null
  _count?: { bookmarks: number }
}

export function ContactsClient() {
  const [contacts, setContacts] = useState<Contact[]>([])
  const [selectedContact, setSelectedContact] = useState<string | null>(null)
  const [bookmarks, setBookmarks] = useState<Bookmark[]>([])
  const [selectedBookmark, setSelectedBookmark] = useState<Bookmark | null>(null)
  const [syncing, setSyncing] = useState(false)
  const [loading, setLoading] = useState(true)

  const loadContacts = useCallback(async () => {
    setLoading(true)
    const res = await fetch('/api/contacts')
    if (res.ok) setContacts(await res.json())
    setLoading(false)
  }, [])

  const loadBookmarks = useCallback(async (contactId: string) => {
    const res = await fetch(`/api/bookmarks?contactId=${contactId}`)
    if (res.ok) setBookmarks(await res.json())
  }, [])

  useEffect(() => { loadContacts() }, [loadContacts])
  useEffect(() => { if (selectedContact) loadBookmarks(selectedContact) }, [selectedContact, loadBookmarks])

  const syncContacts = async () => {
    setSyncing(true)
    toast.info('Configure your Nextcloud settings in Settings → Nextcloud to sync contacts')
    setSyncing(false)
  }

  return (
    <div className="flex gap-6 h-[calc(100vh-120px)]">
      <div className="w-72 shrink-0 space-y-4 overflow-y-auto">
        <div className="flex items-center justify-between">
          <h1 className="font-display text-2xl font-bold tracking-tight">Contacts</h1>
          <Button variant="outline" size="sm" onClick={syncContacts} loading={syncing} className="gap-1.5">
            <RefreshCw className="h-3 w-3" /> Sync
          </Button>
        </div>
        {loading ? (
          <div className="space-y-2">{Array.from({ length: 5 }).map((_, i) => <div key={i} className="h-14 rounded-lg bg-muted animate-pulse" />)}</div>
        ) : contacts.length === 0 ? (
          <div className="text-center py-8">
            <Users className="h-10 w-10 text-muted-foreground/50 mx-auto mb-3" />
            <p className="text-sm text-muted-foreground">No contacts yet</p>
            <p className="text-xs text-muted-foreground mt-1">Sync from Nextcloud in Settings</p>
          </div>
        ) : (
          <div className="space-y-1">
            {contacts.map((c: Contact) => (
              <div
                key={c.id}
                onClick={() => setSelectedContact(c.id)}
                className={`flex items-center gap-3 rounded-lg px-3 py-2.5 cursor-pointer transition-colors ${
                  selectedContact === c.id ? 'bg-accent' : 'hover:bg-muted'
                }`}
              >
                <div className="h-9 w-9 rounded-full bg-primary/20 flex items-center justify-center text-sm font-bold text-primary shrink-0">
                  {(c.displayName ?? '?')[0]?.toUpperCase()}
                </div>
                <div className="min-w-0 flex-1">
                  <p className="text-sm font-medium truncate">{c.displayName}</p>
                  {c.email && <p className="text-xs text-muted-foreground truncate flex items-center gap-1"><Mail className="h-3 w-3" />{c.email}</p>}
                </div>
                <span className="text-xs text-muted-foreground">{c._count?.bookmarks ?? 0}</span>
              </div>
            ))}
          </div>
        )}
      </div>

      <div className="flex-1 overflow-y-auto">
        {selectedContact ? (
          <div className="space-y-4">
            <h2 className="font-medium">Bookmarks for {contacts.find(c => c.id === selectedContact)?.displayName}</h2>
            {bookmarks.length === 0 ? (
              <p className="text-sm text-muted-foreground py-8 text-center">No bookmarks linked to this contact</p>
            ) : (
              <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
                {bookmarks.map((b: Bookmark) => <BookmarkCard key={b.id} bookmark={b} compact onClick={() => setSelectedBookmark(b)} />)}
              </div>
            )}
          </div>
        ) : (
          <div className="flex flex-col items-center justify-center h-full text-center">
            <Users className="h-12 w-12 text-muted-foreground/50 mb-4" />
            <p className="text-muted-foreground">Select a contact to view linked bookmarks</p>
          </div>
        )}
      </div>

      <BookmarkDetailSheet bookmark={selectedBookmark} onClose={() => setSelectedBookmark(null)} onUpdate={() => { if (selectedContact) loadBookmarks(selectedContact) }} />
    </div>
  )
}
