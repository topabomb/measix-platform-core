import { createI18n } from 'vue-i18n'
import { locales, detectLocale, type LocaleCode } from './locales'

const initialLocale = detectLocale()

/**
 * MEASIX Admin Console i18n instance.
 *
 * - Uses Composition API mode (legacy: false) for type safety.
 * - Locale is detected from localStorage > navigator.language > 'en'.
 * - New locale files live under src/i18n/locales/*.ts.
 * - To add a new language:
 *   1. Create src/i18n/locales/<code>.ts with the same key structure.
 *   2. Export it from src/i18n/locales/index.ts (locales map + SUPPORTED_LOCALES).
 *   3. Add a toggle option in the language switcher (AdminLayout.vue).
 */
export const i18n = createI18n({
  legacy: false,
  locale: initialLocale,
  fallbackLocale: 'en',
  messages: locales,
})

/**
 * Programmatically switch the active locale.
 * Persists the choice and updates <html lang>.
 */
export function switchLocale(locale: LocaleCode): void {
  i18n.global.locale.value = locale
  localStorage.setItem('measix.locale', locale)
  document.documentElement.setAttribute('lang', locale)
}

/**
 * The currently active locale code.
 */
export function currentLocale(): LocaleCode {
  return i18n.global.locale.value as LocaleCode
}

// Apply the initial <html lang> on boot.
document.documentElement.setAttribute('lang', initialLocale)

export type { LocaleCode } from './locales'
export { SUPPORTED_LOCALES } from './locales'
