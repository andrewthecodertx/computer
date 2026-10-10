'use client'

// Notion-style "Quick Find": a Cmd/Ctrl+K palette to jump to any page,
// bookmark, tag, or view, and to run the few workspace commands
// (new page, add bookmark, toggle theme). Search is read-only; mutations
// stay on their owning screens.

import { useCallback, useEffect, useState } from 'react'
import { useRouter } from 'next/navigation'
import { useTheme } from 'next-themes'
import { toast } from 'sonner'
import {
  Calendar,
  ExternalLink,
  FilePlus2,
  FileText,
  Kanban,
  Link2,
  Moon,
  Plus,
  Search,
  Settings,
  Shield,
  Sun,
  Tags,
  Users,
} from 'lucide-react'
import {
  CommandDialog,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandSeparator,
} from '@/components/ui/command'
import { nextUntitledTitle, notifyPagesChanged, type PageSummary } from '@/lib/pages'
import type { TagWithCount } from '@/components/app-shell'

type BookmarkHit = { id: string; url: string; title: string | null; favicon: string | null; ogTitle: string | null }

const views = [
  { href: '/bookmarks', label: 'All Bookmarks', icon: Link2 },
  { href: '/calendar', label: 'Calendar', icon: Calendar },
  { href: '/tags', label: 'Tags', icon: Tags },
  { href: '/kanban', label: 'Kanban', icon: Kanban },
  { href: '/contacts', label: 'Contacts', icon: Users },
  { href: '/settings', label: 'Settings', icon: Settings },
]

const host = (u: string) => { try { return new URL(u).hostname.replace(/^www\./, '') } catch { return u } }

interface QuickFindProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  isAdmin?: boolean
  onAddBookmark?: () => void
}

