<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { apiFetch, createIdempotencyKey } from '../api/client'
import type { components } from '../api/generated'
import { useSessionStore } from '../stores/session'
import { useRemoteWorkspaceStore, type WorkspaceService } from '../stores/remoteWorkspace'
import PageHeader from '../components/PageHeader.vue'
import ProblemBanner from '../components/ProblemBanner.vue'
import PagedEntityPicker from '../components/PagedEntityPicker.vue'
import WorkspacePanel from '../components/WorkspacePanel.vue'
import { workspaceLabel } from '../composables/workspaceLabels'

type Operation = components['schemas']['WorkspaceOperation']
const session = useSessionStore(), remoteWorkspace = useRemoteWorkspaceStore(), router = useRouter()
const current = computed(() => remoteWorkspace.current)
const desiredEnabled = ref(false), saving = ref(false), error = ref<unknown>(), notice = ref('')
const name = ref('远程工作区'), adminOrigin = ref(''), mcpOrigin = ref(''), davOrigin = ref(''), bearer = ref('')
const sameDeployment = ref(false), confirmDisable = ref(false)
const selectedUser = ref<string>(), userLabel = ref('')
const workspaces = ref<components['schemas']['WorkspaceList']>({ items: [] })
const savedSecret = ref<components['schemas']['SecretRef']>()
const operation = ref<Operation>(), check = ref<components['schemas']['WorkspaceServiceCheck']>()
let alive = true, timer: ReturnType<typeof setTimeout> | undefined
let pending: { payload: string; key: string } | undefined
const running = computed(() => !!operation.value && ['PENDING', 'RUNNING'].includes(operation.value.state))
const blocked = computed(() => !!operation.value && ['UNKNOWN', 'NEEDS_ATTENTION'].includes(operation.value.state))
const busy = computed(() => saving.value || running.value || remoteWorkspace.loading)
const originChanged = computed(() => !!current.value && (adminOrigin.value.trim() !== current.value.config.adminOrigin || (mcpOrigin.value.trim() || adminOrigin.value.trim()) !== current.value.config.mcpOrigin || davOrigin.value.trim() !== (current.value.config.davOrigin ?? '')))
const hasChanges = computed(() => desiredEnabled.value !== !!current.value?.enabled || name.value.trim() !== current.value?.name || originChanged.value || !!bearer.value || !!savedSecret.value || current.value?.activeConfigRevision !== current.value?.configRevision)
const canSave = computed(() => !busy.value && !blocked.value && (desiredEnabled.value
  ? !!adminOrigin.value.trim() && (!originChanged.value || sameDeployment.value) && (!!current.value || !!bearer.value || !!savedSecret.value) && (hasChanges.value || !remoteWorkspace.enabled)
  : !!current.value?.enabled))
const status = computed(() => !current.value ? '尚未配置' : current.value.state === 'ACTIVE' ? '已启用' : current.value.state === 'SAVED' ? '配置已保存，未启用' : workspaceLabel(current.value.state))

