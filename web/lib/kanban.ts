import { prisma } from '@/lib/prisma'

export const DEFAULT_COLUMNS = [
  { key: 'INBOX', label: 'Inbox', color: '#64748b' },
  { key: 'TODO', label: 'To Do', color: '#3b82f6' },
  { key: 'IN_PROGRESS', label: 'In Progress', color: '#f59e0b' },
  { key: 'DONE', label: 'Done', color: '#22c55e' },
]

/** Returns the user's columns, creating the default set on first use. */
export async function getColumns(userId: string) {
  let cols = await prisma.kanbanColumn.findMany({ where: { userId }, orderBy: { position: 'asc' } })
  if (cols.length === 0) {
    await prisma.kanbanColumn.createMany({
      data: DEFAULT_COLUMNS.map((c, i) => ({ ...c, position: i, userId })),
    })
    cols = await prisma.kanbanColumn.findMany({ where: { userId }, orderBy: { position: 'asc' } })
  }
  return cols
}

/** Column a bookmark belongs to: explicit column, else legacy status mapped by key, else first column. */
export function resolveColumnId(
  b: { kanbanColumnId: string | null; kanbanStatus: string },
  cols: { id: string; key: string | null }[],
): string | null {
  if (b.kanbanColumnId && cols.some((c) => c.id === b.kanbanColumnId)) return b.kanbanColumnId
  return cols.find((c) => c.key === b.kanbanStatus)?.id ?? cols[0]?.id ?? null
}
