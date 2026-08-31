import { computed } from 'vue'
import type { components } from '../api/generated'
import type { useDraftStore } from '../stores/draft'

type ManagedDraftContent = components['schemas']['ManagedDraftContent']
type ValidationIssue = components['schemas']['ValidationIssue']

/** Resource-like shape: any object that carries one of the known ID fields. */
interface ResourceLike {
  assistantDefinitionId?: string
  starterId?: string
  modelId?: string
  ttsId?: string
  asrId?: string
  mcpServerId?: string
  providerId?: string
  resourceId?: string
}

export type DiffResult<T extends ResourceLike> = {
  added: T[]
  changed: { item: T; baseItem?: T }[]
  removed: T[]
}

export type ReviewDiff = {
  providers: DiffResult<ManagedDraftContent['providers'][number]>
  models: DiffResult<ManagedDraftContent['models'][number]>
  tts: DiffResult<ManagedDraftContent['tts'][number]>
  asr: DiffResult<ManagedDraftContent['asr'][number]>
  mcp: DiffResult<ManagedDraftContent['mcp'][number]>
  bindings: DiffResult<ManagedDraftContent['bindings'][number]>
  assistants: DiffResult<NonNullable<ManagedDraftContent['assistants']>[number]>
  starters: DiffResult<NonNullable<ManagedDraftContent['starters']>[number]>
  policyChanged: boolean
}

/**
 * Extract the canonical resource ID from a resource-like object.
 * Checks all known ID fields in order.
 */
export function resourceIdOf(item: ResourceLike): string {
  return item.starterId ?? item.assistantDefinitionId ?? item.modelId ?? item.ttsId ?? item.asrId ?? item.mcpServerId ?? item.providerId ?? item.resourceId ?? ''
}

/**
 * Structured diff between two arrays of resource-like objects.
 * Uses JSON.stringify for deep equality (order-sensitive within objects,
 * which is correct for OpenAPI-generated DTOs with stable field order).
 */
export function diffList<T extends ResourceLike>(
  baseArr: T[],
  localArr: T[],
  idFn: (item: T) => string,
): DiffResult<T> {
  const baseMap = new Map(baseArr.map((item) => [idFn(item), item]))
  const localMap = new Map(localArr.map((item) => [idFn(item), item]))
  const added: T[] = []
  const changed: { item: T; baseItem?: T }[] = []
  const removed: T[] = []
  for (const item of localArr) {
    const id = idFn(item)
    const baseItem = baseMap.get(id)
    if (!baseItem) {
      added.push(item)
    } else if (JSON.stringify(item) !== JSON.stringify(baseItem)) {
      changed.push({ item, baseItem })
    }
  }
  for (const item of baseArr) {
    const id = idFn(item)
    if (!localMap.has(id)) removed.push(item)
  }
  return { added, changed, removed }
}

/**
 * Composable that provides structured diff and validation helpers
 * for the Resources page Review workspace.
 *
 * This extracts domain logic that was previously inlined in ResourcesPage.vue,
 * keeping the page focused on presentation and interaction.
 */
export function useResourceDiff(draft: ReturnType<typeof useDraftStore>) {
  /** Structured diff between baseline and local draft content. */
  const reviewDiff = computed<ReviewDiff | null>(() => {
    const base = draft.baselineContent
    const local = draft.localContent
    if (!base || !local) return null

    return {
      providers: diffList(base.providers, local.providers, (p) => p.providerId),
      models: diffList(base.models, local.models, (m) => m.modelId),
      tts: diffList(base.tts, local.tts, (t) => t.ttsId),
      asr: diffList(base.asr, local.asr, (a) => a.asrId),
      mcp: diffList(base.mcp, local.mcp, (m) => m.mcpServerId),
      assistants: diffList(base.assistants ?? [], local.assistants ?? [], a => a.assistantDefinitionId),
      starters: diffList(base.starters ?? [], local.starters ?? [], s => s.starterId),
      bindings: diffList(base.bindings, local.bindings, (b) => b.resourceId),
      policyChanged: JSON.stringify(base.policy) !== JSON.stringify(local.policy),
    }
  })

  /** Runtime routing impact summary. */
  const routingImpact = computed(() => {
    if (!reviewDiff.value) return { added: 0, changed: 0, removed: 0 }
    const b = reviewDiff.value.bindings
    return { added: b.added.length, changed: b.changed.length, removed: b.removed.length }
  })

  /** Warnings from validation that need acknowledgment before publish. */
  const reviewWarnings = computed(() => draft.validationResult?.warnings ?? [])

  /** Total resource changes count for review summary. */
  const reviewTotalChanges = computed(() => {
    if (!reviewDiff.value) return 0
    const d = reviewDiff.value
    return d.providers.added.length + d.providers.changed.length + d.providers.removed.length
      + d.models.added.length + d.models.changed.length + d.models.removed.length
      + d.tts.added.length + d.tts.changed.length + d.tts.removed.length
      + d.asr.added.length + d.asr.changed.length + d.asr.removed.length
      + d.mcp.added.length + d.mcp.changed.length + d.mcp.removed.length
      + d.assistants.added.length + d.assistants.changed.length + d.assistants.removed.length
      + d.starters.added.length + d.starters.changed.length + d.starters.removed.length
      + d.bindings.added.length + d.bindings.changed.length + d.bindings.removed.length
      + (d.policyChanged ? 1 : 0)
  })

  /** Has blocking errors that prevent publish. */
  const hasBlockingErrors = computed(() => (draft.validationResult?.errors.length ?? 0) > 0)

  /**
   * Filter validation issues that reference a specific resource ID.
   * Uses the structured `resourceId` field when available, falling back
   * to path includes() for backward compatibility.
   */
  function validationIssuesFor(resourceId: string): { errors: ValidationIssue[]; warnings: ValidationIssue[] } {
    const errors = draft.validationResult?.errors.filter((e) =>
      e.resourceId ? e.resourceId === resourceId : e.path.includes(resourceId),
    ) ?? []
    const warnings = draft.validationResult?.warnings.filter((w) =>
      w.resourceId ? w.resourceId === resourceId : w.path.includes(resourceId),
    ) ?? []
    return { errors, warnings }
  }

  return {
    reviewDiff,
    routingImpact,
    reviewWarnings,
    reviewTotalChanges,
    hasBlockingErrors,
    validationIssuesFor,
  }
}
