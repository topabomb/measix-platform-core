import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import {
  Quasar, QLayout, QPage, QPageContainer,
  QCard, QCardSection, QCardActions, QInput, QBtn, QBanner,
  QSelect, QToggle, QDialog, QSeparator, QTab, QTabs, QBadge, QChip,
  QList, QItem, QItemSection, QItemLabel, QMarkupTable, QSpinner, QIcon,
  QBreadcrumbs, QBreadcrumbsEl, QBtnDropdown, QBtnToggle, QExpansionItem,
  ClosePopup,
} from 'quasar'
import { createPinia, setActivePinia } from 'pinia'
import { createRouter, createMemoryHistory } from 'vue-router'
import { h } from 'vue'
import ResourcesPage from './ResourcesPage.vue'
import { useSessionStore } from '../stores/session'
import { useDraftStore } from '../stores/draft'
import * as client from '../api/client'

function mountResourcesPage() {
  const pinia = createPinia()
  setActivePinia(pinia)
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/', component: { template: '<div/>' } }],
  })
  const wrapper = mount(
    {
      components: { ResourcesPage },
      render() {
        return h(QLayout, {}, () => [
          h(QPageContainer, {}, () => [h(ResourcesPage)]),
        ])
      },
    },
    {
      global: {
        plugins: [[Quasar, {
          components: {
            QLayout, QPage, QPageContainer, QCard, QCardSection, QCardActions,
            QInput, QBtn, QBanner, QSelect, QToggle, QDialog, QSeparator, QTab, QTabs,
            QList, QItem, QItemSection, QItemLabel, QMarkupTable, QChip, QBadge,
            QSpinner, QIcon, QBreadcrumbs, QBreadcrumbsEl, QBtnDropdown, QBtnToggle,
            QExpansionItem,
          },
          directives: { ClosePopup },
        }], pinia, router],
      },
    },
  )
  return { wrapper, pinia }
}

function setupSession(pinia: ReturnType<typeof createPinia>) {
  const session = useSessionStore(pinia)
  session.session = {
    user: { userId: 'usr_001', displayName: 'Admin', role: 'ADMIN' as const },
    csrfToken: 'test-csrf',
    expiresAt: '2026-12-31T23:59:59Z',
  }
  return session
}

const EMPTY_DRAFT = {
  draftRevision: 1,
  content: {
    providers: [],
    models: [],
    tts: [],
    asr: [],
    mcp: [],
    bindings: [],
    policy: {
      policyId: 'pol_draft',
      allowLocalProviders: true,
      allowLocalTts: true,
      allowLocalAsr: true,
      allowLocalMcp: true,
    },
  },
}

function findAddBtn(wrapper: ReturnType<typeof mount>, label: string) {
  return wrapper.findAllComponents(QBtn).find((b) => String(b.props('label') ?? '').includes(label))
}

async function switchTab(wrapper: ReturnType<typeof mount>, name: string) {
  const tab = wrapper.findAllComponents(QTab).find((t) => t.props('name') === name)
  expect(tab).toBeTruthy()
  await tab!.trigger('click')
  await flushPromises()
}

