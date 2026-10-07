import { LoginClient } from './client'
import { auth } from '@/auth'
import { redirect } from 'next/navigation'
import { getOidcStatus } from '@/lib/oidc-config'

export const dynamic = 'force-dynamic'

export default async function LoginPage() {
  const session = await auth()
  if (session?.user) redirect('/dashboard')
  return <LoginClient oidcEnabled={getOidcStatus().enabled} />
}
