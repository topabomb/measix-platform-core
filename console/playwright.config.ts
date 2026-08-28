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
  // Exclude deprecated golden-path.spec.ts — it has been split into
  // golden-path-authoring.spec.ts and golden-path-usage.spec.ts
  // which are orchestrated by candidate-orchestrator.mjs / e2e-harness.mjs
  testMatch: /.*\.spec\.ts/,
  testIgnore: /golden-path\.spec\.ts/,
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: 0,
  workers: 1,
  reporter: [
    ['list'],
    // Per audit P1-1: use open: 'never' to prevent the HTML reporter from
    // starting a local server that blocks non-interactive CI/harness runs.
    ['html', { outputFolder: 'playwright-report', open: 'never' }],
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
    // Use 'domcontentloaded' instead of default 'load' to avoid ERR_ABORTED
    // on Windows when the Node.js SPA proxy closes keep-alive connections
    // during asset loading.
    navigationTimeout: 30_000,
    launchOptions: {
      args: [
        '--no-sandbox',
        '--disable-setuid-sandbox',
        '--disable-features=IsolateOrigins,site-per-process',
        '--ignore-certificate-errors',
      ],
    },
  },
  projects: [
    {
      name: 'chromium',
      use: {
        ...devices['Desktop Chrome'],
      },
    },
  ],
})
