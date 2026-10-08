import { backendFetch } from '@/lib/api'
export const VIEW_AS_COOKIE = 'computer_view_as'
export type AuthUser = { id: string; name?: string | null; email?: string | null; image?: string | null; role: 'USER' | 'ADMIN'; actingAdminId?: string }
async function account() { const res = await backendFetch('/api/me'); return res.ok ? res.json() : null }
export async function getRealUser(): Promise<AuthUser | null> { return (await account())?.user ?? null }
export async function getAuthUser(): Promise<AuthUser | null> { const me = await account(); return me ? { ...me.effectiveUser, ...(me.viewingAs ? { actingAdminId: me.user.id } : {}) } : null }
