import { NextRequest } from 'next/server'
import { backendFetch } from '@/lib/api'

export const dynamic = 'force-dynamic'
async function proxy(request: NextRequest) {
  // NextAuth handles its own explicit /api/auth/* route; this catches all data
  // routes and preserves JSON, Markdown downloads, statuses and view-as cookies.
  const headers = new Headers()
  if (request.headers.has('content-type')) headers.set('Content-Type', request.headers.get('content-type')!)
  if (!['GET', 'HEAD'].includes(request.method)) {
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
    const response = await backendFetch(request.nextUrl.pathname + request.nextUrl.search, {
      method: request.method, headers,
      body: ['GET', 'HEAD'].includes(request.method) ? undefined : await request.arrayBuffer(),
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
