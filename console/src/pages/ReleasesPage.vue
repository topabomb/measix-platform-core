<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { components } from '../api/generated'
import { apiFetch } from '../api/client'
import PageHeader from '../components/PageHeader.vue'
import LoadingState from '../components/LoadingState.vue'
import ProblemBanner from '../components/ProblemBanner.vue'
import StatusChip from '../components/StatusChip.vue'
import { useActivationStore } from '../stores/activation'
import { useSessionStore } from '../stores/session'

const { t: $t } = useI18n()

type ReleasePage = components['schemas']['ReleasePage']
type Release = components['schemas']['Release']
type Activation = components['schemas']['Activation']
type ResourceDiff = components['schemas']['ResourceDiff']

const session = useSessionStore()
const detailRelease = ref<Release>()
const detailOpen = ref(false)
const activation = useActivationStore()
const releases = ref<Release[]>([])
const loading = ref(false)
const error = ref<unknown>()

async function refresh() {
  loading.value = true
  error.value = undefined
  try {
    const page = await apiFetch<ReleasePage>('/api/admin/v1/releases?limit=200')
    releases.value = page.items
  } catch (cause) {
    error.value = cause
  } finally {
    loading.value = false
  }
}

async function republish(release: Release) {
  if (!session.csrfToken) return
  if (!window.confirm($t('releases.republishConfirm', { id: release.releaseId, gen: release.managedGeneration }))) return
  activation.resetCommand()
  const key = activation.beginCommand('PUBLISH')
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
  } catch (cause) {
    error.value = cause
  }
}

function showDetail(release: Release) {
  detailRelease.value = release
  detailOpen.value = true
}

function diffText(diff: Release['diffSummary']): string {
  const parts: string[] = []
  if (diff.added) parts.push(`+${diff.added} ${$t('resources.review.added').toLowerCase()}`)
  if (diff.changed) parts.push(`~${diff.changed} ${$t('resources.review.changed').toLowerCase()}`)
  if (diff.removed) parts.push(`-${diff.removed} ${$t('resources.review.removed').toLowerCase()}`)
  return parts.length ? parts.join(' · ') : $t('common.noData')
}

onMounted(refresh)
</script>

<template>
  <q-page padding data-cy="releases-page">
    <PageHeader :title="$t('releases.title')" :subtitle="$t('releases.subtitle')">
      <template #actions>
        <q-btn flat icon="refresh" :loading="loading" @click="refresh" />
      </template>
    </PageHeader>
    <ProblemBanner :error="error" class="q-mb-md" />
    <q-banner v-if="activation.activation" :class="activation.succeeded ? 'bg-green-1' : 'bg-orange-1'" class="q-mb-md rounded-borders">
      <div class="row items-center justify-between">
        <span>{{ $t('system.currentActivation') }} {{ activation.activation.activationId }} ({{ activation.activation.kind }})</span>
        <StatusChip :value="activation.activation.state" />
      </div>
    </q-banner>
    <LoadingState v-if="loading && !releases.length" />
    <q-card v-else flat bordered>
      <q-list separator>
        <q-item v-for="release in releases" :key="release.releaseId" clickable @click="showDetail(release)">
          <q-item-section>
            <q-item-label>{{ $t('releases.generation') }} {{ release.managedGeneration }} <span class="text-grey-7 text-caption">· {{ $t('releases.sourceDraft') }} r{{ release.sourceDraftRevision }}</span></q-item-label>
            <q-item-label caption>
              {{ diffText(release.diffSummary) }} · {{ $t('releases.publishedAt') }} {{ release.publishedAt }} {{ $t('releases.publishedBy') }} {{ release.publishedBy || '—' }}
            </q-item-label>
          </q-item-section>
          <q-item-section side>
            <div class="row items-center q-gutter-sm">
              <StatusChip :value="release.status" />
              <q-btn outline color="primary" :label="$t('releases.republish')" size="sm" stop @click.stop="republish(release)" />
            </div>
          </q-item-section>
        </q-item>
        <q-item v-if="!releases.length"><q-item-section class="text-grey-7">{{ $t('releases.noReleases') }}</q-item-section></q-item>
      </q-list>
    </q-card>

    <q-dialog v-model="detailOpen">
      <q-card v-if="detailRelease" class="responsive-modal" style="max-width: 95vw">
        <q-card-section class="row items-center justify-between">
          <div>
            <div class="text-h6">{{ $t('releases.generation') }} {{ detailRelease.managedGeneration }}</div>
            <div class="text-caption text-grey-7">{{ detailRelease.releaseId }} · {{ detailRelease.snapshotHash }}</div>
          </div>
          <StatusChip :value="detailRelease.status" />
        </q-card-section>
        <q-separator />
        <q-card-section>
          <q-markup-table flat dense>
            <tbody>
              <tr><td class="text-grey-7">{{ $t('releases.sourceDraft') }}</td><td>r{{ detailRelease.sourceDraftRevision }}</td></tr>
              <tr><td class="text-grey-7">{{ $t('releases.publishedAt') }}</td><td>{{ detailRelease.publishedAt }} {{ $t('releases.publishedBy') }} {{ detailRelease.publishedBy || '—' }}</td></tr>
            </tbody>
          </q-markup-table>
          <div class="text-subtitle2 q-mt-md">{{ $t('releases.diff') }}</div>
          <q-markup-table flat dense v-if="detailRelease.diffSummary.details?.length">
            <thead><tr><th>{{ $t('resources.relationship.kind') }}</th><th class="text-right">{{ $t('resources.review.added') }}</th><th class="text-right">{{ $t('resources.review.changed') }}</th><th class="text-right">{{ $t('resources.review.removed') }}</th></tr></thead>
            <tbody>
              <tr v-for="d in (detailRelease.diffSummary.details as ResourceDiff[])" :key="d.kind">
                <td>{{ d.kind }}</td>
                <td class="text-right text-positive">+{{ d.added }}</td>
                <td class="text-right text-warning">~{{ d.changed }}</td>
                <td class="text-right text-negative">-{{ d.removed }}</td>
              </tr>
            </tbody>
          </q-markup-table>
          <div v-else class="text-grey-7">{{ $t('common.noData') }}</div>

          <div class="text-subtitle2 q-mt-md">{{ $t('releases.activationHistory') }}</div>
          <div v-if="detailRelease.activationHistory.length">
            <q-timeline dense>
              <q-timeline-entry v-for="attempt in detailRelease.activationHistory" :key="attempt.activationId"
                :title="attempt.activationId" :subtitle="attempt.createdAt" :color="attempt.state === 'COMPLETED' ? 'positive' : attempt.state === 'FAILED' ? 'negative' : 'primary'">
                <div class="row items-center q-gutter-sm">
                  <StatusChip :value="attempt.state" />
                  <span v-if="attempt.errorCode" class="text-negative text-caption">{{ attempt.errorCode }}</span>
                </div>
              </q-timeline-entry>
            </q-timeline>
          </div>
          <div v-else class="text-grey-7">{{ $t('common.noData') }}</div>
        </q-card-section>
        <q-separator />
        <q-card-actions align="right">
          <q-btn flat :label="$t('common.close')" color="primary" v-close-popup />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </q-page>
</template>
