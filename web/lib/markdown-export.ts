/** Builds downloadable Markdown documents for pages and bookmarks. */

export function slugify(s: string) {
  return (s || 'untitled').toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '').slice(0, 60) || 'untitled'
}

type BmLite = { url: string; title: string | null; ogDescription?: string | null; description?: string | null }

export function pageToMarkdown(page: { title: string; content: string; updatedAt: Date; bookmarks: { bookmark: BmLite }[] }) {
  const lines = [`# ${page.title}`, '', page.content.trim()]
  if (page.bookmarks.length) {
    lines.push('', '## Links', '')
    for (const { bookmark: b } of page.bookmarks) {
      const desc = b.description ?? b.ogDescription
      lines.push(`- [${(b.title ?? b.url).replace(/[[\]]/g, '')}](${b.url})${desc ? ` — ${desc.replace(/\s+/g, ' ').slice(0, 160)}` : ''}`)
    }
  }
  lines.push('', `<!-- exported from computer ${page.updatedAt.toISOString()} -->`, '')
  return lines.join('\n')
}

export function bookmarkToMarkdown(b: BmLite & { notes: string | null; dueDate: Date | null; tags: { tag: { name: string } }[] }) {
  const meta = [`- URL: <${b.url}>`]
  if (b.dueDate) meta.push(`- Date: ${b.dueDate.toISOString().slice(0, 10)}`)
  if (b.tags.length) meta.push(`- Tags: ${b.tags.map((t) => t.tag.name).join(', ')}`)
  return [`# ${b.title ?? b.url}`, '', ...meta, '', (b.notes ?? '').trim(), ''].join('\n')
}

export function markdownResponse(body: string, filename: string) {
  return new Response(body, {
    headers: {
      'Content-Type': 'text/markdown; charset=utf-8',
      'Content-Disposition': `attachment; filename="${filename}.md"`,
    },
  })
}
