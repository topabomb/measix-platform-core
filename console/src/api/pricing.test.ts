import { describe, expect, it } from 'vitest'
import { localPricingTime, normalizePricingRule, pricingRuleError, pricingTime, resourcePricingMeters, type PricingRule } from './pricing'

const rule: PricingRule = { pricingRuleId: 'prc_a', meter: 'INPUT_TOKENS', unitSize: '1000000', unitPrice: '2', currency: 'CNY', effectiveFrom: '2026-10-08T04:23:13.991Z' }

describe('pricing editor values', () => {
  it('preserves the exact time boundary through local date editing', () => {
    expect(pricingTime(localPricingTime(rule.effectiveFrom))).toBe(rule.effectiveFrom)
    expect(pricingTime('')).toBeUndefined()
  })
  it('clears obsolete scopes instead of submitting an empty or hidden resource ID', () => {
    const normalized = normalizePricingRule({ ...rule, resourceId: 'mdl_a', upstreamId: 'ups_a', currency: ' cny ' }, 'GLOBAL')
    expect(normalized.resourceId).toBeUndefined()
    expect(normalized.upstreamId).toBeUndefined()
    expect(normalized.currency).toBe('CNY')
    expect(normalizePricingRule({ ...rule, unitSize: '1M' }, 'GLOBAL').unitSize).toBe('1000000')
  })
  it('rejects incomplete, incompatible and ambiguous rules before saving', () => {
    expect(pricingRuleError({ ...rule, unitSize: '0' }, 'GLOBAL', [])).toBe('invalidUnit')
    expect(pricingRuleError({ ...rule, unitPrice: '1/3' }, 'GLOBAL', [])).toBe('invalidPrice')
    expect(pricingRuleError(rule, 'RESOURCE', [])).toBe('resourceRequired')
    expect(pricingRuleError({ ...rule, pricingRuleId: 'prc_b' }, 'GLOBAL', [rule])).toBe('duplicateStart')
    expect(pricingRuleError({ ...rule, effectiveTo: rule.effectiveFrom }, 'GLOBAL', [])).toBe('invalidTime')
    expect(resourcePricingMeters('asr_a')).toEqual(['AUDIO_SECONDS', 'REQUESTS'])
  })
})
