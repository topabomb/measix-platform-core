import { test, expect } from '@playwright/test'

// Uses the harness production SPA and real Hub; no intercepted Admin responses.
test('All Admin routes remain usable at desktop and 320px; invalid announcements cannot submit', async ({ page }) => {
  const pageErrors: string[] = []
  page.on('pageerror', error => pageErrors.push(error.message))
  await page.goto('/admin/')
  await page.locator('[data-cy="login-username"]').fill('admin')
  await page.locator('[data-cy="login-password"]').fill(process.env.MEASIX_E2E_ADMIN_PASSWORD!)
  await page.locator('[data-cy="login-submit"]').click()
  await expect(page).toHaveURL(/\/admin\/(overview)?$/)
  const routes = ['/', '/users', '/budget-templates', '/resources', '/upstreams', '/remote-workspaces', '/releases', '/enterprise-updates', '/settings', '/usage', '/system']
  for (const width of [1280, 320]) {
    await page.setViewportSize({ width, height: 720 })
    for (const route of routes) {
      await test.step(`${width}px ${route}`, async () => {
        await page.goto(`/admin${route}`)
        await expect(page.locator('.admin-page, .q-page')).toBeVisible()
        await expect(page.locator('.admin-page, .q-page')).not.toHaveText('')
        expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
        await expect(page.locator('[data-cy="problem-banner"]')).toHaveCount(0)
      })
    }
  }
  await page.goto('/admin/settings')
  await expect(page.locator('[data-cy="settings-page"]')).toContainText('Unavailable')
  await expect(page.locator('[data-cy="settings-page"]')).not.toContainText('UNAVAILABLE')
  await page.goto('/admin/enterprise-updates')
  await page.getByRole('button', { name: 'Actions', exact: true }).click()
  await page.getByRole('button', { name: 'Create', exact: true }).click()
  const editor = page.locator('[data-cy="enterprise-update-editor"]')
  await expect(editor.getByRole('button', { name: 'Create', exact: true })).toBeDisabled()
  await editor.getByLabel('Title', { exact: true }).fill('   ')
  await editor.getByLabel('Content', { exact: true }).fill('   ')
  await expect(editor.getByRole('button', { name: 'Create', exact: true })).toBeDisabled()
  await editor.getByLabel('Title', { exact: true }).fill('Review-only draft')
  await editor.getByLabel('Content', { exact: true }).fill('Input validation is complete.')
  await expect(editor.getByRole('button', { name: 'Create', exact: true })).toBeEnabled()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
  await editor.getByRole('button', { name: 'Cancel', exact: true }).click()
  await expect(editor).toHaveCount(0)
  expect(pageErrors).toEqual([])
  await page.screenshot({ path: '../.artifacts/admin-review-mobile.png', fullPage: true })
})
