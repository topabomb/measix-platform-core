<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import type { components } from '../api/generated'
import { apiFetch, ApiProblem } from '../api/client'
import LoadingState from '../components/LoadingState.vue'
import ProblemBanner from '../components/ProblemBanner.vue'
import StatusChip from '../components/StatusChip.vue'
import { useActivationStore } from '../stores/activation'
import { useSessionStore } from '../stores/session'

type Upstream = components['schemas']['Upstream']
type UpstreamPage = components['schemas']['UpstreamPage']
type Activation = components['schemas']['Activation']

const session = useSessionStore()
const activation = useActivationStore()
const upstreams = ref<Upstream[]>([])
const nextCursor = ref<string>()
const selected = ref<Upstream>()
const loading = ref(false)
const error = ref<unknown>()
const createOpen = ref(false)
const detailOpen = ref(false)
const testing = ref(false)
const createForm = ref({ name: '', baseUrl: '', providerKind: 'OPENAI_COMPATIBLE' as string })
const canMutate = computed(() => Boolean(session.csrfToken))

async function refresh() {
  loading.value = true
  error.value = undefined
  try {
    const page = await apiFetch<UpstreamPage>('/api/admin/v1/upstreams?limit=200')
    upstreams.value = page.items
    nextCursor.value = page.nextCursor
  } catch (cause) {
    error.value = cause
  } finally {
    loading.value = false
  }
}

async function createUpstream() {
  if (!session.csrfToken) return
  error.value = undefined
  try {
    await apiFetch<Upstream>('/api/admin/v1/upstreams', {
      method: 'POST',
      body: JSON.stringify({ config: { name: createForm.value.name, baseUrl: createForm.value.baseUrl, providerKind: createForm.value.providerKind } }),
    }, session.csrfToken)
    createOpen.value = false
    createForm.value = { name: '', baseUrl: '', providerKind: 'OPENAI_COMPATIBLE' }
    await refresh()
  } catch (cause) {
    error.value = cause
  }
}

async function openUpstream(upstream: Upstream) {
  selected.value = upstream
  detailOpen.value = true
}

async function testUpstream() {
  if (!selected.value || !session.csrfToken) return
  testing.value = true
  error.value = undefined
  try {
    await apiFetch(`/api/admin/v1/upstreams/${encodeURIComponent(selected.value.upstreamId)}:test`, {
      method: 'POST',
    }, session.csrfToken)
  } catch (cause) {
    error.value = cause
  } finally {
    testing.value = false
  }
}

async function applyUpstream() {
  if (!selected.value || !session.csrfToken) return
  if (!window.confirm(`Apply upstream ${selected.value.name} (revision ${selected.value.configRevision}) to runtime?`)) return
  activation.resetCommand()
  const key = activation.beginCommand('RUNTIME_CONFIG')
  error.value = undefined
  try {
    const result = await apiFetch<Activation>(
      `/api/admin/v1/upstreams/${encodeURIComponent(selected.value.upstreamId)}:apply`,
      { method: 'POST', headers: { 'Idempotency-Key': key } },
      session.csrfToken,
    )
    activation.accept(result)
    if (result.state === 'APPLYING' || result.state === 'UNKNOWN') {
      await activation.poll(result.activationId)
    }
    await refresh()
    if (selected.value) {
      const updated = upstreams.value.find((u) => u.upstreamId === selected.value?.upstreamId)
      if (updated) selected.value = updated
    }
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
        <div class="text-h5 text-weight-bold">Upstreams</div>
        <div class="text-body2 text-grey-7">Model provider upstreams, configuration revisions and runtime activation.</div>
      </div>
      <div class="q-gutter-sm">
        <q-btn flat icon="refresh" @click="refresh" />
        <q-btn color="primary" icon="cloud_queue" label="Create upstream" :disable="!canMutate" @click="createOpen = true" />
      </div>
    </div>
    <ProblemBanner :error="error" class="q-mb-md" />
    <q-banner v-if="activation.activation" :class="activation.succeeded ? 'bg-green-1' : 'bg-orange-1'" class="q-mb-md rounded-borders">
      <div class="row items-center justify-between">
        <span>Activation {{ activation.activation.activationId }} ({{ activation.activation.kind }})</span>
        <StatusChip :value="activation.activation.state" />
      </div>
      <div v-if="activation.activation.errorCode" class="text-caption">{{ activation.activation.errorCode }}</div>
    </q-banner>
    <LoadingState v-if="loading && !upstreams.length" />
    <q-card v-else flat bordered>
      <q-list separator>
        <q-item v-for="upstream in upstreams" :key="upstream.upstreamId" clickable @click="openUpstream(upstream)">
          <q-item-section>
            <q-item-label>{{ upstream.name }}</q-item-label>
            <q-item-label caption>{{ upstream.upstreamId }} · revision {{ upstream.configRevision }}<span v-if="upstream.activeConfigRevision"> · active {{ upstream.activeConfigRevision }}</span></q-item-label>
          </q-item-section>
          <q-item-section side><StatusChip :value="upstream.status" /></q-item-section>
        </q-item>
        <q-item v-if="!upstreams.length"><q-item-section class="text-grey-7">No upstreams configured.</q-item-section></q-item>
      </q-list>
    </q-card>

    <q-dialog v-model="createOpen">
      <q-card style="min-width: 460px">
        <q-card-section class="text-h6">Create upstream</q-card-section>
        <q-card-section class="q-gutter-md">
          <q-input v-model="createForm.name" outlined label="Name" />
          <q-input v-model="createForm.baseUrl" outlined label="Base URL" placeholder="https://api.example.com" />
          <q-select v-model="createForm.providerKind" outlined label="Provider kind" :options="['OPENAI_COMPATIBLE','ANTHROPIC_COMPATIBLE','CUSTOM']" />
        </q-card-section>
        <q-card-actions align="right">
          <q-btn flat label="Cancel" v-close-popup />
          <q-btn color="primary" label="Create" :disable="!createForm.name.trim() || !createForm.baseUrl.trim()" @click="createUpstream" />
        </q-card-actions>
      </q-card>
    </q-dialog>

    <q-dialog v-model="detailOpen">
      <q-card v-if="selected" style="width: 680px; max-width: 95vw">
        <q-card-section class="row items-start justify-between">
          <div>
            <div class="text-h6">{{ selected.name }}</div>
            <div class="text-caption">{{ selected.upstreamId }}</div>
          </div>
          <StatusChip :value="selected.status" />
        </q-card-section>
        <q-separator />
        <q-card-section>
          <div class="row q-gutter-sm q-mb-md">
            <q-btn outline color="primary" label="Test connection" :loading="testing" @click="testUpstream" />
            <q-btn outline color="positive" label="Apply to runtime" @click="applyUpstream" />
          </div>
          <q-markup-table flat dense>
            <tbody>
              <tr><td class="text-grey-7">Config revision</td><td>{{ selected.configRevision }}</td></tr>
              <tr><td class="text-grey-7">Active revision</td><td>{{ selected.activeConfigRevision ?? '—' }}</td></tr>
              <tr v-if="selected.config"><td class="text-grey-7">Base URL</td><td>{{ selected.config.baseUrl ?? '—' }}</td></tr>
              <tr v-if="selected.config"><td class="text-grey-7">Provider kind</td><td>{{ selected.config.providerKind ?? '—' }}</td></tr>
            </tbody>
          </q-markup-table>
        </q-card-section>
        <q-card-actions align="right"><q-btn flat label="Close" v-close-popup /></q-card-actions>
      </q-card>
    </q-dialog>
  </q-page>
</template>
