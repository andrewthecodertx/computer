'use client'

import { AppShell } from '@/components/app-shell'

export function AppShellWrapper({ children }: { children: React.ReactNode }) {
  return <AppShell>{children}</AppShell>
}
