export const dynamic = 'force-dynamic'
import { NextResponse } from 'next/server'
import { prisma } from '@/lib/prisma'
import bcrypt from 'bcryptjs'

export async function POST(req: Request) {
  try {
    const { email, password, name } = await req.json()
    if (!email || !password) {
      return NextResponse.json({ error: 'Email and password are required' }, { status: 400 })
    }
    const existing = await prisma.user.findUnique({ where: { email } })
    if (existing) {
      return NextResponse.json({ error: 'User already exists' }, { status: 409 })
    }
    const hashed = await bcrypt.hash(password, 12)
    // Sane default: the first real account becomes admin when no admin exists yet
    const adminCount = await prisma.user.count({ where: { role: 'ADMIN', NOT: { email: { endsWith: '@example.com' } } } })
    const user = await prisma.user.create({
      data: {
        email,
        name: name ?? email.split('@')[0],
        password: hashed,
        role: adminCount === 0 && !String(email).endsWith('@example.com') ? 'ADMIN' : 'USER',
      } as any,
    })
    return NextResponse.json({ ok: true, userId: user.id }, { status: 201 })
  } catch (error: any) {
    return NextResponse.json({ error: error?.message ?? 'Server error' }, { status: 500 })
  }
}
