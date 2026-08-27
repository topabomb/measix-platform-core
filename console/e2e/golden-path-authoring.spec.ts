import { test, expect, type Page } from '@playwright/test'

/**
 * CAP-C6-001-Authoring — Browser Golden Path Phase 1-8 (Authoring + Publish).
 *
 * This spec covers the authoring and publish phase of the C6 Golden Path:
 *   login → user/enrollment → secret → upstream test/apply →
 *   Provider/Model/TTS/ASR/MCP/Policy/Pricing → Validate → Review → Publish.
 *
 * It must run BEFORE the four-capability runtime traffic is generated,
 * and BEFORE the usage/system verification phase (golden-path-usage.spec.ts).
 *
 * Per audit P0-2: Browser tests are split into authoring/publish and
 * usage/system phases; the four-capability traffic runs between them.
 */

const ADMIN_PASSWORD = process.env.MEASIX_E2E_ADMIN_PASSWORD || 'admin'
const ADAPTER_URL = process.env.MEASIX_E2E_ADAPTER_URL || 'http://127.0.0.1:18099'

async function login(page: Page): Promise<void> {
  await page.goto('/admin/')
  await page.waitForSelector('[data-cy="login-username"]', { state: 'visible' })
  await page.fill('[data-cy="login-username"]', 'admin')
  await page.fill('[data-cy="login-password"]', ADMIN_PASSWORD)
  await page.click('[data-cy="login-submit"]')
  await expect(page).toHaveURL(/\/admin\/(overview)?$/)
}

/**
 * Helper: select an option from a q-select identified by data-cy.
 */
async function selectOption(page: Page, selectCy: string, optionMatcher: string | RegExp): Promise<void> {
  const select = page.locator(`[data-cy="${selectCy}"]`).first()
  await expect(select).toBeVisible({ timeout: 5_000 })
  await select.click()
  await page.waitForTimeout(300)
  const popup = page.locator('.q-popup, .q-menu').first()
  await expect(popup).toBeVisible({ timeout: 5_000 })
  await popup.locator(`text=${optionMatcher}`).first().click()
  await page.waitForTimeout(200)
}