function keyFor(payload: string) {
  if (pending?.payload !== payload) pending = { payload, key: createIdempotencyKey() }
  return pending.key
}
function fillForm() {
  const value = current.value
  desiredEnabled.value = !!value?.enabled
  if (!value) return
  name.value = value.name; adminOrigin.value = value.config.adminOrigin
  mcpOrigin.value = value.config.mcpOrigin; davOrigin.value = value.config.davOrigin ?? ''
  sameDeployment.value = false
}
async function loadSpaces(cursor?: string) {
  if (!current.value) return
  try {
    const result = await apiFetch<components['schemas']['WorkspaceList']>(`/api/admin/v1/remote-workspace/services/${current.value.workspaceServiceId}/workspaces${cursor ? '?cursor=' + encodeURIComponent(cursor) : ''}`)
    if (alive) workspaces.value = result
  } catch (cause) { if (alive) error.value = cause }
}
async function refresh() {
  await remoteWorkspace.load()
  if (!alive) return
  fillForm()
  await loadSpaces()
  if (current.value?.operationId) {
    operation.value = await apiFetch<Operation>('/api/admin/v1/workspace-operations/' + current.value.operationId)
    if (alive && running.value) poll()
  }
}
function requestSave() {
  if (!desiredEnabled.value && current.value?.enabled) confirmDisable.value = true
  else void save()
}
async function save() {
  if (!session.csrfToken) return
  saving.value = true; error.value = undefined; notice.value = ''; confirmDisable.value = false
  try {
    let value = current.value
    if (desiredEnabled.value) {
      let secret = savedSecret.value ?? value?.config.managementSecret
      if (bearer.value) {
        const created = await apiFetch<components['schemas']['Secret']>('/api/admin/v1/secrets', { method: 'POST', body: JSON.stringify({ name: 'Remote workspace management', value: bearer.value }) }, session.csrfToken)
        secret = { secretId: created.secretId, secretVersion: created.secretVersion }
        savedSecret.value = secret; bearer.value = ''
      }
      if (!secret) throw new Error('请输入工作区服务的管理凭据。')
      const config: components['schemas']['AgentSpaceConfig'] = {
        adminOrigin: adminOrigin.value.trim().replace(/\/$/, ''),
        mcpOrigin: (mcpOrigin.value.trim() || adminOrigin.value.trim()).replace(/\/$/, ''),
        ...(davOrigin.value.trim() ? { davOrigin: davOrigin.value.trim().replace(/\/$/, '') } : {}),
        managementSecret: secret,
        connectTimeoutMs: value?.config.connectTimeoutMs ?? 90000, idleTimeoutMs: value?.config.idleTimeoutMs ?? 120000,
      }
      if (!value || name.value.trim() !== value.name || originChanged.value || secret.secretId !== value.config.managementSecret.secretId || secret.secretVersion !== value.config.managementSecret.secretVersion) {
        const body = { expectedRevision: value?.configRevision ?? 0, name: name.value.trim() || '远程工作区', confirmSameDeployment: sameDeployment.value, config }
        const payload = JSON.stringify(body)
        value = await apiFetch<WorkspaceService>('/api/admin/v1/remote-workspace/services' + (value ? '/' + value.workspaceServiceId : ''), {
          method: value ? 'PUT' : 'POST', headers: { 'Idempotency-Key': keyFor(payload) }, body: payload,
        }, session.csrfToken)
        remoteWorkspace.current = value; savedSecret.value = undefined; pending = undefined
      }
    }
    if (!value) return
    const action = desiredEnabled.value ? 'apply' : 'disable'
    const path = `/api/admin/v1/remote-workspace/services/${value.workspaceServiceId}/${action}`
    operation.value = await apiFetch<Operation>(path, { method: 'POST', headers: { 'Idempotency-Key': keyFor(path + '/' + value.configRevision) } }, session.csrfToken)
    pending = undefined; check.value = undefined
    notice.value = desiredEnabled.value ? '配置已保存，正在启用服务并核对已有空间。' : '正在关闭访问并停止工作区，文件会保留。'
    await remoteWorkspace.load()
    if (alive) poll()
  } catch (cause) { if (alive) error.value = cause }
  finally { saving.value = false }
}
function poll() {
  if (timer) clearTimeout(timer)
  timer = setTimeout(() => { void refreshOperation() }, 1000)
}
async function refreshOperation() {
  if (!alive || !operation.value) return
  try {
    const result = await apiFetch<Operation>('/api/admin/v1/workspace-operations/' + operation.value.operationId)
    if (!alive) return
    operation.value = result
    if (running.value) { poll(); return }
    await remoteWorkspace.load(); await loadSpaces()
    if (result.state === 'COMPLETED' && result.step !== 'CHECK_FAILED') {
      fillForm()
      notice.value = remoteWorkspace.enabled ? '已启用。现在可以开通用户工作区；MCP 工具入口可稍后配置。' : '已关闭。已有空间和文件保留，重新启用后可恢复连接。'
    } else notice.value = result.step === 'CHECK_FAILED' ? '配置已保存，但启用检查未通过。请修正连接配置后再次保存。' : '操作尚未确认完成，请查看诊断并核实远端状态。'
  } catch (cause) { if (alive) error.value = cause }
}
async function checkConnection() {
  if (!current.value || !session.csrfToken) return
  saving.value = true; error.value = undefined
  try { check.value = await apiFetch(`/api/admin/v1/remote-workspace/services/${current.value.workspaceServiceId}/check`, { method: 'POST' }, session.csrfToken) }
  catch (cause) { error.value = cause }
  finally { saving.value = false }
}
async function users(query: string, cursor?: string) {
  const params = new URLSearchParams({ limit: '25', query }); if (cursor) params.set('cursor', cursor)
  const page = await apiFetch<components['schemas']['UserPage']>('/api/admin/v1/users?' + params)
  return { items: page.items.map(user => ({ value: user.userId, label: user.displayName, caption: user.username, status: user.status })), nextCursor: page.nextCursor }
}
onMounted(() => { void refresh().catch(cause => { error.value = cause }) })
onBeforeUnmount(() => { alive = false; bearer.value = ''; if (timer) clearTimeout(timer) })
</script>

