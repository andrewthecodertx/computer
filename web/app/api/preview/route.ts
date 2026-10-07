export const dynamic = 'force-dynamic'
import { NextRequest, NextResponse } from 'next/server'
import { getAuthUser } from '@/lib/auth-guard'

export async function GET(req: NextRequest) {
  const user = await getAuthUser()
  if (!user) return NextResponse.json({ error: 'Unauthorized' }, { status: 401 })

  const url = req.nextUrl.searchParams.get('url')
  if (!url) return NextResponse.json({ error: 'URL required' }, { status: 400 })

  try {
    const ogs = (await import('open-graph-scraper')).default
    const { result } = await ogs({ url, timeout: 8000, fetchOptions: { headers: { 'User-Agent': 'Mozilla/5.0 (compatible; computer/1.0)' } } })

    let favicon = result?.favicon ?? null
    if (favicon && !favicon.startsWith('http')) {
      const parsed = new URL(url)
      favicon = favicon.startsWith('/') ? `${parsed.origin}${favicon}` : `${parsed.origin}/${favicon}`
    }
    if (!favicon) {
      try { favicon = `${new URL(url).origin}/favicon.ico` } catch { /* noop */ }
    }

    return NextResponse.json({
      ogTitle: result?.ogTitle ?? null,
      ogDescription: result?.ogDescription ?? null,
      ogImage: result?.ogImage?.[0]?.url ?? null,
      favicon,
    })
  } catch (e: any) {
    return NextResponse.json({ ogTitle: null, ogDescription: null, ogImage: null, favicon: null })
  }
}
