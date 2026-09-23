import { test, expect, type Page } from '@playwright/test'

test('Admin remembered login works over HTTP with an explicit transport warning', async ({ browser, page }) => {
  await page.goto('/admin/', { waitUntil: 'domcontentloaded' })
  await page.fill('[data-cy="login-username"]', 'admin')
  await page.fill('[data-cy="login-password"]', ADMIN_PASSWORD)
  await page.locator('[data-cy="login-remember"]').click()
  if (new URL(page.url()).protocol === 'http:') {
    await expect(page.locator('[data-cy="login-remember-warning"]')).toBeVisible()
  }
  await page.click('[data-cy="login-submit"]')
  await expect(page).toHaveURL(/\/admin\/(overview)?$/)

  const cookie = (await page.context().cookies()).find(item => item.name === 'measix_admin_session')
  expect(cookie).toBeDefined()
  expect(cookie!.httpOnly).toBe(true)
  expect(cookie!.sameSite).toBe('Strict')
  expect(cookie!.secure).toBe(new URL(page.url()).protocol === 'https:')
  expect(cookie!.expires).toBeGreaterThan(Date.now() / 1000 + 29 * 24 * 60 * 60)

  const restoredContext = await browser.newContext({ storageState: await page.context().storageState() })
  const restoredPage = await restoredContext.newPage()
  await restoredPage.goto('/admin/', { waitUntil: 'domcontentloaded' })
  await expect(restoredPage).toHaveURL(/\/admin\/(overview)?$/)
  await restoredContext.close()
})

test('ERX-UPD-001/002 Admin update authoring, safe preview, publish, withdraw and delete', async ({ page }) => {
  await login(page)
  await page.goto('/admin/enterprise-updates')
  const updates = page.locator('[data-cy="enterprise-updates-page"]')
  const title = `Release notice ${Date.now()}`
  const editedTitle = `${title} edited`
  const embeddedRequests: string[] = []
  page.on('request', request => {
    if (request.url().includes('untrusted-embed.png')) embeddedRequests.push(request.url())
  })
  await updates.getByRole('button', { name: 'Create', exact: true }).click()
  let editor = updates.locator('[data-cy="enterprise-update-editor"]')
  await editor.getByLabel('Title', { exact: true }).fill(title)
  await editor.getByLabel('Content', { exact: true }).fill('**safe** <b>raw</b>\n\n![blocked](/untrusted-embed.png)')
  await editor.getByLabel('Format', { exact: true }).click()
  await page.getByRole('option', { name: 'Formatted text (Markdown)', exact: true }).click()
  await editor.getByRole('button', { name: 'Create', exact: true }).click()
  let row = updates.locator('.q-list > .q-item').filter({ hasText: title })
  await expect(row).toContainText('Draft')
  await row.click()
  const detail = updates.locator('[data-cy="enterprise-update-detail"]')
  await expect(detail.locator('.markdown-body strong')).toHaveText('safe')
  await expect(detail.locator('.markdown-body')).toContainText('<b>raw</b>')
  await expect(detail.locator('.markdown-body img, .markdown-body b')).toHaveCount(0)
  await detail.getByRole('button', { name: 'Edit', exact: true }).click()
  editor = updates.locator('[data-cy="enterprise-update-editor"]')
  await editor.getByLabel('Title', { exact: true }).fill(editedTitle)
  await editor.getByRole('button', { name: 'Save', exact: true }).click()
  row = updates.locator('.q-list > .q-item').filter({ hasText: editedTitle })
  await expect(row).toContainText('Draft')
  page.once('dialog', prompt => prompt.accept())
  await row.getByRole('button', { name: 'Publish', exact: true }).click()
  await expect(row).toContainText('Published')
  page.once('dialog', prompt => prompt.accept())
  await row.getByRole('button', { name: 'Withdraw', exact: true }).click()
  await expect(row).toContainText('Withdrawn')
  await page.reload()
  await expect(row).toContainText('Withdrawn')
  page.once('dialog', prompt => prompt.accept())
  await row.getByRole('button', { name: 'Delete', exact: true }).click()
  await expect(row).toHaveCount(0)

  const draftTitle = `Unused draft ${Date.now()}`
  await updates.getByRole('button', { name: 'Create', exact: true }).click()
  editor = updates.locator('[data-cy="enterprise-update-editor"]')
  await editor.getByLabel('Title', { exact: true }).fill(draftTitle)
  await editor.getByLabel('Content', { exact: true }).fill('Never published')
  await editor.getByRole('button', { name: 'Create', exact: true }).click()
  const draftRow = updates.locator('.q-list > .q-item').filter({ hasText: draftTitle })
  await expect(draftRow).toContainText('Draft')
  page.once('dialog', prompt => prompt.accept())
  await draftRow.getByRole('button', { name: 'Delete', exact: true }).click()
  await expect(draftRow).toHaveCount(0)
  expect(embeddedRequests).toEqual([])
})

