<script setup lang="ts">
import { useRouter } from 'vue-router'
import { useSessionStore } from '../stores/session'

const router = useRouter()
const session = useSessionStore()
const items = [
  { label: 'Overview', path: '/', icon: 'dashboard' },
  { label: 'Users', path: '/users', icon: 'group' },
  { label: 'Resources', path: '/resources', icon: 'hub' },
  { label: 'Upstreams', path: '/upstreams', icon: 'cloud' },
  { label: 'Releases', path: '/releases', icon: 'rocket_launch' },
  { label: 'Usage', path: '/usage', icon: 'query_stats' },
  { label: 'System', path: '/system', icon: 'monitor_heart' },
]

async function logout() {
  await session.logout()
  await router.replace('/login')
}
</script>

<template>
  <q-layout view="hHh Lpr fFf">
    <q-header bordered class="bg-white text-dark">
      <q-toolbar>
        <q-toolbar-title class="text-weight-bold">MEASIX Admin</q-toolbar-title>
        <div v-if="session.user" class="text-caption q-mr-md">{{ session.user.displayName }}</div>
        <q-btn v-if="session.authenticated" flat dense icon="logout" label="Sign out" @click="logout" />
      </q-toolbar>
    </q-header>
    <q-drawer show-if-above bordered :width="220">
      <q-list padding>
        <q-item-label header>Runtime Foundation</q-item-label>
        <q-item v-for="item in items" :key="item.path" clickable :to="item.path" exact active-class="bg-grey-2 text-primary">
          <q-item-section avatar><q-icon :name="item.icon" /></q-item-section>
          <q-item-section>{{ item.label }}</q-item-section>
        </q-item>
      </q-list>
    </q-drawer>
    <q-page-container class="bg-grey-1"><router-view /></q-page-container>
  </q-layout>
</template>
