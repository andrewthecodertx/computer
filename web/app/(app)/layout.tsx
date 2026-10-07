import { auth } from '@/auth'
import { redirect } from 'next/navigation'
import { AppShellWrapper } from '@/components/app-shell-wrapper'

export default async function AppLayout({ children }: { children: React.ReactNode }) {
  const session = await auth()
  if (!session?.user) redirect('/login')
  return <AppShellWrapper>{children}</AppShellWrapper>
}
