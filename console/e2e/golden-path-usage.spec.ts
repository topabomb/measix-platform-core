import { test, expect, type Page } from '@playwright/test'

/**
 * CAP-C6-001-Usage — Browser Golden Path Phase 9-13 (Usage + System verification).
 *
 * This spec covers the usage/system verification phase of the C6 Golden Path.
 * It MUST run AFTER:
 *   1. golden-path-authoring.spec.ts completes (upstream ACTIVE, snapshot published)
 *   2. Five-capability runtime traffic has been generated (Model/Image/TTS/ASR/MCP)
 *   3. Usage ingestion has recorded >= 5 requests
 *
 * Per audit P0-2: Browser tests are split into authoring/publish and
 * usage/system phases; the five-capability traffic runs between them.
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

test('CAP-C6-001-Usage Usage/System verification after five-capability traffic', async ({ page }: { page: Page }) => {
  // Login (session from authoring phase should persist)
  await test.step('login as admin', async () => {
    await login(page)
  })

  // ========================================================================
  // Phase 9: Usage verification — CAP-C6-003 Usage Closure
  // Per architecture: usage data MUST exist — empty state is a failure.
  // The e2e-harness must generate runtime traffic (five profiles) before
  // this phase runs.
  // ========================================================================
  await test.step('CAP-C6-003 usage closure — verify data, filters, details, cost', async () => {
    await page.goto('/admin/usage')
    await expect(page.locator('[data-cy="usage-page"]')).toBeVisible()

    // Verify filter controls are visible
    await expect(page.locator('text=/filter|Filter/i').or(page.locator('.q-select')).first()).toBeVisible({ timeout: 5_000 })

    // The five exercised capability profiles must all reach the ledger. Profiles
    // with a semantic parser may be EXACT/PARTIAL while opaque responses remain
    // UNKNOWN, so assert the total instead of encoding parser implementation
    // details into the system test.
    const requestCount = Number.parseInt(await page.locator('[data-cy="usage-request-count"]').innerText(), 10)
    expect(requestCount).toBeGreaterThanOrEqual(5)
    const completeness = page.locator('[data-cy="request-completeness"]')
    const completenessCounts = await completeness.locator('.q-chip').allInnerTexts()
    expect(completenessCounts.reduce((sum, text) => sum + Number.parseInt(text, 10), 0)).toBeGreaterThanOrEqual(5)

    await page.locator('[data-cy="usage-more-filters"]').click()
    const budgetFilter = page.locator('[data-cy="usage-budget-status-filter"]')
    await expect(budgetFilter).toBeVisible()
    const filteredUsers = page.waitForResponse(response =>
      response.url().includes('/api/admin/v1/usage/users') &&
      response.url().includes('budgetStatus=PENDING_RECONCILIATION'),
    )
    await budgetFilter.click()
    await page.getByRole('option', { name: 'Pending reconciliation' }).click()
    expect((await filteredUsers).ok()).toBe(true)
    await page.screenshot({ path: '../.artifacts/admin-usage-summary.png', fullPage: true })

    await page.getByRole('tab', { name: 'Requests', exact: true }).click()
    // Usage data MUST exist — empty state is a failure.
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
    expect(allText).toMatch(/IMAGE_GENERATION|image generation/i)
    expect(allText).toMatch(/TTS|tts/i)
    expect(allText).toMatch(/ASR|asr/i)
    expect(allText).toMatch(/MCP|mcp/i)

    // Open a usage detail row
    await usageRows.first().click()
    await page.waitForTimeout(500)

    const detailPanel = page.locator('[data-cy="usage-detail"]')
    await expect(detailPanel).toBeVisible({ timeout: 5_000 })
    const detailText = await detailPanel.textContent()
    expect(detailText).toMatch(/req_[a-f0-9-]+/)
    expect(detailText).toMatch(/mdl_|img_|tts_|asr_|mcp_/)

    await page.keyboard.press('Escape')
    await page.getByRole('tab', { name: 'Reconciliation' }).click()
    const reconciliation = page.locator('[data-cy="usage-reconciliation-panel"]')
    await expect(reconciliation).toBeVisible()
    await expect(reconciliation.locator('[data-cy="reconciliation-row"]').first()).toBeVisible({ timeout: 10_000 })
    await page.screenshot({ path: '../.artifacts/admin-usage-reconciliation.png', fullPage: true })
  })

  // ========================================================================
  // Phase 10: System verification — CAP-C6-003 runtime/relay health
  // ========================================================================
  await test.step('CAP-C6-003 system closure — verify Hub/Relay health', async () => {
    const pageErrors: string[] = []
    const recordPageError = (error: Error) => pageErrors.push(error.message)
    page.on('pageerror', recordPageError)
    try {
      await page.goto('/admin/system')
      await expect(page.locator('[data-cy="system-page"]')).toBeVisible()
      await expect(page.locator('[data-cy="system-runtime-status"]')).toBeVisible()

      const statusText = await page.locator('[data-cy="system-runtime-status"]').textContent()
      expect(statusText).toMatch(/READY|DEGRADED|NOT_READY/i)

      const relayStatus = page.locator('[data-cy="system-relay-status"]')
      await expect(relayStatus).toBeVisible({ timeout: 5_000 })
      await expect(page.locator('[data-cy="relay-build-version"]')).toHaveText('dev')
      const relayText = await relayStatus.textContent()
      expect(relayText).toMatch(/READY|DEGRADED|NOT_READY|OFFLINE/i)
      await page.getByRole('tab', { name: 'Runtime delivery', exact: true }).click()
      await expect(page.locator('[data-cy="system-convergence-status"]')).toBeVisible()
      await expect(page.locator('[data-cy="lastActivation"]')).toContainText(/Completed/i)
      // The opaque profiles intentionally exercise the reconciliation path.
      // Verify that their incomplete records reach the rendered diagnostics.
      await page.getByRole('tab', { name: 'Metering pipeline', exact: true }).click()
      const unknownCount = page.locator('[data-cy="semantic-unknown-count"]')
      await expect(unknownCount).toHaveText(/^\d+$/)
      expect(Number(await unknownCount.textContent())).toBeGreaterThanOrEqual(1)
      expect(pageErrors).toEqual([])
    } finally {
      page.off('pageerror', recordPageError)
    }
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
    // Close any open dialog (e.g., usage detail) that might intercept clicks
    const backdrop = page.locator('.q-dialog__backdrop')
    if (await backdrop.isVisible().catch(() => false)) {
      await page.keyboard.press('Escape')
      await page.waitForTimeout(300)
    }

    // The logout button is inside a q-menu that opens when clicking the user menu button.
    // On desktop, click the user menu button (data-cy=user-menu-btn) to reveal logout-btn.
    // On mobile, logout-btn-mobile is directly visible.
    const logoutMobile = page.locator('[data-cy="logout-btn-mobile"]')
    const mobileVisible = await logoutMobile.isVisible().catch(() => false)
    if (mobileVisible) {
      await logoutMobile.click()
    } else {
      // Desktop: click the user menu button to open the dropdown
      const userMenuBtn = page.locator('[data-cy="user-menu-btn"]')
      await expect(userMenuBtn).toBeVisible({ timeout: 5_000 })
      await userMenuBtn.click()
      await page.waitForTimeout(300)
      const logoutBtn = page.locator('[data-cy="logout-btn"]')
      await expect(logoutBtn).toBeVisible({ timeout: 5_000 })
      await logoutBtn.click()
    }
    await expect(page).toHaveURL(/\/admin\/$|\/admin\/login/)
  })
})
