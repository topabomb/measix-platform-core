import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import {
  ClosePopup, QBanner, QBtn, QCard, QCardActions, QCardSection, QChip,
  QDialog, QIcon, QInput, QItem, QItemLabel, QItemSection, QList,
  QOptionGroup, QSpinner, Quasar,
} from 'quasar'
import { createPinia, setActivePinia } from 'pinia'
import { i18n } from '../i18n'
import * as client from '../api/client'
import { useSessionStore } from '../stores/session'
import UsageReconciliationPanel from './UsageReconciliationPanel.vue'

let wrapper: VueWrapper | undefined

function mountPanel() {
  const pinia = createPinia()
  setActivePinia(pinia)
  const session = useSessionStore(pinia)
  session.session = {
    user: { userId: 'usr_admin', displayName: 'Admin', role: 'ADMIN' },
    csrfToken: 'csrf',
    expiresAt: '2026-12-31T00:00:00Z',
  }
  wrapper = mount(UsageReconciliationPanel, {
    attachTo: document.body,
    global: {
      plugins: [
        [Quasar, {
          components: {
            QBanner, QBtn, QCard, QCardActions, QCardSection, QChip,
            QDialog, QIcon, QInput, QItem, QItemLabel, QItemSection, QList,
            QOptionGroup, QSpinner,
          },
          directives: { ClosePopup },
        }],
        pinia,
        i18n,
      ],
    },
  })
  return wrapper
}

describe('UsageReconciliationPanel', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    i18n.global.locale.value = 'en'
  })
  afterEach(() => {
    wrapper?.unmount()
    wrapper = undefined
    document.body.innerHTML = ''
  })

  it('requires an audited reason and sends the explicit release action', async () => {
    let reads = 0
    const fetch = vi.spyOn(client, 'apiFetch').mockImplementation(async (_path, init) => {
      if (init?.method === 'POST') {
        return {
          reconciliationId: 'rec_1', requestId: 'req_1', userId: 'usr_1',
          capability: 'MODEL', state: 'RESOLVED', reservation: [], observed: [],
          createdAt: '2026-09-20T00:00:00Z', resolvedAt: '2026-09-20T01:00:00Z',
        }
      }
      reads++
      return reads === 1
        ? {
            items: [{
              reconciliationId: 'rec_1', requestId: 'req_1', userId: 'usr_1',
              capability: 'MODEL', state: 'PENDING',
              reservation: [{ meter: 'TOTAL_TOKENS', quantity: '100' }],
              observed: [{ meter: 'TOTAL_TOKENS', quantity: '75' }],
              createdAt: '2026-09-20T00:00:00Z',
            }],
          }
        : { items: [] }
    })
    const panel = mountPanel()
    await flushPromises()

    expect(panel.text()).toContain('req_1')
    expect(panel.text()).toContain('Observed Total tokens: 75 tokens')
    expect(panel.text()).toContain('Reserved Total tokens: 100 tokens')
    await panel.findAllComponents(QBtn).find(button => button.props('label') === 'Resolve')!.trigger('click')
    await flushPromises()
    const dialog = document.body.querySelector('[data-cy="reconciliation-dialog"]')!
    expect(dialog).toBeTruthy()
    const confirm = document.body.querySelector('[data-cy="confirm-reconciliation"]') as HTMLButtonElement
    expect(confirm.disabled).toBe(true)
    const release = document.body.querySelectorAll<HTMLElement>('.q-radio')[1]
    expect(release).toBeTruthy()
    release.click()
    const reason = document.body.querySelector('textarea') as HTMLTextAreaElement
    reason.value = 'Provider stream ended without final usage'
    reason.dispatchEvent(new Event('input', { bubbles: true }))
    await flushPromises()
    expect(document.body.textContent).toContain('Release the uncertain reservation')
    expect(confirm.disabled).toBe(false)
    confirm.click()
    await flushPromises()

    const call = fetch.mock.calls.find(([, init]) => init?.method === 'POST')!
    expect(call[0]).toBe('/api/admin/v1/usage/reconciliations/req_1:resolve')
    expect(JSON.parse(String(call[1]?.body))).toEqual({
      expectedState: 'PENDING',
      action: 'RELEASE_UNCERTAIN',
      reason: 'Provider stream ended without final usage',
    })
    expect(call[2]).toBe('csrf')
    expect(panel.text()).not.toContain('req_1')
  })
})
