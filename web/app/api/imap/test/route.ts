export const dynamic = 'force-dynamic'
import { NextRequest, NextResponse } from 'next/server'
import { getAuthUser } from '@/lib/auth-guard'

export async function POST(req: NextRequest) {
  const user = await getAuthUser()
  if (!user) return NextResponse.json({ error: 'Unauthorized' }, { status: 401 })

  const { host, port, tls, username, password } = await req.json()
  if (!host || !username || !password) {
    return NextResponse.json({ error: 'Host, username, and password required' }, { status: 400 })
  }

  try {
    // Simple connection test via net/tls
    const net = await import('net')
    const tlsModule = await import('tls')
    
    return await new Promise<NextResponse>((resolve) => {
      const timeout = setTimeout(() => {
        resolve(NextResponse.json({ error: 'Connection timed out' }, { status: 408 }))
      }, 10000)

      const socket = tls !== false
        ? tlsModule.connect({ host, port: port ?? 993, rejectUnauthorized: false }, () => {
            clearTimeout(timeout)
            socket.destroy()
            resolve(NextResponse.json({ ok: true, message: 'Connection successful' }))
          })
        : net.createConnection({ host, port: port ?? 143 }, () => {
            clearTimeout(timeout)
            socket.destroy()
            resolve(NextResponse.json({ ok: true, message: 'Connection successful' }))
          })

      socket.on('error', (err: any) => {
        clearTimeout(timeout)
        resolve(NextResponse.json({ error: `Connection failed: ${err?.message}` }, { status: 502 }))
      })
    })
  } catch (e: any) {
    return NextResponse.json({ error: e?.message ?? 'Test failed' }, { status: 500 })
  }
}
