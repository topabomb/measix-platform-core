<script setup lang="ts">
import { onMounted, ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { components } from '../api/generated'
import { apiFetch } from '../api/client'
import { cursorPath } from '../api/pagination'
import PageHeader from '../components/PageHeader.vue'
import LoadingState from '../components/LoadingState.vue'
import ProblemBanner from '../components/ProblemBanner.vue'
import StatusChip from '../components/StatusChip.vue'
import { useSessionStore } from '../stores/session'
import { renderContent } from '../composables/useMarkdown'

const { t: $t } = useI18n()

type EnterpriseUpdatePage = components['schemas']['EnterpriseUpdatePage']
type EnterpriseUpdate = components['schemas']['EnterpriseUpdate']
type EnterpriseUpdateCategory = components['schemas']['EnterpriseUpdateCategory']
type EnterpriseUpdateSeverity = components['schemas']['EnterpriseUpdateSeverity']
type EnterpriseUpdateContentFormat = components['schemas']['EnterpriseUpdateContentFormat']

const session = useSessionStore()
const updates = ref<EnterpriseUpdate[]>([])
const nextCursor = ref<string>()
async function loadMore() {
  if (!nextCursor.value || loading.value) return
  loading.value = true
  try {
    const page = await apiFetch<EnterpriseUpdatePage>(cursorPath('/api/admin/v1/enterprise-updates?limit=200', nextCursor.value))
    updates.value.push(...page.items)
    nextCursor.value = page.nextCursor
  } catch (cause) { error.value = cause } finally { loading.value = false }
}

const feedRevision = ref(0)
const loading = ref(false)
const error = ref<unknown>()

const CATEGORIES: EnterpriseUpdateCategory[] = ['ANNOUNCEMENT', 'MAINTENANCE', 'NOTICE']
const SEVERITIES: EnterpriseUpdateSeverity[] = ['INFO', 'WARNING', 'CRITICAL']
const CONTENT_FORMATS: EnterpriseUpdateContentFormat[] = ['MARKDOWN', 'PLAIN']

const categoryColor: Record<string, string> = {
  ANNOUNCEMENT: 'primary',
  MAINTENANCE: 'orange',
  NOTICE: 'teal',
}
const severityColor: Record<string, string> = {
  INFO: 'grey',
  WARNING: 'amber',
  CRITICAL: 'negative',
}

// Create dialog
const createOpen = ref(false)
const createTitle = ref('')
const createContent = ref('')
const createContentFormat = ref<EnterpriseUpdateContentFormat>('PLAIN')
const createCategory = ref<EnterpriseUpdateCategory>('NOTICE')
const createSeverity = ref<EnterpriseUpdateSeverity>('INFO')
const creating = ref(false)

// Edit dialog
const editOpen = ref(false)
const editItem = ref<EnterpriseUpdate | null>(null)
const editTitle = ref('')
const editContent = ref('')
const editContentFormat = ref<EnterpriseUpdateContentFormat>('PLAIN')
const editCategory = ref<EnterpriseUpdateCategory>('NOTICE')
const editSeverity = ref<EnterpriseUpdateSeverity>('INFO')
const editing = ref(false)

// Detail dialog
const detailOpen = ref(false)
const detailItem = ref<EnterpriseUpdate | null>(null)

const detailHtml = computed(() => {
  if (!detailItem.value) return ''
  return renderContent(detailItem.value.content, detailItem.value.contentFormat)
})

async function refresh() {
  loading.value = true
  error.value = undefined
  try {
    const page = await apiFetch<EnterpriseUpdatePage>('/api/admin/v1/enterprise-updates?limit=200')
    updates.value = page.items
    nextCursor.value = page.nextCursor
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
  createContentFormat.value = 'PLAIN'
  createCategory.value = 'NOTICE'
  createSeverity.value = 'INFO'
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
        body: JSON.stringify({
          title: createTitle.value,
          content: createContent.value,
          contentFormat: createContentFormat.value,
          category: createCategory.value,
          severity: createSeverity.value,
        }),
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
  editContentFormat.value = item.contentFormat
  editCategory.value = item.category
  editSeverity.value = item.severity
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
        body: JSON.stringify({
          title: editTitle.value,
          content: editContent.value,
          contentFormat: editContentFormat.value,
          category: editCategory.value,
          severity: editSeverity.value,
        }),
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

function openDetail(item: EnterpriseUpdate) {
  detailItem.value = item
  detailOpen.value = true
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
        <q-item v-for="item in updates" :key="item.enterpriseUpdateId" clickable @click="openDetail(item)">
          <q-item-section>
            <q-item-label>{{ item.title }}</q-item-label>
            <q-item-label caption class="text-grey-7" style="white-space: pre-wrap; max-height: 3em; overflow: hidden;">
              {{ item.content }}
            </q-item-label>
            <div class="row q-gutter-xs q-mt-xs">
              <q-badge v-if="item.category" dense :color="categoryColor[item.category] ?? 'grey'" :label="$t(`enterpriseUpdates.category.${item.category.toLowerCase()}`)" />
              <q-badge v-if="item.severity" dense :color="severityColor[item.severity] ?? 'grey'" :label="$t(`enterpriseUpdates.severity.${item.severity.toLowerCase()}`)" />
              <q-badge v-if="item.contentFormat === 'MARKDOWN'" dense color="deep-purple" label="MD" />
            </div>
            <q-item-label caption class="text-grey-6">
              {{ item.enterpriseUpdateId }} · {{ $t('enterpriseUpdates.createdAt') }} {{ item.createdAt }}
            </q-item-label>
          </q-item-section>
          <q-item-section side>
            <div class="row items-center q-gutter-sm">
              <StatusChip :value="item.status" />
              <q-btn v-if="item.status === 'DRAFT'" outline color="primary" icon="edit" size="sm" @click.stop="openEdit(item)" />
              <q-btn v-if="item.status === 'DRAFT'" outline color="positive" icon="publish" size="sm" :label="$t('enterpriseUpdates.publish')" @click.stop="publish(item)" />
              <q-btn v-if="item.status === 'PUBLISHED'" outline color="warning" icon="unpublished" size="sm" :label="$t('enterpriseUpdates.withdraw')" @click.stop="withdraw(item)" />
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
          <div class="row q-gutter-sm q-mb-md">
            <q-select v-model="createContentFormat" :label="$t('enterpriseUpdates.contentFormatLabel')" :options="CONTENT_FORMATS" outlined dense emit-value map-options class="col" />
            <q-select v-model="createCategory" :label="$t('enterpriseUpdates.categoryLabel')" :options="CATEGORIES" outlined dense emit-value map-options class="col" />
            <q-select v-model="createSeverity" :label="$t('enterpriseUpdates.severityLabel')" :options="SEVERITIES" outlined dense emit-value map-options class="col" />
          </div>
          <q-input v-model="createContent" :label="$t('enterpriseUpdates.contentLabel')" type="textarea" outlined dense :input-style="{ minHeight: '150px' }" />
          <div v-if="createContentFormat === 'MARKDOWN'" class="text-caption text-grey-7 q-mt-xs">{{ $t('enterpriseUpdates.markdownHint') }}</div>
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
          <div class="row q-gutter-sm q-mb-md">
            <q-select v-model="editContentFormat" :label="$t('enterpriseUpdates.contentFormatLabel')" :options="CONTENT_FORMATS" outlined dense emit-value map-options class="col" />
            <q-select v-model="editCategory" :label="$t('enterpriseUpdates.categoryLabel')" :options="CATEGORIES" outlined dense emit-value map-options class="col" />
            <q-select v-model="editSeverity" :label="$t('enterpriseUpdates.severityLabel')" :options="SEVERITIES" outlined dense emit-value map-options class="col" />
          </div>
          <q-input v-model="editContent" :label="$t('enterpriseUpdates.contentLabel')" type="textarea" outlined dense :input-style="{ minHeight: '150px' }" />
          <div v-if="editContentFormat === 'MARKDOWN'" class="text-caption text-grey-7 q-mt-xs">{{ $t('enterpriseUpdates.markdownHint') }}</div>
        </q-card-section>
        <q-separator />
        <q-card-actions align="right">
          <q-btn flat :label="$t('common.cancel')" color="primary" v-close-popup />
          <q-btn unelevated color="primary" :label="$t('common.save')" :loading="editing" @click="submitEdit" />
        </q-card-actions>
      </q-card>
    </q-dialog>

    <!-- Detail Dialog -->
    <q-dialog v-model="detailOpen">
      <q-card class="responsive-modal" style="max-width: 90vw; width: 700px;">
        <q-card-section v-if="detailItem">
          <div class="text-h6">{{ detailItem.title }}</div>
          <div class="row q-gutter-xs q-mt-sm">
            <q-badge dense :color="categoryColor[detailItem.category] ?? 'grey'" :label="$t(`enterpriseUpdates.category.${detailItem.category.toLowerCase()}`)" />
            <q-badge dense :color="severityColor[detailItem.severity] ?? 'grey'" :label="$t(`enterpriseUpdates.severity.${detailItem.severity.toLowerCase()}`)" />
            <q-badge v-if="detailItem.contentFormat === 'MARKDOWN'" dense color="deep-purple" label="Markdown" />
            <q-badge dense :color="detailItem.status === 'PUBLISHED' ? 'green' : detailItem.status === 'DRAFT' ? 'orange' : 'grey'" :label="detailItem.status" />
          </div>
          <div class="text-caption text-grey-7 q-mt-xs">
            {{ detailItem.enterpriseUpdateId }} · {{ detailItem.publishedAt ?? detailItem.createdAt }}
          </div>
        </q-card-section>
        <q-separator />
        <q-card-section v-if="detailItem">
          <div v-if="detailItem.contentFormat === 'MARKDOWN'" class="markdown-body" v-html="detailHtml" />
          <div v-else class="text-body2" style="white-space: pre-wrap;">{{ detailItem.content }}</div>
        </q-card-section>
        <q-card-actions align="right">
          <q-btn flat :label="$t('common.close')" v-close-popup />
          <q-btn v-if="detailItem?.status === 'DRAFT'" outline color="primary" icon="edit" :label="$t('common.edit')" @click="editOpen = true; openEdit(detailItem!); detailOpen = false" />
          <q-btn v-if="detailItem?.status === 'DRAFT'" outline color="positive" icon="publish" :label="$t('enterpriseUpdates.publish')" @click="publish(detailItem!); detailOpen = false" />
          <q-btn v-if="detailItem?.status === 'PUBLISHED'" outline color="warning" icon="unpublished" :label="$t('enterpriseUpdates.withdraw')" @click="withdraw(detailItem!); detailOpen = false" />
        </q-card-actions>
      </q-card>
    </q-dialog>
    <q-btn v-if="nextCursor" outline :label="$t('common.loadMore')" :loading="loading" @click="loadMore" data-cy="load-more" class="q-mt-md" />
  </q-page>
</template>

<style scoped>
.markdown-body :deep(h1) { font-size: 1.5rem; font-weight: 500; margin: 0.5em 0; }
.markdown-body :deep(h2) { font-size: 1.25rem; font-weight: 500; margin: 0.5em 0; }
.markdown-body :deep(h3) { font-size: 1.1rem; font-weight: 500; margin: 0.5em 0; }
.markdown-body :deep(p) { margin: 0.5em 0; }
.markdown-body :deep(ul), .markdown-body :deep(ol) { margin: 0.5em 0; padding-left: 1.5em; }
.markdown-body :deep(code) { background: rgba(0,0,0,0.08); padding: 0.1em 0.3em; border-radius: 3px; font-size: 0.9em; }
.markdown-body :deep(pre) { background: rgba(0,0,0,0.08); padding: 0.5em; border-radius: 4px; overflow-x: auto; }
.markdown-body :deep(blockquote) { border-left: 3px solid #ccc; margin: 0.5em 0; padding-left: 1em; color: #666; }
.markdown-body :deep(a) { color: #1976d2; text-decoration: underline; }
.markdown-body :deep(hr) { border: none; border-top: 1px solid #ccc; margin: 1em 0; }
</style>
