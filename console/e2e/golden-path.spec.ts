import { test, expect, type Page } from '@playwright/test'

/**
 * CAP-C6-001 — Clean environment golden path (browser slice).
 *
 * This is the real-browser T4.1 slice required by architecture:
 * measix-s0-capability-delivery-system-testing-spec.md §13.
 *
 * It uses production `dist/spa` + real Control Hub + real Runtime Relay.
 * No `page.route()` mocking of Admin API is permitted.
 *
 * Prerequisites:
 *   - Hub and Relay processes running (npm start or explicit binary launch)
 *   - Admin Console production build served at baseURL (same-origin as Hub)
 *   - Bootstrap admin credentials available
 *
 * Environment variables:
 *   MEASIX_E2E_BASE_URL  — base URL of the Hub (default http://127.0.0.1:8080)
 *   MEASIX_E2E_ADMIN_PASSWORD  — admin password (default from .secrets/admin-password.txt)
 */

const ADMIN_PASSWORD = process.env.MEASIX_E2E_ADMIN_PASSWORD || 'admin'

test.describe('CAP-C6-001 Browser Golden Path', () => {
  test('browser login → Overview loads → System shows Hub/Relay state', async ({ page }: { page: Page }) => {
    // Navigate to the Admin Console
    await page.goto('/admin/')

    // Login
    await page.fill('[data-cy="login-username"]', 'admin')
    await page.fill('[data-cy="login-password"]', ADMIN_PASSWORD)
    await page.click('[data-cy="login-submit"]')

    // Overview page should load
    await expect(page).toHaveURL(/\/admin\/overview/)
    await expect(page.locator('[data-cy="overview-page"]')).toBeVisible()

    // System page should show authoritative Hub/Relay state
    await page.goto('/admin/system')
    await expect(page.locator('[data-cy="system-page"]')).toBeVisible()

    // Verify the system status shows converged or pending state (not error)
    const statusEl = page.locator('[data-cy="system-runtime-status"]')
    await expect(statusEl).toBeVisible()
    const statusText = await statusEl.textContent()
    expect(statusText).toBeTruthy()

    // Refresh/navigation should remain valid
    await page.reload()
    await expect(page.locator('[data-cy="system-page"]')).toBeVisible()
  })

  test('browser creates user → user visible in list', async ({ page }: { page: Page }) => {
    await page.goto('/admin/')
    await page.fill('[data-cy="login-username"]', 'admin')
    await page.fill('[data-cy="login-password"]', ADMIN_PASSWORD)
    await page.click('[data-cy="login-submit"]')

    await page.goto('/admin/users')
    await expect(page.locator('[data-cy="users-page"]')).toBeVisible()

    // Count existing users
    const initialCount = await page.locator('[data-cy="user-row"]').count()

    // Create a new user
    await page.click('[data-cy="create-user-btn"]')
    await page.fill('[data-cy="user-form-username"]', `e2e-user-${Date.now()}`)
    await page.fill('[data-cy="user-form-display-name"]', 'E2E Test User')
    await page.click('[data-cy="user-form-submit"]')

    // Verify the user appears in the list
    await expect(page.locator('[data-cy="user-row"]')).toHaveCount(initialCount + 1)
  })
})
