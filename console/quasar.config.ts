import { defineConfig } from '#q-app'
export default defineConfig(() => ({
  boot: ['api'],
  css: ['app.css'],
  build: { vueRouterMode: 'history', publicPath: '/admin/' },
  devServer: { port: 9000, open: false },
  framework: { plugins: ['Notify', 'Dialog'] }
}))
