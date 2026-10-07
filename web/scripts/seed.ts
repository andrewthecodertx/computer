import { PrismaClient } from '@prisma/client'
import bcrypt from 'bcryptjs'

// Optional: creates an initial user from env vars. Safe to re-run (upsert, never deletes).
//   SEED_EMAIL=you@example.org SEED_PASSWORD=... yarn prisma db seed
const prisma = new PrismaClient()

async function main() {
  const email = process.env.SEED_EMAIL
  const password = process.env.SEED_PASSWORD
  if (!email || !password) {
    console.log('SEED_EMAIL / SEED_PASSWORD not set; nothing to seed.')
    return
  }
  const hashed = await bcrypt.hash(password, 12)
  await prisma.user.upsert({
    where: { email },
    update: {},
    create: { email, name: process.env.SEED_NAME ?? 'Admin', password: hashed } as any,
  })
  console.log(`Seeded user ${email}`)
}

main()
  .catch((e) => { console.error(e); process.exit(1) })
  .finally(() => prisma.$disconnect())
