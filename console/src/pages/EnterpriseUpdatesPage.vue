<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { components } from '../api/generated'
import { apiFetch } from '../api/client'
import PageHeader from '../components/PageHeader.vue'
import LoadingState from '../components/LoadingState.vue'
import ProblemBanner from '../components/ProblemBanner.vue'
import StatusChip from '../components/StatusChip.vue'
import { useSessionStore } from '../stores/session'

const { t: $t } = useI18n()

type EnterpriseUpdatePage = components['schemas']['EnterpriseUpdatePage']
type EnterpriseUpdate = components['schemas']['EnterpriseUpdate']

const session = useSessionStore()
const updates = ref<EnterpriseUpdate[]>([])
const feedRevision = ref(0)
const loading = ref(false)
const error = ref<unknown>()

// Create dialog
const createOpen = ref(false)
const createTitle = ref('')
const createContent = ref('')
const creating = ref(false)

// Edit dialog
const editOpen = ref(false)
const editItem = ref<EnterpriseUpdate | null>(null)
const editTitle = ref('')
const editContent = ref('')
const editing = ref(false)

async function refresh() {
  loading.value = true
  error.value = undefined
  try {
    const page = await apiFetch<EnterpriseUpdatePage>('/api/admin/v1/enterprise-updates?limit=200')
    updates.value = page.items
    feedRevision.value = page.feedRevision
  } catch (cause) {
    error.value = cause
  } finally {
    loading.value = false
  }
}

function openCreate() {
  createTitle.value = ''
  createContent.value = ''
  createOpen.value = true
}

async function submitCreate() {
  if (!session.csrfToken) return
  creating.value = true
  error.value = undefined
  try {
    await apiFetch<EnterpriseUpdate>(
      '/api/admin/v1/enterprise-updates',
      {
        method: 'POST',
        body: JSON.stringify({ title: createTitle.value, content: createContent.value }),
      },
      session.csrfToken,
    )
    createOpen.value = false
    await refresh()
  } catch (cause) {
    error.value = cause
  } finally {
    creating.value = false
  }
}

function openEdit(item: EnterpriseUpdate) {
  editItem.value = item
  editTitle.value = item.title
  editContent.value = item.content
  editOpen.value = true
}

async function submitEdit() {
  if (!session.csrfToken || !editItem.value) return
  editing.value = true
  error.value = undefined
  try {
    await apiFetch<EnterpriseUpdate>(
      `/api/admin/v1/enterprise-updates/${encodeURIComponent(editItem.value.enterpriseUpdateId)}`,
      {
        method: 'PUT',
        body: JSON.stringify({ title: editTitle.value, content: editContent.value }),
      },
      session.csrfToken,
    )
    editOpen.value = false
    await refresh()
  } catch (cause) {
    error.value = cause
  } finally {
    editing.value = false
  }
}

async function publish(item: EnterpriseUpdate) {
  if (!session.csrfToken) return
  if (!window.confirm($t('enterpriseUpdates.publishConfirm', { id: item.enterpriseUpdateId }))) return
  error.value = undefined
  try {
    await apiFetch<EnterpriseUpdate>(
      `/api/admin/v1/enterprise-updates/${encodeURIComponent(item.enterpriseUpdateId)}:publish`,
      { method: 'POST' },
      session.csrfToken,
    )
    await refresh()
  } catch (cause) {
    error.value = cause
  }
}

async function withdraw(item: EnterpriseUpdate) {
  if (!session.csrfToken) return
  if (!window.confirm($t('enterpriseUpdates.withdrawConfirm', { id: item.enterpriseUpdateId }))) return
  error.value = undefined
  try {
    await apiFetch<EnterpriseUpdate>(
      `/api/admin/v1/enterprise-updates/${encodeURIComponent(item.enterpriseUpdateId)}:withdraw`,
      { method: 'POST' },
      session.csrfToken,
    )
    await refresh()
  } catch (cause) {
    error.value = cause
  }
}

onMounted(refresh)
</script>

