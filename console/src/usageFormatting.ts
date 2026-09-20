import type { PricingMeter } from './api/usageBudget'

export type MeterUnitLabels = Record<'tokens' | 'characters' | 'seconds' | 'minutes' | 'requests' | 'images', string>

function integer(value: string): bigint | undefined {
  try {
    return /^\d+$/.test(value) ? BigInt(value) : undefined
  } catch {
    return undefined
  }
}

export function formatInteger(value: string, locale: string, compact: boolean): string {
  const parsed = integer(value)
  if (parsed === undefined) return value
  return new Intl.NumberFormat(locale, compact
    ? { notation: 'compact', maximumFractionDigits: 1 }
    : { maximumFractionDigits: 0 }).format(parsed)
}

export function meterUnit(meter: PricingMeter, labels: MeterUnitLabels): string {
  if (meter.includes('TOKEN')) return labels.tokens
  if (meter === 'CHARACTERS') return labels.characters
  if (meter === 'AUDIO_SECONDS') return labels.seconds
  if (meter === 'REQUESTED_IMAGES') return labels.images
  return labels.requests
}

export function formatMeter(
  quantity: string,
  meter: PricingMeter,
  locale: string,
  labels: MeterUnitLabels,
  compact = true,
): string {
  const parsed = integer(quantity)
  if (meter === 'AUDIO_SECONDS' && parsed !== undefined && compact && parsed >= 60n) {
    const minutes = Number(parsed) / 60
    return `${new Intl.NumberFormat(locale, { maximumFractionDigits: 1 }).format(minutes)} ${labels.minutes}`
  }
  return `${formatInteger(quantity, locale, compact)} ${meterUnit(meter, labels)}`
}
