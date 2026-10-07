'use client'

import { ExternalLink, Calendar as CalendarIcon, Clock } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'
import type { Bookmark } from '@/components/app-shell'

interface BookmarkCardProps {
  bookmark: Bookmark
  onClick?: () => void
  compact?: boolean
}

export function BookmarkCard({ bookmark, onClick, compact }: BookmarkCardProps) {
  const domain = (() => { try { return new URL(bookmark?.url ?? '').hostname } catch { return '' } })()
  const tags = bookmark?.tags ?? []

  return (
    <div
      onClick={onClick}
      className={cn(
        'group relative flex cursor-pointer flex-col rounded-lg border bg-card transition-all duration-normal hover:shadow-md hover:-translate-y-0.5 hover:border-primary/20 overflow-hidden',
        compact ? 'p-3' : 'p-0'
      )}
    >
      {!compact && bookmark?.ogImage && (
        <div className="relative w-full aspect-video bg-muted">
          <img
            src={bookmark.ogImage}
            alt={bookmark?.title ?? 'Bookmark'}
            className="object-cover w-full h-full"
            onError={(e: any) => { e.target.style.display = 'none' }}
          />
        </div>
      )}
      <div className={cn('flex flex-col gap-2', compact ? '' : 'p-4')}>
        <div className="flex items-start gap-2">
          {bookmark?.favicon && (
            <img
              src={bookmark.favicon}
              alt=""
              className="h-4 w-4 mt-0.5 rounded shrink-0"
              onError={(e: any) => { e.target.style.display = 'none' }}
            />
          )}
          <div className="min-w-0 flex-1">
            <h3 className="text-sm font-medium leading-tight truncate">
              {bookmark?.title ?? bookmark?.url ?? 'Untitled'}
            </h3>
            <p className="text-xs text-muted-foreground truncate mt-0.5">{domain}</p>
          </div>
          <a
            href={bookmark?.url ?? '#'}
            target="_blank"
            rel="noopener noreferrer"
            onClick={(e: React.MouseEvent) => e.stopPropagation()}
            className="shrink-0 opacity-0 group-hover:opacity-100 transition-opacity"
          >
            <ExternalLink className="h-4 w-4 text-muted-foreground hover:text-primary" />
          </a>
        </div>
        {!compact && bookmark?.ogDescription && (
          <p className="text-xs text-muted-foreground line-clamp-2">{bookmark.ogDescription}</p>
        )}
        <div className="flex flex-wrap items-center gap-1.5">
          {tags.slice(0, 3).map((bt: any) => (
            <Badge
              key={bt?.tag?.id}
              variant="secondary"
              className="text-[10px] px-1.5 py-0"
              style={{ backgroundColor: `${bt?.tag?.color ?? '#6366f1'}20`, color: bt?.tag?.color ?? '#6366f1' }}
            >
              {bt?.tag?.name ?? ''}
            </Badge>
          ))}
          {tags.length > 3 && (
            <span className="text-[10px] text-muted-foreground">+{tags.length - 3}</span>
          )}
        </div>
        {(bookmark?.dueDate || bookmark?.alertAt) && (
          <div className="flex items-center gap-2 text-xs text-muted-foreground">
            {bookmark?.dueDate && (
              <span className="flex items-center gap-1">
                <CalendarIcon className="h-3 w-3" />
                {new Date(bookmark.dueDate).toLocaleDateString('en-US', { month: 'short', day: 'numeric' })}
              </span>
            )}
            {bookmark?.alertAt && (
              <span className="flex items-center gap-1">
                <Clock className="h-3 w-3" />
                Alert set
              </span>
            )}
          </div>
        )}
      </div>
    </div>
  )
}
