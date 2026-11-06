import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { Quasar, QChip } from 'quasar'
import HealthIndicator from './HealthIndicator.vue'
import * as client from '../api/client'

const hash = (fill: string) => `sha256:${fill.repeat(64)}`

const base = {
  buildVersion: 'v0.1.0',
  dbHealth: 'OK',
  schemaIdentity: hash('a'),
  runtimeStatus: 'READY',
  activeManagedGeneration: 1,
  managedStateRevision: 1,
  desiredControlRevision: 4,
  appliedControlRevision: 4,
  desiredBundleHash: hash('b'),
  appliedBundleHash: hash('b'),
  relayReady: true,
}

function mountIndicator() {
  return mount(HealthIndicator, { global: { plugins: [[Quasar, { components: { QChip } }]] } })
}

// HealthIndicator is mounted against the real composable so the degraded /
// unconfigured / relay wiring is exercised. Mocking useSystemHealth would only
// prove that a mock renders.
describe('HealthIndicator', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
  })

  it('does not claim the runtime is ready while status is unknown', async () => {
    vi.spyOn(client, 'apiFetch').mockRejectedValue(new Error('hub unavailable'))
    const wrapper = mountIndicator()
    await flushPromises()
    expect(wrapper.text()).toMatch(/unknown/i)
    expect(wrapper.text()).not.toMatch(/ready/i)
    wrapper.unmount()
  })

  it('shows a calm READY chip when the published release is applied', async () => {
    vi.spyOn(client, 'apiFetch').mockResolvedValue(base)
    const wrapper = mountIndicator()
    await flushPromises()
    expect(wrapper.text()).toMatch(/ready/i)
    expect(wrapper.text()).not.toMatch(/relay/i)
    wrapper.unmount()
  })

  it('reports an unpublished deployment as not configured, not a Relay failure', async () => {
    vi.spyOn(client, 'apiFetch').mockResolvedValue({
      ...base, activeManagedGeneration: 0, desiredControlRevision: 0, appliedControlRevision: 0,
    })
    const wrapper = mountIndicator()
    await flushPromises()
    expect(wrapper.text()).toMatch(/no published configuration/i)
    expect(wrapper.text()).not.toMatch(/relay not ready/i)
    wrapper.unmount()
  })

  it('reports Relay not ready only when an applied state actually diverges', async () => {
    vi.spyOn(client, 'apiFetch').mockResolvedValue({ ...base, appliedControlRevision: 3 })
    const wrapper = mountIndicator()
    await flushPromises()
    expect(wrapper.text()).toMatch(/relay not ready/i)
    wrapper.unmount()
  })

  it('reports runtime degraded when the database health is not OK', async () => {
    vi.spyOn(client, 'apiFetch').mockResolvedValue({ ...base, dbHealth: 'DEGRADED' })
    const wrapper = mountIndicator()
    await flushPromises()
    expect(wrapper.text()).toMatch(/runtime degraded/i)
    wrapper.unmount()
  })
})
