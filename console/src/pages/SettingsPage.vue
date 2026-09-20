<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { components } from '../api/generated'
import { apiFetch } from '../api/client'
import { useSessionStore } from '../stores/session'
import PageHeader from '../components/PageHeader.vue'
import LoadingState from '../components/LoadingState.vue'
import ProblemBanner from '../components/ProblemBanner.vue'

const { t: $t } = useI18n()
const session = useSessionStore()

type DeploymentSettings = components['schemas']['DeploymentSettings']
type SystemStatus = components['schemas']['SystemStatus']

const settings = ref<DeploymentSettings>()
const system = ref<SystemStatus>()
const name = ref('')
const publicOrigin = ref('')
const loading = ref(false)
const saving = ref(false)
const saved = ref(false)
const confirmOriginChange = ref(false)
const error = ref<unknown>()

const normalizedName = computed(() => name.value.trim())
const normalizedPublicOrigin = computed(() => canonicalPublicOrigin(publicOrigin.value))
const originChanged = computed(() => Boolean(settings.value && normalizedPublicOrigin.value && normalizedPublicOrigin.value !== settings.value.publicOrigin))
const dirty = computed(() => Boolean(settings.value && (normalizedName.value !== settings.value.name || normalizedPublicOrigin.value !== settings.value.publicOrigin)))
const canSave = computed(() => Boolean(session.csrfToken && dirty.value && normalizedName.value && normalizedName.value.length <= 120 && normalizedPublicOrigin.value))

function canonicalPublicOrigin(value: string): string | undefined {
  const trimmed = value.trim()
  try {
    const url = new URL(trimmed)
    if (!['http:', 'https:'].includes(url.protocol) || url.username || url.password || url.pathname !== '/' || url.search || url.hash) return undefined
    return url.origin
  } catch {
    return undefined
  }
}

async function refresh() {
  loading.value = true
  error.value = undefined
  saved.value = false
  try {
    const [current, status] = await Promise.all([
      apiFetch<DeploymentSettings>('/api/admin/v1/deployment/settings'),
      apiFetch<SystemStatus>('/api/admin/v1/system/status'),
    ])
    settings.value = current
    system.value = status
    name.value = current.name
    publicOrigin.value = current.publicOrigin
  } catch (cause) {
    error.value = cause
  } finally {
    loading.value = false
  }
}

function requestSave() {
  if (!canSave.value) return
  if (originChanged.value) confirmOriginChange.value = true
  else void save()
}

async function save() {
  if (!settings.value || !session.csrfToken || !canSave.value) return
  confirmOriginChange.value = false
  saving.value = true
  saved.value = false
  error.value = undefined
  try {
    const updated = await apiFetch<DeploymentSettings>('/api/admin/v1/deployment/settings', {
      method: 'PUT',
      body: JSON.stringify({ expectedUpdatedAt: settings.value.updatedAt, name: normalizedName.value, publicOrigin: normalizedPublicOrigin.value }),
    }, session.csrfToken)
    settings.value = updated
    name.value = updated.name
    publicOrigin.value = updated.publicOrigin
    saved.value = true
  } catch (cause) {
    error.value = cause
  } finally {
    saving.value = false
  }
}

onMounted(refresh)
</script>

