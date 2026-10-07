import { auth } from '@/auth'
import { cookies } from 'next/headers'
import { prisma } from '@/lib/prisma'

export const VIEW_AS_COOKIE = 'computer_view_as'

export type AuthUser = {
  id: string
  name?: string | null
  email?: string | null
  image?: string | null
  role: 'USER' | 'ADMIN'
  /** Set when an admin is acting inside another user's account ("view as"). */
  actingAdminId?: string
}

function adminEmails(): string[] {
  return (process.env.ADMIN_EMAILS ?? '')
    .split(',')
    .map((s) => s.trim().toLowerCase())
    .filter(Boolean)
}

/** Admin = role ADMIN in the DB, or email listed in ADMIN_EMAILS (promoted to ADMIN on first check). */
async function resolveRole(u: { id: string; email: string | null; role: string }): Promise<'USER' | 'ADMIN'> {
  if (u.role === 'ADMIN') return 'ADMIN'
  if (u.email && adminEmails().includes(u.email.toLowerCase())) {
    await prisma.user.update({ where: { id: u.id }, data: { role: 'ADMIN' } })
    return 'ADMIN'
  }
  return 'USER'
}

/** The person actually signed in, ignoring "view as". Use for admin endpoints. */
export async function getRealUser(): Promise<AuthUser | null> {
  const session = await auth()
  const id = (session?.user as any)?.id as string | undefined
  if (!id) return null
  const u = await prisma.user.findUnique({ where: { id }, select: { id: true, name: true, email: true, image: true, role: true } })
  if (!u) return null
  return { ...u, role: await resolveRole(u) }
}

/**
 * The user whose data the request operates on. For admins with an active "view as" cookie this is
 * the target account, so every data API transparently works inside that account.
 */
export async function getAuthUser(): Promise<AuthUser | null> {
  const real = await getRealUser()
  if (!real) return null
  if (real.role !== 'ADMIN') return real
  const store = await cookies()
  const targetId = store.get(VIEW_AS_COOKIE)?.value
  if (!targetId || targetId === real.id) return real
  const target = await prisma.user.findUnique({ where: { id: targetId }, select: { id: true, name: true, email: true, image: true, role: true } })
  if (!target) return real
  return { ...target, role: target.role === 'ADMIN' ? 'ADMIN' : 'USER', actingAdminId: real.id }
}
