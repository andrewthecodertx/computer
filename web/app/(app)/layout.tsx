import { auth } from '@/auth'
import { redirect } from 'next/navigation'
import { AppShellWrapper } from '@/components/app-shell-wrapper'

export default async function AppLayout({ children }: { children: React.ReactNode }) {
  const session = await auth()
  if (!session?.user) redirect('/login')
  // Pass the server session down so the shell and screens render during SSR
  // instead of being discarded and refetched on the client.
  return <AppShellWrapper session={session}>{children}</AppShellWrapper>
}
