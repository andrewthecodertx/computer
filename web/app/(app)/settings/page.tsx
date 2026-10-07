import { SettingsClient } from './client'
import { getOidcStatus } from '@/lib/oidc-config'
import { headers } from 'next/headers'

export const dynamic = 'force-dynamic'

export default async function SettingsPage() {
  const h = await headers()
  const host = h.get('x-forwarded-host') ?? h.get('host') ?? ''
  const proto = h.get('x-forwarded-proto') ?? 'https'
  const callbackUrl = host ? `${proto}://${host}/api/auth/callback/authelia` : '/api/auth/callback/authelia'
  return <SettingsClient oidc={getOidcStatus()} callbackUrl={callbackUrl} />
}