/**
 * CAP-C6-001-Authoring — Browser Golden Path Phase 1-8 (Authoring + Publish).
 *
 * This spec covers the authoring and publish phase of the C6 Golden Path:
 *   login → user/enrollment → secret → upstream test/apply →
 *   Provider/Model/TTS/ASR/MCP/Policy/Pricing → Validate → Review → Publish.
 *
 * It must run BEFORE the five-capability runtime traffic is generated,
 * and BEFORE the usage/system verification phase (golden-path-usage.spec.ts).
 *
 * Per audit P0-2: Browser tests are split into authoring/publish and
 * usage/system phases; the five-capability traffic runs between them.
 */

const ADMIN_PASSWORD = process.env.MEASIX_E2E_ADMIN_PASSWORD || 'admin'
const ADAPTER_URL = process.env.MEASIX_E2E_ADAPTER_URL || 'http://127.0.0.1:18099'

async function login(page: Page): Promise<void> {
  await page.goto('/admin/', { waitUntil: 'domcontentloaded' })
  await page.waitForSelector('[data-cy="login-username"]', { state: 'visible' })
  await expect(page.locator('[data-cy="login-password-toggle"]')).toBeVisible()
  const remember = page.locator('[data-cy="login-remember"]')
  await expect(remember).toBeVisible()
  await expect(remember).toBeEnabled()
  if (new URL(page.url()).protocol === 'http:') {
    await remember.click()
    await expect(page.locator('[data-cy="login-remember-warning"]')).toBeVisible()
    await remember.click()
  }
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
  const entityPicker = page.locator('[data-cy="entity-picker-dialog"]')
  if (await entityPicker.isVisible()) {
    await entityPicker.locator('[data-cy="entity-picker-option"]').filter({ hasText: optionMatcher }).first().click()
  } else {
    const popup = page.locator('.q-menu').first()
    await expect(popup).toBeVisible({ timeout: 5_000 })
    await popup.getByText(optionMatcher, { exact: typeof optionMatcher === 'string' }).first().click()
  }
  await page.waitForTimeout(200)
}

