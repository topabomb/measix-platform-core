<script setup lang="ts">
import { ref, watch } from 'vue'
import { useQuasar } from 'quasar'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useSessionStore } from '../stores/session'
import { visibleNavItems } from '../router/navigation'
import HealthIndicator from '../components/HealthIndicator.vue'
import { switchLocale, currentLocale, SUPPORTED_LOCALES, type LocaleCode } from '../i18n'

// Layout strategy — leveraging Quasar's built-in responsive drawer logic:
//
//   • show-if-above="true"  →  Quasar auto-opens the drawer on wide screens
//     (above the breakpoint) and auto-closes it when shrinking below.
//   • breakpoint="1023"     →  the px threshold; screens > 1023px get a
//     persistent rail, screens ≤ 1023px get an overlay drawer.
//   • v-model="drawerOpen"  →  only tracks user toggles in overlay mode.
//     On wide screens Quasar manages visibility internally.
//
// CRITICAL: do NOT set drawerOpen=false on route change when the screen
// is above the breakpoint — that would close the persistent drawer.
// Only close on narrow screens (overlay mode).

const $t = useI18n().t
const $q = useQuasar()
const route = useRoute()
const router = useRouter()
const session = useSessionStore()
const drawerOpen = ref(!$q.screen.lt.md)
const drawerMini = ref(false)
const navItems = visibleNavItems()
const deliveryNavItems = navItems.filter(item => item.group === 'configuration')
const diagnosticNavItems = navItems.filter(item => item.group === 'operations')

// Close the overlay drawer after navigating — but ONLY on narrow screens.
// On wide screens the drawer is persistent (managed by Quasar show-if-above).
watch(
  () => route?.fullPath,
  () => {
    // Quasar's md breakpoint starts at 1024px, matching the drawer's 1023px
    // breakpoint. lg starts at 1440px and would incorrectly hide the drawer on
    // common 1280/1366px desktop displays.
    if ($q.screen.lt.md) {
      drawerOpen.value = false
    }
  },
)

watch(
  () => $q.screen.lt.md,
  narrow => {
    if (narrow) drawerOpen.value = false
    else drawerOpen.value = true
  },
)

function toggleNavigation() {
  if ($q.screen.lt.md) drawerOpen.value = !drawerOpen.value
  else drawerMini.value = !drawerMini.value
}

async function logout() {
  await session.logout()
  await router.replace('/login')
}

/** Map nav item id to i18n key.
 *  Multi-word ids use camelCase in the i18n registry (e.g. enterpriseUpdates),
 *  so we cannot just lower-case the id — we need an explicit mapping. */
const NAV_I18N_KEYS: Record<string, string> = {
  Overview: 'nav.overview',
  Users: 'nav.users',
  BudgetTemplates: 'nav.budgetTemplates',
  Resources: 'nav.resources',
  Upstreams: 'nav.upstreams',
  Releases: 'nav.releases',
  EnterpriseUpdates: 'nav.enterpriseUpdates',
  Settings: 'nav.settings',
  Usage: 'nav.usage',
  System: 'nav.system',
}

function navLabel(id: string): string {
  const key = NAV_I18N_KEYS[id] ?? `nav.${id.toLowerCase()}`
  const translated = $t(key)
  return typeof translated === 'string' ? translated : id
}

/** Language switcher: cycles through supported locales. */
function onSwitchLocale() {
  const current = currentLocale()
  const idx = SUPPORTED_LOCALES.indexOf(current)
  const next = SUPPORTED_LOCALES[(idx + 1) % SUPPORTED_LOCALES.length] as LocaleCode
  switchLocale(next)
}

const LOCALE_LABELS: Record<LocaleCode, string> = {
  en: 'English',
  zh: '中文',
}
</script>