describe('ResourcesPage', () => {
  beforeEach(() => {
    vi.spyOn(client, 'apiFetch').mockImplementation(async (path: string) => {
      if (path === '/api/admin/v1/draft') return structuredClone(EMPTY_DRAFT)
      if (path.startsWith('/api/admin/v1/upstreams')) return { items: [], nextCursor: undefined }
      return {}
    })
  })

  it('renders five resource tabs: Models, TTS, ASR, MCP, Policy', async () => {
    const { wrapper, pinia } = mountResourcesPage()
    setupSession(pinia)
    await flushPromises()

    const tabs = wrapper.findAllComponents(QTab).map((t) => String(t.props('name')))
    for (const expected of ['overview', 'models', 'tts', 'asr', 'mcp', 'policy']) {
      expect(tabs).toContain(expected)
    }
  })

  it('can add a TTS resource through the Add button', async () => {
    const { wrapper, pinia } = mountResourcesPage()
    setupSession(pinia)
    await flushPromises()

    const draft = useDraftStore(pinia)
    expect(draft.localContent?.tts).toHaveLength(0)

    await switchTab(wrapper, 'tts')
    const addTtsBtn = findAddBtn(wrapper, 'Add')
    expect(addTtsBtn).toBeTruthy()
    await addTtsBtn!.trigger('click')
    await flushPromises()

    expect(draft.localContent?.tts).toHaveLength(1)
    expect(draft.localContent?.tts[0].ttsId).toMatch(/^tts_/)
    expect(draft.localContent?.tts[0].clientProtocol).toBe('OPENAI_AUDIO_SPEECH')
    expect(draft.dirty).toBe(true)
  })

  it('can add an ASR resource through the Add button', async () => {
    const { wrapper, pinia } = mountResourcesPage()
    setupSession(pinia)
    await flushPromises()

    const draft = useDraftStore(pinia)
    expect(draft.localContent?.asr).toHaveLength(0)

    await switchTab(wrapper, 'asr')
    const addAsrBtn = findAddBtn(wrapper, 'Add')
    expect(addAsrBtn).toBeTruthy()
    await addAsrBtn!.trigger('click')
    await flushPromises()

    expect(draft.localContent?.asr).toHaveLength(1)
    expect(draft.localContent?.asr[0].asrId).toMatch(/^asr_/)
    expect(draft.localContent?.asr[0].clientProtocol).toBe('OPENAI_AUDIO_TRANSCRIPTIONS')
    expect(draft.dirty).toBe(true)
  })

  it('can add an MCP server through the Add button', async () => {
    const { wrapper, pinia } = mountResourcesPage()
    setupSession(pinia)
    await flushPromises()

    const draft = useDraftStore(pinia)
    expect(draft.localContent?.mcp).toHaveLength(0)

    await switchTab(wrapper, 'mcp')
    const addMcpBtn = findAddBtn(wrapper, 'Add')
    expect(addMcpBtn).toBeTruthy()
    await addMcpBtn!.trigger('click')
    await flushPromises()

    expect(draft.localContent?.mcp).toHaveLength(1)
    expect(draft.localContent?.mcp[0].mcpServerId).toMatch(/^mcp_/)
    expect(draft.localContent?.mcp[0].clientProtocol).toBe('MCP_STREAMABLE_HTTP')
    expect(draft.dirty).toBe(true)
  })

  it('renders Policy editor with toggles for local allow flags', async () => {
    const { wrapper, pinia } = mountResourcesPage()
    setupSession(pinia)
    await flushPromises()

    await switchTab(wrapper, 'policy')
    const toggles = wrapper.findAllComponents(QToggle)
    expect(toggles.length).toBeGreaterThanOrEqual(4)

    const draft = useDraftStore(pinia)
    expect(draft.localContent?.policy?.allowLocalProviders).toBe(true)
  })

  it('shows relationship rows in the Overview tab for each resource to its upstream', async () => {
    const { wrapper, pinia } = mountResourcesPage()
    setupSession(pinia)
    await flushPromises()

    const draft = useDraftStore(pinia)
    draft.localContent!.providers.push({
      providerId: 'prv_openai',
      displayName: 'OpenAI',
      clientProtocol: 'OPENAI_CHAT_COMPLETIONS',
      enabled: true,
    })
    const modelId = draft.addModel('prv_openai')
    draft.setBinding(modelId, 'ups_test', 'HTTP_STREAMING_SSE')
    await flushPromises()

    const html = wrapper.html()
    expect(html).toContain('Resource → Upstream relationships')
    expect(html).toContain('ups_test')
  })

  it('renders an enable toggle for each resource in the relationship view', async () => {
    const { wrapper, pinia } = mountResourcesPage()
    setupSession(pinia)
    await flushPromises()

    const draft = useDraftStore(pinia)
    draft.localContent!.providers.push({
      providerId: 'prv_openai', displayName: 'OpenAI', clientProtocol: 'OPENAI_CHAT_COMPLETIONS', enabled: true,
    })
    const modelId = draft.addModel('prv_openai')
    expect(draft.localContent!.models[0].enabled).toBe(true)
    await flushPromises()

    // The Overview relationship view renders an enable toggle per resource row.
    const html = wrapper.html()
    expect(html).toContain('relationship-enable-toggle')
    expect(modelId).toBeDefined()
  })

  it('adds a model only when a real provider exists, never a placeholder', async () => {
    const { wrapper, pinia } = mountResourcesPage()
    setupSession(pinia)
    await flushPromises()

    const draft = useDraftStore(pinia)
    await switchTab(wrapper, 'models')
    const addModelBtn = findAddBtn(wrapper, 'Add')
    expect(addModelBtn!.props('disable')).toBe(true)

    // Add a real provider, then the model button becomes usable and binds to it.
    draft.localContent!.providers.push({
      providerId: 'prv_openai',
      displayName: 'OpenAI',
      clientProtocol: 'OPENAI_CHAT_COMPLETIONS',
      enabled: true,
    })
    draft.markDirty()
    await flushPromises()

    const enabledBtn = findAddBtn(wrapper, 'Add')
    await enabledBtn!.trigger('click')
    await flushPromises()

    expect(draft.localContent!.models).toHaveLength(1)
    expect(draft.localContent!.models[0].providerId).toBe('prv_openai')
    expect(draft.localContent!.models[0].providerId).not.toMatch(/placeholder/)
  })

  it('publishes with expectedDraftRevision and acknowledged warning codes', async () => {
    const fetchSpy = vi.spyOn(client, 'apiFetch')
    const draftWithWarnings = structuredClone(EMPTY_DRAFT) as {
      draftRevision: number
      content: {
        providers: { providerId: string; displayName: string; clientProtocol: string; enabled: boolean }[]
        models: unknown[]; tts: unknown[]; asr: unknown[]; mcp: unknown[]; bindings: unknown[]
        policy: Record<string, unknown>
      }
    }
    draftWithWarnings.content.providers = [{
      providerId: 'prv_openai', displayName: 'OpenAI', clientProtocol: 'OPENAI_CHAT_COMPLETIONS', enabled: true,
    }]
    fetchSpy.mockImplementation(async (path: string, init?: RequestInit) => {
      if (path === '/api/admin/v1/draft') return draftWithWarnings
      if (path.startsWith('/api/admin/v1/upstreams')) return { items: [], nextCursor: undefined }
      if (path === '/api/admin/v1/draft:validate') {
        return {
          valid: true,
          errors: [],
          warnings: [{ code: 'WARN_UNKNOWN_COST', path: '$', message: 'cost unknown', severity: 'WARNING' }],
        }
      }
      if (path === '/api/admin/v1/draft:preview') {
        return {
          draftRevision: 1,
          projectionHash: 'sha256:preview',
          providers: draftWithWarnings.content.providers,
          models: [],
          tts: [],
          asr: [],
          mcp: [],
          policy: { policyId: 'pol_draft', allowLocalProviders: true, allowLocalTts: true, allowLocalAsr: true, allowLocalMcp: true },
        }
      }
      if (path === '/api/admin/v1/draft:publish') {
        return { activationId: 'act_001', kind: 'PUBLISH', state: 'COMPLETED', desiredControlRevision: 1, createdAt: new Date().toISOString(), updatedAt: new Date().toISOString() }
      }
      if (path.startsWith('/api/admin/v1/activations/')) {
        return { activationId: 'act_001', kind: 'PUBLISH', state: 'COMPLETED', desiredControlRevision: 1, createdAt: new Date().toISOString(), updatedAt: new Date().toISOString() }
      }
      return {}
    })

    const { wrapper, pinia } = mountResourcesPage()
    setupSession(pinia)
    await flushPromises()

    const draft = useDraftStore(pinia)
    draft.validationResult = {
      valid: true,
      errors: [],
      warnings: [{ code: 'WARN_UNKNOWN_COST', path: '$', message: 'cost unknown', severity: 'WARNING' }],
    }
    await flushPromises()

    // Click "Review & Publish" to open the structured review dialog
    const reviewBtn = findAddBtn(wrapper, 'Review & Publish')
    expect(reviewBtn).toBeTruthy()
    await reviewBtn!.trigger('click')
    await flushPromises()

    // The review dialog opens; find the actual Publish button inside
    const publishBtn = wrapper.findAllComponents(QBtn).find(
      (b) => String(b.props('label') ?? '').startsWith('Publish'),
    )
    expect(publishBtn).toBeTruthy()
    await publishBtn!.trigger('click')
    await flushPromises()

    const publishCall = fetchSpy.mock.calls.find((c) => c[0] === '/api/admin/v1/draft:publish')
    expect(publishCall).toBeDefined()
    const body = JSON.parse((publishCall![1] as RequestInit).body as string)
    expect(body.expectedDraftRevision).toBe(1)
    expect(body.acknowledgedWarningCodes).toEqual(['WARN_UNKNOWN_COST'])
  })

  it('can preview snapshot and shows hash and resource counts', async () => {
    const fetchSpy = vi.spyOn(client, 'apiFetch')
    fetchSpy.mockImplementation(async (path: string, init?: RequestInit) => {
      if (path === '/api/admin/v1/draft') return structuredClone(EMPTY_DRAFT)
      if (path.startsWith('/api/admin/v1/upstreams')) return { items: [], nextCursor: undefined }
      if (path === '/api/admin/v1/draft:preview' && init?.method === 'POST') {
        return {
          draftRevision: 1,
          projectionHash: 'sha256:abc123',
          providers: [],
          models: [],
          tts: [],
          asr: [],
          mcp: [],
          policy: { policyId: 'pol_draft', allowLocalProviders: true, allowLocalTts: true, allowLocalAsr: true, allowLocalMcp: true },
        }
      }
      return {}
    })

    const { wrapper, pinia } = mountResourcesPage()
    setupSession(pinia)
    await flushPromises()

    const btns = wrapper.findAllComponents(QBtn)
    const previewBtn = btns.find((b) => String(b.props('label') ?? '').includes('Snapshot Preview'))
    expect(previewBtn).toBeTruthy()
    await previewBtn!.trigger('click')
    await flushPromises()

    const previewCallAfter = fetchSpy.mock.calls.find(
      (c) => c[0] === '/api/admin/v1/draft:preview' && (c[1] as RequestInit)?.method === 'POST',
    )
    expect(previewCallAfter).toBeDefined()

    const body = document.body.innerHTML
    expect(body).toContain('sha256:abc123')
    expect(body).toContain('Snapshot Preview')
    expect(body).toContain('Policy ID')
  })
})
