import type { components } from './generated'
import { metersForCapability, normalizeBudgetLimitInput, type BudgetCapability, type PricingMeter } from './usageBudget'

export type PricingRule = components['schemas']['PricingRule']
export type PricingScope = 'GLOBAL' | 'UPSTREAM' | 'RESOURCE' | 'RESOURCE_UPSTREAM'
export type PricingResource = { value: string; label: string; kind: BudgetCapability }

export function pricingResources(content: components['schemas']['ManagedDraftContent']): PricingResource[] {
  return [
    ...content.models.map(item => ({ value: item.modelId, label: item.displayName, kind: 'MODEL' as const })),
    ...(content.imageGenerators ?? []).map(item => ({ value: item.imageId, label: item.displayName, kind: 'IMAGE_GENERATION' as const })),
    ...content.tts.map(item => ({ value: item.ttsId, label: item.displayName, kind: 'TTS' as const })),
    ...content.asr.map(item => ({ value: item.asrId, label: item.displayName, kind: 'ASR' as const })),
    ...content.mcp.map(item => ({ value: item.mcpServerId, label: item.displayName, kind: 'MCP' as const })),
  ]
}

export function pricingScope(rule: PricingRule): PricingScope {
  return rule.resourceId ? rule.upstreamId ? 'RESOURCE_UPSTREAM' : 'RESOURCE' : rule.upstreamId ? 'UPSTREAM' : 'GLOBAL'
}

export function resourcePricingMeters(resourceId: string | undefined): PricingMeter[] {
  const kinds: Record<string, BudgetCapability> = { mdl: 'MODEL', img: 'IMAGE_GENERATION', tts: 'TTS', asr: 'ASR', mcp: 'MCP' }
  const kind = resourceId ? kinds[resourceId.split('_')[0] ?? ''] : undefined
  return kind ? metersForCapability(kind) : ['INPUT_TOKENS', 'OUTPUT_TOKENS', 'CACHED_TOKENS', 'TOTAL_TOKENS', 'REQUESTED_IMAGES', 'CHARACTERS', 'AUDIO_SECONDS', 'REQUESTS']
}

export function localPricingTime(value: string | undefined): string {
  if (!value || !Number.isFinite(Date.parse(value))) return ''
  const date = new Date(value)
  return new Date(date.getTime() - date.getTimezoneOffset() * 60_000).toISOString().slice(0, -1)
}

export function pricingTime(value: string): string | undefined {
  if (!value || !Number.isFinite(Date.parse(value))) return undefined
  return new Date(value).toISOString()
}

export function normalizePricingRule(rule: PricingRule, scope: PricingScope): PricingRule {
  return {
    ...rule,
    resourceId: scope.includes('RESOURCE') ? rule.resourceId?.trim() || undefined : undefined,
    upstreamId: scope.includes('UPSTREAM') ? rule.upstreamId?.trim() || undefined : undefined,
    unitSize: normalizeBudgetLimitInput(rule.unitSize) ?? rule.unitSize.trim(), unitPrice: rule.unitPrice.trim(), currency: rule.currency.trim().toUpperCase(),
    effectiveTo: rule.effectiveTo || undefined,
  }
}

// Return a translation key; quantities remain decimal strings throughout.
export function pricingRuleError(rule: PricingRule, scope: PricingScope, rules: PricingRule[]): string | undefined {
  if (scope.includes('RESOURCE') && !/^(mdl|img|tts|asr|mcp)_[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(rule.resourceId ?? '')) return 'resourceRequired'
  if (scope.includes('UPSTREAM') && !rule.upstreamId) return 'upstreamRequired'
  if (!resourcePricingMeters(rule.resourceId).includes(rule.meter)) return 'incompatibleMeter'
  if (!/^\d+(?:\.\d+)?$/.test(rule.unitSize) || !/[1-9]/.test(rule.unitSize)) return 'invalidUnit'
  if (!/^\d+(?:\.\d+)?$/.test(rule.unitPrice)) return 'invalidPrice'
  if (!/^[A-Z]{3}$/.test(rule.currency)) return 'invalidCurrency'
  if (!Number.isFinite(Date.parse(rule.effectiveFrom)) || rule.effectiveTo && (!Number.isFinite(Date.parse(rule.effectiveTo)) || Date.parse(rule.effectiveTo) <= Date.parse(rule.effectiveFrom))) return 'invalidTime'
  if (rules.some(other => other.pricingRuleId !== rule.pricingRuleId && other.meter === rule.meter && other.resourceId === rule.resourceId && other.upstreamId === rule.upstreamId && Date.parse(other.effectiveFrom) === Date.parse(rule.effectiveFrom))) return 'duplicateStart'
  return undefined
}
