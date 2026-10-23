import { onBeforeUnmount, ref, type Ref } from 'vue'
import { cursorPath } from '../api/pagination'

export interface CursorPage<T> {
  items: T[]
  nextCursor?: string
}

/**
 * Bounded cursor paging for management collections.
 *
 * Pickers intentionally accumulate options while they are open. Main entity
 * collections do not: they replace the current page so thousands of rows do
 * not remain mounted, while retaining cursors for explicit previous/next.
 */
export function useCursorPager<T, P extends CursorPage<T>>(
  basePath: Ref<string>,
  fetchPage: (path: string) => Promise<P>,
  onPage?: (page: P) => void,
) {
  const items = ref<T[]>([]) as Ref<T[]>
  const nextCursor = ref<string>()
  const pageNumber = ref(1)
  const pageCursors = ref<Array<string | undefined>>([undefined])
  const loading = ref(false)
  const error = ref<unknown>()
  let sequence = 0

  async function loadPage(targetPage: number, cursor?: string) {
    const current = ++sequence
    loading.value = true
    error.value = undefined
    try {
      const path = cursor ? cursorPath(basePath.value, cursor) : basePath.value
      const result = await fetchPage(path)
      if (current !== sequence) return
      items.value = result.items
      nextCursor.value = result.nextCursor
      pageNumber.value = targetPage
      onPage?.(result)
    } catch (cause) {
      if (current === sequence) error.value = cause
    } finally {
      if (current === sequence) loading.value = false
    }
  }

  async function reset() {
    pageCursors.value = [undefined]
    await loadPage(1)
  }

  async function nextPage() {
    if (!nextCursor.value || loading.value) return
    pageCursors.value[pageNumber.value] = nextCursor.value
    await loadPage(pageNumber.value + 1, nextCursor.value)
  }

  async function previousPage() {
    if (pageNumber.value <= 1 || loading.value) return
    const target = pageNumber.value - 1
    await loadPage(target, pageCursors.value[target - 1])
  }

  onBeforeUnmount(() => { sequence++ })

  return { items, nextCursor, pageNumber, loading, error, reset, nextPage, previousPage }
}
