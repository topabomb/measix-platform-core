import { apiFetch } from './client'

export function cursorPath(path: string, cursor: string): string {
  const [base, query] = path.split('?')
  const params = new URLSearchParams(query)
  params.set('cursor', cursor)
  return base + '?' + params.toString()
}

// Configuration selectors need the complete set, not a silently truncated page.
// Large history views use explicit Load more instead.
export async function fetchAllPages<T>(path: string): Promise<T[]> {
  const items: T[] = []
  const seen = new Set<string>()
  let next = path
  do {
    const page = await apiFetch<{ items: T[]; nextCursor?: string }>(next)
    items.push(...page.items)
    if (!page.nextCursor) return items
    if (seen.has(page.nextCursor)) throw new Error('Server repeated a pagination cursor')
    seen.add(page.nextCursor)
    next = cursorPath(path, page.nextCursor)
  } while (true)
}