test('CAP-C6-001-Authoring Login, Setup, Upstream Apply/Publish', async ({ page }: { page: Page }) => {
  // ========================================================================
  // Phase 1: Login as admin
  // ========================================================================
  await test.step('login as admin → Overview loads', async () => {
    await login(page)
    await expect(page.locator('[data-cy="overview-page"]')).toBeVisible()
  })

  // ========================================================================
  // Phase 2: Create user + enrollment
  // ========================================================================
  await test.step('create user + enrollment code', async () => {
    await page.goto('/admin/users')
    await expect(page.locator('[data-cy="users-page"]')).toBeVisible()

    const initialCount = await page.locator('[data-cy="user-row"]').count()

    await page.click('[data-cy="create-user-btn"]')
    await page.fill('[data-cy="user-form-username"]', `e2e-golden-${Date.now()}`)
    await page.fill('[data-cy="user-form-display-name"]', 'E2E Golden Path User')
    await page.click('[data-cy="user-form-submit"]')

    await expect(page.locator('[data-cy="user-row"]')).toHaveCount(initialCount + 1)

    await page.locator('[data-cy="user-row"]').last().click()

    await page.click('[data-cy="generate-enrollment-btn"]')
    await expect(page.locator('[data-cy="enrollment-code-field"]')).toBeVisible({ timeout: 10_000 })

    const codeField = page.locator('[data-cy="enrollment-code-field"]')
    await expect(codeField).not.toBeEmpty({ timeout: 10_000 })

    await page.keyboard.press('Escape')
  })

  // ========================================================================
  // Phase 3: Create secret → create upstream → Test → Apply
  // ========================================================================
  await test.step('create secret → create upstream → Test → Apply', async () => {
    await page.goto('/admin/upstreams')
    await expect(page.locator('[data-cy="upstreams-page"]')).toBeVisible()

    // Create secret first
    await page.click('button:has-text("Create Secret")')
    await expect(page.locator('[data-cy="secret-form-name"]')).toBeVisible()
    await page.fill('[data-cy="secret-form-name"]', `e2e-secret-${Date.now()}`)
    await page.fill('[data-cy="secret-form-value"]', 'sk-test-deterministic-key')
    await page.click('[data-cy="secret-form-submit"]')

    await expect(page.locator('[data-cy="secret-form-name"]')).not.toBeVisible({ timeout: 5_000 })

    // Create upstream
    await page.click('[data-cy="create-upstream-btn"]')
    await expect(page.locator('[data-cy="upstream-form-name"]')).toBeVisible()
    await page.fill('[data-cy="upstream-form-name"]', `e2e-upstream-${Date.now()}`)

    await page.fill('[data-cy="upstream-form-base-url"]', ADAPTER_URL)

    // Set auth type to BEARER
    const authSelect = page.locator('.q-card').locator('label:has-text("Auth Mode")').locator('..')
    await authSelect.click()
    await page.waitForTimeout(300)
    const popup = page.locator('.q-popup, .q-menu').locator('text=BEARER').first()
    await popup.click({ timeout: 5000 })

    await page.click('[data-cy="upstream-form-submit"]')

    await expect(page.locator('[data-cy="upstream-row"]')).toHaveCount(1, { timeout: 10_000 })

    // Open the upstream detail
    await page.locator('[data-cy="upstream-row"]').first().click()

    // Test the upstream
    await page.click('[data-cy="upstream-test-btn"]')
    await expect(page.locator('text=/reachable|Reachable/i')).toBeVisible({ timeout: 15_000 })

    // Accept the confirmation dialog before Apply
    // Per audit P0-1: window.confirm must be explicitly accepted.
    page.once('dialog', dialog => {
      console.log(`[test] Accepting confirm dialog: ${dialog.message()}`)
      dialog.accept()
    })

    // Apply the upstream
    await page.click('[data-cy="upstream-apply-btn"]')

    // Wait for apply to complete — must be exactly ACTIVE (not INACTIVE)
    await expect(page.locator('.q-chip').filter({ hasText: /^ACTIVE$/i }).first()).toBeVisible({ timeout: 30_000 })
    await expect(page.locator('.q-chip').filter({ hasText: /INACTIVE/i })).toHaveCount(0)

    await page.keyboard.press('Escape')
  })

  // ========================================================================
  // Phase 4: Create Provider + Model + TTS + ASR + MCP + Policy + Pricing
  // ========================================================================
  await test.step('create resources: Provider, Model, TTS, ASR, MCP, Policy, Pricing', async () => {
    await page.goto('/admin/resources')
    await expect(page.locator('[data-cy="resources-page"]')).toBeVisible()
    await expect(page.locator('[data-cy="tab-models"]')).toBeVisible({ timeout: 10_000 })

    // --- 4a: Create a Provider ---
    await page.click('text=Providers')
    await expect(page.locator('text=No providers')).toBeVisible({ timeout: 5_000 })
    await page.click('[data-cy="add-provider-btn"]')
    await page.waitForTimeout(300)
    const providerInput = page.locator('.q-list input').first()
    await providerInput.fill('E2E Test Provider')
    await page.waitForTimeout(200)

    // --- 4b: Create a Model ---
    await page.click('[data-cy="tab-models"]')
    await page.waitForTimeout(500)
    await page.click('[data-cy="add-model-btn"]')
    await page.waitForTimeout(500)
    await page.fill('[data-cy="model-display-name"]', 'E2E Test Model')
    await selectOption(page, 'model-provider-select', 'E2E Test Provider')
    await page.fill('[data-cy="model-upstream-key"]', 'gpt-4o')
    await selectOption(page, 'model-upstream-select', /e2e-upstream/)
    await page.fill('[data-cy="model-runtime-path"]', '/v1/chat/completions')

    // --- 4c: Create a TTS ---
    await page.click('[data-cy="tab-tts"]')
    await page.waitForTimeout(500)
    await page.click('[data-cy="add-tts-btn"]')
    await page.waitForTimeout(500)
    await page.fill('[data-cy="tts-display-name"]', 'E2E Test TTS')
    await page.fill('[data-cy="tts-model-key"]', 'tts-1')
    await page.fill('[data-cy="tts-voice"]', 'alloy')
    await selectOption(page, 'tts-upstream-select', /e2e-upstream/)
    await page.fill('[data-cy="tts-runtime-path"]', '/v1/audio/speech')

    // --- 4d: Create an ASR ---
    await page.click('[data-cy="tab-asr"]')
    await page.waitForTimeout(500)
    await page.click('[data-cy="add-asr-btn"]')
    await page.waitForTimeout(500)
    await page.fill('[data-cy="asr-display-name"]', 'E2E Test ASR')
    await page.fill('[data-cy="asr-model-key"]', 'whisper-1')
    await selectOption(page, 'asr-upstream-select', /e2e-upstream/)
    await page.fill('[data-cy="asr-runtime-path"]', '/v1/audio/transcriptions')

    // --- 4e: Create an MCP ---
    await page.click('[data-cy="tab-mcp"]')
    await page.waitForTimeout(500)
    await page.click('[data-cy="add-mcp-btn"]')
    await page.waitForTimeout(500)
    await page.fill('[data-cy="mcp-display-name"]', 'E2E Test MCP')
    await selectOption(page, 'mcp-upstream-select', /e2e-upstream/)
    await page.fill('[data-cy="mcp-runtime-path"]', '/mcp')

    // --- 4f: Configure Policy ---
    await page.click('[data-cy="tab-policy"]')
    await page.waitForTimeout(500)

    const policyFlags = [
      { label: 'Allow Local Models', dataCy: 'policy-allow-local-models' },
      { label: 'Allow Local TTS', dataCy: 'policy-allow-local-tts' },
      { label: 'Allow Local ASR', dataCy: 'policy-allow-local-asr' },
      { label: 'Allow Local MCP', dataCy: 'policy-allow-local-mcp' },
    ]
    for (const flag of policyFlags) {
      const toggle = page.getByRole('switch', { name: flag.label })
      await expect(toggle).toBeVisible({ timeout: 5_000 })
      await toggle.check({ force: true })
      await page.waitForTimeout(300)
      await expect(toggle).toBeChecked()
    }

    // Save the draft
    const saveBtn = page.locator('[data-cy="draft-save-btn"]')
    await expect(saveBtn).toBeEnabled({ timeout: 5_000 })
    await saveBtn.click()
    await expect(page.locator('.q-badge').filter({ hasText: /dirty/i })).not.toBeVisible({ timeout: 10_000 })

    // --- 4g: Configure Pricing ---
    await page.goto('/admin/usage')
    await expect(page.locator('[data-cy="usage-page"]')).toBeVisible({ timeout: 10_000 })

    await page.click('text=/pricing|Pricing/i')
    await page.waitForTimeout(500)

    await page.click('[data-cy="pricing-add-rule-btn"]')
    await page.waitForTimeout(500)

    const unitPriceInput = page.locator('[data-cy="pricing-unit-price"]').first()
    await expect(unitPriceInput).toBeVisible({ timeout: 5_000 })
    await unitPriceInput.fill('0.001')
    await page.waitForTimeout(200)

    const pricingSaveBtn = page.locator('[data-cy="pricing-save-btn"]')
    await expect(pricingSaveBtn).toBeVisible({ timeout: 5_000 })
    await pricingSaveBtn.click()
    await page.waitForTimeout(2_000)

    // Verify pricing persists after reload
    await page.reload()
    await expect(page.locator('[data-cy="usage-page"]')).toBeVisible({ timeout: 10_000 })
    await page.click('text=/pricing|Pricing/i')
    await page.waitForTimeout(500)
    const savedPriceInput = page.locator('[data-cy="pricing-unit-price"]').first()
    await expect(savedPriceInput).toBeVisible({ timeout: 5_000 })
    const savedValue = await savedPriceInput.inputValue()
    expect(savedValue).toBe('0.001')

    // Navigate back to resources
    await page.goto('/admin/resources')
    await expect(page.locator('[data-cy="resources-page"]')).toBeVisible()
    await expect(page.locator('[data-cy="tab-models"]')).toBeVisible({ timeout: 10_000 })
  })

  // ========================================================================
  // Phase 5: Validate draft
  // ========================================================================
  await test.step('validate draft — no blocking errors', async () => {
    await page.goto('/admin/resources')
    await expect(page.locator('[data-cy="resources-page"]')).toBeVisible()

    await page.click('[data-cy="draft-validate-btn"]')
    await page.waitForTimeout(2_000)

    const errorChip = page.locator('.q-chip.text-negative, .q-chip.bg-negative').first()
    const hasErrors = await errorChip.isVisible().catch(() => false)
    expect(hasErrors, 'Validation should not have blocking errors').toBe(false)
    const validationChip = page.locator('.q-chip').filter({ hasText: /valid|warning|error/i }).first()
    await expect(validationChip).toBeVisible({ timeout: 10_000 })
  })

  // ========================================================================
  // Phase 6: Review Client Snapshot Preview
  // ========================================================================
  await test.step('review client snapshot preview — no server-only fields', async () => {
    await page.goto('/admin/resources')
    await expect(page.locator('[data-cy="resources-page"]')).toBeVisible()

    await page.click('[data-cy="draft-preview-btn"]')

    await expect(page.locator('text=/projection hash|Projection Hash|hash/i')).toBeVisible({ timeout: 10_000 })

    const previewDialog = page.locator('.q-dialog')
    await expect(previewDialog).toBeVisible()
    await expect(previewDialog.locator('text=Providers').first()).toBeVisible()
    await expect(previewDialog.locator('text=Models').first()).toBeVisible()
    await expect(previewDialog.locator('text=TTS').first()).toBeVisible()
    await expect(previewDialog.locator('text=ASR').first()).toBeVisible()
    await expect(previewDialog.locator('text=MCP').first()).toBeVisible()

    // Verify no server-only data leaks
    const previewContent = await page.locator('.q-dialog').textContent()
    expect(previewContent).not.toMatch(/http:\/\/\S+/)
    expect(previewContent).not.toMatch(/sec_[a-f0-9-]+/i)
    expect(previewContent).not.toMatch(/ups_[a-f0-9-]+/i)

    await page.keyboard.press('Escape')
    await page.waitForTimeout(500)
  })

  // ========================================================================
  // Phase 7: Publish → wait activation completed
  // ========================================================================
  await test.step('publish draft → wait activation completed → verify generation increment', async () => {
    // Re-apply the upstream to ensure it is ACTIVE before publishing
    await page.goto('/admin/upstreams')
    await expect(page.locator('[data-cy="upstreams-page"]')).toBeVisible()
    await expect(page.locator('[data-cy="upstream-row"]')).toBeVisible({ timeout: 5_000 })
    await page.locator('[data-cy="upstream-row"]').first().click()
    await page.waitForTimeout(500)
    const applyBtn = page.locator('[data-cy="upstream-apply-btn"]')
    if (await applyBtn.isVisible().catch(() => false)) {
      page.once('dialog', dialog => {
        console.log(`[test] Accepting confirm dialog: ${dialog.message()}`)
        dialog.accept()
      })
      await applyBtn.click()
      await expect(page.locator('.q-chip').filter({ hasText: /^ACTIVE$/i }).first()).toBeVisible({ timeout: 30_000 })
    }
    await page.keyboard.press('Escape')

    await page.goto('/admin/resources')
    await expect(page.locator('[data-cy="resources-page"]')).toBeVisible()

    // Record pre-publish generation
    const genBadge = page.locator('[data-cy="active-generation-badge"]')
    let prePublishGeneration = 0
    if (await genBadge.isVisible().catch(() => false)) {
      const genText = await genBadge.textContent()
      const match = genText?.match(/(\d+)/)
      if (match) prePublishGeneration = parseInt(match[1], 10)
    }

    // Click Review
    await page.click('[data-cy="draft-review-btn"]')
    await expect(page.locator('.q-dialog').locator('text=/review|Review/i')).toBeVisible({ timeout: 10_000 })

    // Accept publish confirm dialog
    page.once('dialog', dialog => {
      console.log(`[test] Accepting publish confirm dialog: ${dialog.message()}`)
      dialog.accept()
    })

    await page.click('[data-cy="draft-publish-btn"]')

    // Check for publish error
    await page.waitForTimeout(2_000)
    const errorBanner = page.locator('.q-banner.bg-red-1, .q-banner .text-negative').first()
    if (await errorBanner.isVisible().catch(() => false)) {
      const errorText = await errorBanner.textContent()
      const pageSnapshot = await page.evaluate(() => document.body.innerText.slice(0, 2000))
      throw new Error(`Publish failed: ${errorText}\nPage snapshot: ${pageSnapshot}`)
    }

    // Wait for activation to complete
    await expect(page.locator('text=/COMPLETED|completed/i')).toBeVisible({ timeout: 60_000 })

    // Verify generation increment
    if (prePublishGeneration > 0) {
      await page.reload()
      await expect(page.locator('[data-cy="resources-page"]')).toBeVisible({ timeout: 10_000 })
      const postGenBadge = page.locator('[data-cy="active-generation-badge"]')
      if (await postGenBadge.isVisible().catch(() => false)) {
        const postGenText = await postGenBadge.textContent()
        const postMatch = postGenText?.match(/(\d+)/)
        if (postMatch) {
          const postPublishGeneration = parseInt(postMatch[1], 10)
          expect(postPublishGeneration).toBeGreaterThan(prePublishGeneration)
        }
      }
    }

    await page.keyboard.press('Escape')
    await page.waitForTimeout(500)
  })

  // ========================================================================
  // Phase 8: Reload — verify state persists
  // ========================================================================
  await test.step('reload page → state persists after publish, Policy flags verified', async () => {
    await page.reload()
    await expect(page).toHaveURL(/\/admin\/resources/)
    await expect(page.locator('[data-cy="resources-page"]')).toBeVisible()

    await expect(page.locator('[data-cy="tab-models"]')).toBeVisible({ timeout: 10_000 })

    await page.click('[data-cy="tab-models"]')
    await page.waitForTimeout(500)

    const modelItems = page.locator('.q-card:has-text("Models") .q-item')
    await expect(modelItems.first()).toBeVisible({ timeout: 5_000 })

    // Verify Policy flags are still ON
    await page.click('[data-cy="tab-policy"]')
    await page.waitForTimeout(500)
    const policyFlagsAfter = [
      'Allow Local Models',
      'Allow Local TTS',
      'Allow Local ASR',
      'Allow Local MCP',
    ]
    for (const flagLabel of policyFlagsAfter) {
      const toggle = page.getByRole('switch', { name: flagLabel })
      await expect(toggle).toBeVisible({ timeout: 5_000 })
      await expect(toggle).toBeChecked()
    }
  })
})
