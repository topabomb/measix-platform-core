import { test, expect, type Page } from '@playwright/test'

// HUB-ID-020 / Admin Users and Upstreams requirements: production SPA + real
// Hub/SQLite/Relay from the shared harness, without intercepted API responses.
test.use({ trace: 'off', video: 'off' })
const targetPassword = 'synthetic browser target password'
const replacementPassword = 'synthetic browser replacement password'

async function login(page: Page, username = 'admin', password = process.env.MEASIX_E2E_ADMIN_PASSWORD!) {
  await page.goto(`${process.env.MEASIX_E2E_BASE_URL}/admin/`)
  await page.locator('[data-cy="login-username"]').fill(username)
  await page.locator('[data-cy="login-password"]').fill(password)
  await page.locator('[data-cy="login-submit"]').click()
  await expect(page).toHaveURL(/\/admin\/(overview)?$/)
}

async function fillAccountDialog(page: Page, password?: string) {
  const dialog = page.locator('[data-cy="admin-account-dialog"]')
  await expect(dialog).toBeVisible()
  await expect(dialog.locator('[data-cy="account-submit"]')).not.toContainText('common.confirm')
  await dialog.locator('[data-cy="account-current-password"]').fill(process.env.MEASIX_E2E_ADMIN_PASSWORD!)
  if (password) {
    await dialog.locator('[data-cy="account-new-password"]').fill(password)
    await dialog.locator('[data-cy="account-confirm-password"]').fill(password)
  }
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  await dialog.locator('[data-cy="account-submit"]').click()
  await expect(dialog).toHaveCount(0)
}

async function openAccountAction(page: Page, action: 'role' | 'password') {
  await page.locator('[data-cy="user-detail"]').getByRole('button', { name: 'Expand "Actions"', exact: true }).click()
  await page.locator(`[data-cy="set-user-${action}"]`).click()
}

test('Admin creates/promotes/resets/regrants accounts and deletes an unused connection at desktop and mobile widths', async ({ page, browser }) => {
  const errors: string[] = []
  page.on('pageerror', error => errors.push(error.message))
  await login(page)
  await page.setViewportSize({ width: 1280, height: 800 })
  await page.goto('/admin/users')
  const username = `account_${Date.now()}`
  await page.locator('[data-cy="create-user-btn"]:visible').click()
  await page.locator('[data-cy="user-form-username"]').fill(username)
  await page.locator('[data-cy="user-form-display-name"]').fill('Account browser target')
  await page.locator('[data-cy="user-form-submit"]').click()
  const row = page.locator('[data-cy="users-page"] .q-list > .q-item').filter({ hasText: username })
  await expect(row).toBeVisible()
  await row.click()
  await openAccountAction(page, 'role')
  await expect(page.locator('[data-cy="account-submit"]')).toBeDisabled()
  await fillAccountDialog(page, targetPassword)
  await expect(page.locator('[data-cy="user-detail"]')).toContainText(`${username} · Admin`)

  const targetContext = await browser.newContext()
  const targetPage = await targetContext.newPage()
  try {
    await login(targetPage, username, targetPassword)
    expect((await targetContext.request.get(`${process.env.MEASIX_E2E_BASE_URL}/api/admin/v1/session`)).status()).toBe(200)
    await page.setViewportSize({ width: 390, height: 844 })
    await openAccountAction(page, 'password')
    await fillAccountDialog(page, replacementPassword)
    expect((await targetContext.request.get(`${process.env.MEASIX_E2E_BASE_URL}/api/admin/v1/session`)).status()).toBe(401)
    await targetContext.clearCookies()
    await login(targetPage, username, replacementPassword)
    await openAccountAction(page, 'role')
    await expect(page.locator('[data-cy="account-new-password"]')).toHaveCount(0)
    await fillAccountDialog(page)
    await expect(page.locator('[data-cy="user-detail"]')).toContainText('Member')
    await openAccountAction(page, 'role')
    await expect(page.locator('[data-cy="account-new-password"]')).toHaveCount(0)
    await fillAccountDialog(page)
    await expect(page.locator('[data-cy="user-detail"]')).toContainText(`${username} · Admin`)
    expect((await targetContext.request.get(`${process.env.MEASIX_E2E_BASE_URL}/api/admin/v1/session`)).status()).toBe(401)
    await targetContext.clearCookies()
    await login(targetPage, username, replacementPassword)
  } finally { await targetContext.close() }

  // A newly created ADMIN must be immediately able to authenticate.
  await page.setViewportSize({ width: 1280, height: 800 })
  await page.locator('[data-cy="create-user-btn"]:visible').click()
  await page.locator('[data-cy="user-form-username"]').fill(`${username}_admin`)
  await page.locator('[data-cy="user-form-display-name"]').fill('Created browser administrator')
  await page.getByLabel('Role', { exact: true }).click()
  await page.getByRole('option', { name: 'Admin', exact: true }).click()
  await expect(page.locator('[data-cy="user-form-submit"]')).toBeDisabled()
  await page.locator('[data-cy="create-admin-current-password"]').fill(process.env.MEASIX_E2E_ADMIN_PASSWORD!)
  await page.locator('[data-cy="create-admin-new-password"]').fill(targetPassword)
  await page.locator('[data-cy="create-admin-confirm-password"]').fill(targetPassword)
  await page.locator('[data-cy="user-form-submit"]').click()
  await expect(page.locator('[data-cy="user-form-submit"]')).toHaveCount(0)
  const createdContext = await browser.newContext()
  try { await login(await createdContext.newPage(), `${username}_admin`, targetPassword) } finally { await createdContext.close() }

  await page.goto('/admin/upstreams')
  await page.locator('[data-cy="create-upstream-btn"]:visible').click()
  await page.locator('[data-cy="upstream-form-name"]').fill(`Unused ${username}`)
  await page.locator('[data-cy="upstream-form-base-url"]').fill(process.env.MEASIX_E2E_ADAPTER_URL!)
  await page.locator('[data-cy="upstream-form-submit"]').click()
  const connection = page.locator('[data-cy="upstreams-page"] .q-list > .q-item').filter({ hasText: `Unused ${username}` })
  await expect(connection).toBeVisible()
  await connection.click()
  await page.setViewportSize({ width: 390, height: 844 })
  await page.locator('[data-cy="delete-upstream-btn"]').click()
  await expect(page.locator('[data-cy="delete-upstream-dialog"]')).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  await page.locator('[data-cy="confirm-delete-upstream"]').click()
  await expect(page.locator('[data-cy="delete-upstream-dialog"]')).toHaveCount(0)
  await expect(connection).toHaveCount(0)
  await page.reload()
  await expect(connection).toHaveCount(0)
  // A retained published connection stays protected and offers refresh.
  await page.locator('[data-cy="upstreams-page"] .q-list > .q-item').first().click()
  await page.locator('[data-cy="delete-upstream-btn"]').click()
  await page.locator('[data-cy="confirm-delete-upstream"]').click()
  await expect(page.locator('[data-cy="delete-upstream-dialog"]')).toContainText('referenced')
  await expect(page.locator('[data-cy="reload-delete-upstream"]')).toBeEnabled()
  await page.locator('[data-cy="delete-upstream-dialog"]').getByRole('button', { name: 'Cancel', exact: true }).click()
  expect(errors).toEqual([])
})

