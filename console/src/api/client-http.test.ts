import { afterEach, expect, it, vi } from 'vitest'

afterEach(() => { vi.unstubAllGlobals(); vi.resetModules() })

it('creates resource and command identities on ordinary HTTP without randomUUID', async () => {
  const getRandomValues = crypto.getRandomValues.bind(crypto)
  vi.stubGlobal('crypto', { getRandomValues })
  vi.resetModules()
  const { createCandidateId, createIdempotencyKey } = await import('./client')
  const ids = [createCandidateId('prv'), createCandidateId('prc'), createIdempotencyKey()]
  for (const id of ids) expect(id).toMatch(/^(prv|prc|idem)_[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/)
  expect(new Set(ids.map(id => id.slice(id.indexOf('_') + 1))).size).toBe(3)
})
