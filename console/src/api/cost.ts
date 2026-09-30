import type { components } from './generated'

type CostAnalysis = components['schemas']['CostAnalysis']

export function costAmounts(cost: CostAnalysis | undefined): string {
  if (!cost?.amounts?.length) {
    if (cost?.amount && cost.currency) return `${cost.amount} ${cost.currency}`
    if (cost?.status === 'KNOWN' && cost.pricedRequests) return '0'
    return '—'
  }
  return cost.amounts.map(item => `${item.amount} ${item.currency}`).join(' · ')
}
