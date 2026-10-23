<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { QSelect, QDialog } from 'quasar'
import type { components } from '../api/generated'
import { apiFetch, createIdempotencyKey, commandResultUncertain } from '../api/client'
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
const pending = ref<{ payload: string; key: string }>()
const confirming = ref(false)
const controller = new AbortController()
watch(() => dav.value?.desiredAccessMode, mode => { if (mode && !edited.value) selected.value = mode }, { immediate: true })
const canSave = computed(() => !props.disabled && !saving.value && !!dav.value?.desiredBindingRevision && (!!pending.value || selected.value !== dav.value?.desiredAccessMode))
function askSave() {
  if (!canSave.value) return
  if (!pending.value && selected.value === 'READ_ONLY' && (dav.value?.desiredAccessMode === 'FULL' || dav.value?.lastConfirmedAccessMode === 'FULL')) confirming.value = true
  else void save()
}
async function save() {
  confirming.value = false
  if (!canSave.value || !session.csrfToken) return
  saving.value = true; error.value = undefined
  const body: components['schemas']['WorkspaceCommand'] = { action: 'SET_DAV_ACCESS', expectedRevision: props.workspace.bindingRevision, expectedDAVRevision: dav.value!.desiredBindingRevision!, davAccessMode: selected.value }
  const payload = JSON.stringify(body)
  pending.value ??= { payload, key: createIdempotencyKey() }
  try {
    await apiFetch(props.baseUrl, { method: 'POST', headers: { 'Idempotency-Key': pending.value.key }, body: pending.value.payload, signal: controller.signal }, session.csrfToken)
    if (controller.signal.aborted) return
    pending.value = undefined; edited.value = false; emit('changed')
  } catch (cause) {
    if (!controller.signal.aborted) {
      error.value = cause
      if (!commandResultUncertain(cause)) { pending.value = undefined; emit('changed') }
    }
  }
  finally { saving.value = false }
}
onBeforeUnmount(() => controller.abort())
</script>
<template>
  <section v-if="dav" class="q-pt-sm" data-cy="workspace-dav-access">
    
    <div role="status" class="text-caption q-my-sm">{{ dav.available ? `当前生效：${label(dav.accessMode)}` : workspaceLabel(dav.reason) }}<span v-if="!dav.available && dav.lastConfirmedAccessMode"> · 上次确认：{{ label(dav.lastConfirmedAccessMode) }}</span></div>
    <div v-if="dav.desiredAccessMode" class="row items-start q-col-gutter-sm">
      <div class="col-12 col-sm"><QSelect :model-value="selected" :options="options" emit-value map-options outlined dense label="WebDAV 访问权限" :disable="disabled || saving || !!pending" @update:model-value="mode => { selected = mode; edited = true }" /></div>
      <div class="col-auto"><q-btn outline color="primary" :label="pending && error ? '核实原操作' : '应用权限'" :loading="saving" :disable="!canSave" @click="askSave" /></div>
    </div>
    <p class="text-caption text-grey-7 q-mt-sm q-mb-none">仅限客户端 WebDAV，不改变 Console 文件管理权限。</p>
    <ProblemBanner :error="error" />
    <p v-if="pending && error" class="text-caption">请求结果尚未确认；核实将重用原操作，不提交新的权限变更。</p>
    <QDialog v-model="confirming"><q-card class="app-dialog app-dialog--sm"><q-card-section class="text-subtitle1 text-weight-medium">将 WebDAV 权限降为只读</q-card-section><q-card-section class="app-dialog__body">将中止旧 WebDAV 连接；已转发的写入可能已生效，不能回滚。Console 文件管理权限不变。</q-card-section><q-card-actions align="right"><q-btn flat label="取消" v-close-popup/><q-btn color="primary" label="确认降权" data-cy="confirm-dav-downgrade" :disable="!canSave" @click="save"/></q-card-actions></q-card></QDialog>
  </section>
</template>
