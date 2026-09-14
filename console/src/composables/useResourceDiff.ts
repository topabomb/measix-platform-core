import { computed } from 'vue'
import type { components } from '../api/generated'
import type { useDraftStore } from '../stores/draft'

type ValidationIssue = components['schemas']['ValidationIssue']

/** Validation helpers for the Resources workspace. Published-to-candidate diff
 * is authoritative only when returned by the Hub preview endpoint. */
export function useResourceDiff(draft: ReturnType<typeof useDraftStore>) {
  const reviewWarnings = computed(() => draft.validationResult?.warnings ?? [])
  const hasBlockingErrors = computed(() => (draft.validationResult?.errors.length ?? 0) > 0)

  function validationIssuesFor(resourceId: string): { errors: ValidationIssue[]; warnings: ValidationIssue[] } {
    const errors = draft.validationResult?.errors.filter(issue => issue.resourceId === resourceId) ?? []
    const warnings = draft.validationResult?.warnings.filter(issue => issue.resourceId === resourceId) ?? []
    return { errors, warnings }
  }

  return { reviewWarnings, hasBlockingErrors, validationIssuesFor }
}
