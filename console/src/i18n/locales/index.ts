import en from './en'
import zh from './zh'

export type MessageSchema = typeof en

export const locales = {
  en,
  zh,
} as const

export type LocaleCode = keyof typeof locales

export const SUPPORTED_LOCALES: LocaleCode[] = ['en', 'zh']

/**
 * Detect the best matching locale from:
 * 1. localStorage (user's previous choice)
 * 2. navigator.language / navigator.languages
 * 3. fallback to 'en'
 */
export function detectLocale(): LocaleCode {
  // 1. User's previous explicit choice
  const stored = localStorage.getItem('measix.locale')
  if (stored && (stored === 'en' || stored === 'zh')) {
    return stored
  }

  // 2. Browser language preference
  const nav =
    navigator.languages?.[0] ??
    navigator.language ??
    'en'

  // Match zh-CN, zh-TW, zh-HK, zh etc.
  if (nav.toLowerCase().startsWith('zh')) {
    return 'zh'
  }

  // 3. Default
  return 'en'
}

/**
 * Persist the user's locale choice so it survives refresh/restart.
 */
export function setLocale(locale: LocaleCode): void {
  localStorage.setItem('measix.locale', locale)
}

/**
 * Apply the locale to the <html lang> attribute for accessibility/SEO.
 */
export function applyHtmlLang(locale: LocaleCode): void {
  document.documentElement.setAttribute('lang', locale)
}
