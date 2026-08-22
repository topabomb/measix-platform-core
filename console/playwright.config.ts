import { defineConfig, devices } from '@playwright/test'

/**
 * Playwright E2E configuration for the S0.1 Admin Console browser gate.
 *
 * Browser E2E is NOT part of default GitHub Actions CI/CD. It must be
 * executed explicitly on the exact candidate SHA via `make console-e2e`.
 *
 * The tests use production `dist/spa` + real Control Hub + real Runtime Relay.
 * Mocking `page.route('/api/**')` is forbidden as T4.1 Green evidence.
 *
 * Architecture authority: measix-s0-capability-delivery-system-testing-spec.md
 * §13 CAP-C6-001 — Clean environment golden path (browser).
 */
export default defineConfig({
  testDir: './e2e',
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: 0,
  workers: 1,
  reporter: [
    ['list'],
    ['html', { outputFolder: 'playwright-report' }],
    ['json', { outputFile: '../.artifacts/e2e-playwright.json' }],
  ],
  timeout: 120_000,
  expect: {
    timeout: 10_000,
  },
  use: {
    baseURL: process.env.MEASIX_E2E_BASE_URL || 'http://127.0.0.1:8080',
    trace: 'on-first-retry',
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
  ],
})
