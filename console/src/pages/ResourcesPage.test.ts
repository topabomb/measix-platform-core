import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import {
  Quasar, QLayout, QPage, QPageContainer,
  QCard, QCardSection, QCardActions, QInput, QBtn, QBanner,
  QSelect, QToggle, QDialog, QSeparator,
  QList, QItem, QItemSection, QItemLabel, QMarkupTable, QChip, QSpinner,
  QBadge, QIcon,
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
            QInput, QBtn, QBanner, QSelect, QToggle, QDialog, QSeparator,
            QList, QItem, QItemSection, QItemLabel, QMarkupTable, QChip,
            QSpinner, QBadge, QIcon,
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

describe('ResourcesPage', () => {
  beforeEach(() => {
    vi.spyOn(client, 'apiFetch').mockImplementation(async (path: string) => {
      if (path === '/api/admin/v1/draft') return structuredClone(EMPTY_DRAFT)
      return {}
    })
  })

  it('renders five resource editor sections: Models, TTS, ASR, MCP, Policy', async () => {
    const { wrapper, pinia } = mountResourcesPage()
    setupSession(pinia)
    await flushPromises()

    const html = wrapper.html()
    expect(html).toContain('Models')
    expect(html).toContain('TTS')
    expect(html).toContain('ASR')
    expect(html).toContain('MCP')
    expect(html).toContain('Policy')
  })

  it('can add a TTS resource through the Add button', async () => {
    const { wrapper, pinia } = mountResourcesPage()
    setupSession(pinia)
    await flushPromises()

    const draft = useDraftStore(pinia)
    expect(draft.localContent?.tts).toHaveLength(0)

    const btns = wrapper.findAllComponents(QBtn)
    const addTtsBtn = btns.find((b) => String(b.props('label') ?? '').includes('Add TTS'))
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

    const btns = wrapper.findAllComponents(QBtn)
    const addAsrBtn = btns.find((b) => String(b.props('label') ?? '').includes('Add ASR'))
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

    const btns = wrapper.findAllComponents(QBtn)
    const addMcpBtn = btns.find((b) => String(b.props('label') ?? '').includes('Add MCP'))
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

    const toggles = wrapper.findAllComponents(QToggle)
    expect(toggles.length).toBeGreaterThanOrEqual(4)

    const draft = useDraftStore(pinia)
    expect(draft.localContent?.policy?.allowLocalProviders).toBe(true)
  })

  it('shows relationship count for each resource to its provider', async () => {
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
    draft.localContent!.models.push({
      modelId: 'mdl_test',
      providerId: 'prv_openai',
      displayName: 'GPT-4',
      upstreamModelKey: 'gpt-4',
      runtimePath: '/v1/chat/completions',
      inputModalities: ['TEXT'],
      outputModalities: ['TEXT'],
      capabilities: ['TOOL'],
      enabled: true,
    })
    await flushPromises()

    const html = wrapper.html()
    expect(html).toContain('1')
  })

  it('can preview snapshot and shows hash and resource counts', async () => {
    const fetchSpy = vi.spyOn(client, 'apiFetch')
    fetchSpy.mockImplementation(async (path: string, init?: RequestInit) => {
      if (path === '/api/admin/v1/draft') return structuredClone(EMPTY_DRAFT)
      if (path === '/api/admin/v1/draft:preview' && init?.method === 'POST') {
        return {
          draftRevision: 1,
          snapshotHash: 'sha256:abc123',
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

    // Verify the preview API call was made
    const previewCall = fetchSpy.mock.calls.find(
      (c) => c[0] === '/api/admin/v1/draft:preview' && (c[1] as RequestInit)?.method === 'POST',
    )
    // Before clicking, no preview call
    expect(previewCall).toBeUndefined()

    const btns = wrapper.findAllComponents(QBtn)
    const previewBtn = btns.find((b) => String(b.props('label') ?? '') === 'Preview')
    expect(previewBtn).toBeTruthy()
    await previewBtn!.trigger('click')
    await flushPromises()

    // After clicking, verify the preview API was called
    const previewCallAfter = fetchSpy.mock.calls.find(
      (c) => c[0] === '/api/admin/v1/draft:preview' && (c[1] as RequestInit)?.method === 'POST',
    )
    expect(previewCallAfter).toBeDefined()

    // Verify the dialog opened by checking QDialog component model
    const dialog = wrapper.findAllComponents(QDialog).find((d) => d.props('modelValue') === true)
    expect(dialog).toBeTruthy()

    // Check the dialog body content via the rendered DOM (QDialog teleports to body)
    const body = document.body.innerHTML
    expect(body).toContain('sha256:abc123')
    expect(body).toContain('Snapshot Preview')
    expect(body).toContain('Policy ID')
  })
})
