import { test, expect } from '@playwright/test'

/**
 * Browser topology security tests.
 *
 * Per architecture: the public platformUrl must NOT expose /internal/* routes.
 * Admin/Client must not access internal APIs through the client-facing topology.
 *
 * Test title contains CAP-C6-BROWSER-006 for stable scenario ID mapping.
 */
test.describe('Browser Topology Security', () => {
  test('CAP-C6-BROWSER-006 public origin blocks /internal/* routes', async ({ request }) => {
    // /internal/* must NOT be reachable from the public origin
    const response = await request.get('/internal/v1/usage/request-events:batch')
    // Must be 404 (not proxied) — not 200, 401, or 403 (which would mean it's reachable)
    expect(response.status()).toBe(404)

    // Also verify other internal paths are blocked
    const internalPaths = [
      '/internal/v1/usage/request-events:batch',
      '/internal/v1/something',
      '/internal/live',
      '/internal/ready',
    ]

    for (const path of internalPaths) {
      const resp = await request.get(path)
      expect(resp.status()).toBe(404)
    }
  })
})
