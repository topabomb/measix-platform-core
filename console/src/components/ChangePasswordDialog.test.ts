import { describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { Quasar, QBanner, QBtn, QCard, QCardActions, QCardSection, QCheckbox, QDialog, QInput, QSeparator } from 'quasar'
import ChangePasswordDialog from './ChangePasswordDialog.vue'

function mountDialog() {
  return mount(ChangePasswordDialog, {
    props: { modelValue: true, csrfToken: 'csrf-token' },
    global: {
      plugins: [[Quasar, { components: { QBanner, QBtn, QCard, QCardActions, QCardSection, QCheckbox, QDialog, QInput, QSeparator } }]],
      stubs: {
        QDialog: { template: '<div><slot /></div>' },
      },
    },
  })
}

describe('ChangePasswordDialog', () => {
  it('requires a valid confirmed password before submission', async () => {
    const wrapper = mountDialog()
    const submit = wrapper.find('[data-cy="change-password-submit"]')
    expect((submit.element as HTMLButtonElement).disabled).toBe(true)
    const inputs = wrapper.findAllComponents(QInput)
    expect(inputs).toHaveLength(3)

    await inputs[0]!.setValue('current password value')
    await inputs[1]!.setValue('new password value')
    await inputs[2]!.setValue('different value')
    expect((submit.element as HTMLButtonElement).disabled).toBe(true)

    await inputs[2]!.setValue('new password value')
    expect((submit.element as HTMLButtonElement).disabled).toBe(false)
  })

  it('sends the CSRF-protected request and emits changed', async () => {
    const fetchMock = vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) => new Response(null, { status: 204 }))
    vi.stubGlobal('fetch', fetchMock)
    const wrapper = mountDialog()
    const inputs = wrapper.findAllComponents(QInput)
    await inputs[0]!.setValue('current password value')
    await inputs[1]!.setValue('new password value')
    await inputs[2]!.setValue('new password value')
    await wrapper.find('[data-cy="change-password-submit"]').trigger('click')
    await flushPromises()

    expect(fetchMock).toHaveBeenCalledWith('/api/admin/v1/session:change-password', expect.objectContaining({
      method: 'POST', credentials: 'same-origin',
    }))
    const init = fetchMock.mock.calls[0]![1] as RequestInit
    expect(new Headers(init.headers).get('X-CSRF-Token')).toBe('csrf-token')
    expect(wrapper.emitted('changed')).toHaveLength(1)
  })
})
