import { defineRouter } from '@quasar/app-vite/wrappers'
import { createRouter, createWebHistory } from 'vue-router'
import routes from './routes'
export default defineRouter(() => createRouter({ history: createWebHistory('/admin/'), routes }))