<template>
  <q-page class="admin-page" data-cy="settings-page">
    <PageHeader :title="$t('settings.title')" :subtitle="$t('settings.subtitle')">
      <template #actions>
        <q-btn flat dense icon="refresh" :aria-label="$t('common.refresh')" :loading="loading" @click="refresh" />
      </template>
    </PageHeader>
    <ProblemBanner :error="error" class="q-mb-xs" />
    <q-banner v-if="saved" dense class="bg-green-1 text-positive q-mb-xs rounded-borders" data-cy="settings-saved">
      {{ $t('settings.saved') }}
    </q-banner>
    <LoadingState v-if="loading && !settings" />
    <div v-else-if="settings" class="settings-grid">
      <q-card flat bordered>
        <q-card-section class="q-py-sm">
          <div class="text-subtitle2">{{ $t('settings.profile') }}</div>
          <div class="text-caption text-grey-7">{{ $t('settings.profileHint') }}</div>
        </q-card-section>
        <q-separator />
        <q-card-section class="q-py-sm">
          <q-input
            v-model="name"
            outlined dense
            :label="$t('settings.enterpriseName')"
            maxlength="120"
            counter
            data-cy="settings-name"
            @keyup.enter="requestSave"
          />
          <q-input
            v-model="publicOrigin"
            outlined dense
            class="q-mt-xs"
            :label="$t('settings.publicOrigin')"
            :hint="$t('settings.publicOriginHint')"
            :error="Boolean(publicOrigin.trim() && !normalizedPublicOrigin)"
            :error-message="$t('settings.publicOriginInvalid')"
            maxlength="1024"
            data-cy="settings-public-origin"
            @keyup.enter="requestSave"
          />
          <div class="text-caption text-grey-7 q-mt-xs text-break">{{ settings.deploymentId }}</div>
          <div class="text-caption text-grey-7">{{ $t('settings.updatedAt') }}: {{ new Date(settings.updatedAt).toLocaleString() }}</div>
        </q-card-section>
        <q-card-actions align="right" class="q-px-sm q-py-xs">
          <q-btn flat dense :label="$t('common.discard')" :disable="!dirty" @click="name = settings.name; publicOrigin = settings.publicOrigin" />
          <q-btn dense color="primary" icon="save" :label="$t('common.save')" :disable="!canSave" :loading="saving" data-cy="settings-save" @click="requestSave" />
        </q-card-actions>
      </q-card>

      <q-card flat bordered>
        <q-card-section class="q-py-sm">
          <div class="row items-center q-gutter-xs">
            <q-icon name="lock" color="grey-7" />
            <div class="text-subtitle2">{{ $t('settings.protected') }}</div>
          </div>
          <div class="text-caption text-grey-7">{{ $t('settings.protectedHint') }}</div>
        </q-card-section>
        <q-separator />
        <q-list dense separator>
          <q-item>
            <q-item-section>
              <q-item-label>{{ $t('settings.timezone') }}</q-item-label>
              <q-item-label caption>{{ $t('settings.timezoneHint') }}</q-item-label>
            </q-item-section>
            <q-item-section side><code>{{ settings.timezone }}</code></q-item-section>
          </q-item>
          <q-item>
            <q-item-section><q-item-label>{{ $t('settings.portalMode') }}</q-item-label></q-item-section>
            <q-item-section side>{{ system?.portalMode ?? '—' }}</q-item-section>
          </q-item>
        </q-list>
        <q-card-section class="q-py-xs text-caption text-grey-7">{{ $t('settings.runtimeHint') }}</q-card-section>
        <q-card-actions align="right" class="q-px-sm q-py-xs">
          <q-btn flat dense no-caps icon="monitor_heart" :to="{ name: 'System' }" :label="$t('settings.openSystem')" />
        </q-card-actions>
      </q-card>
    </div>

    <q-dialog v-model="confirmOriginChange" persistent>
      <q-card style="width: min(520px, calc(100vw - 24px))" data-cy="settings-origin-confirm">
        <q-card-section class="q-pb-xs">
          <div class="text-subtitle1 text-weight-medium">{{ $t('settings.confirmOriginTitle') }}</div>
          <div class="text-body2 q-mt-xs">{{ $t('settings.confirmOriginIntro', { origin: normalizedPublicOrigin }) }}</div>
        </q-card-section>
        <q-card-section class="q-py-xs text-body2">
          <ul class="impact-list">
            <li>{{ $t('settings.confirmOriginPortal') }}</li>
            <li>{{ $t('settings.confirmOriginClients') }}</li>
            <li>{{ $t('settings.confirmOriginIngress') }}</li>
          </ul>
        </q-card-section>
        <q-card-actions align="right" class="q-px-sm q-py-xs">
          <q-btn v-close-popup flat dense :label="$t('common.cancel')" />
          <q-btn dense color="primary" :label="$t('settings.applyOrigin')" :loading="saving" data-cy="settings-origin-apply" @click="save" />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </q-page>
</template>

<style scoped>
.settings-grid {
  display: grid;
  grid-template-columns: minmax(320px, 1fr) minmax(320px, .9fr);
  gap: 4px;
  align-items: start;
}

@media (max-width: 760px) {
  .settings-grid { grid-template-columns: minmax(0, 1fr); }
}

.impact-list {
  margin: 0;
  padding-left: 20px;
}

.impact-list li + li { margin-top: 4px; }
</style>
