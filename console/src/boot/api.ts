import { defineBoot } from '#q-app'
import { createPinia } from 'pinia'
import { setUnauthorizedHandler } from '../api/client'
import { useSessionStore } from '../stores/session'

export default defineBoot(({ app, router }) => {
  const pinia = createPinia()
  app.use(pinia)
  const session = useSessionStore(pinia)

  setUnauthorizedHandler(async () => {
    session.clear()
    if (router.currentRoute.value.path !== '/login') await router.replace('/login')
  })

  router.beforeEach(async (to) => {
    if (to.path === '/login') {
      if (session.authenticated) return { path: '/' }
      return true
    }
    if (session.authenticated) return true
    try {
      await session.restore()
      return true
    } catch {
      return { path: '/login', query: { redirect: to.fullPath } }
    }
  })
})
