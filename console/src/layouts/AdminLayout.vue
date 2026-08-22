<script setup lang="ts">
import { ref, watch } from 'vue'
import { useQuasar } from 'quasar'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useSessionStore } from '../stores/session'
import { visibleNavItems } from '../router/navigation'
import HealthIndicator from '../components/HealthIndicator.vue'
import { switchLocale, currentLocale, SUPPORTED_LOCALES, type LocaleCode } from '../i18n'

const $t = useI18n().t
const $q = useQuasar()
const route = useRoute()
const router = useRouter()
const session = useSessionStore()
const drawerOpen = ref(false)
const navItems = visibleNavItems()

// On wide screens (lg+) the drawer is persistent; ensure it starts open.
// On medium and below it is an overlay that starts closed.
if ($q.screen.gt.md) {
  drawerOpen.value = true
}

// On medium and below the drawer is an overlay; close it after navigating.
// On wide screens the drawer is persistent; do NOT close it.
function onNavigate() {
  if (!$q.screen.gt.md) {
    drawerOpen.value = false
  }
}

// On mobile the drawer is an overlay; close it after navigating so the user
// returns to the chosen page (implementation §5 Mobile, §12).
// On wide screens the drawer is persistent; do NOT close it.
watch(
  () => route.fullPath,
  () => {
    if (!$q.screen.gt.md) {
      drawerOpen.value = false
    }
  },
)

async function logout() {
  await session.logout()
  await router.replace('/login')
}

/** Map nav item id to i18n key. */
function navLabel(id: string): string {
  const key = `nav.${id.toLowerCase()}`
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
          flat round dense icon="menu"
          class="lt-lg"
          :aria-label="$t('nav.system')"
          @click="drawerOpen = !drawerOpen"
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

    <!-- Responsive primary navigation (implementation §5):
         Wide = persistent rail, Compact = collapsible mini, Mobile = overlay drawer. -->
    <q-drawer
      v-model="drawerOpen"
      :show-if-above="$q.screen.gt.md"
      bordered
      :width="240"
      :mini="false"
    >
      <q-list padding>
        <q-item-label header class="gt-sm">{{ $t('nav.runtimeFoundation') }}</q-item-label>
        <q-item
          v-for="item in navItems"
          :key="item.id"
          clickable
          :to="item.path"
          exact
          active-class="bg-grey-2 text-primary"
          @click="onNavigate"
        >
          <q-item-section avatar><q-icon :name="item.icon" /></q-item-section>
          <q-item-section>{{ navLabel(item.id) }}</q-item-section>
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
