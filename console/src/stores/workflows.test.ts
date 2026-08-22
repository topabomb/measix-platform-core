import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useSessionStore } from './session'
import { useDraftStore } from './draft'
import { useActivationStore } from './activation'
import { useOperationalApplyStore } from './operationalApply'
import { setUnauthorizedHandler } from '../api/client'

beforeEach(() => {
  setActivePinia(createPinia())
  vi.unstubAllGlobals()
  setUnauthorizedHandler(undefined)
})

describe('SessionStore', () => {
  it('clears session when the central API hook reports 401', async () => {
    // First call: successful session restore.
    // Second call: 401 Unauthorized.
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({
        user: { userId: 'usr_00000000-0000-4000-8000-000000000001', displayName: 'Admin', role: 'ADMIN' },
        csrfToken: 'csrf-1', expiresAt: '2026-08-19T12:00:00Z',
      }), { status: 200, headers: { 'Content-Type': 'application/json' } }))
      .mockResolvedValueOnce(new Response(
        JSON.stringify({ type: 'about:blank', title: 'Unauthorized', status: 401, code: 'invalid_admin_session' }),
        { status: 401, headers: { 'Content-Type': 'application/problem+json' } },
      ))
    vi.stubGlobal('fetch', fetchMock)

    const store = useSessionStore()
    await store.restore()
    expect(store.authenticated).toBe(true)
    expect(store.csrfToken).toBe('csrf-1')

    // Register the central 401 handler that clears the session.
    setUnauthorizedHandler(() => { store.clear() })

    // The next API call gets 401 — the central hook must clear the session.
    await expect(store.restore()).rejects.toMatchObject({ status: 401 })
    expect(store.authenticated).toBe(false)
    expect(store.csrfToken).toBeUndefined()
  })
})

describe('DraftStore', () => {
  it('upserts a runtime binding for a resource and reuses its stable runtimeRouteId', async () => {
    const initial = {
      draftId: 'dft_00000000-0000-4000-8000-000000000001', draftRevision: 1,
      content: { providers: [], models: [], tts: [], asr: [], mcp: [], bindings: [], policy: { policyId: 'pol_00000000-0000-4000-8000-000000000001', allowLocalProviders: true, allowLocalTts: true, allowLocalAsr: true, allowLocalMcp: true } },
    }
    vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify(initial), { status: 200, headers: { 'Content-Type': 'application/json' } })))

    const store = useDraftStore()
    await store.load()
    const modelId = store.addModel('prv_00000000-0000-4000-8000-000000000001')

    store.setBinding(modelId, 'ups_00000000-0000-4000-8000-000000000001', 'HTTP_STREAMING_SSE')
    const first = store.bindingFor(modelId)
    expect(first).toBeDefined()
    expect(first!.upstreamId).toBe('ups_00000000-0000-4000-8000-000000000001')
    expect(first!.transportPolicy).toBe('HTTP_STREAMING_SSE')
    expect(first!.allowedMethods).toContain('POST')
    expect(first!.allowedPathPrefixes).toContain('/')

    // Editing keeps the same runtimeRouteId so candidate IDs are stable.
    store.setBinding(modelId, 'ups_00000000-0000-4000-8000-000000000002', 'HTTP_REQUEST_RESPONSE')
    const second = store.bindingFor(modelId)
    expect(second!.runtimeRouteId).toBe(first!.runtimeRouteId)
    expect(second!.upstreamId).toBe('ups_00000000-0000-4000-8000-000000000002')
    expect(second!.transportPolicy).toBe('HTTP_REQUEST_RESPONSE')
  })

  it('removes a binding when the resource is unbound (empty upstream)', async () => {
    const initial = {
      draftId: 'dft_00000000-0000-4000-8000-000000000001', draftRevision: 1,
      content: { providers: [], models: [], tts: [], asr: [], mcp: [], bindings: [], policy: { policyId: 'pol_00000000-0000-4000-8000-000000000001', allowLocalProviders: true, allowLocalTts: true, allowLocalAsr: true, allowLocalMcp: true } },
    }
    vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify(initial), { status: 200, headers: { 'Content-Type': 'application/json' } })))

    const store = useDraftStore()
    await store.load()
    const modelId = store.addModel('prv_00000000-0000-4000-8000-000000000001')
    store.setBinding(modelId, 'ups_00000000-0000-4000-8000-000000000001', 'HTTP_STREAMING_SSE')
    expect(store.bindingFor(modelId)).toBeDefined()

    store.setBinding(modelId, '', 'HTTP_STREAMING_SSE')
    expect(store.bindingFor(modelId)).toBeUndefined()
  })

  it('keeps local dirty content and stable candidate ids after stale revision conflict', async () => {
    const initial = {
      draftId: 'dft_00000000-0000-4000-8000-000000000001', draftRevision: 7,
      content: { providers: [], models: [], tts: [], asr: [], mcp: [], bindings: [], policy: { policyId: 'pol_00000000-0000-4000-8000-000000000001', allowLocalProviders: true, allowLocalTts: true, allowLocalAsr: true, allowLocalMcp: true } },
    }
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify(initial), { status: 200, headers: { 'Content-Type': 'application/json' } }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ type: 'about:blank', title: 'Conflict', status: 409, code: 'stale_draft_revision', currentDraftRevision: 8 }), { status: 409, headers: { 'Content-Type': 'application/problem+json' } }))
    vi.stubGlobal('fetch', fetchMock)

    const store = useDraftStore()
    await store.load()
    const modelId = store.addModel('prv_00000000-0000-4000-8000-000000000001')
    expect(store.dirty).toBe(true)
    await expect(store.save('csrf-1')).rejects.toMatchObject({ code: 'stale_draft_revision' })
    expect(store.dirty).toBe(true)
    expect(store.localContent?.models.some((model) => model.modelId === modelId)).toBe(true)
    expect(store.conflictRevision).toBe(8)
  })
})

describe('ActivationStore', () => {
  it('reuses one idempotency key for retry and reports success only after COMPLETED', async () => {
    const store = useActivationStore()
    const key = store.beginCommand('PUBLISH')
    expect(key).toMatch(/^idem_/)
    expect(store.retryKey).toBe(key)
    store.accept({ activationId: 'act_00000000-0000-4000-8000-000000000001', kind: 'PUBLISH', state: 'APPLYING', desiredControlRevision: 9, createdAt: '2026-08-19T10:00:00Z', updatedAt: '2026-08-19T10:00:00Z' })
    expect(store.succeeded).toBe(false)
    store.accept({ activationId: 'act_00000000-0000-4000-8000-000000000001', kind: 'PUBLISH', state: 'COMPLETED', desiredControlRevision: 9, createdAt: '2026-08-19T10:00:00Z', updatedAt: '2026-08-19T10:00:01Z' })
    expect(store.succeeded).toBe(true)
    expect(store.retryKey).toBe(key)
  })
})

describe('OperationalApplyStore', () => {
  it('keeps candidate and active revisions distinct while apply is pending', () => {
    const store = useOperationalApplyStore()
    store.observe({ upstreamId: 'ups_00000000-0000-4000-8000-000000000001', name: 'adapter', configRevision: 5, activeConfigRevision: 3, status: 'APPLYING' })
    expect(store.candidateRevision).toBe(5)
    expect(store.activeRevision).toBe(3)
    expect(store.pending).toBe(true)
  })
})