export function QuickFind({ open, onOpenChange, isAdmin = false, onAddBookmark }: QuickFindProps) {
  const router = useRouter()
  const { theme, setTheme } = useTheme()
  const [query, setQuery] = useState('')
  const [pages, setPages] = useState<PageSummary[]>([])
  const [tags, setTags] = useState<TagWithCount[]>([])
  const [bookmarks, setBookmarks] = useState<BookmarkHit[]>([])

  // Notion binds both Cmd/Ctrl+K and Cmd/Ctrl+P to Quick Find.
  useEffect(() => {
    const down = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && (e.key === 'k' || e.key === 'K')) {
        e.preventDefault()
        onOpenChange(!open)
      }
    }
    document.addEventListener('keydown', down)
    return () => document.removeEventListener('keydown', down)
  }, [open, onOpenChange])

  // Load the workspace index each time the palette opens; results stay fresh
  // without polling while it is closed.
  useEffect(() => {
    if (!open) return
    fetch('/api/pages').then((r) => (r.ok ? r.json() : [])).then((d: PageSummary[]) => setPages(d ?? [])).catch(() => {})
    fetch('/api/tags').then((r) => (r.ok ? r.json() : [])).then((d: TagWithCount[]) => setTags(d ?? [])).catch(() => {})
  }, [open])

  // Bookmark search hits the API (debounced) because the server matches
  // notes and URLs too, not just titles.
  useEffect(() => {
    if (!open) return
    const q = query.trim()
    const t = setTimeout(async () => {
      if (q.length < 2) { setBookmarks([]); return }
      try {
        const params = new URLSearchParams({ search: q, limit: '6' })
        const res = await fetch(`/api/bookmarks?${params}`)
        if (res.ok) setBookmarks(((await res.json()) as BookmarkHit[]).slice(0, 6))
      } catch { /* transient; retried on next keystroke */ }
    }, q.length < 2 ? 0 : 200)
    return () => clearTimeout(t)
  }, [query, open])

  const go = useCallback((href: string) => {
    onOpenChange(false)
    setQuery('')
    router.push(href)
  }, [onOpenChange, router])

  const createPage = useCallback(async () => {
    onOpenChange(false)
    setQuery('')
    const title = nextUntitledTitle(pages.map((p) => p.title))
    try {
      const res = await fetch('/api/pages', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ title }) })
      if (!res.ok) { toast.error('Could not create page'); return }
      const p = await res.json()
      notifyPagesChanged()
      router.push(`/pages?id=${p.id}`)
    } catch { toast.error('Could not create page') }
  }, [onOpenChange, pages, router])

  return (
    <CommandDialog open={open} onOpenChange={(next) => { onOpenChange(next); if (!next) setQuery('') }}>
      <CommandInput
        placeholder="Search pages, bookmarks, tags, and commands…"
        value={query}
        onValueChange={(v) => { setQuery(v); }}
      />
      <CommandList>
        <CommandEmpty>No results found.</CommandEmpty>
        <CommandGroup heading="Commands">
          <CommandItem value="new page create" onSelect={createPage}>
            <FilePlus2 className="mr-2 h-4 w-4 text-muted-foreground" />
            New page
          </CommandItem>
          {onAddBookmark && (
            <CommandItem value="add bookmark new link url" onSelect={() => { onOpenChange(false); setQuery(''); onAddBookmark() }}>
              <Plus className="mr-2 h-4 w-4 text-muted-foreground" />
              Add bookmark
            </CommandItem>
          )}
          <CommandItem value="toggle theme dark light mode appearance" onSelect={() => { onOpenChange(false); setTheme(theme === 'dark' ? 'light' : 'dark') }}>
            {theme === 'dark' ? <Sun className="mr-2 h-4 w-4 text-muted-foreground" /> : <Moon className="mr-2 h-4 w-4 text-muted-foreground" />}
            Toggle dark mode
          </CommandItem>
        </CommandGroup>
        <CommandSeparator />
        <CommandGroup heading="Views">
          {views.map((v) => (
            <CommandItem key={v.href} value={`view go to ${v.label}`} onSelect={() => go(v.href)}>
              <v.icon className="mr-2 h-4 w-4 text-muted-foreground" />
              {v.label}
            </CommandItem>
          ))}
          {isAdmin && (
            <CommandItem value="view go to admin administration" onSelect={() => go('/admin')}>
              <Shield className="mr-2 h-4 w-4 text-muted-foreground" />
              Admin
            </CommandItem>
          )}
        </CommandGroup>
        {pages.length > 0 && (
          <>
            <CommandSeparator />
            <CommandGroup heading="Pages">
              {pages.map((p) => (
                <CommandItem key={p.id} value={`page ${p.title}`} onSelect={() => go(`/pages?id=${p.id}`)}>
                  <FileText className="mr-2 h-4 w-4 text-muted-foreground" />
                  <span className="truncate">{p.title}</span>
                </CommandItem>
              ))}
            </CommandGroup>
          </>
        )}
        {tags.length > 0 && (
          <>
            <CommandSeparator />
            <CommandGroup heading="Tags">
              {tags.map((t) => (
                <CommandItem key={t.id} value={`tag ${t.name}`} onSelect={() => go(`/tags?tag=${t.id}`)}>
                  <Tags className="mr-2 h-4 w-4" style={{ color: t.color }} />
                  <span className="truncate">{t.name}</span>
                  <span className="ml-auto text-xs text-muted-foreground">{t._count?.bookmarks ?? 0}</span>
                </CommandItem>
              ))}
            </CommandGroup>
          </>
        )}
        {query.trim().length >= 2 && (
          <>
            <CommandSeparator />
            <CommandGroup heading="Search">
              <CommandItem value={`zz-search-bookmarks-for-${query}`} onSelect={() => go(`/bookmarks?search=${encodeURIComponent(query.trim())}`)}>
                <Search className="mr-2 h-4 w-4 text-muted-foreground" />
                Search bookmarks for “{query.trim()}”
              </CommandItem>
            </CommandGroup>
          </>
        )}
        {bookmarks.length > 0 && (
          <>
            <CommandSeparator />
            <CommandGroup heading="Bookmarks">
              {bookmarks.map((b) => (
                <CommandItem key={b.id} value={`bookmark ${b.title ?? ''} ${b.ogTitle ?? ''} ${b.url}`} onSelect={() => go(`/bookmarks?open=${b.id}`)}>
                  {b.favicon ? (
                    // eslint-disable-next-line @next/next/no-img-element
                    <img src={b.favicon} alt="" className="mr-2 h-4 w-4 rounded" onError={(e) => { (e.target as HTMLImageElement).style.visibility = 'hidden' }} />
                  ) : (
                    <Link2 className="mr-2 h-4 w-4 text-muted-foreground" />
                  )}
                  <span className="min-w-0 flex-1">
                    <span className="block truncate">{b.title || b.ogTitle || b.url}</span>
                    <span className="block truncate text-xs text-muted-foreground">{host(b.url)}</span>
                  </span>
                  <ExternalLink className="ml-2 h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                </CommandItem>
              ))}
            </CommandGroup>
          </>
        )}
      </CommandList>
    </CommandDialog>
  )
}
