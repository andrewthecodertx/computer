import { PagesClient } from './client'

export default async function PagesPage({ searchParams }: { searchParams: Promise<{ id?: string }> }) {
  const { id } = await searchParams
  return <PagesClient initialId={id ?? null} />
}
