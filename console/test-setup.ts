/**
 * Vitest global setup: ensures every component test has vue-i18n injected,
 * so `$t()` and `useI18n()` work without each test file re-registering the plugin.
 *
 * This mirrors the Quasar boot file (src/boot/i18n.ts) used at runtime.
 */
import { config } from '@vue/test-utils'
import { i18n } from './src/i18n'

config.global.plugins = [i18n]
