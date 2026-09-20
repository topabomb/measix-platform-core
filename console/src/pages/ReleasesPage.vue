<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { components } from '../api/generated'
import { apiFetch } from '../api/client'
import PageHeader from '../components/PageHeader.vue'
import LoadingState from '../components/LoadingState.vue'
import ProblemBanner from '../components/ProblemBanner.vue'
import StatusChip from '../components/StatusChip.vue'
import DetailWorkspace from '../components/DetailWorkspace.vue'
import CursorPager from '../components/CursorPager.vue'
import { useCursorPager } from '../composables/useCursorPager'
import { useActivationStore } from '../stores/activation'
import { useSessionStore } from '../stores/session'

const { t: $t, locale } = useI18n()

type ReleasePage = components['schemas']['ReleasePage']
type Release = components['schemas']['Release']
type Activation = components['schemas']['Activation']
type ResourceDiff = components['schemas']['ResourceDiff']

const session = useSessionStore()
const detailRelease = ref<Release>()
const activation = useActivationStore()
const listPath = ref('/api/admin/v1/releases?limit=50')
const {
  items: releases,
  nextCursor,
  pageNumber,
  loading,
  error,
  reset: resetReleases,
  nextPage,
  previousPage,
} = useCursorPager<Release, ReleasePage>(listPath, path => apiFetch<ReleasePage>(path))

async function refresh() {
  await resetReleases()
}

async function republish(release: Release) {
  if (!session.csrfToken) return
  if (!window.confirm($t('releases.republishConfirm', { gen: release.managedGeneration }))) return
  const key = activation.beginCommand('PUBLISH', 'release:' + release.releaseId)
  error.value = undefined
  try {
    const result = await apiFetch<Activation>(
      `/api/admin/v1/releases/${encodeURIComponent(release.releaseId)}:republish`,
      { method: 'POST', headers: { 'Idempotency-Key': key } },
      session.csrfToken,
    )
    activation.accept(result)
    if (result.state === 'APPLYING' || result.state === 'UNKNOWN') {
      await activation.pollUntilSettled(result.activationId, { timeoutMs: 60_000 })
    }
    await refresh()
    detailRelease.value = undefined
  } catch (cause) {
    error.value = cause
  }
}

function showDetail(release: Release) {
  detailRelease.value = release
}

function diffText(diff: Release['diffSummary']): string {
  const parts: string[] = []
  if (diff.added) parts.push($t('releases.addedCount', { count: diff.added }))
  if (diff.changed) parts.push($t('releases.changedCount', { count: diff.changed }))
  if (diff.removed) parts.push($t('releases.removedCount', { count: diff.removed }))
  return parts.length ? parts.join(' · ') : $t('common.noData')
}

function localTime(value: string): string {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString(locale.value === 'zh' ? 'zh-CN' : 'en-US')
}

function diffKindLabel(kind: string): string {
  const labels: Record<string, string> = {
    PROVIDER: $t('resources.preview.providers'), MODEL: $t('resources.tabs.models'), TTS: 'TTS', ASR: 'ASR',
    MCP: 'MCP', ASSISTANT: $t('experience.tab'), STARTER: $t('experience.starters'),
    POLICY: $t('resources.tabs.policy'), BINDING: $t('resources.relationship.title'),
  }
  return labels[kind.toUpperCase()] ?? kind
}

onMounted(refresh)
</script>