<template>
  <q-page padding data-cy="enterprise-updates-page">
    <PageHeader :title="$t('enterpriseUpdates.title')" :subtitle="$t('enterpriseUpdates.subtitle')">
      <template #actions>
        <q-btn flat icon="refresh" :loading="loading" @click="refresh" />
        <q-btn unelevated color="primary" icon="add" :label="$t('enterpriseUpdates.create')" @click="openCreate" />
      </template>
    </PageHeader>
    <ProblemBanner :error="error" class="q-mb-md" />
    <q-banner v-if="feedRevision" class="bg-blue-1 q-mb-md rounded-borders">
      <span class="text-grey-8">{{ $t('enterpriseUpdates.feedRevision') }}: {{ feedRevision }}</span>
    </q-banner>
    <LoadingState v-if="loading && !updates.length" />
    <q-card v-else flat bordered>
      <q-list separator>
        <q-item v-for="item in updates" :key="item.enterpriseUpdateId">
          <q-item-section>
            <q-item-label>{{ item.title }}</q-item-label>
            <q-item-label caption class="text-grey-7" style="white-space: pre-wrap; max-height: 3em; overflow: hidden;">
              {{ item.content }}
            </q-item-label>
            <q-item-label caption class="text-grey-6">
              {{ item.enterpriseUpdateId }} · {{ $t('enterpriseUpdates.createdAt') }} {{ item.createdAt }}
            </q-item-label>
          </q-item-section>
          <q-item-section side>
            <div class="row items-center q-gutter-sm">
              <StatusChip :value="item.status" />
              <q-btn v-if="item.status === 'DRAFT'" outline color="primary" icon="edit" size="sm" @click="openEdit(item)" />
              <q-btn v-if="item.status === 'DRAFT'" outline color="positive" icon="publish" size="sm" :label="$t('enterpriseUpdates.publish')" @click="publish(item)" />
              <q-btn v-if="item.status === 'PUBLISHED'" outline color="warning" icon="unpublished" size="sm" :label="$t('enterpriseUpdates.withdraw')" @click="withdraw(item)" />
            </div>
          </q-item-section>
        </q-item>
        <q-item v-if="!updates.length"><q-item-section class="text-grey-7">{{ $t('enterpriseUpdates.noUpdates') }}</q-item-section></q-item>
      </q-list>
    </q-card>

    <!-- Create Dialog -->
    <q-dialog v-model="createOpen">
      <q-card class="responsive-modal" style="max-width: 80vw; width: 600px;">
        <q-card-section>
          <div class="text-h6">{{ $t('enterpriseUpdates.createTitle') }}</div>
        </q-card-section>
        <q-separator />
        <q-card-section>
          <q-input v-model="createTitle" :label="$t('enterpriseUpdates.titleLabel')" outlined dense class="q-mb-md" />
          <q-input v-model="createContent" :label="$t('enterpriseUpdates.contentLabel')" type="textarea" outlined dense :input-style="{ minHeight: '150px' }" />
        </q-card-section>
        <q-separator />
        <q-card-actions align="right">
          <q-btn flat :label="$t('common.cancel')" color="primary" v-close-popup />
          <q-btn unelevated color="primary" :label="$t('common.create')" :loading="creating" @click="submitCreate" />
        </q-card-actions>
      </q-card>
    </q-dialog>

    <!-- Edit Dialog -->
    <q-dialog v-model="editOpen">
      <q-card class="responsive-modal" style="max-width: 80vw; width: 600px;">
        <q-card-section>
          <div class="text-h6">{{ $t('enterpriseUpdates.editTitle') }}</div>
          <div class="text-caption text-grey-7">{{ editItem?.enterpriseUpdateId }}</div>
        </q-card-section>
        <q-separator />
        <q-card-section>
          <q-input v-model="editTitle" :label="$t('enterpriseUpdates.titleLabel')" outlined dense class="q-mb-md" />
          <q-input v-model="editContent" :label="$t('enterpriseUpdates.contentLabel')" type="textarea" outlined dense :input-style="{ minHeight: '150px' }" />
        </q-card-section>
        <q-separator />
        <q-card-actions align="right">
          <q-btn flat :label="$t('common.cancel')" color="primary" v-close-popup />
          <q-btn unelevated color="primary" :label="$t('common.save')" :loading="editing" @click="submitEdit" />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </q-page>
</template>
