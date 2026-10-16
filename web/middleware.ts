import { NextResponse, type NextRequest } from 'next/server'

// Review CR-10: the production nginx template used to send a static
// `script-src 'self'` CSP. The App Router renders inline bootstrap scripts
// (flight-data pushes) that a nonce-less policy blocks: HTML arrives 200 but
// hydration never completes and the UI stays inert. The CSP is therefore
// owned here: every response gets a fresh nonce, and Next.js — which reads
// the CSP from the forwarded request headers — attaches it to the inline
// scripts it emits. nginx passes this header through and must NOT add its
// own script-src policy on top (browsers intersect multiple policies, which
// would strip the nonce again). This is a nonce strategy, not a blanket
// inline-script relaxation.
export function middleware(request: NextRequest) {
  const nonce = Buffer.from(crypto.randomUUID()).toString('base64')
  // Dev-mode HMR uses a websocket on the same host; production never needs it.
  const connectSrc = process.env.NODE_ENV === 'development' ? `'self' ws:` : `'self'`
  const csp = [
    `default-src 'self'`,
    `script-src 'self' 'nonce-${nonce}'`,
    // Inline styles stay allowed (UI kit + sonner inject style attributes),
    // matching the previous nginx baseline.
    `style-src 'self' 'unsafe-inline'`,
    // Remote previews: og-image/favicon render from arbitrary https origins.
    `img-src 'self' data: blob: https:`,
    `font-src 'self' data:`,
    `connect-src ${connectSrc}`,
    `frame-ancestors 'none'`,
    `object-src 'none'`,
    `base-uri 'self'`,
    `form-action 'self'`,
  ].join('; ')

  const requestHeaders = new Headers(request.headers)
  requestHeaders.set('x-nonce', nonce)
  requestHeaders.set('Content-Security-Policy', csp)
  const response = NextResponse.next({ request: { headers: requestHeaders } })
  response.headers.set('Content-Security-Policy', csp)
  return response
}

export const config = {
  // Everything except immutable static assets (they cannot carry a per-request
  // nonce anyway and contain no bootstrap scripts).
  matcher: ['/((?!_next/static/|_next/image|favicon.ico|.*\\.(?:svg|png|jpg|jpeg|gif|webp|ico)$).*)'],
}
