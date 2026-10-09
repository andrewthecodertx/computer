'use client'

import { SessionProvider } from 'next-auth/react'
import type { Session } from 'next-auth'
import { AppShell } from '@/components/app-shell'

// Nested provider seeded with the server session: useSession() in AppShell
// has data during SSR, so the signed-in shell is server-rendered.
export function AppShellWrapper({ session, children }: { session: Session; children: React.ReactNode }) {
  return (
    <SessionProvider session={session}>
      <AppShell>{children}</AppShell>
    </SessionProvider>
  )
}
