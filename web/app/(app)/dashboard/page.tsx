import { redirect } from 'next/navigation'

// The dashboard was folded into the "All Bookmarks" filter view.
export default async function DashboardPage({ searchParams }: { searchParams: Promise<{ search?: string }> }) {
  const { search } = await searchParams
  redirect(search ? `/bookmarks?search=${encodeURIComponent(search)}` : '/bookmarks')
}
