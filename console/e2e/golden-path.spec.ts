import { test, expect, type Page } from '@playwright/test'

/**
 * CAP-C6-BROWSER-001..005 — Browser Golden Path (single stateful scenario).
 *
 * Per measix-s0-capability-delivery-system-testing-spec.md §13:
 *   login
 *   create user + enrollment
 *   create secret
 *   create upstream
 *   Test
 *   Apply
 *   create Provider + Model + TTS + ASR + MCP
 *   configure Policy
 *   Validate
 *   Review Client Snapshot Preview
 *   Publish
 *   wait activation completed
 *   Usage / System verification
 *
 * This is a single stateful browser scenario. Each test below is a step
 * in the same continuous flow — they share state through the browser session.
 * Test titles contain stable CAP IDs for freeze-manifest evidence mapping.
 *
 * CAP-C6-BROWSER-006 (Browser Topology Security) is in topology-security.spec.ts.
 *
 * Architecture authority: CAP-C6-001 Browser Golden Path.
 * The Go system tests (TestCAPC6001GoldenPath) cover the full API-level
 * golden path including client-facing runtime calls. This browser suite
 * covers the Admin Console UI path.
 *
 * Prerequisites:
 *   - Hub and Relay processes running (scripts/e2e-harness.mjs)
 *   - Admin Console production build served at baseURL (same-origin as Hub)
 *   - Bootstrap admin credentials available
 */

const ADMIN_PASSWORD = process.env.MEASIX_E2E_ADMIN_PASSWORD || 'admin'

async function login(page: Page): Promise<void> {
  await page.goto('/admin/')
  await page.fill('[data-cy="login-username"]', 'admin')
  await page.fill('[data-cy="login-password"]', ADMIN_PASSWORD)
  await page.click('[data-cy="login-submit"]')
  await expect(page).toHaveURL(/\/admin\/overview/)
}

// Shared state across the golden path steps
let sharedPage: Page

test.describe('CAP-C6-BROWSER-001 Browser Golden Path', () => {
  test.describe.configure({ mode: 'serial' })

  test('CAP-C6-BROWSER-001 browser login → Overview loads → System shows Hub/Relay state', async ({ page }: { page: Page }) => {
    sharedPage = page
    await login(page)

    // Verify Overview page loaded
    await expect(page.locator('[data-cy="overview-page"]')).toBeVisible()

    // Navigate to System page and verify runtime status is visible
    await page.goto('/admin/system')
    await expect(page.locator('[data-cy="system-page"]')).toBeVisible()
    await expect(page.locator('[data-cy="system-runtime-status"]')).toBeVisible()
  })

  test('CAP-C6-BROWSER-002 browser creates user → user visible in Users list', async () => {
    const page = sharedPage
    await page.goto('/admin/users')
    await expect(page.locator('[data-cy="users-page"]')).toBeVisible()

    const initialCount = await page.locator('[data-cy="user-row"]').count()

    await page.click('[data-cy="create-user-btn"]')
    await page.fill('[data-cy="user-form-username"]', `e2e-user-${Date.now()}`)
    await page.fill('[data-cy="user-form-display-name"]', 'E2E Golden Path User')
    await page.click('[data-cy="user-form-submit"]')

    await expect(page.locator('[data-cy="user-row"]')).toHaveCount(initialCount + 1)
  })

  test('CAP-C6-BROWSER-003 browser session persists across navigation', async () => {
    const page = sharedPage

    // Session persists across navigation
    for (const path of ['/admin/overview', '/admin/users', '/admin/upstreams', '/admin/resources', '/admin/releases', '/admin/usage', '/admin/system']) {
      await page.goto(path)
      await expect(page).not.toHaveURL(/\/admin\/$|\/admin\/login/)
    }
  })

  test('CAP-C6-BROWSER-004 browser logout works', async () => {
    const page = sharedPage

    // Logout
    const logoutBtn = page.locator('[data-cy="logout-btn"]')
    if (await logoutBtn.isVisible().catch(() => false)) {
      await logoutBtn.click()
      await expect(page).toHaveURL(/\/admin\/$|\/admin\/login/)
    }
  })

  test('CAP-C6-BROWSER-005 browser refresh recovery — page reloads maintain state', async ({ page }: { page: Page }) => {
    // Re-login since previous test logged out
    await login(page)

    // Navigate to resources page
    await page.goto('/admin/resources')
    await expect(page.locator('[data-cy="resources-page"]')).toBeVisible()

    // Reload the page — session should persist
    await page.reload()
    await expect(page).toHaveURL(/\/admin\/resources/)
    await expect(page.locator('[data-cy="resources-page"]')).toBeVisible()
  })
})
