import { auth } from '@/auth'
import { redirect } from 'next/navigation'

export default async function Home() {
  const session = await auth()
  if (session?.user) redirect('/pages')
  redirect('/login')
}
