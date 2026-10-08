import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { ClosePopup, Quasar, QBtn, QInput, QSelect, QCard, QCardSection, QCardActions, QSeparator, QBanner, QChip, QList, QItem, QItemSection, QItemLabel, QIcon, QDialog, QField, QSpinner } from 'quasar'
import { i18n } from '../i18n'
import PricingPanel from './PricingPanel.vue'
import { useSessionStore } from '../stores/session'
import * as client from '../api/client'

const asrId = 'asr_00000000-0000-0000-0000-000000000001'
const original = { pricingRuleId: 'prc_a', meter: 'INPUT_TOKENS', unitSize: '1000000', unitPrice: '2', currency: 'CNY', effectiveFrom: '2026-09-23T04:23:13.991Z', effectiveTo: '2026-12-31T00:00:00Z' }
let mountedPanel: VueWrapper | undefined

function panel() {
  const pinia = createPinia()
  setActivePinia(pinia)
  useSessionStore().session = { user: { userId: 'usr_a', displayName: 'Admin', role: 'ADMIN' }, csrfToken: 'csrf', expiresAt: '2026-12-31T00:00:00Z' }
  mountedPanel = mount(PricingPanel, { attachTo: document.body, global: { plugins: [[Quasar, { components: { QBtn, QInput, QSelect, QCard, QCardSection, QCardActions, QSeparator, QBanner, QChip, QList, QItem, QItemSection, QItemLabel, QIcon, QDialog, QField, QSpinner }, directives: { ClosePopup } }], pinia] } })
  return mountedPanel
}

describe('pricing management workflow', () => {
  afterEach(() => { mountedPanel?.unmount(); mountedPanel = undefined; document.body.innerHTML = '' })
  beforeEach(() => {
    i18n.global.locale.value = 'en'
    vi.restoreAllMocks()
    vi.spyOn(client, 'apiFetch').mockImplementation(async (path, init) => {
      if (path === '/api/admin/v1/draft') return { content: { models: [], imageGenerators: [], tts: [], asr: [{ asrId, displayName: 'Meeting transcription' }], mcp: [] } }
      if (path === '/api/admin/v1/usage/summary') return { cost: { status: 'PARTIAL', amounts: [{ amount: '2', currency: 'CNY' }, { amount: '1', currency: 'USD' }], missingPricingRequests: 1 } }
      if (init?.method === 'PUT') return { pricingRevision: 8, rules: JSON.parse(String(init.body)).rules }
      return { pricingRevision: 7, rules: [structuredClone(original)] }
    })
  })
  it.each([new client.ApiProblem(409, 'pricing_revision_conflict', 'Changed elsewhere'), new TypeError('Response lost')])('requires a fresh review after a failed save (%s)', async cause => {
    let failed = false
    vi.mocked(client.apiFetch).mockImplementation(async (path, init) => {
      if (path.endsWith('/draft')) return { content: { models: [], tts: [], asr: [], mcp: [] } }
      if (path.endsWith('/summary')) return { cost: { status: 'UNKNOWN' } }
      if (init?.method === 'PUT') { failed = true; throw cause }
      return { pricingRevision: failed ? 9 : 7, rules: [{ ...original, unitPrice: failed ? '4' : '2' }] }
    })
    const wrapper = panel()
    await flushPromises()
    await wrapper.get('[data-cy="pricing-edit-rule"]').trigger('click')
    await flushPromises()
    await wrapper.findAllComponents(QInput).find(input => input.props('label') === 'Price')!.setValue('3')
    await wrapper.findAllComponents(QBtn).find(button => button.attributes('data-cy') === 'pricing-save-btn')!.trigger('click')
    await flushPromises()
    expect(wrapper.findAllComponents(QBtn).find(button => button.attributes('data-cy') === 'pricing-save-btn')!.props('disable')).toBe(true)
    await wrapper.findAllComponents(QBtn).find(button => button.props('label') === 'Refresh current rules and review again')!.trigger('click')
    await flushPromises()
    expect(wrapper.findAllComponents(QInput).find(input => input.props('label') === 'Price')!.props('modelValue')).toBe('4')
  })
  it('shows readable summaries and edits only the requested rule', async () => {
    const wrapper = panel()
    await flushPromises()
    const row = wrapper.get('[data-cy="pricing-rule-row"]')
    expect(row.text()).toContain('Global')
    expect(row.text()).toContain('1,000,000')
    expect(row.text()).not.toContain(original.effectiveFrom)
    expect(wrapper.get('[data-cy="pricing-cost-amount"]').text()).toBe('2 CNY · 1 USD')
    expect(wrapper.findAllComponents(QInput)).toHaveLength(0)
    await row.get('[data-cy="pricing-edit-rule"]').trigger('click')
    await flushPromises()
    expect(wrapper.findAllComponents(QInput).some(input => input.props('type') === 'datetime-local')).toBe(true)
    expect(document.body.textContent).toContain('historical estimates')
  })
  it('selects a named resource and offers only its compatible meters', async () => {
    const wrapper = panel()
    await flushPromises()
    await wrapper.get('[data-cy="pricing-add-rule-btn"]').trigger('click')
    await flushPromises()
    const scope = wrapper.findAllComponents(QSelect).find(input => input.props('label') === 'Scope')!
    await scope.setValue('RESOURCE')
    await flushPromises()
    const picker = wrapper.findAllComponents({ name: 'PagedEntityPicker' }).find(input => input.props('label') === 'Resource')!
    const page = await picker.props('fetchPage')('meeting')
    expect(page.items[0]!.label).toBe('Meeting transcription')
    picker.vm.$emit('update:modelValue', asrId)
    await flushPromises()
    const meter = wrapper.findAllComponents(QSelect).find(input => input.props('label') === 'Meter')!
    expect((meter.props('options') ?? []).map((option: { value: string }) => option.value)).toEqual(['AUDIO_SECONDS', 'REQUESTS'])
    expect((meter.props('options') ?? [])[0].label).toBe('Audio (sec)')
    expect(wrapper.findAllComponents(QInput).find(input => input.props('label') === 'Unit size')!.props('modelValue')).toBe('1')
  })
  it('preserves millisecond boundaries and submits the whole set with its revision', async () => {
    const wrapper = panel()
    await flushPromises()
    await wrapper.get('[data-cy="pricing-edit-rule"]').trigger('click')
    await flushPromises()
    await wrapper.findAllComponents(QInput).find(input => input.props('label') === 'Price')!.setValue('3')
    await wrapper.findAllComponents(QBtn).find(button => button.attributes('data-cy') === 'pricing-save-btn')!.trigger('click')
    await flushPromises()
    const put = vi.mocked(client.apiFetch).mock.calls.find(([, init]) => init?.method === 'PUT')!
    const body = JSON.parse(String(put[1]?.body))
    expect(body.expectedPricingRevision).toBe(7)
    expect(body.rules[0]).toEqual({ ...original, unitPrice: '3' })
    expect(put[2]).toBe('csrf')
    expect(vi.mocked(client.apiFetch).mock.calls.filter(([path]) => path === '/api/admin/v1/usage/summary')).toHaveLength(2)
  })
  it('requires review before removing a rule that prices history', async () => {
    const wrapper = panel()
    await flushPromises()
    await wrapper.get('[data-cy="pricing-delete-rule"]').trigger('click')
    await flushPromises()
    expect(vi.mocked(client.apiFetch).mock.calls.some(([, init]) => init?.method === 'PUT')).toBe(false)
    expect(document.body.textContent).toContain('historical estimates')
  })
})
