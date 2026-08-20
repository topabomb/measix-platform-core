<script setup lang="ts">
import { onMounted, ref } from 'vue'
import type { components } from '../api/generated'
import { apiFetch } from '../api/client'
import LoadingState from '../components/LoadingState.vue'
import ProblemBanner from '../components/ProblemBanner.vue'
import StatusChip from '../components/StatusChip.vue'
import { useActivationStore } from '../stores/activation'
import { useSessionStore } from '../stores/session'

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
  if (!window.confirm(`Republish release ${release.releaseId} (generation ${release.managedGeneration}) as a new runtime generation?`)) return
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
      await activation.poll(result.activationId)
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
  if (diff.added) parts.push(`+${diff.added} added`)
  if (diff.changed) parts.push(`~${diff.changed} changed`)
  if (diff.removed) parts.push(`-${diff.removed} removed`)
  return parts.length ? parts.join(' · ') : 'no changes'
}

onMounted(refresh)
</script>

<template>
  <q-page padding>
    <div class="row items-center justify-between q-mb-lg">
      <div>
        <div class="text-h5 text-weight-bold">Releases</div>
        <div class="text-body2 text-grey-7">Immutable staged releases and republish history.</div>
      </div>
      <q-btn flat icon="refresh" @click="refresh" />
    </div>
    <ProblemBanner :error="error" class="q-mb-md" />
    <q-banner v-if="activation.activation" :class="activation.succeeded ? 'bg-green-1' : 'bg-orange-1'" class="q-mb-md rounded-borders">
      <div class="row items-center justify-between">
        <span>Activation {{ activation.activation.activationId }} ({{ activation.activation.kind }})</span>
        <StatusChip :value="activation.activation.state" />
      </div>
    </q-banner>
    <LoadingState v-if="loading && !releases.length" />
    <q-card v-else flat bordered>
      <q-list separator>
        <q-item v-for="release in releases" :key="release.releaseId" clickable @click="showDetail(release)">
          <q-item-section>
            <q-item-label>Generation {{ release.managedGeneration }} <span class="text-grey-7 text-caption">· draft r{{ release.sourceDraftRevision }}</span></q-item-label>
            <q-item-label caption>
              {{ diffText(release.diffSummary) }} · published {{ release.publishedAt }} by {{ release.publishedBy || '—' }}
            </q-item-label>
          </q-item-section>
          <q-item-section side>
            <div class="row items-center q-gutter-sm">
              <StatusChip :value="release.status" />
              <q-btn outline color="primary" label="Republish" size="sm" stop @click.stop="republish(release)" />
            </div>
          </q-item-section>
        </q-item>
        <q-item v-if="!releases.length"><q-item-section class="text-grey-7">No releases published.</q-item-section></q-item>
      </q-list>
    </q-card>

    <q-dialog v-model="detailOpen">
      <q-card v-if="detailRelease" style="min-width: 480px; max-width: 640px">
        <q-card-section class="row items-center justify-between">
          <div>
            <div class="text-h6">Generation {{ detailRelease.managedGeneration }}</div>
            <div class="text-caption text-grey-7">{{ detailRelease.releaseId }} · {{ detailRelease.snapshotHash }}</div>
          </div>
          <StatusChip :value="detailRelease.status" />
        </q-card-section>
        <q-separator />
        <q-card-section>
          <q-markup-table flat dense>
            <tbody>
              <tr><td class="text-grey-7">Source draft revision</td><td>r{{ detailRelease.sourceDraftRevision }}</td></tr>
              <tr><td class="text-grey-7">Published</td><td>{{ detailRelease.publishedAt }} by {{ detailRelease.publishedBy || '—' }}</td></tr>
            </tbody>
          </q-markup-table>
          <div class="text-subtitle2 q-mt-md">Diff summary</div>
          <q-markup-table flat dense v-if="detailRelease.diffSummary.details?.length">
            <thead><tr><th>Kind</th><th class="text-right">Added</th><th class="text-right">Changed</th><th class="text-right">Removed</th></tr></thead>
            <tbody>
              <tr v-for="d in (detailRelease.diffSummary.details as ResourceDiff[])" :key="d.kind">
                <td>{{ d.kind }}</td>
                <td class="text-right text-positive">+{{ d.added }}</td>
                <td class="text-right text-warning">~{{ d.changed }}</td>
                <td class="text-right text-negative">-{{ d.removed }}</td>
              </tr>
            </tbody>
          </q-markup-table>
          <div v-else class="text-grey-7">No resource changes.</div>

          <div class="text-subtitle2 q-mt-md">Activation history</div>
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
          <div v-else class="text-grey-7">No activation attempts recorded.</div>
        </q-card-section>
        <q-separator />
        <q-card-actions align="right">
          <q-btn flat label="Close" color="primary" v-close-popup />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </q-page>
</template>
