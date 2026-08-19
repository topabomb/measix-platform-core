import type { RouteRecordRaw } from 'vue-router'

const routes: RouteRecordRaw[] = [
  { path: '/login', component: () => import('@/pages/LoginPage.vue') },
  {
    path: '/',
    component: () => import('@/layouts/AdminLayout.vue'),
    children: [
      { path: '', name: 'Overview', component: () => import('@/pages/OverviewPage.vue') },
      { path: 'users', name: 'Users', component: () => import('@/pages/UsersPage.vue') },
      { path: 'resources', name: 'Resources', component: () => import('@/pages/ResourcesPage.vue') },
      { path: 'upstreams', name: 'Upstreams', component: () => import('@/pages/UpstreamsPage.vue') },
      { path: 'releases', name: 'Releases', component: () => import('@/pages/ReleasesPage.vue') },
      { path: 'usage', name: 'Usage', component: () => import('@/pages/UsagePage.vue') },
      { path: 'system', name: 'System', component: () => import('@/pages/SystemPage.vue') }
    ]
  }
]

export default routes