<template>
  <q-layout view="hHh Lpr fFf" class="admin-shell">
    <q-header bordered class="bg-white text-dark">
      <q-toolbar>
        <q-btn
          flat round dense
          :icon="$q.screen.lt.md ? 'menu' : (drawerMini ? 'menu_open' : 'menu')"
          :aria-label="$q.screen.lt.md ? $t('nav.menu') : $t(drawerMini ? 'nav.expand' : 'nav.collapse')"
          @click="toggleNavigation"
        />
        <q-toolbar-title class="text-weight-bold" style="min-width: 0">MEASIX Admin</q-toolbar-title>

        <!-- Language switcher -->
        <q-btn
          flat dense
          no-caps
          class="q-mr-xs"
          :aria-label="$t('common.switchLanguage')"
          @click="onSwitchLocale"
        >
          <q-icon name="language" class="q-mr-xs" />
          <span class="gt-sm">{{ LOCALE_LABELS[currentLocale()] }}</span>
        </q-btn>

        <!-- Global high-priority runtime indicator (product §4.1). -->
        <HealthIndicator v-if="session.authenticated" class="q-mr-xs" />

        <!-- Current admin identity + sign out. -->
        <q-btn
          v-if="session.authenticated"
          flat
          no-caps
          class="gt-sm"
          data-cy="user-menu-btn"
          color="primary"
          :label="session.user?.displayName"
        >
          <q-menu>
            <q-list>
              <q-item clickable v-close-popup data-cy="logout-btn" @click="logout">
                <q-item-section avatar><q-icon name="logout" /></q-item-section>
                <q-item-section>{{ $t('login.signOut') }}</q-item-section>
              </q-item>
            </q-list>
          </q-menu>
        </q-btn>
        <q-btn
          v-if="session.authenticated"
          flat dense round icon="logout"
          class="lt-md"
          data-cy="logout-btn-mobile"
          :aria-label="$t('login.signOut')"
          @click="logout"
        />
      </q-toolbar>
    </q-header>

    <!-- Responsive navigation drawer:
         • show-if-above: Quasar auto-opens on wide screens (> breakpoint)
         • breakpoint: 1023px — above = persistent rail, below = overlay
         • Quasar manages the belowBreakpoint ↔ aboveBreakpoint transition
           internally, including auto-show when resizing narrow → wide. -->
    <q-drawer
      v-model="drawerOpen"
      show-if-above
      :breakpoint="1023"
      bordered
      :width="196"
      :mini-width="56"
      :mini="!$q.screen.lt.md && drawerMini"
    >
      <q-list class="admin-nav q-py-xs">
        <q-item-label v-if="!drawerMini || $q.screen.lt.md" header>{{ $t('nav.configDelivery') }}</q-item-label>
        <q-item
          v-for="item in deliveryNavItems"
          :key="item.id"
          clickable
          :to="item.path"
          exact
          active-class="bg-grey-2 text-primary"
          dense
        >
          <q-item-section avatar><q-icon :name="item.icon" /></q-item-section>
          <q-item-section>{{ navLabel(item.id) }}</q-item-section>
          <q-tooltip v-if="drawerMini && !$q.screen.lt.md" anchor="center right" self="center left">{{ navLabel(item.id) }}</q-tooltip>
        </q-item>
        <q-separator class="q-my-xs" />
        <q-item-label v-if="!drawerMini || $q.screen.lt.md" header>{{ $t('nav.operationsDiagnostics') }}</q-item-label>
        <q-item
          v-for="item in diagnosticNavItems"
          :key="item.id"
          clickable
          :to="item.path"
          exact
          active-class="bg-grey-2 text-primary"
          dense
        >
          <q-item-section avatar><q-icon :name="item.icon" /></q-item-section>
          <q-item-section>{{ navLabel(item.id) }}</q-item-section>
          <q-tooltip v-if="drawerMini && !$q.screen.lt.md" anchor="center right" self="center left">{{ navLabel(item.id) }}</q-tooltip>
        </q-item>
      </q-list>
    </q-drawer>

    <!-- No local background: the page container uses the body background so the
         shell shows one colour instead of two near-identical greys. -->
    <q-page-container>
      <!-- Fluid, full width. The admin console is a data surface, not prose:
           a centered max-width column starved the card grids and left large
           dead margins on wide screens. Page margin is the 4px .admin-page. -->
      <router-view />
    </q-page-container>
  </q-layout>
</template>

<style scoped>
.admin-nav :deep(.q-item) {
  min-height: 38px;
  padding: 4px 12px;
}

.admin-nav :deep(.q-item__section--avatar) {
  min-width: 32px;
}

.admin-nav :deep(.q-item__label--header) {
  min-height: 30px;
  padding: 6px 12px;
}
</style>
