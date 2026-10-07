import { prisma } from '@/lib/prisma'

/**
 * Generic signal-source registry. Each source type declares its config fields and a `check`
 * that returns new signals for one watcher. Adding a source = adding an entry here.
 */
export type SignalInput = { externalId: string; title: string; summary?: string; url?: string; occurredAt?: Date }

export type ConfigField = { key: string; label: string; placeholder?: string }

export type SignalSourceType = {
  type: string
  label: string
  description: string
  stub: boolean
  configFields: ConfigField[]
  check: (ctx: { userId: string; config: Record<string, any> }) => Promise<{ status: string; signals: SignalInput[] }>
}

const emailSource: SignalSourceType = {
  type: 'email',
  label: 'Email (IMAP)',
  description: 'Watches your IMAP mailbox (Settings → IMAP) for emails matching a sender and/or subject.',
  stub: true,
  configFields: [
    { key: 'from', label: 'From contains', placeholder: 'e.g. billing@example.org' },
    { key: 'subject', label: 'Subject contains', placeholder: 'e.g. invoice' },
  ],
  async check({ userId }) {
    const imap = await prisma.imapConfig.findUnique({ where: { userId }, select: { id: true } })
    if (!imap) return { status: 'No IMAP mailbox configured (Settings → IMAP)', signals: [] }
    // STUB: mailbox searching is not implemented yet. Real implementation returns matching messages here.
    return { status: 'Stub: mailbox search not implemented yet', signals: [] }
  },
}

export const SIGNAL_SOURCES: Record<string, SignalSourceType> = { [emailSource.type]: emailSource }

export function publicSourceTypes() {
  return Object.values(SIGNAL_SOURCES).map(({ check: _c, ...rest }) => rest)
}

/** Runs every enabled watcher for a user (optionally one bookmark) and stores new signals. */
export async function runSignalChecks(userId: string, bookmarkId?: string) {
  const sources = await prisma.signalSource.findMany({ where: { ownerId: userId, enabled: true, ...(bookmarkId && { bookmarkId }) } })
  let created = 0
  for (const s of sources) {
    const def = SIGNAL_SOURCES[s.type]
    let status = 'Unknown source type'
    if (def) {
      try {
        const result = await def.check({ userId, config: (s.config as Record<string, any>) ?? {} })
        status = result.status
        for (const sig of result.signals) {
          const exists = await prisma.signal.findUnique({ where: { sourceId_externalId: { sourceId: s.id, externalId: sig.externalId } } })
          if (exists) continue
          await prisma.signal.create({
            data: { sourceType: s.type, sourceId: s.id, bookmarkId: s.bookmarkId, ownerId: userId, title: sig.title, summary: sig.summary, url: sig.url, externalId: sig.externalId, occurredAt: sig.occurredAt ?? new Date() },
          })
          created++
        }
      } catch (e: any) {
        status = `Error: ${e?.message ?? 'check failed'}`
      }
    }
    await prisma.signalSource.update({ where: { id: s.id }, data: { lastCheckedAt: new Date(), lastStatus: status } })
  }
  return { checked: sources.length, created }
}
