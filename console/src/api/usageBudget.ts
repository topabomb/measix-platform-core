import type { components } from './generated'

type Schemas = components['schemas']

export type PricingMeter = Schemas['PricingMeter']
export type BudgetCapability = Schemas['BudgetCapability']
export type BudgetMode = Schemas['BudgetMode']
export type BudgetSource = Schemas['BudgetSource']
export type BudgetPeriod = Schemas['BudgetPeriod']
export type BudgetStatus = Schemas['BudgetStatus']
export type UsageClientProtocol = Schemas['UsageClientProtocol']
export type UsageCompleteness = Schemas['RequestUsageView']['requestCompleteness']
export type BudgetLimitDefinition = Schemas['BudgetLimitDefinition']
export type BudgetLimitState = Schemas['BudgetLimitState']
export type BudgetCapabilityView = Schemas['BudgetCapabilityView']
export type UserBudgetView = Schemas['UserBudgetView']
export type PutBudgetRequest = Schemas['PutBudgetRequest']
export type BudgetAuditItem = Schemas['BudgetAuditItem']
export type BudgetAuditPage = Schemas['BudgetAuditPage']
export type BudgetTemplateRule = Schemas['BudgetTemplateRule']
export type BudgetTemplate = Schemas['BudgetTemplate']
export type BudgetTemplatePage = Schemas['BudgetTemplatePage']
export type BudgetTemplateAssignment = Schemas['BudgetTemplateAssignment']
export type CreateBudgetTemplateRequest = Schemas['CreateBudgetTemplateRequest']
export type UpdateBudgetTemplateRequest = Schemas['UpdateBudgetTemplateRequest']
export type AssignBudgetTemplateRequest = Schemas['AssignBudgetTemplateRequest']
export type MeterQuantity = Schemas['MeterQuantity']
export type BudgetContext = Schemas['BudgetContext']
export type UsageTrendPoint = Schemas['UsageTrendPoint']
export type UsageTrend = Schemas['UsageTrend']
export type UsageDistributionItem = Schemas['UsageDistributionItem']
export type UsageDistribution = Schemas['UsageDistribution']
export type UserUsageView = Schemas['UserUsageView']
export type UserUsagePage = Schemas['UserUsagePage']
export type RequestUsageS02 = Schemas['RequestUsageView']
export type ReconciliationView = Schemas['ReconciliationView']
export type ReconciliationPage = Schemas['ReconciliationPage']
export type ResolveReconciliationRequest = Schemas['ResolveReconciliationRequest']

export const capabilities: BudgetCapability[] = ['MODEL', 'TTS', 'ASR', 'MCP', 'IMAGE_GENERATION']
export const budgetPeriods: BudgetPeriod[] = ['DAY', 'WEEK', 'MONTH', 'LIFETIME']
export const pricingMeters: PricingMeter[] = [
  'INPUT_TOKENS', 'OUTPUT_TOKENS', 'CACHED_TOKENS', 'TOTAL_TOKENS',
  'CHARACTERS', 'AUDIO_SECONDS', 'REQUESTS', 'REQUESTED_IMAGES',
]

const maxBudgetLimit = 9223372036854775807n
const budgetLimitMultipliers: Record<string, bigint> = { '': 1n, k: 1000n, m: 1000000n }

/**
 * Accept a plain non-negative integer or the concise decimal suffixes used by
 * the Admin form. The API and durable state continue to receive only the exact
 * base-10 integer string.
 */
export function normalizeBudgetLimitInput(input: string): string | undefined {
  const match = input.trim().match(/^(0|[1-9]\d*)([kKmM]?)$/)
  if (!match) return undefined
  const value = BigInt(match[1]!) * budgetLimitMultipliers[match[2]!.toLowerCase()]!
  return value <= maxBudgetLimit ? value.toString() : undefined
}

export function canonicalBudgetLimits(limits: BudgetLimitDefinition[]): BudgetLimitDefinition[] {
  return limits.map(limit => ({ ...limit, limit: normalizeBudgetLimitInput(limit.limit)! }))
}

type ComparableBudgetRule = Pick<BudgetTemplateRule, 'mode' | 'limits'>

export function budgetRuleValueKey(rule: ComparableBudgetRule | undefined): string {
  if (!rule) return 'DEFAULT'
  if (rule.mode === 'UNLIMITED') return 'UNLIMITED'
  const limits = rule.limits
    .map(({ period, meter, limit }) => `${period}:${meter}:${limit}`)
    .sort()
  return `LIMITED:${limits.join('|')}`
}

export const clientProtocols = [
  'OPENAI_CHAT_COMPLETIONS',
  'OPENAI_RESPONSES',
  'ANTHROPIC_MESSAGES',
  'GOOGLE_GENERATE_CONTENT',
  'OPENAI_IMAGES_GENERATIONS',
  'DASHSCOPE_MULTIMODAL_GENERATION',
  'OPENAI_AUDIO_SPEECH',
  'GEMINI_GENERATE_CONTENT_TTS',
  'MIMO_CHAT_COMPLETIONS_TTS',
  'OPENAI_AUDIO_TRANSCRIPTIONS',
  'DASHSCOPE_HTTP_ASR',
  'OPENAI_REALTIME_TRANSCRIPTION',
  'DASHSCOPE_REALTIME_ASR',
  'MCP_STREAMABLE_HTTP',
] as const satisfies readonly UsageClientProtocol[]

export function metersForCapability(capability: BudgetCapability): PricingMeter[] {
  switch (capability) {
    case 'MODEL': return ['TOTAL_TOKENS', 'INPUT_TOKENS', 'OUTPUT_TOKENS', 'CACHED_TOKENS', 'REQUESTS']
    case 'TTS': return ['CHARACTERS', 'AUDIO_SECONDS', 'REQUESTS']
    case 'ASR': return ['AUDIO_SECONDS', 'REQUESTS']
    case 'MCP': return ['REQUESTS']
    case 'IMAGE_GENERATION': return ['REQUESTS', 'REQUESTED_IMAGES']
  }
}
