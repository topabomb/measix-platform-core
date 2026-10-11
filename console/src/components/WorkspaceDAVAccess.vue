<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { QSelect } from 'quasar'
import type { components } from '../api/generated'
import { apiFetch, createIdempotencyKey } from '../api/client'
import { useSessionStore } from '../stores/session'
import { workspaceLabel } from '../composables/workspaceLabels'
import ProblemBanner from './ProblemBanner.vue'

type Mode = components['schemas']['DAVAccessMode']
const props = defineProps<{ workspace: components['schemas']['WorkspaceProjection']; baseUrl: string; disabled: boolean }>()
const emit = defineEmits<{ changed: [] }>()
const session = useSessionStore()
const selected = ref<Mode>('FULL'), edited = ref(false), saving = ref(false), error = ref<unknown>()
const dav = computed(() => props.workspace.webdav)
const options = [{ label: '只读', value: 'READ_ONLY' }, { label: '完整', value: 'FULL' }]
const label = (mode?: Mode) => mode === 'READ_ONLY' ? '只读' : mode === 'FULL' ? '完整' : '尚未确认'
let pending: { payload: string; key: string } | undefined
const controller = new AbortController()
watch(() => dav.value?.desiredAccessMode, mode => { if (mode && !edited.value) selected.value = mode }, { immediate: true })
const canSave = computed(() => !props.disabled && !saving.value && !!dav.value?.desiredBindingRevision && selected.value !== dav.value?.desiredAccessMode)
async function save() {
  if (!canSave.value || !session.csrfToken) return
  saving.value = true; error.value = undefined
  const body: components['schemas']['WorkspaceCommand'] = { action: 'SET_DAV_ACCESS', expectedRevision: props.workspace.bindingRevision, expectedDAVRevision: dav.value!.desiredBindingRevision!, davAccessMode: selected.value }
  const payload = JSON.stringify(body)
  if (pending?.payload !== payload) pending = { payload, key: createIdempotencyKey() }
  try {
    await apiFetch(props.baseUrl, { method: 'POST', headers: { 'Idempotency-Key': pending.key }, body: payload, signal: controller.signal }, session.csrfToken)
    if (controller.signal.aborted) return
    pending = undefined; edited.value = false; emit('changed')
  } catch (cause) { if (!controller.signal.aborted) error.value = cause }
  finally { saving.value = false }
}
onBeforeUnmount(() => controller.abort())
</script>
<template>
  <section v-if="dav" class="q-pt-sm" data-cy="workspace-dav-access">
    
    <div role="status" class="text-caption q-my-sm">{{ dav.available ? `当前生效：${label(dav.accessMode)}` : workspaceLabel(dav.reason) }}<span v-if="!dav.available && dav.lastConfirmedAccessMode"> · 上次确认：{{ label(dav.lastConfirmedAccessMode) }}</span></div>
    <div v-if="dav.desiredAccessMode" class="row items-start q-col-gutter-sm">
      <div class="col-12 col-sm"><QSelect :model-value="selected" :options="options" emit-value map-options outlined dense label="WebDAV 访问权限" :disable="disabled || saving" @update:model-value="mode => { selected = mode; edited = true }" /></div>
      <div class="col-auto"><q-btn outline color="primary" label="应用权限" :loading="saving" :disable="!canSave" @click="save" /></div>
    </div>
    <p class="text-caption text-grey-7 q-mt-sm q-mb-none">不改变 Console 文件管理权限。只读允许浏览和下载；降权会中止旧连接，已转发的写入可能已生效。</p>
    <ProblemBanner :error="error" />
  </section>
</template>
