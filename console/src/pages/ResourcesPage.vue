<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import type { components } from '../api/generated'
import { apiFetch } from '../api/client'
import { useDraftStore } from '../stores/draft'
import { useSessionStore } from '../stores/session'
import { useActivationStore } from '../stores/activation'
import LoadingState from '../components/LoadingState.vue'
import ProblemBanner from '../components/ProblemBanner.vue'
import StatusChip from '../components/StatusChip.vue'

type Activation = components['schemas']['Activation']

const draft = useDraftStore()
const session = useSessionStore()
const activation = useActivationStore()
const error = ref<unknown>()
const publishing = ref(false)
const canMutate = computed(() => Boolean(session.csrfToken))

async function refresh() {
  error.value = undefined
  try { await draft.load() } catch (cause) { error.value = cause }
}

async function save() {
  if (!session.csrfToken) return
  error.value = undefined
  try { await draft.save(session.csrfToken) } catch (cause) { error.value = cause }
}

async function validate() {
  if (!session.csrfToken) return
  error.value = undefined
  try { await draft.validate(session.csrfToken) } catch (cause) { error.value = cause }
}

async function publish() {
  if (!session.csrfToken) return
  if (!window.confirm('Publish current draft as a new Release? This creates an immutable staged release.')) return
  publishing.value = true
  activation.resetCommand()
  const key = activation.beginCommand('PUBLISH')
  error.value = undefined
  try {
    const result = await apiFetch<Activation>('/api/admin/v1/draft:publish', {
      method: 'POST',
      headers: { 'Idempotency-Key': key },
    }, session.csrfToken)
    activation.accept(result)
    if (result.state === 'APPLYING' || result.state === 'UNKNOWN') {
      await activation.poll(result.activationId)
    }
    await refresh()
  } catch (cause) {
    error.value = cause
  } finally {
    publishing.value = false
  }
}

onMounted(refresh)
</script>

<template>
  <q-page padding>
    <div class="row items-center justify-between q-mb-lg">
      <div>
        <div class="text-h5 text-weight-bold">Draft</div>
        <div class="text-body2 text-grey-7">Edit managed configuration before publishing as an immutable release.</div>
      </div>
      <div class="q-gutter-sm">
        <q-btn flat icon="refresh" @click="refresh" />
        <q-btn outline color="primary" label="Validate" :disable="!canMutate || draft.loading" @click="validate" />
        <q-btn outline color="primary" label="Save" :disable="!canMutate || !draft.dirty" :loading="draft.saving" @click="save" />
        <q-btn color="positive" icon="rocket_launch" label="Publish" :disable="!canMutate" :loading="publishing" @click="publish" />
      </div>
    </div>
    <ProblemBanner :error="error" class="q-mb-md" />
    <q-banner v-if="draft.conflictRevision !== undefined" class="bg-orange-1 q-mb-md rounded-borders">
      <div class="text-weight-medium">Stale draft (revision {{ draft.baselineRevision }})</div>
      <div class="text-body2">Server is at revision {{ draft.conflictRevision }}. Reload to pick up the latest changes.</div>
      <template #action><q-btn flat label="Reload" @click="refresh" /></template>
    </q-banner>
    <q-banner v-if="activation.activation" :class="activation.succeeded ? 'bg-green-1' : 'bg-orange-1'" class="q-mb-md rounded-borders">
      <div class="row items-center justify-between">
        <span>Activation {{ activation.activation.activationId }} ({{ activation.activation.kind }})</span>
        <StatusChip :value="activation.activation.state" />
      </div>
    </q-banner>

    <LoadingState v-if="draft.loading" />
    <template v-else-if="draft.localContent">
      <q-card flat bordered class="q-mb-md">
        <q-card-section>
          <div class="row items-center justify-between">
            <div class="text-subtitle1">Revision {{ draft.baselineRevision }}<q-badge v-if="draft.dirty" color="orange" label="unsaved" class="q-ml-sm" /></div>
          </div>
        </q-card-section>
      </q-card>

      <div class="row q-gutter-md">
        <q-card flat bordered class="col">
          <q-card-section>
            <div class="row items-center justify-between">
              <div class="text-subtitle2">Providers</div>
              <q-btn flat dense icon="add" label="Add model" size="sm" @click="draft.addModel('prv_placeholder'); draft.markDirty()" />
            </div>
            <q-list dense>
              <q-item v-for="(model, idx) in draft.localContent.models" :key="model.modelId">
                <q-item-section>
                  <q-input v-model="model.displayName" dense outlined label="Name" @update:model-value="draft.markDirty()" />
                  <q-input v-model="model.upstreamModelKey" dense outlined label="Upstream model key" class="q-mt-xs" @update:model-value="draft.markDirty()" />
                  <div class="text-caption text-grey-7">{{ model.modelId }} · {{ model.providerId }}</div>
                </q-item-section>
                <q-item-section side>
                  <q-toggle v-model="model.enabled" @update:model-value="draft.markDirty()" />
                  <q-btn flat dense color="negative" icon="delete" size="sm" @click="draft.localContent!.models.splice(idx, 1); draft.markDirty()" />
                </q-item-section>
              </q-item>
              <q-item v-if="!draft.localContent.models.length"><q-item-section class="text-grey-7">No models.</q-item-section></q-item>
            </q-list>
          </q-card-section>
        </q-card>

        <q-card flat bordered class="col">
          <q-card-section>
            <div class="text-subtitle2">Validation</div>
            <div v-if="!draft.validationResult" class="text-body2 text-grey-7 q-mt-sm">Click Validate to check the draft.</div>
            <div v-else>
              <q-banner v-if="draft.validationResult.errors.length === 0 && draft.validationResult.warnings.length === 0" class="bg-green-1 q-mt-sm rounded-borders">
                Draft is valid.
              </q-banner>
              <q-banner v-else class="bg-orange-1 q-mt-sm rounded-borders">
                <div>{{ draft.validationResult.errors.length }} errors · {{ draft.validationResult.warnings.length }} warnings</div>
              </q-banner>
              <q-list dense class="q-mt-sm">
                <q-item v-for="e in draft.validationResult.errors" :key="e.path + e.code">
                  <q-item-section avatar><q-icon name="error" color="negative" /></q-item-section>
                  <q-item-section>{{ e.path }}: {{ e.code }} — {{ e.detail }}</q-item-section>
                </q-item>
                <q-item v-for="w in draft.validationResult.warnings" :key="w.path + w.code">
                  <q-item-section avatar><q-icon name="warning" color="amber-8" /></q-item-section>
                  <q-item-section>{{ w.path }}: {{ w.code }} — {{ w.detail }}</q-item-section>
                </q-item>
              </q-list>
            </div>
          </q-card-section>
        </q-card>
      </div>
    </template>
    <div v-else class="text-body2 text-grey-7">No draft loaded.</div>
  </q-page>
</template>
