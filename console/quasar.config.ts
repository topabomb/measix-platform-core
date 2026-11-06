import { defineConfig } from '@quasar/app-vite/wrappers'
export default defineConfig(() => ({
  boot: ['i18n', 'api'],
  css: ['app.css'],
  extras: [
    'material-icons',
  ],
  build: { vueRouterMode: 'history', publicPath: '/admin/' },
  devServer: {
    port: 9000,
    open: false,
    proxy: {
      '/api': { target: 'http://localhost:8080', changeOrigin: true },
      '/live': { target: 'http://localhost:8080', changeOrigin: true },
      '/ready': { target: 'http://localhost:8080', changeOrigin: true },
      '/.well-known': { target: 'http://localhost:8080', changeOrigin: true },
      '/runtime': { target: 'http://localhost:8090', changeOrigin: true },
    },
  },
  framework: { plugins: ['Notify', 'Dialog'] }
}))
