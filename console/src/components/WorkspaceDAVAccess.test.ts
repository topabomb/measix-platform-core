import { afterEach, expect, it, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia } from 'pinia'
import { Quasar, QBtn, QSelect, QDialog } from 'quasar'
import WorkspaceDAVAccess from './WorkspaceDAVAccess.vue'
import { useSessionStore } from '../stores/session'
import * as client from '../api/client'
import type { components } from '../api/generated'

afterEach(() => vi.restoreAllMocks())

it('keeps an edited permission during polling and retries the same CAS command idempotently', async () => {
  const pinia = createPinia()
  useSessionStore(pinia).session = { csrfToken: 'test-csrf' } as components['schemas']['AdminSession']
  const api = vi.spyOn(client, 'apiFetch').mockRejectedValueOnce(new Error('network')).mockResolvedValue({ operationId: 'wop_test' })
  const workspace: components['schemas']['WorkspaceProjection'] = { schemaVersion: 1, bindingRevision: 7, serviceState: 'ENABLED', state: 'CONNECTED', mcpAvailable: true, mcpReason: "available", filesAvailable: true, filesReason: "available", webdav: { resourceId: 'dav_test', available: true, accessMode: 'FULL', desiredAccessMode: 'FULL', desiredBindingRevision: 2, reason: 'available' } }
  const wrapper = mount(WorkspaceDAVAccess, { props: { workspace, baseUrl: '/test/workspace', disabled: false }, global: { plugins: [pinia, [Quasar, { components: { QBtn } }]], stubs: { ProblemBanner: true, QDialog: { name: 'QDialog', props: ['modelValue'], template: '<div v-if="modelValue"><slot /></div>' } } } })
  try {
    wrapper.findComponent(QSelect).vm.$emit('update:model-value', 'READ_ONLY')
    await wrapper.setProps({ workspace: { ...workspace, webdav: { ...workspace.webdav!, desiredBindingRevision: 3 } } })
    expect(wrapper.findComponent(QSelect).props('modelValue')).toBe('READ_ONLY')
    await wrapper.findComponent(QBtn).trigger('click'); await flushPromises()
    expect(api).not.toHaveBeenCalled()
    expect(wrapper.findComponent(QDialog).props('modelValue')).toBe(true)
    await wrapper.find('[data-cy="confirm-dav-downgrade"]').trigger('click'); await flushPromises()
    expect(JSON.parse(api.mock.calls[0]![1]!.body as string)).toEqual({ action: 'SET_DAV_ACCESS', expectedRevision: 7, expectedDAVRevision: 3, davAccessMode: 'READ_ONLY' })
    await wrapper.setProps({ workspace: { ...workspace, bindingRevision: 8, webdav: { ...workspace.webdav!, desiredAccessMode: 'READ_ONLY', desiredBindingRevision: 4 } } })
    expect(wrapper.findComponent(QSelect).props('disable')).toBe(true)
    await wrapper.findComponent(QBtn).trigger('click'); await flushPromises()
    expect(api.mock.calls[1]![1]!.headers).toEqual(api.mock.calls[0]![1]!.headers)
    expect(api.mock.calls[1]![1]!.body).toEqual(api.mock.calls[0]![1]!.body)
    expect(wrapper.emitted('changed')).toHaveLength(1)
    await wrapper.setProps({ disabled: true })
    expect(wrapper.findComponent(QSelect).props('disable')).toBe(true)
    expect(wrapper.findComponent(QBtn).props('disable')).toBe(true)
  } finally { wrapper.unmount() }
})
