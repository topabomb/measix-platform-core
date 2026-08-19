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

const session = useSessionStore()
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
        <q-item v-for="release in releases" :key="release.releaseId">
          <q-item-section>
            <q-item-label>Generation {{ release.managedGeneration }}</q-item-label>
            <q-item-label caption>{{ release.releaseId }} · {{ release.snapshotHash }} · {{ release.createdAt }}</q-item-label>
          </q-item-section>
          <q-item-section side>
            <div class="row items-center q-gutter-sm">
              <StatusChip :value="release.status" />
              <q-btn outline color="primary" label="Republish" size="sm" @click="republish(release)" />
            </div>
          </q-item-section>
        </q-item>
        <q-item v-if="!releases.length"><q-item-section class="text-grey-7">No releases published.</q-item-section></q-item>
      </q-list>
    </q-card>
  </q-page>
</template>