test('Role conflict reloads the real account and retains an actionable notice', async ({ page }, testInfo) => {
  await login(page)
  const sessionResponse = await page.request.get('/api/admin/v1/session')
  const session = await sessionResponse.json() as { csrfToken: string }
  const headers = { 'X-CSRF-Token': session.csrfToken }
  const username = `conflict_${Date.now()}`
  const created = await page.request.post('/api/admin/v1/users', { headers, data: { username, displayName: 'Role conflict review', role: 'MEMBER' } })
  expect(created.status()).toBe(201)
  const user = await created.json() as { userId: string }
  await page.goto('/admin/users')
  await page.locator('[data-cy="user-row"]').filter({ hasText: username }).click()
  await openAccountAction(page, 'role')
  await expect(page.locator('[data-cy="account-submit"]')).toContainText('Make administrator')
  // A second real admin request changes the role after this dialog captured it.
  const concurrent = await page.request.post(`/api/admin/v1/users/${user.userId}:set-role`, {
    headers, data: { role: 'ADMIN', expectedRole: 'MEMBER', currentPassword: process.env.MEASIX_E2E_ADMIN_PASSWORD!, newPassword: targetPassword, confirmPassword: targetPassword },
  })
  expect(concurrent.status()).toBe(200)
  const dialog = page.locator('[data-cy="admin-account-dialog"]')
  await dialog.locator('[data-cy="account-current-password"]').fill(process.env.MEASIX_E2E_ADMIN_PASSWORD!)
  await dialog.locator('[data-cy="account-new-password"]').fill(targetPassword)
  await dialog.locator('[data-cy="account-confirm-password"]').fill(targetPassword)
  await dialog.locator('[data-cy="account-submit"]').click()
  await expect(dialog).toHaveCount(0)
  await expect(page.locator('[data-cy="users-page"]')).toContainText('Someone changed this account’s role.')
  await expect(page.locator('[data-cy="user-detail"]')).toContainText(`${username} · Admin`)
  // The next action was rendered as removal, but another request already
  // removed the role. Opening must refresh and stop, never turn into a grant.
  const removed = await page.request.post(`/api/admin/v1/users/${user.userId}:set-role`, {
    headers, data: { role: 'MEMBER', expectedRole: 'ADMIN', currentPassword: process.env.MEASIX_E2E_ADMIN_PASSWORD! },
  })
  expect(removed.status()).toBe(200)
  await openAccountAction(page, 'role')
  await expect(dialog).toHaveCount(0)
  await expect(page.locator('[data-cy="user-detail"]')).toContainText(`${username} · Member`)
  await expect(page.locator('[data-cy="users-page"]')).toContainText('Someone changed this account’s role.')
  await page.screenshot({ path: testInfo.outputPath('role-conflict-reviewed.png') })
})
