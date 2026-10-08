import { describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { Quasar, QBanner, QBtn, QCard, QCardActions, QCardSection, QDialog, QInput, QSeparator } from 'quasar'
import AdminAccountDialog from './AdminAccountDialog.vue'
import { i18n } from '../i18n'

function mountDialog(mode: 'password' | 'role', passwordConfigured = false) {
  return mount(AdminAccountDialog, {
    props: { modelValue: true, mode, csrfToken: 'csrf-token', user: { userId: 'usr_target', username: 'target', displayName: 'Target', role: 'MEMBER', status: 'ACTIVE', passwordConfigured, createdAt: '2026-10-07T00:00:00Z', updatedAt: '2026-10-07T00:00:00Z' } },
    global: { plugins: [[Quasar, { components: { QBanner, QBtn, QCard, QCardActions, QCardSection, QDialog, QInput, QSeparator } }]], stubs: { QDialog: { template: '<div><slot /></div>' } } },
  })
}

describe('AdminAccountDialog', () => {
  it.each([['en', 'Make administrator'], ['zh', '设为管理员']] as const)('names the action in %s', async (locale, label) => {
    const previous = i18n.global.locale.value
    i18n.global.locale.value = locale
    try {
      const wrapper = mountDialog('role')
      expect(wrapper.get('[data-cy="account-submit"]').text()).toBe(label)
      wrapper.unmount()
    } finally { i18n.global.locale.value = previous }
  })

  it('treats a server failure as an unknown command result', async () => {
    const fetchMock = vi.fn(async () => Response.json({ code: 'internal_error' }, { status: 502 }))
    vi.stubGlobal('fetch', fetchMock)
    const wrapper = mountDialog('password')
    const inputs = wrapper.findAllComponents(QInput)
    await inputs[0]!.setValue('synthetic actor password')
    await inputs[1]!.setValue('synthetic new password')
    await inputs[2]!.setValue('synthetic new password')
    await wrapper.get('[data-cy="account-submit"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-cy="account-result-uncertain"]').exists()).toBe(true)
    expect(inputs.every(input => input.props('modelValue') === '')).toBe(true)
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it('requires initialization on promotion, sends CAS and CSRF, then clears all passwords', async () => {
    const fetchMock = vi.fn(async () => Response.json({ userId: 'usr_target', role: 'ADMIN', passwordConfigured: true }))
    vi.stubGlobal('fetch', fetchMock)
    const wrapper = mountDialog('role')
    const inputs = wrapper.findAllComponents(QInput)
    await inputs[0]!.setValue('synthetic actor password')
    expect((wrapper.get('[data-cy="account-submit"]').element as HTMLButtonElement).disabled).toBe(true)
    await inputs[1]!.setValue('synthetic new password')
    await inputs[2]!.setValue('synthetic new password')
    await wrapper.get('[data-cy="account-submit"]').trigger('click')
    await flushPromises()
    const [path, init] = fetchMock.mock.calls[0] as unknown as [string, RequestInit]
    expect(path).toBe('/api/admin/v1/users/usr_target:set-role')
    expect(new Headers(init.headers).get('X-CSRF-Token')).toBe('csrf-token')
    expect(JSON.parse(init.body as string)).toEqual({ role: 'ADMIN', expectedRole: 'MEMBER', currentPassword: 'synthetic actor password', newPassword: 'synthetic new password', confirmPassword: 'synthetic new password' })
    expect(inputs.every(input => input.props('modelValue') === '')).toBe(true)
    expect(wrapper.emitted('completed')?.[0]?.[0]).toBe('usr_target')
  })

  it('preserves an existing password when promoting and clears the operator password on cancel', async () => {
    const wrapper = mountDialog('role', true)
    const inputs = wrapper.findAllComponents(QInput)
    expect(inputs).toHaveLength(1)
    await inputs[0]!.setValue('synthetic actor password')
    await wrapper.findAllComponents(QBtn).find(button => button.props('label') === 'Cancel')!.trigger('click')
    expect(inputs[0]!.props('modelValue')).toBe('')
    expect(wrapper.emitted('update:modelValue')).toEqual([[false]])
  })

  it('clears passwords on an unknown result and does not replay the command', async () => {
    const fetchMock = vi.fn(async () => { throw new TypeError('connection closed') })
    vi.stubGlobal('fetch', fetchMock)
    const wrapper = mountDialog('password')
    const inputs = wrapper.findAllComponents(QInput)
    await inputs[0]!.setValue('synthetic actor password')
    await inputs[1]!.setValue('synthetic new password')
    await inputs[2]!.setValue('synthetic new password')
    await wrapper.get('[data-cy="account-submit"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-cy="account-result-uncertain"]').exists()).toBe(true)
    expect(inputs.every(input => input.props('modelValue') === '')).toBe(true)
    await wrapper.get('[data-cy="account-submit"]').trigger('click')
    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(wrapper.emitted('completed')).toBeUndefined()
  })

  it('reloads on a role conflict and clears the credentials', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => Response.json({ code: 'user_role_conflict', message: 'Role changed', status: 409 }, { status: 409 })))
    const wrapper = mountDialog('role', true)
    const input = wrapper.findComponent(QInput)
    await input.setValue('synthetic actor password')
    await wrapper.get('[data-cy="account-submit"]').trigger('click')
    await flushPromises()
    expect(wrapper.emitted('conflict')?.[0]?.[0]).toBe('usr_target')
    expect(input.props('modelValue')).toBe('')
  })
})
