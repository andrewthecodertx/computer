import { SettingsClient } from './client'
import { getOidcStatus } from '@/lib/oidc-config'
import { headers } from 'next/headers'

export const dynamic = 'force-dynamic'

export default async function SettingsPage() {
  // The redirect URI shown for Authelia registration derives from the same
  // configured origin auth.ts trusts — never from client-controllable request
  // headers in production (header poisoning could hand an admin an
  // attacker-hosted callback URL to register).
  const envUrl = process.env.NEXTAUTH_URL || process.env.AUTH_URL || ''
  let callbackUrl = '/api/auth/callback/authelia'
  try {
    if (envUrl) callbackUrl = `${new URL(envUrl).origin}/api/auth/callback/authelia`
  } catch { /* keep the relative fallback */ }
  if (!envUrl && process.env.NODE_ENV !== 'production') {
    const h = await headers()
    const host = h.get('x-forwarded-host') ?? h.get('host') ?? ''
    const proto = h.get('x-forwarded-proto') ?? 'http'
    if (host) callbackUrl = `${proto}://${host}/api/auth/callback/authelia`
  }
  return <SettingsClient oidc={getOidcStatus()} callbackUrl={callbackUrl} />
}
