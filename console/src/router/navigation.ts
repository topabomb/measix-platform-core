// Navigation registry (implementation §3.2): drives the primary navigation from
// stable route metadata instead of a hardcoded array in AdminLayout. S0.1 keeps
// the exact product IA: Overview / Users / Resources / Upstreams / Releases /
// Usage / System — no coming-soon entries.

export interface NavItem {
  /** Stable route id — matches the router route name. */
  id: string
  label: string
  icon: string
  /** Display order in the primary nav. */
  order: number
  /** Absolute path within /admin/. */
  path: string
  /** Visible when a capability is actually present. Always true in S0.1. */
  visible: boolean
}

export const NAV_ITEMS: NavItem[] = [
  { id: 'Overview', label: 'Overview', icon: 'dashboard', order: 0, path: '/', visible: true },
  { id: 'Users', label: 'Users', icon: 'group', order: 10, path: '/users', visible: true },
  { id: 'Resources', label: 'Resources', icon: 'hub', order: 20, path: '/resources', visible: true },
  { id: 'Upstreams', label: 'Upstreams', icon: 'cloud', order: 30, path: '/upstreams', visible: true },
  { id: 'Releases', label: 'Releases', icon: 'rocket_launch', order: 40, path: '/releases', visible: true },
  { id: 'EnterpriseUpdates', label: 'Enterprise Updates', icon: 'campaign', order: 45, path: '/enterprise-updates', visible: true },
  { id: 'Usage', label: 'Usage', icon: 'query_stats', order: 50, path: '/usage', visible: true },
  { id: 'System', label: 'System', icon: 'monitor_heart', order: 60, path: '/system', visible: true },
]

/** Primary navigation entries, ordered for display. */
export function visibleNavItems(): NavItem[] {
  return NAV_ITEMS
    .filter((item) => item.visible)
    .slice()
    .sort((a, b) => a.order - b.order)
}

/** Look up a nav entry by route name/id. */
export function navItemById(id: string): NavItem | undefined {
  return NAV_ITEMS.find((item) => item.id === id)
}
