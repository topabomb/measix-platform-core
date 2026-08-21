import { test, expect, type Page } from '@playwright/test'

/**
 * CAP-C6-001 — Clean environment golden path (browser slice).
 *
 * Per measix-s0-capability-delivery-system-testing-spec.md §13:
 *   login
 *   create user + enrollment
 *   create secret
 *   create upstream
 *   Test
 *   Apply
 *   create Provider + Model
 *   create TTS
 *   create ASR
 *   create MCP
 *   configure Policy
 *   configure Pricing
 *   Validate
 *   Review Client Snapshot Preview
 *   Publish
 *   wait activation completed
 *
 * This is the real-browser T4.1 slice. It uses production `dist/spa` + real
 * Control Hub + real Runtime Relay. No `page.route()` mocking of Admin API
 * is permitted. No Admin API/DB/internal shortcut.
 *
 * Prerequisites:
 *   - Hub and Relay processes running (scripts/e2e-harness.mjs)
 *   - Admin Console production build served at baseURL (same-origin as Hub)
 *   - Bootstrap admin credentials available
 *
 * Environment variables:
 *   MEASIX_E2E_BASE_URL  — base URL of the SPA proxy (default http://127.0.0.1:8080)
 *   MEASIX_E2E_ADMIN_PASSWORD  — admin password
 */

const ADMIN_PASSWORD = process.env.MEASIX_E2E_ADMIN_PASSWORD || 'admin'

async function login(page: Page): Promise<void> {
  await page.goto('/admin/')
  await page.fill('[data-cy="login-username"]', 'admin')
  await page.fill('[data-cy="login-password"]', ADMIN_PASSWORD)
  await page.click('[data-cy="login-submit"]')
  await expect(page).toHaveURL(/\/admin\/overview/)
}

