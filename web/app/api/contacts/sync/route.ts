export const dynamic = 'force-dynamic'
import { NextRequest, NextResponse } from 'next/server'
import { prisma } from '@/lib/prisma'
import { getAuthUser } from '@/lib/auth-guard'

export async function POST(req: NextRequest) {
  const user = await getAuthUser()
  if (!user) return NextResponse.json({ error: 'Unauthorized' }, { status: 401 })

  // Read user's CardDAV config from request body or settings
  const { nextcloudUrl, cardDavPath, username, password } = await req.json()

  if (!nextcloudUrl || !username || !password) {
    return NextResponse.json({ error: 'Nextcloud URL, username, and password are required' }, { status: 400 })
  }

  const path = cardDavPath ?? `/remote.php/dav/addressbooks/users/${username}/contacts/`
  const fullUrl = `${nextcloudUrl.replace(/\/$/, '')}${path}`

  try {
    const response = await fetch(fullUrl, {
      method: 'REPORT',
      headers: {
        'Content-Type': 'application/xml; charset=utf-8',
        'Depth': '1',
        'Authorization': 'Basic ' + Buffer.from(`${username}:${password}`).toString('base64'),
      },
      body: `<?xml version="1.0" encoding="utf-8"?>
<c:addressbook-query xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:carddav">
  <d:prop><d:getetag /><c:address-data /></d:prop>
</c:addressbook-query>`,
    })

    if (!response.ok) {
      return NextResponse.json({ error: `CardDAV error: ${response.status}` }, { status: 502 })
    }

    const text = await response.text()
    const { XMLParser } = await import('fast-xml-parser')
    const parser = new XMLParser({ ignoreAttributes: false, removeNSPrefix: true })
    const parsed = parser.parse(text)

    const responses = Array.isArray(parsed?.multistatus?.response)
      ? parsed.multistatus.response
      : parsed?.multistatus?.response ? [parsed.multistatus.response] : []

    let synced = 0
    for (const r of responses) {
      const propstat = r?.propstat
      const addressData = propstat?.prop?.['address-data'] ?? propstat?.prop?.addressdata
      if (!addressData || typeof addressData !== 'string') continue

      const lines = addressData.split('\n')
      let uid = '', fn = '', email = '', tel = ''
      for (const line of lines) {
        if (line.startsWith('UID:')) uid = line.substring(4).trim()
        if (line.startsWith('FN:')) fn = line.substring(3).trim()
        if (line.includes('EMAIL') && line.includes(':')) email = line.split(':').pop()?.trim() ?? ''
        if (line.includes('TEL') && line.includes(':')) tel = line.split(':').pop()?.trim() ?? ''
      }

      if (!uid || !fn) continue

      await prisma.contact.upsert({
        where: { uid_userId: { uid, userId: user.id } },
        create: { uid, displayName: fn, email: email || null, phone: tel || null, cardDavUrl: r?.href ?? null, userId: user.id },
        update: { displayName: fn, email: email || null, phone: tel || null, cardDavUrl: r?.href ?? null },
      })
      synced++
    }

    return NextResponse.json({ synced })
  } catch (e: any) {
    return NextResponse.json({ error: e?.message ?? 'Sync failed' }, { status: 500 })
  }
}
