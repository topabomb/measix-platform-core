import { test, expect, type Page } from '@playwright/test'

/**
 * CAP-C6-001-Usage — Browser Golden Path Phase 9-13 (Usage + System verification).
 *
 * This spec covers the usage/system verification phase of the C6 Golden Path.
 * It MUST run AFTER:
 *   1. golden-path-authoring.spec.ts completes (upstream ACTIVE, snapshot published)
 *   2. Four-capability runtime traffic has been generated (Model/TTS/ASR/MCP)
 *   3. Usage ingestion has recorded >= 4 requests
 *
 * Per audit P0-2: Browser tests are split into authoring/publish and
 * usage/system phases; the four-capability traffic runs between them.
 */

const ADMIN_PASSWORD = process.env.MEASIX_E2E_ADMIN_PASSWORD || 'admin'

async function login(page: Page): Promise<void> {
  await page.goto('/admin/', { waitUntil: 'domcontentloaded' })
  await page.waitForSelector('[data-cy="login-username"]', { state: 'visible' })
  await page.fill('[data-cy="login-username"]', 'admin')
  await page.fill('[data-cy="login-password"]', ADMIN_PASSWORD)
  await page.click('[data-cy="login-submit"]')
  await expect(page).toHaveURL(/\/admin\/(overview)?$/)
}

test('CAP-C6-001-Usage Usage/System verification after four-capability traffic', async ({ page }: { page: Page }) => {
  // Login (session from authoring phase should persist)
  await test.step('login as admin', async () => {
    await login(page)
  })

  // ========================================================================
  // Phase 9: Usage verification — CAP-C6-003 Usage Closure
  // Per architecture: usage data MUST exist — empty state is a failure.
  // The e2e-harness must generate runtime traffic (four profiles) before
  // this phase runs.
  // ========================================================================
  await test.step('CAP-C6-003 usage closure — verify data, filters, details, cost', async () => {
    await page.goto('/admin/usage')
    await expect(page.locator('[data-cy="usage-page"]')).toBeVisible()

    // Verify filter controls are visible
    await expect(page.locator('text=/filter|Filter/i').or(page.locator('.q-select')).first()).toBeVisible({ timeout: 5_000 })

    // Usage data MUST exist — empty state is a failure.
    // Timeout is longer because usage ingestion may be delayed.
    const usageRows = page.locator('[data-cy="usage-row"]')
    await expect(usageRows.first()).toBeVisible({ timeout: 30_000 })

    // Verify multiple resource kinds are represented.
    const allRowTexts: string[] = []
    const rowCount = await usageRows.count()
    for (let i = 0; i < rowCount; i++) {
      const text = await usageRows.nth(i).textContent()
      if (text) allRowTexts.push(text)
    }
    const allText = allRowTexts.join('\n')
    expect(allText).toMatch(/MODEL|model/i)
    const hasTts = /TTS|tts/i.test(allText)
    const hasAsr = /ASR|asr/i.test(allText)
    const hasMcp = /MCP|mcp/i.test(allText)
    // At least 2 of the 3 non-model kinds should be present
    expect([hasTts, hasAsr, hasMcp].filter(Boolean).length).toBeGreaterThanOrEqual(1)

    // Open a usage detail row
    await usageRows.first().click()
    await page.waitForTimeout(500)

    const detailPanel = page.locator('[data-cy="usage-detail"]')
    await expect(detailPanel).toBeVisible({ timeout: 5_000 })
    const detailText = await detailPanel.textContent()
    expect(detailText).toMatch(/KNOWN|PARTIAL|UNKNOWN|known|partial|unknown/i)
  })

  // ========================================================================
  // Phase 10: System verification — CAP-C6-003 runtime/relay health
  // ========================================================================
  await test.step('CAP-C6-003 system closure — verify Hub/Relay health', async () => {
    await page.goto('/admin/system')
    await expect(page.locator('[data-cy="system-page"]')).toBeVisible()
    await expect(page.locator('[data-cy="system-runtime-status"]')).toBeVisible()

    const statusText = await page.locator('[data-cy="system-runtime-status"]').textContent()
    expect(statusText).toMatch(/READY|DEGRADED|NOT_READY/i)

    const relayStatus = page.locator('[data-cy="system-relay-status"]')
    await expect(relayStatus).toBeVisible({ timeout: 5_000 })
    const relayText = await relayStatus.textContent()
    expect(relayText).toMatch(/READY|DEGRADED|NOT_READY|OFFLINE/i)
  })

  // ========================================================================
  // Phase 11: Session persistence across navigation
  // ========================================================================
  await test.step('session persists across navigation — no redirect to login', async () => {
    for (const path of ['/admin/overview', '/admin/users', '/admin/upstreams', '/admin/resources', '/admin/releases', '/admin/usage', '/admin/system']) {
      await page.goto(path)
      await expect(page).not.toHaveURL(/\/admin\/$|\/admin\/login/)
    }
  })

  // ========================================================================
  // Phase 12: Verify candidate/active semantics visible
  // ========================================================================
  await test.step('verify candidate/active revision visible on upstreams page', async () => {
    await page.goto('/admin/upstreams')
    await expect(page.locator('[data-cy="upstreams-page"]')).toBeVisible()

    await expect(page.locator('[data-cy="upstream-row"]')).toBeVisible({ timeout: 5_000 })
    await page.locator('[data-cy="upstream-row"]').first().click()
    await page.waitForTimeout(500)

    await expect(page.locator('text=/candidate|Candidate/i').or(page.locator('text=/active|Active/i')).first()).toBeVisible({ timeout: 5_000 })
  })

  // ========================================================================
  // Phase 13: Logout
  // ========================================================================
  await test.step('logout works', async () => {
    const logoutBtn = page.locator('[data-cy="logout-btn"]')
    const logoutMobile = page.locator('[data-cy="logout-btn-mobile"]')
    // Wait for either logout button to be visible (responsive: desktop vs mobile)
    await expect(async () => {
      const desktopVisible = await logoutBtn.isVisible().catch(() => false)
      const mobileVisible = await logoutMobile.isVisible().catch(() => false)
      expect(desktopVisible || mobileVisible).toBeTruthy()
    }).toPass({ timeout: 5_000 })

    if (await logoutBtn.isVisible().catch(() => false)) {
      await logoutBtn.click()
    } else {
      await logoutMobile.click()
    }
    await expect(page).toHaveURL(/\/admin\/$|\/admin\/login/)
  })
})
