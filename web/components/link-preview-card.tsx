'use client'

import { useLinkPreview } from '@/lib/link-preview'

const hostOf = (u: string) => {
  try { return new URL(u).hostname.replace(/^www\./, '') } catch { return u }
}

// Notion-style inline preview for a link that sits alone on its own line in
// page Markdown. While the preview loads (or if nothing resolves) the card
// still shows the link text/host, so content never disappears or shifts.
export function LinkPreviewCard({ url, label }: { url: string; label?: string }) {
  const preview = useLinkPreview(url)
  const title = preview?.ogTitle || preview?.title || label || url
  const description = preview?.ogDescription || preview?.description || null
  return (
    <a
      href={url}
      target="_blank"
      rel="noopener noreferrer"
      className="my-2 flex gap-3 rounded-lg bg-card p-3 no-underline shadow-sm ring-1 ring-border/50 transition-shadow hover:shadow-md"
    >
      {preview?.ogImage ? (
        // eslint-disable-next-line @next/next/no-img-element
        <img
          src={preview.ogImage}
          alt=""
          className="h-16 w-24 shrink-0 rounded bg-muted object-cover"
          onError={(e) => { (e.target as HTMLImageElement).style.display = 'none' }}
        />
      ) : null}
      <span className="min-w-0 flex-1">
        <span className="flex items-center gap-1.5">
          {preview?.favicon ? (
            // eslint-disable-next-line @next/next/no-img-element
            <img
              src={preview.favicon}
              alt=""
              className="h-3.5 w-3.5 shrink-0"
              onError={(e) => { (e.target as HTMLImageElement).style.display = 'none' }}
            />
          ) : null}
          <span className="truncate text-sm font-medium text-foreground">{title}</span>
        </span>
        {description ? (
          <span className="mt-0.5 line-clamp-2 block text-xs text-muted-foreground">{description}</span>
        ) : null}
        <span className="mt-1 block text-[11px] text-muted-foreground/80">{hostOf(url)}</span>
      </span>
    </a>
  )
}
