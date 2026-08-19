import { defineRouter } from '#q-app'
import { createRouter, createWebHistory } from 'vue-router'
import routes from './routes'
export default defineRouter(() => createRouter({ history: createWebHistory('/admin/'), routes }))
