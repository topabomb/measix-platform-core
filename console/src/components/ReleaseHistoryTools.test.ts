import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { Quasar, QBtn, QCard, QCardSection, QCardActions, QBanner, QSelect, QInput, QToggle } from 'quasar'
import ReleaseHistoryTools from './ReleaseHistoryTools.vue'
import { useSessionStore } from '../stores/session'
import * as client from '../api/client'

function setup() {
  const pinia = createPinia(); setActivePinia(pinia)
  useSessionStore().session = { user: { userId: 'usr_admin', displayName: 'Admin', role: 'ADMIN' }, csrfToken: 'csrf', expiresAt: '2027-01-01T00:00:00Z' }
  return mount(ReleaseHistoryTools, { props: { selection: ['rel_old'] }, global: { plugins: [[Quasar, { components: { QBtn, QCard, QCardSection, QCardActions, QBanner, QSelect, QInput, QToggle } }], pinia], stubs: { QDialog: { template: '<div><slot /></div>' } } } })
}
const preview = { previewHash: 'preview-one', candidates: [{ releaseId: 'rel_old', managedGeneration: 1, status: 'SUPERSEDED', bytes: 1024, reason: '' }], protected: [{ releaseId: 'rel_active', managedGeneration: 2, status: 'ACTIVE', bytes: 0, reason: 'active' }], reclaimableBytes: 1024, hasMore: false, activationBlocked: false, releasedUpstreamIds: ['ups_old'] }
describe('Release history cleanup', () => {
  beforeEach(() => { vi.restoreAllMocks() })
  it('requires a server preview and consumes it after a stale or uncertain execution', async () => {
    const fetch = vi.spyOn(client, 'apiFetch').mockImplementation(async (path) => {
      if (path.endsWith(':preview')) return structuredClone(preview)
      if (path.endsWith('/cleanup')) throw new client.ApiProblem(409, 'stale_release_cleanup_preview', 'Preview again')
      return {}
    })
    const wrapper = setup()
    const execute = () => wrapper.findAllComponents(QBtn).find(b => b.attributes('data-cy') === 'release-cleanup-execute')!
    expect(execute().props('disable')).toBe(true)
    await wrapper.get('[data-cy="release-cleanup-open"]').trigger('click'); await flushPromises()
    await wrapper.get('[data-cy="release-cleanup-preview"]').trigger('click'); await flushPromises()
    expect(fetch.mock.calls.filter(([path]) => path.endsWith('/cleanup'))).toHaveLength(0)
    expect(wrapper.get('[data-cy="release-cleanup-protected"]').text()).toContain('Current release')
    expect(execute().props('disable')).toBe(false)
    await wrapper.get('[data-cy="release-cleanup-execute"]').trigger('click'); await flushPromises()
    const call = fetch.mock.calls.find(([path]) => path.endsWith('/cleanup'))!
    expect(JSON.parse(call[1]!.body as string)).toEqual({ selection: { releaseIds: ['rel_old'] }, previewHash: 'preview-one' })
    expect(call[2]).toBe('csrf')
    expect(execute().props('disable')).toBe(true)
    expect(fetch.mock.calls.filter(([path]) => path.endsWith('/cleanup'))).toHaveLength(1)
    expect(wrapper.emitted('cleaned')).toHaveLength(1)
    wrapper.unmount()
  })
  it('cannot execute while runtime activation is unresolved', async () => {
    vi.spyOn(client, 'apiFetch').mockResolvedValue({ ...preview, activationBlocked: true })
    const wrapper = setup()
    await wrapper.get('[data-cy="release-cleanup-open"]').trigger('click'); await flushPromises()
    await wrapper.get('[data-cy="release-cleanup-preview"]').trigger('click'); await flushPromises()
    expect(wrapper.findAllComponents(QBtn).find(b => b.attributes('data-cy') === 'release-cleanup-execute')!.props('disable')).toBe(true)
    wrapper.unmount()
  })
})
