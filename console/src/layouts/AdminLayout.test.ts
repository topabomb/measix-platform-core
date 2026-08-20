import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import {
  Quasar, QLayout, QHeader, QToolbar, QBtn, QDrawer, QList, QItem,
  QItemSection, QItemLabel, QPageContainer, QIcon, QToolbarTitle, QBadge, QChip, QMenu,
} from 'quasar'
import { createPinia, setActivePinia } from 'pinia'
import { createRouter, createMemoryHistory } from 'vue-router'
import AdminLayout from './AdminLayout.vue'
import * as client from '../api/client'

function mountLayout() {
  const pinia = createPinia()
  setActivePinia(pinia)
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/login', component: { template: '<div/>' } },
      { path: '/', component: { template: '<router-view/>' }, children: [
        { path: '', name: 'Overview', component: { template: '<div>overview</div>' } },
        { path: 'users', name: 'Users', component: { template: '<div>users</div>' } },
        { path: 'resources', name: 'Resources', component: { template: '<div>resources</div>' } },
        { path: 'upstreams', name: 'Upstreams', component: { template: '<div>upstreams</div>' } },
        { path: 'releases', name: 'Releases', component: { template: '<div>releases</div>' } },
        { path: 'usage', name: 'Usage', component: { template: '<div>usage</div>' } },
        { path: 'system', name: 'System', component: { template: '<div>system</div>' } },
      ] },
    ],
  })
  router.push('/')
  const wrapper = mount(AdminLayout, {
    global: {
      plugins: [[Quasar, { components: { QLayout, QHeader, QToolbar, QBtn, QDrawer, QList, QItem, QItemSection, QItemLabel, QPageContainer, QIcon, QToolbarTitle, QBadge, QChip, QMenu } }], pinia, router],
    },
  })
  return { wrapper, pinia, router }
}

describe('AdminLayout', () => {
  beforeEach(() => {
    // System health polling hits the Hub; neutralise it in tests.
    vi.spyOn(client, 'apiFetch').mockResolvedValue(undefined as never)
  })

  it('renders all seven S0.1 primary navigation entries', async () => {
    const { wrapper } = mountLayout()
    await flushPromises()
    const labels = wrapper.findAllComponents(QItem).map((i) => i.text())
    for (const expected of ['Overview', 'Users', 'Resources', 'Upstreams', 'Releases', 'Usage', 'System']) {
      expect(labels.some((l) => l.includes(expected))).toBe(true)
    }
  })

  it('renders each primary nav item with a stable route link', async () => {
    const { wrapper } = mountLayout()
    await flushPromises()
    const items = wrapper.findAllComponents(QItem)
    const resources = items.find((i) => i.text().includes('Resources'))
    expect(resources).toBeTruthy()
    expect((resources!.props('to'))).toBe('/resources')
  })

  it('mounts a single global QLayout shell (no nested layout divergence)', async () => {
    const { wrapper } = mountLayout()
    await flushPromises()
    expect(wrapper.findComponent(QLayout).exists()).toBe(true)
  })
})