<template>
  <q-page class="admin-page" data-cy="releases-page">
    <PageHeader :title="$t('releases.title')" :subtitle="$t('releases.subtitle')">
      <template #actions>
        <q-btn flat icon="refresh" :aria-label="$t('common.refresh')" :loading="loading" @click="refresh" />
      </template>
    </PageHeader>
    <ProblemBanner :error="error" class="q-mb-xs" />
    <q-banner class="bg-blue-1 q-mb-xs rounded-borders">
      {{ $t('releases.deviceApplicationHint') }}
      <q-btn flat no-caps :to="{ name: 'Users' }" :label="$t('releases.viewDevices')" />
    </q-banner>
    <q-banner v-if="activation.activation" :class="activation.succeeded ? 'bg-green-1' : 'bg-orange-1'" class="q-mb-xs rounded-borders">
      <div class="row items-center justify-between">
        <span>{{ $t('resources.draft.latestOperation') }}</span>
        <StatusChip :value="activation.activation.state" />
      </div>
      <details class="text-caption text-grey-7" data-cy="release-activation-details"><summary>{{ $t('resources.review.technicalDetails') }}</summary>{{ activation.activation.activationId }} · {{ activation.activation.kind }}</details>
    </q-banner>
    <DetailWorkspace :detail-open="Boolean(detailRelease)" list-width="minmax(300px, 400px)">
      <template #list>
    <LoadingState v-if="loading && !releases.length" />
    <q-card v-else flat bordered>
      <q-card-section class="row items-center justify-between q-py-xs">
        <div class="text-subtitle2">{{ $t('releases.title') }}</div>
      </q-card-section>
      <q-separator />
      <q-list separator>
        <q-item v-for="release in releases" :key="release.releaseId" clickable @click="showDetail(release)">
          <q-item-section>
            <q-item-label>{{ $t('releases.versionLabel', { generation: release.managedGeneration }) }}</q-item-label>
            <q-item-label caption>
              {{ diffText(release.diffSummary) }} · {{ $t('releases.publishedAt') }} {{ localTime(release.publishedAt) }}
            </q-item-label>
          </q-item-section>
          <q-item-section side>
            <div class="row items-center q-gutter-xs">
              <StatusChip :value="release.status" />
            </div>
          </q-item-section>
        </q-item>
        <q-item v-if="!releases.length"><q-item-section class="text-grey-7">{{ $t('releases.noReleases') }}</q-item-section></q-item>
      </q-list>
      <q-separator />
      <CursorPager :page="pageNumber" :count="releases.length" :has-next="Boolean(nextCursor)" :loading="loading" @previous="previousPage" @next="nextPage" />
    </q-card>

      </template>
      <template #detail>
      <q-card v-if="detailRelease" flat bordered data-cy="release-detail">
        <q-card-section class="row items-center justify-between">
          <div>
            <div class="text-h6">{{ $t('releases.versionLabel', { generation: detailRelease.managedGeneration }) }}</div>
            <div class="text-caption text-grey-7">{{ $t('releases.publishedAt') }} {{ localTime(detailRelease.publishedAt) }}</div>
          </div>
          <StatusChip :value="detailRelease.status" />
        </q-card-section>
        <q-separator />
        <q-card-section>
          <div class="text-subtitle2">{{ $t('releases.diff') }}</div>
          <q-markup-table flat dense v-if="detailRelease.diffSummary.details?.length">
            <thead><tr><th>{{ $t('resources.relationship.kind') }}</th><th class="text-right">{{ $t('resources.review.added') }}</th><th class="text-right">{{ $t('resources.review.changed') }}</th><th class="text-right">{{ $t('resources.review.removed') }}</th></tr></thead>
            <tbody>
              <tr v-for="d in (detailRelease.diffSummary.details as ResourceDiff[]).filter(row => row.added + row.changed + row.removed > 0)" :key="d.kind">
                <td>{{ diffKindLabel(d.kind) }}</td>
                <td class="text-right text-positive">{{ d.added ? '+' + d.added : '—' }}</td>
                <td class="text-right text-warning">{{ d.changed ? '~' + d.changed : '—' }}</td>
                <td class="text-right text-negative">{{ d.removed ? '-' + d.removed : '—' }}</td>
              </tr>
            </tbody>
          </q-markup-table>
          <div v-else class="text-grey-7">{{ $t('common.noData') }}</div>

          <div class="text-subtitle2 q-mt-xs">{{ $t('releases.activationHistory') }}</div>
          <div class="text-caption text-grey-7">{{ $t('releases.activationHistoryRecent') }}</div>
          <div v-if="detailRelease.activationHistory.length">
            <q-timeline dense>
              <q-timeline-entry v-for="attempt in detailRelease.activationHistory" :key="attempt.activationId"
                :title="localTime(attempt.createdAt)" :color="attempt.state === 'COMPLETED' ? 'positive' : attempt.state === 'FAILED' ? 'negative' : 'primary'">
                <div class="row items-center q-gutter-xs">
                  <StatusChip :value="attempt.state" />
                  <span v-if="attempt.errorCode" class="text-negative text-caption">{{ attempt.errorCode }}</span>
                </div>
                <details class="text-caption text-grey-7"><summary class="cursor-pointer">{{ $t('resources.review.technicalDetails') }}</summary>{{ attempt.activationId }}</details>
              </q-timeline-entry>
            </q-timeline>
          </div>
          <div v-else class="text-grey-7">{{ $t('common.noData') }}</div>
          <details class="text-caption text-grey-7 q-mt-xs"><summary class="cursor-pointer">{{ $t('resources.review.technicalDetails') }}</summary>
            {{ $t('releases.sourceDraft') }} r{{ detailRelease.sourceDraftRevision }} · {{ $t('releases.publishedBy') }} {{ detailRelease.publishedBy || '—' }}<br>
            {{ detailRelease.releaseId }} · {{ detailRelease.snapshotHash }}
          </details>
        </q-card-section>
        <q-separator />
        <q-card-actions align="right">
          <q-btn flat icon="arrow_back" :label="$t('common.close')" color="primary" @click="detailRelease = undefined" />
          <q-btn outline color="primary" :label="$t('releases.republish')" :disable="!session.csrfToken" @click="republish(detailRelease)" />
        </q-card-actions>
      </q-card>
      </template>
    </DetailWorkspace>
  </q-page>
</template>
