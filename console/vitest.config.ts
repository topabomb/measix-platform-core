import { fileURLToPath } from 'node:url'
import { defineConfig } from 'vitest/config'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: {
      // Node resolution picks quasar's SSR bundle (no render functions).
      // Component tests need the client build.
      quasar: fileURLToPath(new URL('./node_modules/quasar/dist/quasar.client.js', import.meta.url)),
    },
  },
  test: {
    environment: 'jsdom',
    include: ['src/**/*.test.ts'],
  },
})
