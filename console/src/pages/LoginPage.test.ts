import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import {
  Quasar, QLayout, QPage, QPageContainer,
  QCard, QCardSection, QCardActions, QInput, QBtn, QBanner,
} from 'quasar'
import { createPinia } from 'pinia'
import { createRouter, createMemoryHistory } from 'vue-router'
import LoginPage from './LoginPage.vue'

// LoginPage renders outside AdminLayout (route without layout), so it must
// provide its own QLayout -> QPageContainer -> QPage hierarchy. Rendering a
// bare QPage produced the production white-screen regression: "QPage needs to
// be a deep child of QLayout". This spec pins that regression.
let errors: string[] = []

beforeEach(() => {
  errors = []
  vi.spyOn(console, 'error').mockImplementation((...args: unknown[]) => {
    errors.push(args.map((a) => String(a)).join(' '))
  })
})

function mountLoginPage() {
  const pinia = createPinia()
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/', component: { template: '<div>home</div>' } }],
  })
  return mount(LoginPage, {
    global: {
      plugins: [[Quasar, {
        components: { QLayout, QPage, QPageContainer, QCard, QCardSection, QCardActions, QInput, QBtn, QBanner },
      }], pinia, router],
    },
  })
}

describe('LoginPage', () => {
  it('renders standalone with its own QLayout so QPage has a valid parent (no white screen)', () => {
    const wrapper = mountLoginPage()
    expect(wrapper.findComponent(QLayout).exists()).toBe(true)
    expect(wrapper.findComponent(QPage).exists()).toBe(true)
    const pageErrors = errors.filter((e) => e.includes('QPage needs to be a deep child of QLayout'))
    expect(pageErrors).toEqual([])
  })

  it('shows username/password inputs and disables Sign in until both are filled', async () => {
    const wrapper = mountLoginPage()
    expect(wrapper.findAll('input').length).toBe(2)
    expect(wrapper.find('input[type="password"]').exists()).toBe(true)
    const button = wrapper.find('button')
    expect((button.element as HTMLButtonElement).disabled).toBe(true)
  })
})
