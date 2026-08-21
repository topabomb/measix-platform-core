import { test, expect } from '@playwright/test'

/**
 * Browser topology security tests.
 *
 * Per architecture: the public platformUrl must NOT expose /internal/* routes.
 * Admin/Client must not access internal APIs through the client-facing topology.
 *
 * This test verifies BOTH:
 *   1. The SPA proxy (same-origin as browser) blocks /internal/* — returns 404
 *   2. The Hub's public listener directly blocks /internal/* — returns 404
 *      (This is the real topology test: the Hub itself must not route /internal/*
 *      on its public listener, not just that the SPA proxy hides it.)
 *
 * Test title contains CAP-C6-BROWSER-006 for stable scenario ID mapping.
 *
 * The Hub's internal listener (separate port) IS expected to serve /internal/*
 * — that's the private topology for Relay→Hub service APIs.
 */

/** Hub public base URL — set by e2e-harness via MEASIX_E2E_HUB_BASE_URL */
const HUB_PUBLIC_BASE_URL = process.env.MEASIX_E2E_HUB_BASE_URL || ''

test.describe('Browser Topology Security', () => {
  test('CAP-C6-BROWSER-006 public origin blocks /internal/* routes', async ({ request }) => {
    // --- Part 1: SPA proxy (same-origin as browser) blocks /internal/* ---
    // /internal/* must NOT be reachable from the public origin (SPA proxy)
    const internalPaths = [
      '/internal/v1/usage/request-events:batch',
      '/internal/v1/something',
      '/internal/live',
      '/internal/ready',
    ]

    for (const path of internalPaths) {
      const resp = await request.get(path)
      // Must be 404 (not proxied) — not 200, 401, or 403 (which would mean it's reachable)
      expect(resp.status(), `SPA proxy: ${path} should be 404`).toBe(404)
    }

    // --- Part 2: Hub public listener directly blocks /internal/* ---
    // If HUB_PUBLIC_BASE_URL is set (by e2e-harness), verify the Hub's
    // own public listener does NOT route /internal/* — this proves the
    // topology separation is in the Hub itself, not just the SPA proxy.
    if (HUB_PUBLIC_BASE_URL) {
      for (const path of internalPaths) {
        const resp = await request.get(`${HUB_PUBLIC_BASE_URL}${path}`)
        // Must be 404 — the Hub public listener must not have /internal/* routes
        expect(resp.status(), `Hub public listener: ${path} should be 404`).toBe(404)
      }

      // Verify /live IS reachable on the public listener (health check)
      const liveResp = await request.get(`${HUB_PUBLIC_BASE_URL}/live`)
      expect(liveResp.status(), 'Hub public /live should be 200').toBe(200)
    }
  })
})
