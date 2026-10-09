export const dynamic = 'force-dynamic'
import { redirect } from 'next/navigation'
import { getRealUser } from '@/lib/auth-guard'
import { AdminClient } from './client'

export default async function AdminPage() {
  const user = await getRealUser()
  if (!user) redirect('/login')
  if (user.role !== 'ADMIN') redirect('/pages')
  return <AdminClient currentUserId={user.id} />
}
