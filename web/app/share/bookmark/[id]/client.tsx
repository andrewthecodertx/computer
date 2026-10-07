'use client'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { ExternalLink, Link2, Calendar, User } from 'lucide-react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'

export function PublicBookmarkView({ bookmark }: { bookmark: any }) {
  const domain = (() => { try { return new URL(bookmark?.url ?? '').hostname } catch { return '' } })()

  return (
    <div className="min-h-screen bg-background">
      <header className="border-b">
        <div className="max-w-3xl mx-auto px-4 py-4 flex items-center gap-2">
          <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-primary text-primary-foreground">
            <Link2 className="h-4 w-4" />
          </div>
          <span className="font-display font-bold">computer</span>
          <span className="text-muted-foreground text-sm">— Shared Bookmark</span>
        </div>
      </header>

      <main className="max-w-3xl mx-auto px-4 py-8 space-y-6">
        <a
          href={bookmark?.url ?? '#'}
          target="_blank"
          rel="noopener noreferrer"
          className="flex items-center gap-3 rounded-lg bg-primary/10 px-6 py-4 text-lg font-medium text-primary hover:bg-primary/20 transition-colors"
        >
          <ExternalLink className="h-5 w-5 shrink-0" />
          <span className="truncate">{bookmark?.url ?? ''}</span>
        </a>

        <div>
          <h1 className="font-display text-3xl font-bold tracking-tight">{bookmark?.title ?? 'Untitled'}</h1>
          {bookmark?.ogDescription && <p className="text-muted-foreground mt-2">{bookmark.ogDescription}</p>}
        </div>

        {bookmark?.ogImage && (
          <div className="rounded-lg border overflow-hidden">
            <img src={bookmark.ogImage} alt={bookmark?.title ?? ''} className="w-full aspect-video object-cover" />
          </div>
        )}

        <div className="flex flex-wrap gap-2">
          {(bookmark?.tags ?? []).map((bt: any) => (
            <Badge key={bt?.tag?.id} variant="secondary" style={{ backgroundColor: `${bt?.tag?.color ?? '#6366f1'}20`, color: bt?.tag?.color ?? '#6366f1' }}>
              {bt?.tag?.name}
            </Badge>
          ))}
        </div>

        {bookmark?.notes && (
          <div className="prose prose-sm dark:prose-invert max-w-none rounded-lg border p-6">
            <ReactMarkdown remarkPlugins={[remarkGfm]}>{bookmark.notes}</ReactMarkdown>
          </div>
        )}

        <div className="flex items-center gap-4 text-sm text-muted-foreground">
          {bookmark?.owner?.name && (
            <span className="flex items-center gap-1"><User className="h-4 w-4" /> Shared by {bookmark.owner.name}</span>
          )}
          {bookmark?.dueDate && (
            <span className="flex items-center gap-1"><Calendar className="h-4 w-4" /> {new Date(bookmark.dueDate).toLocaleDateString('en-US', { month: 'long', day: 'numeric', year: 'numeric' })}</span>
          )}
        </div>
      </main>
    </div>
  )
}
