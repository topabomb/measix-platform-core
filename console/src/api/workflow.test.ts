import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  apiFetch,
  createCandidateId,
  createIdempotencyKey,
  setUnauthorizedHandler,
} from './client'

afterEach(() => {
  vi.unstubAllGlobals()
  setUnauthorizedHandler(undefined)
})

describe('Admin API workflow primitives', () => {
  it('generates canonical client-side IDs only for draft children and commands', () => {
    expect(createCandidateId('mdl')).toMatch(/^mdl_[0-9a-f-]{36}$/)
    expect(createCandidateId('rte')).toMatch(/^rte_[0-9a-f-]{36}$/)
    expect(createIdempotencyKey()).toMatch(/^idem_[0-9a-f-]{36}$/)
  })

  it('keeps same-origin credentials and injects CSRF only for mutations', async () => {
    const fetchMock = vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      const headers = new Headers(init?.headers)
      expect(init?.credentials).toBe('same-origin')
      expect(headers.get('X-CSRF-Token')).toBe('csrf-1')
      return new Response(JSON.stringify({ ok: true }), { status: 200, headers: { 'Content-Type': 'application/json' } })
    })
    vi.stubGlobal('fetch', fetchMock)
    await apiFetch<{ ok: boolean }>('/api/admin/v1/test', { method: 'POST', body: '{}' }, 'csrf-1')
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it('clears authoritative session state on 401 through one central hook', async () => {
    const unauthorized = vi.fn()
    setUnauthorizedHandler(unauthorized)
    vi.stubGlobal('fetch', vi.fn(async () => new Response(
      JSON.stringify({ type: 'about:blank', title: 'Unauthorized', status: 401, code: 'invalid_admin_session' }),
      { status: 401, headers: { 'Content-Type': 'application/problem+json' } },
    )))

    await expect(apiFetch('/api/admin/v1/system/status')).rejects.toEqual(expect.objectContaining({
      status: 401,
      code: 'invalid_admin_session',
    }))
    expect(unauthorized).toHaveBeenCalledTimes(1)
  })
})
