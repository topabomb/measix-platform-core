<script setup lang="ts">
import { ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useSessionStore } from '../stores/session'
import { visibleNavItems } from '../router/navigation'
import HealthIndicator from '../components/HealthIndicator.vue'

const route = useRoute()
const router = useRouter()
const session = useSessionStore()
const drawerOpen = ref(false)
const navItems = visibleNavItems()

// On mobile the drawer is an overlay; close it after navigating so the user
// returns to the chosen page (implementation §5 Mobile, §12).
watch(
  () => route.fullPath,
  () => {
    drawerOpen.value = false
  },
)

async function logout() {
  await session.logout()
  await router.replace('/login')
}
</script>

<template>
  <q-layout view="hHh Lpr fFf" class="admin-shell">
    <q-header bordered class="bg-white text-dark">
      <q-toolbar>
        <q-btn
          flat round dense icon="menu"
          class="lt-md"
          aria-label="Menu"
          @click="drawerOpen = !drawerOpen"
        />
        <q-toolbar-title class="text-weight-bold" style="min-width: 0">MEASIX Admin</q-toolbar-title>

        <!-- Global high-priority runtime indicator (product §4.1). -->
        <HealthIndicator v-if="session.authenticated" class="q-mr-sm" />

        <!-- Current admin identity + sign out. -->
        <q-btn
          v-if="session.authenticated"
          flat
          no-caps
          class="gt-sm"
          color="primary"
          :label="session.user?.displayName"
        >
          <q-menu>
            <q-list>
              <q-item clickable v-close-popup @click="logout">
                <q-item-section avatar><q-icon name="logout" /></q-item-section>
                <q-item-section>Sign out</q-item-section>
              </q-item>
            </q-list>
          </q-menu>
        </q-btn>
        <q-btn
          v-if="session.authenticated"
          flat dense round icon="logout"
          class="lt-md"
          aria-label="Sign out"
          @click="logout"
        />
      </q-toolbar>
    </q-header>

    <!-- Responsive primary navigation (implementation §5):
         Wide = persistent rail, Compact = collapsible mini, Mobile = overlay drawer. -->
    <q-drawer
      v-model="drawerOpen"
      :show-if-above="$q.screen.gt.sm"
      bordered
      :width="$q.screen.gt.md ? 240 : 240"
      :mini="$q.screen.md && !$q.screen.gt.md && !drawerOpen"
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
          @click="drawerOpen = false"
        >
          <q-item-section avatar><q-icon :name="item.icon" /></q-item-section>
          <q-item-section>{{ item.label }}</q-item-section>
        </q-item>
      </q-list>
    </q-drawer>

    <q-page-container class="bg-grey-1">
      <!-- Center the content column on very wide screens for readable line lengths
           while staying fluid down to mobile (implementation §5 Desktop/Wide). -->
      <div class="admin-content">
        <router-view />
      </div>
    </q-page-container>
  </q-layout>
</template>

<style scoped>
.admin-content {
  margin: 0 auto;
  max-width: 1280px;
}
</style>
