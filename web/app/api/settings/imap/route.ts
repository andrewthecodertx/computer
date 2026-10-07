export const dynamic = 'force-dynamic'
import { NextRequest, NextResponse } from 'next/server'
import { prisma } from '@/lib/prisma'
import { getAuthUser } from '@/lib/auth-guard'
import { encrypt, decrypt } from '@/lib/crypto'

export async function GET() {
  const user = await getAuthUser()
  if (!user) return NextResponse.json({ error: 'Unauthorized' }, { status: 401 })

  const config = await prisma.imapConfig.findUnique({ where: { userId: user.id } })
  if (!config) return NextResponse.json(null)

  return NextResponse.json({
    ...config,
    password: '••••••••',
  })
}

export async function POST(req: NextRequest) {
  const user = await getAuthUser()
  if (!user) return NextResponse.json({ error: 'Unauthorized' }, { status: 401 })

  const { host, port, tls, username, password, folder } = await req.json()
  if (!host || !username || !password) {
    return NextResponse.json({ error: 'Host, username, and password required' }, { status: 400 })
  }

  const encrypted = encrypt(password)

  const config = await prisma.imapConfig.upsert({
    where: { userId: user.id },
    create: { host, port: port ?? 993, tls: tls ?? true, username, password: encrypted, folder: folder ?? 'INBOX', userId: user.id },
    update: { host, port: port ?? 993, tls: tls ?? true, username, password: encrypted, folder: folder ?? 'INBOX' },
  })

  return NextResponse.json({ ...config, password: '••••••••' })
}
