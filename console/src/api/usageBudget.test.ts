import { describe, expect, it } from 'vitest'
import { budgetRuleValueKey, capabilities, metersForCapability, normalizeBudgetLimitInput, pricingMeters } from './usageBudget'

describe('usage budget capability registry', () => {
  it('keeps image generation as a fifth capability with only request meters', () => {
    expect(capabilities).toEqual(['MODEL', 'TTS', 'ASR', 'MCP', 'IMAGE_GENERATION'])
    expect(metersForCapability('IMAGE_GENERATION')).toEqual(['REQUESTS', 'REQUESTED_IMAGES'])
    expect(pricingMeters).toContain('REQUESTED_IMAGES')
  })

  it('compares effective rule values independently of limit display order', () => {
    const first = { mode: 'LIMITED' as const, limits: [
      { period: 'MONTH' as const, meter: 'REQUESTS' as const, limit: '20' },
      { period: 'DAY' as const, meter: 'REQUESTED_IMAGES' as const, limit: '2' },
    ] }
    const reordered = { mode: 'LIMITED' as const, limits: [...first.limits].reverse() }
    expect(budgetRuleValueKey(first)).toBe(budgetRuleValueKey(reordered))
    expect(budgetRuleValueKey(undefined)).not.toBe(budgetRuleValueKey({ mode: 'UNLIMITED', limits: [] }))
  })

  it('normalizes concise human limit input to the exact integer wire value', () => {
    expect(normalizeBudgetLimitInput('100k')).toBe('100000')
    expect(normalizeBudgetLimitInput('2M')).toBe('2000000')
    expect(normalizeBudgetLimitInput(' 750 ')).toBe('750')
    expect(normalizeBudgetLimitInput('1.5k')).toBeUndefined()
    expect(normalizeBudgetLimitInput('100,000')).toBeUndefined()
    expect(normalizeBudgetLimitInput('01k')).toBeUndefined()
    expect(normalizeBudgetLimitInput('9223372036854775808')).toBeUndefined()
  })
})