<template>
  <q-page padding>
    <PageHeader title="远程工作区" subtitle="连接工作区服务，为用户提供独立空间、文件管理和可选的助手工具。" />
    <ProblemBanner :error="error || remoteWorkspace.error" />
    <q-banner v-if="notice" class="bg-blue-1 q-mb-md" data-cy="workspace-save-notice">{{ notice }}</q-banner>
    <div class="remote-workspace-grid">
      <q-card flat bordered>
        <q-card-section>
          <div class="row items-center justify-between q-gutter-sm"><div class="text-h6">服务连接</div><q-badge :color="remoteWorkspace.enabled ? 'positive' : 'grey-7'">{{ status }}</q-badge></div>
          <q-toggle v-model="desiredEnabled" label="启用远程工作区" :disable="busy || blocked" class="q-mt-sm" />
          <p class="text-caption text-grey-7 q-mb-none">开关在保存后生效。关闭会停止工作区并撤销工具和文件访问，保留空间及文件。</p>
        </q-card-section>
        <template v-if="desiredEnabled || current">
          <q-separator />
          <q-card-section class="workspace-connection-fields">
            <q-input v-model="name" outlined dense label="连接名称" :disable="busy || !desiredEnabled" />
            <q-input v-model="adminOrigin" outlined dense label="管理服务地址" placeholder="https://space.example.com" :disable="busy || !desiredEnabled" />
            <q-input v-model="davOrigin" outlined dense label="文件服务地址（可选）" hint="须与 Agent Space 的 WebDAV 对外地址一致；非回环地址须使用 HTTPS。留空只使用工具能力。" :disable="busy || !desiredEnabled" />
            <q-input v-model="bearer" outlined dense type="password" autocomplete="new-password" :label="current ? '管理凭据（留空保留原值）' : '管理凭据'" :disable="busy || !desiredEnabled" />
            <details>
              <summary class="cursor-pointer text-primary">工具服务地址（可选）</summary>
              <q-input v-model="mcpOrigin" outlined dense label="MCP 服务地址" hint="留空使用管理服务地址。填写地址不等于发布 MCP 工具。" class="q-mt-sm" :disable="busy || !desiredEnabled" />
            </details>
            <q-checkbox v-if="originChanged" v-model="sameDeployment" dense label="新地址仍指向原服务和原有空间" :disable="busy || !desiredEnabled" />
          </q-card-section>
        </template>
        <q-card-actions class="q-pa-md q-pt-none">
          <q-btn color="primary" label="保存配置" :loading="saving || running" :disable="!canSave" @click="requestSave" />
          <q-btn v-if="current" flat label="检查已保存连接" :disable="busy || hasChanges" @click="checkConnection" />
        </q-card-actions>
        <q-card-section v-if="operation || check" class="q-pt-none">
          <q-banner v-if="check" dense :class="check.managementReady ? 'bg-green-1' : 'bg-orange-1'">管理连接：{{ check.managementReady ? '可用' : '未确认' }}<div v-if="check.diagnosticCode">{{ workspaceLabel(check.diagnosticCode) }}</div></q-banner>
          <q-banner v-if="operation && (running || blocked || operation.step === 'CHECK_FAILED')" dense class="bg-blue-1">{{ workspaceLabel(operation.state) }}<div>{{ workspaceLabel(operation.diagnosticCode) }}</div><q-btn flat dense label="刷新进度" @click="refreshOperation" /></q-banner>
          <details v-if="current" class="text-caption q-mt-sm"><summary>连接诊断</summary>配置版本 {{ current.configRevision }} · 生效版本 {{ current.activeConfigRevision ?? '无' }}<div v-if="operation">{{ operation.operationId }} · {{ operation.step }}</div></details>
        </q-card-section>
      </q-card>
      <div>
        <q-card flat bordered class="q-mb-md">
          <q-card-section>
            <div class="row items-center justify-between q-mb-sm"><div class="text-h6">用户工作区</div><q-btn flat round dense icon="refresh" aria-label="刷新空间列表" :disable="busy" @click="loadSpaces()" /></div>
            <p v-if="!remoteWorkspace.enabled" class="text-grey-7">先启用并保存左侧服务连接，再为用户开通空间。已有空间会保留在此，便于检查进度和清理。</p>
            <template v-else><p class="text-caption text-grey-7">选择有效用户，开通独立空间后即可浏览文件，无需先配置 MCP。</p><PagedEntityPicker v-model="selectedUser" label="选择企业用户" empty-label="选择用户查看或开通" :fetch-page="users" :selected-option="selectedUser && userLabel ? { value: selectedUser, label: userLabel } : undefined" @selected="option => userLabel = option?.label ?? ''" /><q-btn flat dense color="primary" label="管理用户" class="q-mt-sm" @click="router.push('/users')" /></template>
            <q-list v-if="!selectedUser || !remoteWorkspace.enabled" separator>
              <q-item v-for="row in workspaces.items" :key="row.userId" clickable @click="selectedUser = row.userId; userLabel = row.displayName"><q-item-section>{{ row.displayName }}<q-item-label caption>{{ workspaceLabel(row.workspace.state) }}</q-item-label></q-item-section><q-item-section side><q-icon name="chevron_right" /></q-item-section></q-item>
              <q-item v-if="!workspaces.items.length && remoteWorkspace.enabled"><q-item-section class="text-grey-7">尚未开通用户空间</q-item-section></q-item>
            </q-list>
            <q-btn v-if="workspaces.nextCursor && !selectedUser" flat label="下一页" @click="loadSpaces(workspaces.nextCursor)" />
          </q-card-section>
        </q-card>
        <q-btn v-if="selectedUser" flat dense icon="arrow_back" label="返回空间列表" class="q-mb-sm" @click="selectedUser = undefined; userLabel = ''; loadSpaces()" />
        <WorkspacePanel v-if="selectedUser" :key="selectedUser" :user-id="selectedUser" :display-name="userLabel" :service-enabled="remoteWorkspace.enabled" class="q-mb-md" />
        <q-card v-if="remoteWorkspace.enabled || current?.mcpPublished" flat bordered>
          <q-card-section><div class="text-subtitle1">助手工具（可选）</div><p class="text-caption text-grey-7">需要助手读写或执行工作区工具时，再在企业配置中添加远程工作区 MCP，并审查发布。文件管理不依赖此步骤。</p><div class="text-caption q-mb-sm">{{ current?.mcpPublished ? 'MCP 定义已发布' : '尚未发布 MCP 工具入口' }}<span v-if="!remoteWorkspace.enabled"> · 服务已关闭，工具暂不可用</span></div><q-btn flat color="primary" label="前往 MCP 配置" @click="router.push('/resources?section=mcp')" /></q-card-section>
        </q-card>
      </div>
    </div>
    <q-dialog v-model="confirmDisable"><q-card style="max-width:480px"><q-card-section class="text-h6">保存并关闭远程工作区</q-card-section><q-card-section>将停止用户工作区，撤销 MCP 和 WebDAV 访问。连接配置、空间和文件保留；远端不可达时会继续显示待处理进度。</q-card-section><q-card-actions align="right"><q-btn flat label="取消" v-close-popup /><q-btn color="negative" label="保存并关闭" @click="save" /></q-card-actions></q-card></q-dialog>
  </q-page>
</template>

<style scoped>
.remote-workspace-grid{display:grid;grid-template-columns:minmax(320px,420px) minmax(0,1fr);gap:16px;align-items:start;overflow-wrap:anywhere}
.remote-workspace-grid>*{min-width:0}.workspace-connection-fields{display:flex;flex-direction:column;gap:16px}
@media(max-width:1000px){.remote-workspace-grid{grid-template-columns:minmax(0,1fr)}}
</style>
