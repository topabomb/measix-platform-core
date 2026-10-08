import { defineBoot } from '@quasar/app-vite/wrappers'
import { i18n } from '../i18n'

/**
 * Quasar boot file: registers vue-i18n on the Vue app.
 *
 * This runs before the app mounts so all components can use `$t()`
 * and the `useI18n()` composable from the very first render.
 */
export default defineBoot(({ app }) => {
  app.use(i18n)
})
