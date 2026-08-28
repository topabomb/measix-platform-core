import { describe, expect, it } from 'vitest'
import { navItemById, visibleNavItems } from './navigation'

describe('navigation registry', () => {
  it('exposes exactly the S0.1 product IA entries in order', () => {
    const items = visibleNavItems()
    expect(items.map((i) => i.id)).toEqual([
      'Overview',
      'Users',
      'Resources',
      'Upstreams',
      'Releases',
      'EnterpriseUpdates',
      'Usage',
      'System',
    ])
  })

  it('resolves an entry by stable route id', () => {
    expect(navItemById('Upstreams')?.path).toBe('/upstreams')
    expect(navItemById('Missing')).toBeUndefined()
  })

  it('sorts by order and keeps only visible entries', () => {
    const items = visibleNavItems()
    const orders = items.map((i) => i.order)
    expect([...orders].sort((a, b) => a - b)).toEqual(orders)
    for (const item of items) expect(item.visible).toBe(true)
  })
})
