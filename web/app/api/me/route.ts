export const dynamic = 'force-dynamic'
import { NextResponse } from 'next/server'
import { getAuthUser, getRealUser } from '@/lib/auth-guard'

export async function GET() {
  const real = await getRealUser()
  if (!real) return NextResponse.json({ error: 'Unauthorized' }, { status: 401 })
  const effective = await getAuthUser()
  return NextResponse.json({
    user: real,
    isAdmin: real.role === 'ADMIN',
    viewingAs: effective && effective.id !== real.id ? { id: effective.id, name: effective.name, email: effective.email } : null,
  })
}
