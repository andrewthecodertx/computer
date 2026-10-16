import { NextRequest } from 'next/server'
import { backendFetch, observedClientIP } from '@/lib/api'

export const dynamic = 'force-dynamic'
// Go's handlers cap JSON bodies at 1 MiB (http.MaxBytesReader). Enforce the
// same cap at the proxy boundary (review: frontend proxy body limits) so
// oversized or chunked payloads are rejected here instead of being buffered
// through the Next server first.
const MAX_BODY_BYTES = 1 << 20
// Review CR-14: the cap is enforced WHILE streaming — a chunked body without
// a usable Content-Length is cancelled as soon as it crosses the limit
// instead of being fully buffered first — and concurrent body readers are
// bounded, so oversized-upload floods cannot pile up in memory.
const MAX_CONCURRENT_BODY_READS = 32
let activeBodyReads = 0

// readBodyLimited returns the buffered body, null when it exceeded the cap
// (the reader is cancelled promptly), or 'busy' when too many bodies are
// already being read.
async function readBodyLimited(request: NextRequest): Promise<ArrayBuffer | null | 'busy'> {
  if (activeBodyReads >= MAX_CONCURRENT_BODY_READS) return 'busy'
  activeBodyReads++
  try {
    if (!request.body) return new ArrayBuffer(0)
    const reader = request.body.getReader()
    const chunks: Uint8Array[] = []
    let total = 0
    for (;;) {
      const { done, value } = await reader.read()
      if (done) break
      if (!value) continue
      total += value.byteLength
      if (total > MAX_BODY_BYTES) {
        await reader.cancel().catch(() => { /* already broken */ })
        return null
      }
      chunks.push(value)
    }
    const out = new Uint8Array(total)
    let offset = 0
    for (const chunk of chunks) { out.set(chunk, offset); offset += chunk.byteLength }
    return out.buffer
  } finally {
    activeBodyReads--
  }
}

async function proxy(request: NextRequest) {
  // NextAuth handles its own explicit /api/auth/* route; this catches all data
  // routes and preserves JSON, Markdown downloads, statuses and view-as cookies.
  const headers = new Headers()
  if (request.headers.has('content-type')) headers.set('Content-Type', request.headers.get('content-type')!)
  // Review SEC-2: forward the observed browser IP only when TRUSTED_PROXY=1
  // pins the boundary that overwrites X-Real-IP (the shipped nginx template
  // does). X-Forwarded-For is deliberately ignored: its leftmost entry is
  // client-controlled. backendFetch adds the timestamped HMAC signature.
  const clientIP = observedClientIP(request.headers)
  if (clientIP) headers.set('x-computer-client-ip', clientIP)
  if (!['GET', 'HEAD'].includes(request.method)) {
    const declared = Number(request.headers.get('content-length') || 0)
    if (declared > MAX_BODY_BYTES) {
      return Response.json({ error: 'Request body too large' }, { status: 413 })
    }
    // Defense in depth: the SameSite session cookie is the primary CSRF
    // barrier. Origin is checked when present (parsed env origin, so a
    // trailing slash or path in NEXTAUTH_URL cannot break the match);
    // Sec-Fetch-Site closes the gap for mutated requests whose Origin was
    // stripped by an intermediary. An allowlisted origin is deliberately
    // permitted even though the browser labels it cross-site.
    const origin = request.headers.get('origin')
    if (origin) {
      const envUrl = process.env.NEXTAUTH_URL || process.env.AUTH_URL
      let envOrigin: string | null = null
      try { envOrigin = envUrl ? new URL(envUrl).origin : null } catch { envOrigin = null }
      if (origin !== request.nextUrl.origin && origin !== envOrigin) {
        return Response.json({ error: 'Invalid request origin' }, { status: 403 })
      }
    } else if (request.headers.get('sec-fetch-site') === 'cross-site') {
      return Response.json({ error: 'Invalid request origin' }, { status: 403 })
    }
  }
  try {
    let body: ArrayBuffer | undefined
    if (!['GET', 'HEAD'].includes(request.method)) {
      // Review CR-14: streamed, counted and cancelled at the limit — an
      // oversized chunked body without Content-Length never lands fully in
      // memory before its 413.
      const limited = await readBodyLimited(request)
      if (limited === 'busy') {
        return Response.json({ error: 'Too many concurrent request bodies' }, { status: 429 })
      }
      if (limited === null) {
        return Response.json({ error: 'Request body too large' }, { status: 413 })
      }
      body = limited
    }
    const response = await backendFetch(request.nextUrl.pathname + request.nextUrl.search, {
      method: request.method, headers, body,
    })
    const outgoing = new Headers()
    for (const name of ['content-type', 'content-disposition', 'set-cookie', 'x-next-cursor']) {
      const value = response.headers.get(name)
      if (value) outgoing.set(name, value)
    }
    outgoing.set('Cache-Control', 'no-store')
    return new Response(response.body, { status: response.status, headers: outgoing })
  } catch (error) {
    console.error('Go API unavailable', error)
    return Response.json({ error: 'API unavailable' }, { status: 502 })
  }
}
export { proxy as GET, proxy as HEAD, proxy as POST, proxy as PUT, proxy as PATCH, proxy as DELETE }
