import { test, expect } from '@playwright/test'

/**
 * Browser topology security tests.
 *
 * Per architecture: the public platformUrl must NOT expose /internal/* routes.
 * Admin/Client must not access internal APIs through the client-facing topology.
 *
 * This test verifies that from the public origin (SPA proxy):
 *   - /admin/* is reachable (serves SPA)
 *   - /api/* is reachable (Admin and Client APIs)
 *   - /internal/* is NOT reachable (must return 404 or be blocked)
 */
test.describe('Browser Topology Security', () => {
  test('public origin blocks /internal/* routes', async ({ request }) => {
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

  test('public origin serves /admin/* SPA', async ({ page }) => {
    // /admin/ should serve the SPA (redirect to login or show login page)
    const response = await page.goto('/admin/')
    expect(response?.ok()).toBe(true)
  })

  test('public origin serves /live and /ready health endpoints', async ({ request }) => {
    // Health endpoints should be reachable from public origin
    const liveResp = await request.get('/live')
    expect(liveResp.ok()).toBe(true)

    const readyResp = await request.get('/ready')
    expect(readyResp.ok()).toBe(true)
  })
})
