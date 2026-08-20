<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { useSessionStore } from '../stores/session'
import { visibleNavItems } from '../router/navigation'
import HealthIndicator from '../components/HealthIndicator.vue'

const router = useRouter()
const session = useSessionStore()
const drawerOpen = ref(false)
const navItems = visibleNavItems()

async function logout() {
  await session.logout()
  await router.replace('/login')
}
</script>

<template>
  <q-layout view="hHh Lpr fFf" class="admin-shell">
    <q-header bordered class="bg-white text-dark">
      <q-toolbar>
        <q-btn flat round dense icon="menu" class="lt-md" aria-label="Menu" @click="drawerOpen = !drawerOpen" />
        <q-toolbar-title class="text-weight-bold" style="min-width: 0">MEASIX Admin</q-toolbar-title>
        <!-- Global high-priority runtime indicator (product §4.1). -->
        <HealthIndicator v-if="session.authenticated" class="q-mr-sm" />
        <div v-if="session.user" class="text-caption q-mr-md gt-sm">{{ session.user.displayName }}</div>
        <q-btn v-if="session.authenticated" flat dense icon="logout" label="Sign out" class="gt-xs" @click="logout" />
        <q-btn v-if="session.authenticated" flat dense round icon="logout" class="lt-sm" aria-label="Sign out" @click="logout" />
      </q-toolbar>
    </q-header>

    <!-- Responsive primary navigation: Wide = persistent, Compact = collapsible,
         Mobile = overlay drawer (implementation §5). -->
    <q-drawer
      v-model="drawerOpen"
      :show-if-above="$q.screen.gt.sm"
      bordered
      :width="$q.screen.gt.md ? 240 : 220"
      :mini="$q.screen.md && !drawerOpen"
      :mini-to-overlay="true"
    >
      <q-list padding>
        <q-item-label header class="gt-sm">Runtime Foundation</q-item-label>
        <q-item
          v-for="item in navItems"
          :key="item.id"
          clickable
          :to="item.path"
          exact
          active-class="bg-grey-2 text-primary"
        >
          <q-item-section avatar><q-icon :name="item.icon" /></q-item-section>
          <q-item-section>{{ item.label }}</q-item-section>
        </q-item>
      </q-list>
    </q-drawer>

    <q-page-container class="bg-grey-1"><router-view /></q-page-container>
  </q-layout>
</template>