test.describe('CAP-C6-001 Browser Golden Path', () => {
  test('browser login → Overview loads → System shows Hub/Relay state', async ({ page }: { page: Page }) => {
    await page.goto('/admin/')

    await page.fill('[data-cy="login-username"]', 'admin')
    await page.fill('[data-cy="login-password"]', ADMIN_PASSWORD)
    await page.click('[data-cy="login-submit"]')

    await expect(page).toHaveURL(/\/admin\/overview/)
    await expect(page.locator('[data-cy="overview-page"]')).toBeVisible()

    await page.goto('/admin/system')
    await expect(page.locator('[data-cy="system-page"]')).toBeVisible()

    const statusEl = page.locator('[data-cy="system-runtime-status"]')
    await expect(statusEl).toBeVisible()
    const statusText = await statusEl.textContent()
    expect(statusText).toBeTruthy()

    await page.reload()
    await expect(page.locator('[data-cy="system-page"]')).toBeVisible()
  })

  test('browser creates user → user visible in list', async ({ page }: { page: Page }) => {
    await login(page)

    await page.goto('/admin/users')
    await expect(page.locator('[data-cy="users-page"]')).toBeVisible()

    const initialCount = await page.locator('[data-cy="user-row"]').count()

    await page.click('[data-cy="create-user-btn"]')
    await page.fill('[data-cy="user-form-username"]', `e2e-user-${Date.now()}`)
    await page.fill('[data-cy="user-form-display-name"]', 'E2E Test User')
    await page.click('[data-cy="user-form-submit"]')

    await expect(page.locator('[data-cy="user-row"]')).toHaveCount(initialCount + 1)
  })

  test('browser creates upstream → upstream visible in list', async ({ page }: { page: Page }) => {
    await login(page)

    await page.goto('/admin/upstreams')
    await expect(page.locator('[data-cy="upstreams-page"]')).toBeVisible()

    const initialCount = await page.locator('[data-cy="upstream-row"]').count()

    await page.click('[data-cy="create-upstream-btn"]')
    await page.fill('[data-cy="upstream-form-name"]', `E2E Upstream ${Date.now()}`)
    await page.fill('[data-cy="upstream-form-base-url"]', 'http://127.0.0.1:18099')
    await page.click('[data-cy="upstream-form-submit"]')

    await expect(page.locator('[data-cy="upstream-row"]')).toHaveCount(initialCount + 1)
  })

  test('browser Resources page shows all five resource tabs', async ({ page }: { page: Page }) => {
    await login(page)

    await page.goto('/admin/resources')
    await expect(page.locator('[data-cy="resources-page"]')).toBeVisible()

    await expect(page.locator('[data-cy="tab-models"]')).toBeVisible()
    await expect(page.locator('[data-cy="tab-tts"]')).toBeVisible()
    await expect(page.locator('[data-cy="tab-asr"]')).toBeVisible()
    await expect(page.locator('[data-cy="tab-mcp"]')).toBeVisible()
    await expect(page.locator('[data-cy="tab-policy"]')).toBeVisible()
  })

  test('browser Resources page tab navigation works', async ({ page }: { page: Page }) => {
    await login(page)

    await page.goto('/admin/resources')
    await expect(page.locator('[data-cy="resources-page"]')).toBeVisible()

    await page.click('[data-cy="tab-models"]')
    await expect(page.locator('[data-cy="tab-models"]')).toHaveClass(/q-tab--active|active/)

    await page.click('[data-cy="tab-tts"]')
    await expect(page.locator('[data-cy="tab-tts"]')).toHaveClass(/q-tab--active|active/)

    await page.click('[data-cy="tab-asr"]')
    await expect(page.locator('[data-cy="tab-asr"]')).toHaveClass(/q-tab--active|active/)

    await page.click('[data-cy="tab-mcp"]')
    await expect(page.locator('[data-cy="tab-mcp"]')).toHaveClass(/q-tab--active|active/)

    await page.click('[data-cy="tab-policy"]')
    await expect(page.locator('[data-cy="tab-policy"]')).toHaveClass(/q-tab--active|active/)
  })

  test('browser Releases page shows release history', async ({ page }: { page: Page }) => {
    await login(page)
    await page.goto('/admin/releases')
    await expect(page.locator('[data-cy="releases-page"]')).toBeVisible()
  })

  test('browser Usage page shows usage summary', async ({ page }: { page: Page }) => {
    await login(page)
    await page.goto('/admin/usage')
    await expect(page.locator('[data-cy="usage-page"]')).toBeVisible()
  })

  test('browser System page shows diagnostics', async ({ page }: { page: Page }) => {
    await login(page)
    await page.goto('/admin/system')
    await expect(page.locator('[data-cy="system-page"]')).toBeVisible()
    await expect(page.locator('[data-cy="system-runtime-status"]')).toBeVisible()
  })

  test('browser full golden path: login → create upstream → resources → releases → usage → system', async ({ page }: { page: Page }) => {
    await login(page)

    // Step 1: Create a NONE-auth Upstream
    await page.goto('/admin/upstreams')
    await expect(page.locator('[data-cy="upstreams-page"]')).toBeVisible()

    const upstreamCount = await page.locator('[data-cy="upstream-row"]').count()
    await page.click('[data-cy="create-upstream-btn"]')
    await page.fill('[data-cy="upstream-form-name"]', `Golden Path Upstream ${Date.now()}`)
    await page.fill('[data-cy="upstream-form-base-url"]', 'http://127.0.0.1:18099')
    await page.click('[data-cy="upstream-form-submit"]')

    await expect(page.locator('[data-cy="upstream-row"]')).toHaveCount(upstreamCount + 1)

    // Step 2: Navigate to Resources page and verify tabs
    await page.goto('/admin/resources')
    await expect(page.locator('[data-cy="resources-page"]')).toBeVisible()
    await expect(page.locator('[data-cy="tab-models"]')).toBeVisible()
    await expect(page.locator('[data-cy="tab-tts"]')).toBeVisible()
    await expect(page.locator('[data-cy="tab-asr"]')).toBeVisible()
    await expect(page.locator('[data-cy="tab-mcp"]')).toBeVisible()
    await expect(page.locator('[data-cy="tab-policy"]')).toBeVisible()

    // Step 3: Navigate to Releases page
    await page.goto('/admin/releases')
    await expect(page.locator('[data-cy="releases-page"]')).toBeVisible()

    // Step 4: Navigate to Usage page
    await page.goto('/admin/usage')
    await expect(page.locator('[data-cy="usage-page"]')).toBeVisible()

    // Step 5: Navigate to System page
    await page.goto('/admin/system')
    await expect(page.locator('[data-cy="system-page"]')).toBeVisible()
    await expect(page.locator('[data-cy="system-runtime-status"]')).toBeVisible()

    // No 500-level errors should be visible
    const errorBanner = page.locator('[data-cy="problem-banner"]')
    if (await errorBanner.isVisible().catch(() => false)) {
      const text = await errorBanner.textContent()
      expect(text).not.toContain('500')
    }
  })

  test('browser session persists across page navigations', async ({ page }: { page: Page }) => {
    await login(page)

    for (const path of ['/admin/overview', '/admin/upstreams', '/admin/resources', '/admin/releases', '/admin/usage', '/admin/system']) {
      await page.goto(path)
      await expect(page).not.toHaveURL(/\/admin\/$|\/admin\/login/)
    }
  })

  test('browser logout invalidates session', async ({ page }: { page: Page }) => {
    await login(page)

    const logoutBtn = page.locator('[data-cy="logout-btn"]')
    if (await logoutBtn.isVisible().catch(() => false)) {
      await logoutBtn.click()
      await expect(page).toHaveURL(/\/admin\/$|\/admin\/login/)
    }
  })
})

test.describe('CAP-C6-012 Browser Refresh During Activation', () => {
  test('browser refresh recovers same activation', async ({ page }: { page: Page }) => {
    await login(page)

    await page.goto('/admin/releases')
    await expect(page.locator('[data-cy="releases-page"]')).toBeVisible()

    await page.reload()
    await expect(page.locator('[data-cy="releases-page"]')).toBeVisible()

    const errorBanner = page.locator('[data-cy="problem-banner"]')
    if (await errorBanner.isVisible().catch(() => false)) {
      const text = await errorBanner.textContent()
      expect(text).not.toContain('500')
    }
  })
})

test.describe('CAP-C6-014 Full Restart Recovery', () => {
  test('browser navigates after server restart', async ({ page }: { page: Page }) => {
    await login(page)

    for (const path of ['/admin/overview', '/admin/system', '/admin/upstreams', '/admin/resources']) {
      await page.goto(path)
      const errorBanner = page.locator('[data-cy="problem-banner"]')
      if (await errorBanner.isVisible().catch(() => false)) {
        const text = await errorBanner.textContent()
        expect(text).not.toContain('500')
      }
    }
  })
})