test('CAP-C6-001-Authoring Login, Setup, Upstream Apply/Publish', async ({ page }: { page: Page }) => {
  const goldenUsername = `e2e-golden-${Date.now()}`
  const budgetTemplateName = `E2E image template ${Date.now()}`
  const fallbackTemplateName = `E2E fallback template ${Date.now()}`
  // ========================================================================
  // Phase 1: Login as admin
  // ========================================================================
  await test.step('login as admin → Overview loads', async () => {
    await login(page)
    await expect(page.locator('[data-cy="overview-page"]')).toBeVisible()
  })

  await test.step('create a reusable image-generation budget template', async () => {
    await page.goto('/admin/budget-templates')
    await page.getByRole('button', { name: 'Create template', exact: true }).click()
    await page.getByLabel('Template name', { exact: true }).fill(budgetTemplateName)
    await page.getByLabel('Description', { exact: true }).fill('Live-linked browser verification template')

    const imageRule = page.locator('[data-cy="template-rule-IMAGE_GENERATION"]')
    await imageRule.getByText('Template rule', { exact: true }).click()
    await imageRule.getByRole('button', { name: 'Limited', exact: true }).click()
    await imageRule.getByLabel('Meter', { exact: true }).click()
    await page.getByRole('option', { name: 'Requested images', exact: true }).click()
    await imageRule.getByLabel('Limit', { exact: true }).fill('9')
    await page.getByLabel('Reason for change', { exact: true }).fill('Create browser verification template')
    await page.getByRole('button', { name: 'Save', exact: true }).last().click()
    await page.locator('[data-cy="confirm-budget-template-save"]').click()
    await expect(page.locator('[data-cy="budget-template-row"]').filter({ hasText: budgetTemplateName })).toBeVisible()

    await page.getByRole('button', { name: 'Create template', exact: true }).click()
    await page.getByLabel('Template name', { exact: true }).fill(fallbackTemplateName)
    await page.getByLabel('Description', { exact: true }).fill('Template with deployment-default fallthrough')
    await page.getByLabel('Reason for change', { exact: true }).fill('Create replacement verification template')
    await page.getByRole('button', { name: 'Save', exact: true }).last().click()
    await page.locator('[data-cy="confirm-budget-template-save"]').click()
    await expect(page.locator('[data-cy="budget-template-row"]').filter({ hasText: fallbackTemplateName })).toBeVisible()
  })

  // ========================================================================
  // Phase 2: Create user + enrollment
  // ========================================================================
  await test.step('create user + enrollment code', async () => {
    await page.goto('/admin/users')
    await expect(page.locator('[data-cy="users-page"]')).toBeVisible()

    const initialCount = await page.locator('[data-cy="user-row"]').count()

    await page.click('[data-cy="create-user-btn"]')
    await page.fill('[data-cy="user-form-username"]', goldenUsername)
    await page.fill('[data-cy="user-form-display-name"]', 'E2E Golden Path User')
    await page.click('[data-cy="user-form-submit"]')

    await expect(page.locator('[data-cy="user-row"]')).toHaveCount(initialCount + 1)

    await page.locator('[data-cy="user-row"]').filter({ hasText: goldenUsername }).click()
    await page.getByRole('tab', { name: 'Usage budgets', exact: true }).click()

    const templateAssignment = page.locator('[data-cy="budget-template-assignment"]')
    await templateAssignment.locator('[data-cy="entity-picker-trigger"]').click()
    await page.locator('[data-cy="entity-picker-option"]').filter({ hasText: budgetTemplateName }).click()
    await templateAssignment.getByLabel('Reason for change', { exact: true }).fill('Assign browser verification template')
    await templateAssignment.getByRole('button', { name: 'Assign / replace', exact: true }).click()
    await page.locator('[data-cy="confirm-budget-template-assignment"]').click()

    const imageBudget = page.locator('[data-cy="budget-IMAGE_GENERATION"]')
    await expect(imageBudget).toContainText('Budget template')
    await expect(imageBudget).toContainText('9')

    await templateAssignment.locator('[data-cy="entity-picker-trigger"]').click()
    await page.locator('[data-cy="entity-picker-option"]').filter({ hasText: fallbackTemplateName }).click()
    await templateAssignment.getByLabel('Reason for change', { exact: true }).fill('Replace browser verification template')
    await templateAssignment.getByRole('button', { name: 'Assign / replace', exact: true }).click()
    await page.locator('[data-cy="confirm-budget-template-assignment"]').click()
    await expect(imageBudget).toContainText('Deployment default')

    await templateAssignment.locator('[data-cy="entity-picker-trigger"]').click()
    await page.locator('[data-cy="entity-picker-option"]').filter({ hasText: budgetTemplateName }).click()
    await templateAssignment.getByLabel('Reason for change', { exact: true }).fill('Restore image template before unassign')
    await templateAssignment.getByRole('button', { name: 'Assign / replace', exact: true }).click()
    await page.locator('[data-cy="confirm-budget-template-assignment"]').click()
    await expect(imageBudget).toContainText('Budget template')

    await templateAssignment.getByLabel('Reason for change', { exact: true }).fill('Verify template unassignment')
    await templateAssignment.getByRole('button', { name: 'Unassign', exact: true }).click()
    await page.locator('[data-cy="confirm-budget-template-assignment"]').click()
    await expect(imageBudget).toContainText('Deployment default')

    await templateAssignment.locator('[data-cy="entity-picker-trigger"]').click()
    await page.locator('[data-cy="entity-picker-option"]').filter({ hasText: budgetTemplateName }).click()
    await templateAssignment.getByLabel('Reason for change', { exact: true }).fill('Restore image template for propagation check')
    await templateAssignment.getByRole('button', { name: 'Assign / replace', exact: true }).click()
    await page.locator('[data-cy="confirm-budget-template-assignment"]').click()
    await expect(imageBudget).toContainText('Budget template')

    await page.goto('/admin/budget-templates')
    await page.locator('[data-cy="budget-template-row"]').filter({ hasText: budgetTemplateName }).click()
    await page.getByLabel('Reason for change', { exact: true }).fill('Delete must remain blocked while assigned')
    await expect(page.getByRole('button', { name: 'Delete', exact: true })).toBeDisabled()
    await expect(page.getByText(/Remove assignments before deleting/i)).toBeVisible()
    const imageTemplateRule = page.locator('[data-cy="template-rule-IMAGE_GENERATION"]')
    await imageTemplateRule.getByLabel('Limit', { exact: true }).fill('12')
    await page.getByLabel('Reason for change', { exact: true }).fill('Verify live-linked propagation')
    await page.getByRole('button', { name: 'Save', exact: true }).last().click()
    await page.locator('[data-cy="confirm-budget-template-save"]').click()

    await page.goto('/admin/users')
    await page.locator('[data-cy="user-row"]').filter({ hasText: goldenUsername }).click()
    await page.getByRole('tab', { name: 'Usage budgets', exact: true }).click()
    await expect(imageBudget).toContainText('Budget template')
    await expect(imageBudget).toContainText('12')
    await imageBudget.getByRole('button', { name: 'Edit budget' }).click()
    await imageBudget.getByRole('button', { name: 'Limited', exact: true }).click()
    await imageBudget.getByLabel('Limit', { exact: true }).fill('3')
    await imageBudget.getByLabel('Reason for change').fill('Verify image capability override')
    await imageBudget.locator('[data-cy="save-budget"]').click()
    await expect(imageBudget).toContainText('User override')
    await imageBudget.locator('[data-cy="clear-budget-override-IMAGE_GENERATION"]').click()
    await expect(imageBudget).toContainText('Budget template')
    await expect(imageBudget).toContainText('12')

    const modelBudget = page.locator('[data-cy="budget-MODEL"]')
    await expect(modelBudget).toBeVisible({ timeout: 10_000 })
    await expect(page.locator('.budget-card[data-cy^="budget-"]')).toHaveCount(5)
    await expect(page.locator('[data-cy="budget-IMAGE_GENERATION"]')).toBeVisible()
    await expect(modelBudget).toContainText('Deployment default')
    await modelBudget.getByRole('button', { name: 'Edit budget' }).click()
    await modelBudget.getByRole('button', { name: 'Limited', exact: true }).click()
    await modelBudget.getByLabel('Limit', { exact: true }).fill('2')
    await modelBudget.getByLabel('Reason for change').fill('Browser production budget verification')
    await modelBudget.locator('[data-cy="save-budget"]').click()
    await expect(modelBudget).toContainText('User override')
    await expect(modelBudget).toContainText('2')
    await page.screenshot({ path: '../.artifacts/admin-user-budget.png', fullPage: true })

    await page.click('[data-cy="generate-enrollment-btn"]')
    await expect(page.locator('[data-cy="enrollment-material-field"]')).toBeVisible({ timeout: 10_000 })
    await page.locator('[data-cy="enrollment-code-details"] summary').click()
    await expect(page.locator('[data-cy="enrollment-code-field"]')).toBeVisible({ timeout: 10_000 })

    const codeField = page.locator('[data-cy="enrollment-code-field"]')
    await expect(codeField).not.toBeEmpty({ timeout: 10_000 })
    const retiredEnrollmentCode = await codeField.inputValue()
    const materialField = page.locator('[data-cy="enrollment-material-field"]')
    const material = JSON.parse(await materialField.inputValue())
    expect(material).toEqual({
      formatVersion: 1,
      kind: 'PLATFORM_ENROLLMENT',
      platformUrl: new URL(page.url()).origin,
      code: await codeField.inputValue(),
      expiresAt: expect.any(String),
    })
    const enrollmentRemainingMillis = Date.parse(material.expiresAt) - Date.now()
    expect(enrollmentRemainingMillis).toBeGreaterThan(55 * 60 * 1000)
    expect(enrollmentRemainingMillis).toBeLessThanOrEqual(60 * 60 * 1000)
    await expect(page.locator('[data-cy="copy-enrollment-material"]')).toBeVisible()

    await page.keyboard.press('Escape')

    await page.getByRole('button', { name: /Actions/ }).click()
    await page.locator('[data-cy="delete-user-btn"]').click()
    const deleteConfirm = page.locator('[data-cy="confirm-delete-user"]')
    await expect(deleteConfirm).toBeDisabled()
    await page.locator('[data-cy="delete-user-confirmation"]').fill(`${goldenUsername}-wrong`)
    await page.locator('[data-cy="delete-user-reason"]').fill('Remove isolated browser verification user')
    await expect(deleteConfirm).toBeDisabled()
    await page.locator('[data-cy="delete-user-confirmation"]').fill(goldenUsername)
    await expect(deleteConfirm).toBeEnabled()
    await deleteConfirm.click()
    await expect(page.locator('[data-cy="user-row"]')).toHaveCount(initialCount)

    await page.click('[data-cy="create-user-btn"]')
    await page.fill('[data-cy="user-form-username"]', goldenUsername)
    await page.fill('[data-cy="user-form-display-name"]', 'E2E Recreated User')
    await page.click('[data-cy="user-form-submit"]')
    await expect(page.locator('[data-cy="user-row"]')).toHaveCount(initialCount + 1)
    await page.locator('[data-cy="user-row"]').filter({ hasText: goldenUsername }).click()
    await page.getByRole('tab', { name: 'Usage budgets', exact: true }).click()

    const freshModelBudget = page.locator('[data-cy="budget-MODEL"]')
    await expect(freshModelBudget).toBeVisible({ timeout: 10_000 })
    await expect(freshModelBudget).toContainText('Deployment default')
    await expect(freshModelBudget).not.toContainText('User override')

    await page.click('[data-cy="generate-enrollment-btn"]')
    await expect(page.locator('[data-cy="enrollment-material-field"]')).toBeVisible({ timeout: 10_000 })
    await page.locator('[data-cy="enrollment-code-details"] summary').click()
    const freshEnrollmentCode = await page.locator('[data-cy="enrollment-code-field"]').inputValue()
    expect(freshEnrollmentCode).not.toBe(retiredEnrollmentCode)
    await page.keyboard.press('Escape')

    await page.getByRole('button', { name: /Actions/ }).click()
    await page.locator('[data-cy="delete-user-btn"]').click()
    await page.locator('[data-cy="delete-user-confirmation"]').fill(goldenUsername)
    await page.locator('[data-cy="delete-user-reason"]').fill('Remove recreated browser verification user')
    await page.locator('[data-cy="confirm-delete-user"]').click()
    await expect(page.locator('[data-cy="user-row"]')).toHaveCount(initialCount)
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
    await page.locator('.q-dialog .q-select').first().click()
    await page.getByRole('option', { name: 'Bearer token', exact: true }).click({ timeout: 5000 })

    await page.click('[data-cy="upstream-form-submit"]')

    await expect(page.locator('[data-cy="upstream-row"]')).toHaveCount(1, { timeout: 10_000 })

    // Open the upstream detail
    await page.locator('[data-cy="upstream-row"]').first().click()

    // Test the upstream
    await page.click('[data-cy="upstream-test-btn"]')
    await expect(page.locator('text=/reachable|Reachable/i')).toBeVisible({ timeout: 15_000 })
    await expect(page.locator('[data-cy="upstream-test-http-status"]')).toHaveText(/^\d{3}$/)
    await expect(page.getByText(/This checks connectivity only/)).toBeVisible()

    // Apply the upstream
    await page.click('[data-cy="upstream-apply-btn"]')
    await page.click('[data-cy="upstream-apply-confirm"]')

    // Wait for apply to complete — must be exactly ACTIVE (not INACTIVE)
    await expect(page.locator('.q-chip').filter({ hasText: /^ACTIVE$/i }).first()).toBeVisible({ timeout: 30_000 })
    await expect(page.locator('.q-chip').filter({ hasText: /INACTIVE/i })).toHaveCount(0)

    await page.keyboard.press('Escape')
  })

  // ========================================================================
  // Phase 4: Create Provider + Model + Image Generation + TTS + ASR + MCP + Policy + Pricing
  // ========================================================================
  await test.step('create resources: Provider, Model, Image Generation, TTS, ASR, MCP, Policy, Pricing', async () => {
    await page.goto('/admin/resources')
    await expect(page.locator('[data-cy="resources-page"]')).toBeVisible()
    await expect(page.locator('[data-cy="config-section-models"]')).toBeVisible({ timeout: 10_000 })

    // --- 4a: Create a Provider ---
    await page.click('text=Providers')
    await expect(page.locator('text=No providers')).toBeVisible({ timeout: 5_000 })
    await page.click('[data-cy="add-provider-btn"]')
    await page.waitForTimeout(300)
    const providerInput = page.locator('.q-list input').first()
    await providerInput.fill('E2E Test Provider')
    await page.waitForTimeout(200)

    // --- 4b: Create a Model ---
    await page.click('[data-cy="config-section-models"]')
    await page.waitForTimeout(500)
    await page.click('[data-cy="add-model-btn"]')
    await page.waitForTimeout(500)
    await page.fill('[data-cy="model-display-name"]', 'E2E Test Model')
    await selectOption(page, 'model-provider-select', 'E2E Test Provider')
    await page.fill('[data-cy="model-upstream-key"]', 'gpt-4o')
    await selectOption(page, 'model-upstream-select', /e2e-upstream/)
    await page.fill('[data-cy="model-runtime-path"]', '/v1/chat/completions')
    await selectOption(page, 'model-input-modalities', 'Images')

    // --- 4c: Create an Image Generation resource ---
    await page.click('[data-cy="config-section-image-generation"]')
    await page.waitForTimeout(500)
    await page.click('[data-cy="add-image-generation-btn"]')
    await page.waitForTimeout(500)
    await page.fill('[data-cy="image-generation-display-name"]', 'E2E Image Generation')
    await page.fill('[data-cy="image-generation-model-key"]', 'wan2.7-image')
    await page.fill('[data-cy="image-generation-max-images"]', '4')
    await selectOption(page, 'image-generation-protocol', 'DashScope Multimodal Generation')
    await selectOption(page, 'image-generation-upstream-select', /e2e-upstream/)
    await expect(page.locator('[data-cy="image-generation-runtime-path"]')).toHaveValue('/api/v1/services/aigc/multimodal-generation/generation')

    // --- 4d: Create a TTS ---
    await page.click('[data-cy="config-section-tts"]')
    await page.waitForTimeout(500)
    await page.click('[data-cy="add-tts-btn"]')
    await page.waitForTimeout(500)
    await page.fill('[data-cy="tts-display-name"]', 'E2E Test TTS')
    await page.fill('[data-cy="tts-model-key"]', 'tts-1')
    await page.fill('[data-cy="tts-voice"]', 'alloy')
    await selectOption(page, 'tts-upstream-select', /e2e-upstream/)
    await page.fill('[data-cy="tts-runtime-path"]', '/v1/audio/speech')

    // --- 4e: Create an ASR ---
    await page.click('[data-cy="config-section-asr"]')
    await page.waitForTimeout(500)
    await page.click('[data-cy="add-asr-btn"]')
    await page.waitForTimeout(500)
    await page.fill('[data-cy="asr-display-name"]', 'E2E Test ASR')
    await page.fill('[data-cy="asr-model-key"]', 'whisper-1')
    await selectOption(page, 'asr-upstream-select', /e2e-upstream/)
    await page.fill('[data-cy="asr-runtime-path"]', '/v1/audio/transcriptions')

    // --- 4f: Create an MCP ---
    await page.click('[data-cy="config-section-mcp"]')
    await page.waitForTimeout(500)
    await page.click('[data-cy="add-mcp-btn"]')
    await page.waitForTimeout(500)
    await page.fill('[data-cy="mcp-display-name"]', 'E2E Test MCP')
    await selectOption(page, 'mcp-upstream-select', /e2e-upstream/)
    await page.fill('[data-cy="mcp-runtime-path"]', '/mcp')

    // --- 4g: Configure Policy ---
    await page.click('[data-cy="config-section-policy"]')
    await page.waitForTimeout(500)

    const policyFlags = [
      { label: 'Allow user Providers/models', dataCy: 'policy-allow-local-models' },
      { label: 'Allow user TTS', dataCy: 'policy-allow-local-tts' },
      { label: 'Allow user ASR', dataCy: 'policy-allow-local-asr' },
      { label: 'Allow user MCP', dataCy: 'policy-allow-local-mcp' },
      { label: 'Allow user assistants', dataCy: 'policy-allow-local-assistants' },
    ]
    for (const flag of policyFlags) {
      const toggle = page.getByRole('switch', { name: flag.label })
      await expect(toggle).toBeVisible({ timeout: 5_000 })
      await toggle.click({ force: true })
      await page.waitForTimeout(300)
      await expect(toggle).toHaveAttribute('aria-checked', 'true')
    }
    for (const selector of [
      'policy-default-model',
      'policy-default-fast-model',
      'policy-default-title-model',
      'policy-default-attachment-inspection-model',
      'policy-default-suggestion-model',
      'policy-default-compress-model',
    ]) {
      await selectOption(page, selector, 'E2E Test Model')
    }
    await selectOption(page, 'policy-default-tts', 'E2E Test TTS')
    await selectOption(page, 'policy-default-asr', 'E2E Test ASR')
    await selectOption(page, 'policy-default-image-generation', 'E2E Image Generation')
    const imageDefault = page.locator('[data-cy="policy-default-image-generation"]')
    await imageDefault.click()
    await page.keyboard.press('Backspace')
    await expect(imageDefault).not.toContainText('E2E Image Generation')

    // S0.2 typed experience authoring shares this same Draft and Publish.
    await page.click('[data-cy="config-section-assistants"]')
    await page.click('[data-cy="assistant-add"]')
    await page.locator('[data-cy="assistant-name"]').fill('E2E Assistant')
    await page.click('[data-cy="assistant-section-connections"]')
    await selectOption(page, 'assistant-model', 'E2E Test Model')
    await page.click('[data-cy="assistant-section-prompt"]')
    await page.locator('[data-cy="assistant-prompt"]').fill('Synthetic enterprise guidance')
    await page.click('[data-cy="assistant-section-memory"]')
    await page.click('[data-cy="seed-add"]')
    await page.locator('[data-cy="seed-input-0"]').fill('z authored first')
    await page.click('[data-cy="seed-add"]')
    await page.locator('[data-cy="seed-input-1"]').fill('a authored second')
    await page.click('[data-cy="assistant-section-starters"]')
    await page.click('[data-cy="starter-add"]')
    await page.locator('[data-cy="starter-title"]').fill('E2E Starter')
    await page.locator('[data-cy="starter-prompt"]').fill('Synthetic starter question')

    await page.click('[data-cy="config-section-policy"]')
    await selectOption(page, 'policy-default-assistant', 'E2E Assistant')

    // Save the draft
    const saveBtn = page.locator('[data-cy="draft-save-btn"]')
    await expect(saveBtn).toBeEnabled({ timeout: 5_000 })
    await saveBtn.click()
    await expect(page.locator('.q-badge').filter({ hasText: /dirty/i })).not.toBeVisible({ timeout: 10_000 })

    // --- 4h: Configure Pricing ---
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
    await expect(page.locator('[data-cy="config-section-models"]')).toBeVisible({ timeout: 10_000 })
  })

  // ========================================================================
  // Phase 5: Validate draft
  // ========================================================================
  await test.step('validate draft — no blocking errors', async () => {
    await page.goto('/admin/resources')
    await expect(page.locator('[data-cy="resources-page"]')).toBeVisible()

    await page.click('[data-cy="draft-validate-btn"]')
    await page.waitForTimeout(2_000)

    const validationSummary = page.locator('[data-cy="draft-validation-summary"]')
    await expect(validationSummary.locator('.q-banner')).toContainText('Draft is valid.', { timeout: 10_000 })
  })

  // ========================================================================
  // Phase 6: Review Client Snapshot Preview
  // ========================================================================
  await test.step('review client snapshot preview — no server-only fields', async () => {
    await page.goto('/admin/resources')
    await expect(page.locator('[data-cy="resources-page"]')).toBeVisible()

    const responsePromise = page.waitForResponse(r => r.url().endsWith('/api/admin/v1/draft:preview') && r.request().method() === 'POST')
    await page.click('[data-cy="draft-preview-btn"]')
    const previewResponse = await responsePromise
    expect(previewResponse.status()).toBe(200)
    const projection = await previewResponse.json()
    expect(projection.assistants).toHaveLength(1)
    expect(projection.imageGenerators).toHaveLength(1)
    expect(projection.imageGenerators[0]).toMatchObject({
      displayName: 'E2E Image Generation',
      upstreamModelKey: 'wan2.7-image',
      clientProtocol: 'DASHSCOPE_MULTIMODAL_GENERATION',
      runtimePath: '/api/v1/services/aigc/multimodal-generation/generation',
      allowedSizes: ['1024x1024'],
      maxImagesPerRequest: 4,
    })
    expect(projection.policy.defaultImageGenerationId).toBeUndefined()
    expect(projection.assistants[0].memorySeed).toEqual(['z authored first', 'a authored second'])
    expect(projection.starters).toHaveLength(1)
    expect(projection.starters[0].assistantDefinitionId).toBe(projection.assistants[0].assistantDefinitionId)

    const previewSurface = page.locator('[data-cy="snapshot-preview-surface"]')
    await expect(previewSurface).toBeVisible()
    await expect(previewSurface.getByText(/Hash:/i)).toBeVisible({ timeout: 10_000 })
    await expect(previewSurface.getByText(/Providers \(1\)/)).toBeVisible()
    await expect(previewSurface.getByText(/Models \(1\)/)).toBeVisible()
    await expect(previewSurface.getByText(/Image Generation \(1\)/)).toBeVisible()
    await expect(previewSurface.getByText(/TTS \(1\)/)).toBeVisible()
    await expect(previewSurface.getByText(/ASR \(1\)/)).toBeVisible()
    await expect(previewSurface.getByText(/MCP \(1\)/)).toBeVisible()

    // Verify no server-only data leaks
    const previewContent = await previewSurface.textContent()
    expect(previewContent).not.toMatch(/http:\/\/\S+/)
    expect(previewContent).not.toMatch(/sec_[a-f0-9-]+/i)
    expect(previewContent).not.toMatch(/ups_[a-f0-9-]+/i)

    await previewSurface.getByRole('button', { name: 'Close', exact: true }).click()
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
      await applyBtn.click()
      await page.click('[data-cy="upstream-apply-confirm"]')
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
    await expect(page.locator('[data-cy="publish-review-surface"]')).toBeVisible({ timeout: 10_000 })

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

  await test.step('set image default in the UI → preview → publish second generation', async () => {
    await page.goto('/admin/resources')
    await expect(page.locator('[data-cy="resources-page"]')).toBeVisible({ timeout: 10_000 })
    await page.click('[data-cy="config-section-policy"]')
    await selectOption(page, 'policy-default-image-generation', 'E2E Image Generation')

    const saveBtn = page.locator('[data-cy="draft-save-btn"]')
    await expect(saveBtn).toBeEnabled({ timeout: 5_000 })
    await saveBtn.click()
    await expect(page.locator('.q-badge').filter({ hasText: /dirty/i })).not.toBeVisible({ timeout: 10_000 })

    const responsePromise = page.waitForResponse(r => r.url().endsWith('/api/admin/v1/draft:preview') && r.request().method() === 'POST')
    await page.click('[data-cy="draft-preview-btn"]')
    const previewResponse = await responsePromise
    expect(previewResponse.status()).toBe(200)
    const projection = await previewResponse.json()
    expect(projection.policy.defaultImageGenerationId).toBe(projection.imageGenerators[0].imageId)
    for (const field of [
      'defaultModelId',
      'defaultFastModelId',
      'defaultTitleModelId',
      'defaultAttachmentInspectionModelId',
      'defaultSuggestionModelId',
      'defaultCompressModelId',
    ]) {
      expect(projection.policy[field]).toBe(projection.models[0].modelId)
    }
    expect(projection.policy.defaultTtsId).toBe(projection.tts[0].ttsId)
    expect(projection.policy.defaultAsrId).toBe(projection.asr[0].asrId)
    expect(projection.policy.defaultAssistantId).toBe(projection.assistants[0].assistantDefinitionId)
    await page.locator('[data-cy="snapshot-preview-surface"]').getByRole('button', { name: 'Close', exact: true }).click()

    await page.click('[data-cy="draft-review-btn"]')
    await expect(page.locator('[data-cy="publish-review-surface"]')).toBeVisible({ timeout: 10_000 })
    page.once('dialog', dialog => dialog.accept())
    await page.click('[data-cy="draft-publish-btn"]')
    await expect(page.locator('text=/COMPLETED|completed/i')).toBeVisible({ timeout: 60_000 })
    await page.keyboard.press('Escape')
  })

  // ========================================================================
  // Phase 8: Reload — verify state persists
  // ========================================================================
  await test.step('reload page → state persists after publish, Policy flags verified', async () => {
    await page.reload()
    await expect(page).toHaveURL(/\/admin\/resources/)
    await expect(page.locator('[data-cy="resources-page"]')).toBeVisible()

    await expect(page.locator('[data-cy="config-section-models"]')).toBeVisible({ timeout: 10_000 })

    await page.click('[data-cy="config-section-models"]')
    await page.waitForTimeout(500)

    const modelItems = page.locator('.q-card:has-text("Models") .q-item')
    await expect(modelItems.first()).toBeVisible({ timeout: 5_000 })

    // Verify Policy flags are still ON
    await page.click('[data-cy="config-section-policy"]')
    await page.waitForTimeout(500)
    const policyFlagsAfter = [
      'Allow user Providers/models',
      'Allow user TTS',
      'Allow user ASR',
      'Allow user MCP',
      'Allow user assistants',
    ]
    for (const flagLabel of policyFlagsAfter) {
      const toggle = page.getByRole('switch', { name: flagLabel })
      await expect(toggle).toBeVisible({ timeout: 5_000 })
      await expect(toggle).toHaveAttribute('aria-checked', 'true')
    }
    for (const selector of [
      'policy-default-model',
      'policy-default-fast-model',
      'policy-default-title-model',
      'policy-default-attachment-inspection-model',
      'policy-default-suggestion-model',
      'policy-default-compress-model',
    ]) {
      await expect(page.locator(`[data-cy="${selector}"]`)).toContainText('E2E Test Model')
    }
    await expect(page.locator('[data-cy="policy-default-tts"]')).toContainText('E2E Test TTS')
    await expect(page.locator('[data-cy="policy-default-asr"]')).toContainText('E2E Test ASR')
    await expect(page.locator('[data-cy="policy-default-assistant"]')).toContainText('E2E Assistant')
    await expect(page.locator('[data-cy="policy-default-image-generation"]')).toContainText('E2E Image Generation')
  })

  await test.step('clear auxiliary defaults → save → validate → preview keeps them unset', async () => {
    const auxiliarySelectors = [
      'policy-default-fast-model',
      'policy-default-title-model',
      'policy-default-attachment-inspection-model',
      'policy-default-suggestion-model',
      'policy-default-compress-model',
    ]
    for (const selector of auxiliarySelectors) {
      const select = page.locator(`[data-cy="${selector}"]`)
      const field = select.locator('xpath=ancestor-or-self::*[contains(concat(" ", normalize-space(@class), " "), " q-field ")][1]')
      await field.getByRole('button', { name: 'Clear' }).click()
      await expect(select).not.toContainText('E2E Test Model')
    }
    await page.locator('[data-cy="draft-save-btn"]').click()

    const validationResponsePromise = page.waitForResponse(r => r.url().endsWith('/api/admin/v1/draft:validate') && r.request().method() === 'POST')
    await page.locator('[data-cy="draft-validate-btn"]').click()
    const validation = await (await validationResponsePromise).json()
    expect(validation.valid).toBe(true)

    const previewResponsePromise = page.waitForResponse(r => r.url().endsWith('/api/admin/v1/draft:preview') && r.request().method() === 'POST')
    await page.locator('[data-cy="draft-preview-btn"]').click()
    const projection = await (await previewResponsePromise).json()
    for (const field of [
      'defaultFastModelId',
      'defaultTitleModelId',
      'defaultAttachmentInspectionModelId',
      'defaultSuggestionModelId',
      'defaultCompressModelId',
    ]) {
      expect(projection.policy[field]).toBeUndefined()
    }
    expect(projection.policy.defaultModelId).toBe(projection.models[0].modelId)
    await page.locator('[data-cy="snapshot-preview-surface"]').getByRole('button', { name: 'Close', exact: true }).click()
  })
})
