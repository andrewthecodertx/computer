/**
 * Authelia OIDC configuration (STUB-AWARE).
 *
 * Authelia sign-in is only enabled when all three env vars hold real values:
 *   OIDC_ISSUER, OIDC_CLIENT_ID, OIDC_CLIENT_SECRET
 *
 * Empty values or obvious placeholders (REPLACE_ME, example.com, changeme, ...)
 * are treated as "not connected": the Authelia provider is not registered,
 * the login button is shown disabled, and email/password login keeps working.
 *
 * See docs/AUTHELIA.md for the full connection guide.
 */

const PLACEHOLDER_PATTERNS = [/replace[_-]?me/i, /example\.(com|org|net)/i, /changeme/i, /placeholder/i, /^<.*>$/, /^your[_-]/i, /^todo$/i]

function isRealValue(value: string | undefined): value is string {
  const v = (value ?? '').trim()
  if (!v) return false
  return !PLACEHOLDER_PATTERNS.some((p) => p.test(v))
}

export interface OidcStatus {
  enabled: boolean
  issuerSet: boolean
  clientIdSet: boolean
  clientSecretSet: boolean
  /** Only exposed when the issuer is a real value (never exposes secrets). */
  issuer: string | null
}

export function getOidcStatus(): OidcStatus {
  const issuerSet = isRealValue(process.env.OIDC_ISSUER)
  const clientIdSet = isRealValue(process.env.OIDC_CLIENT_ID)
  const clientSecretSet = isRealValue(process.env.OIDC_CLIENT_SECRET)
  return {
    enabled: issuerSet && clientIdSet && clientSecretSet,
    issuerSet,
    clientIdSet,
    clientSecretSet,
    issuer: issuerSet ? process.env.OIDC_ISSUER!.trim() : null,
  }
}
