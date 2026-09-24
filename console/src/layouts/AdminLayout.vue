<script setup lang="ts">
import { ref } from 'vue'
import { useQuasar } from 'quasar'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useSessionStore } from '../stores/session'
import { visibleNavItems } from '../router/navigation'
import HealthIndicator from '../components/HealthIndicator.vue'
import OrchelmBrand from '../components/OrchelmBrand.vue'
import ChangePasswordDialog from '../components/ChangePasswordDialog.vue'
import { switchLocale, currentLocale, SUPPORTED_LOCALES, type LocaleCode } from '../i18n'

const $t = useI18n().t
const $q = useQuasar()
const router = useRouter()
const session = useSessionStore()
const drawerOpen = ref(false)
const drawerMini = ref(false)
const changePasswordOpen = ref(false)
const navItems = visibleNavItems()
const deliveryNavItems = navItems.filter(item => item.group === 'configuration')
const diagnosticNavItems = navItems.filter(item => item.group === 'operations')

function toggleNavigation() {
  if ($q.screen.lt.md) drawerOpen.value = !drawerOpen.value
  else drawerMini.value = !drawerMini.value
}

async function logout() {
  await session.logout()
  await router.replace('/login')
}

async function passwordChanged() {
  session.clear()
  changePasswordOpen.value = false
  await router.replace({ path: '/login', query: { passwordChanged: '1' } })
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
    <q-header bordered class="orchelm-header text-dark">
      <q-toolbar class="orchelm-toolbar">
        <q-btn
          flat round dense
          color="primary"
          :icon="$q.screen.lt.md ? 'menu' : (drawerMini ? 'menu_open' : 'menu')"
          :aria-label="$q.screen.lt.md ? $t('nav.menu') : $t(drawerMini ? 'nav.expand' : 'nav.collapse')"
          @click="toggleNavigation"
        />
        <q-toolbar-title style="min-width: 0"><OrchelmBrand compact /></q-toolbar-title>

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
              <q-item clickable v-close-popup data-cy="change-password-btn" @click="changePasswordOpen = true">
                <q-item-section avatar><q-icon name="password" /></q-item-section>
                <q-item-section>{{ $t('account.changePassword') }}</q-item-section>
              </q-item>
              <q-item clickable v-close-popup data-cy="logout-btn" @click="logout">
                <q-item-section avatar><q-icon name="logout" /></q-item-section>
                <q-item-section>{{ $t('login.signOut') }}</q-item-section>
              </q-item>
            </q-list>
          </q-menu>
        </q-btn>
        <q-btn
          v-if="session.authenticated"
          flat dense round icon="account_circle"
          class="lt-md"
          data-cy="user-menu-btn-mobile"
          :aria-label="session.user?.displayName"
        >
          <q-menu>
            <q-list>
              <q-item clickable v-close-popup data-cy="change-password-btn-mobile" @click="changePasswordOpen = true">
                <q-item-section avatar><q-icon name="password" /></q-item-section>
                <q-item-section>{{ $t('account.changePassword') }}</q-item-section>
              </q-item>
              <q-item clickable v-close-popup data-cy="logout-btn-mobile" @click="logout">
                <q-item-section avatar><q-icon name="logout" /></q-item-section>
                <q-item-section>{{ $t('login.signOut') }}</q-item-section>
              </q-item>
            </q-list>
          </q-menu>
        </q-btn>
      </q-toolbar>
    </q-header>

    <!-- Use Screen's breakpoint as the explicit behavior signal: QDrawer can
         otherwise miss a width update while its mobile overlay locks scroll. -->
    <q-drawer
      v-model="drawerOpen"
      show-if-above
      :breakpoint="1023"
      :behavior="$q.screen.lt.md ? 'mobile' : 'desktop'"
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
          :aria-label="navLabel(item.id)"
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
          :aria-label="navLabel(item.id)"
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
    <ChangePasswordDialog v-model="changePasswordOpen" :csrf-token="session.csrfToken" @changed="passwordChanged" />
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
