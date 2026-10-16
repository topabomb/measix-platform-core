<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { apiFetch, commandResultUncertain, createIdempotencyKey } from '../api/client'
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
const mcpUsesAdmin = ref(true), confirmDisable = ref(false), confirmReplacement = ref(false), replacementConfirmed = ref(false)
const selectedUser = ref<string>(), userLabel = ref('')
const workspaces = ref<components['schemas']['WorkspaceList']>({ items: [] })
const savedSecret = ref<components['schemas']['SecretRef']>()
const operation = ref<Operation>(), check = ref<components['schemas']['WorkspaceServiceCheck']>()
const pendingCommand = ref<{ path: string; body?: string; key: string }>()
let alive = true, timer: ReturnType<typeof setTimeout> | undefined
let pending: { payload: string; key: string } | undefined
const running = computed(() => !!operation.value && ['PENDING', 'RUNNING'].includes(operation.value.state))
const blocked = computed(() => !!operation.value && ['UNKNOWN', 'NEEDS_ATTENTION'].includes(operation.value.state))
const busy = computed(() => saving.value || running.value || remoteWorkspace.loading)
const resolvedMcpOrigin = computed(() => mcpUsesAdmin.value ? adminOrigin.value.trim() : mcpOrigin.value.trim())
const originChanged = computed(() => !!current.value && (adminOrigin.value.trim() !== current.value.config.adminOrigin || resolvedMcpOrigin.value !== current.value.config.mcpOrigin || davOrigin.value.trim() !== (current.value.config.davOrigin ?? '')))
const connectionChanged = computed(() => name.value.trim() !== current.value?.name || originChanged.value || !!bearer.value || !!savedSecret.value)
const formChanged = computed(() => desiredEnabled.value !== !!current.value?.enabled || connectionChanged.value)
const notApplied = computed(() => !!current.value && current.value.activeConfigRevision !== current.value.configRevision)
const canSave = computed(() => !busy.value && !blocked.value && (!!pendingCommand.value || (desiredEnabled.value
  ? !!adminOrigin.value.trim() && !!resolvedMcpOrigin.value && (!!current.value || !!bearer.value || !!savedSecret.value) && (formChanged.value || notApplied.value || !remoteWorkspace.enabled)
  : !!current.value?.enabled)))
const status = computed(() => !current.value ? '尚未配置' : running.value && operation.value?.action === 'REPLACE' ? '修复中' : notApplied.value ? '新配置未生效' : current.value.state === 'ACTIVE' ? '已启用' : current.value.state === 'SAVED' ? '未启用' : workspaceLabel(current.value.state))
const canRepair = computed(() => !!current.value && desiredEnabled.value && !connectionChanged.value && !blocked.value && !running.value && check.value?.configRevision === current.value.configRevision && !check.value.managementReady && !!check.value.replacementAvailable && !!check.value.missingWorkspaces?.length)
const feedback = computed(() => {
  if (saving.value) return notice.value || '正在处理连接，请稍候。'
  if (pendingCommand.value) return '请求结果未确认。请核实原操作；将使用原请求，不会重复开通。'
  if (running.value) {
    const progress = operation.value?.replacementProgress
    return progress ? `正在重新开通：${progress.completed}/${progress.total}${progress.needsAttention ? `。请在用户工作区核实 ${progress.needsAttention} 个原操作。` : ''}` : notice.value || '正在处理连接，请稍候。'
  }
  if (check.value) return check.value.managementReady ? '连接检查通过。' : canRepair.value ? '原有空间在此服务中均不存在。确认旧部署已清理后，可重新开通。' : workspaceLabel(check.value.diagnosticCode) || '连接检查未通过，请检查地址和管理凭据。'
  return notice.value || (blocked.value ? '操作结果待核实，请查看诊断。' : notApplied.value ? '新配置已保存，连接生效后才会用于用户空间。' : '')
})

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
  mcpUsesAdmin.value = value.config.mcpOrigin === value.config.adminOrigin
}
async function loadSpaces(cursor?: string) {
  if (!current.value) return
  try {
    const result = await apiFetch<components['schemas']['WorkspaceList']>(`/api/admin/v1/remote-workspace/services/${current.value.workspaceServiceId}/workspaces${cursor ? '?cursor=' + encodeURIComponent(cursor) : ''}`)
    if (alive) workspaces.value = result
  } catch (cause) { if (alive) { error.value = cause; notice.value = '' } }
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
  if (pendingCommand.value) { void save(); return }
  if (!desiredEnabled.value && current.value?.enabled) confirmDisable.value = true
  else void save()
}
async function save() {
  if (!session.csrfToken) return
  saving.value = true; error.value = undefined; notice.value = ''; confirmDisable.value = false
  try {
    if (pendingCommand.value) { await sendOperation(); return }
    check.value = undefined
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
        mcpOrigin: resolvedMcpOrigin.value.replace(/\/$/, ''),
        ...(davOrigin.value.trim() ? { davOrigin: davOrigin.value.trim().replace(/\/$/, '') } : {}),
        managementSecret: secret,
        connectTimeoutMs: value?.config.connectTimeoutMs ?? 90000, idleTimeoutMs: value?.config.idleTimeoutMs ?? 120000,
      }
      if (!value || name.value.trim() !== value.name || originChanged.value || secret.secretId !== value.config.managementSecret.secretId || secret.secretVersion !== value.config.managementSecret.secretVersion) {
        const body = { expectedRevision: value?.configRevision ?? 0, name: name.value.trim() || '远程工作区', config }
        const payload = JSON.stringify(body)
        const saved = await apiFetch<WorkspaceService>('/api/admin/v1/remote-workspace/services' + (value ? '/' + value.workspaceServiceId : ''), {
          method: value ? 'PUT' : 'POST', headers: { 'Idempotency-Key': keyFor(payload) }, body: payload,
        }, session.csrfToken)
        value = { ...value, ...saved }
        remoteWorkspace.current = value; savedSecret.value = undefined; pending = undefined
        fillForm(); desiredEnabled.value = true
      }
    }
    if (!value) return
    if (desiredEnabled.value) {
      check.value = await apiFetch(`/api/admin/v1/remote-workspace/services/${value.workspaceServiceId}/check`, { method: 'POST' }, session.csrfToken)
      if (!check.value?.managementReady) return
    }
    const action = desiredEnabled.value ? 'apply' : 'disable'
    const path = `/api/admin/v1/remote-workspace/services/${value.workspaceServiceId}/${action}`
    notice.value = desiredEnabled.value ? '正在启用连接。' : '正在关闭访问并停止工作区，文件会保留。'
    await sendOperation(path, desiredEnabled.value ? JSON.stringify({ expectedConfigRevision: value.configRevision }) : undefined)
  } catch (cause) { if (alive) error.value = cause }
  finally { saving.value = false }
}
async function sendOperation(path?: string, body?: string) {
  if (!session.csrfToken) return
  if (!pendingCommand.value && path) pendingCommand.value = { path, body, key: createIdempotencyKey() }
  const command = pendingCommand.value
  if (!command) return
  try {
    operation.value = await apiFetch<Operation>(command.path, { method: 'POST', headers: { 'Idempotency-Key': command.key }, ...(command.body ? { body: command.body } : {}) }, session.csrfToken)
    pendingCommand.value = undefined; check.value = undefined; confirmReplacement.value = false
    await remoteWorkspace.load()
    if (alive) poll()
  } catch (cause) {
    if (!commandResultUncertain(cause)) { pendingCommand.value = undefined; check.value = undefined; confirmReplacement.value = false }
    throw cause
  }
}
async function replaceDeployment() {
  if (!pendingCommand.value && (!canRepair.value || !replacementConfirmed.value || !check.value?.missingWorkspaces)) return
  saving.value = true; error.value = undefined; notice.value = '正在重新开通用户空间。'
  try {
    if (pendingCommand.value) await sendOperation()
    else await sendOperation(`/api/admin/v1/remote-workspace/services/${current.value!.workspaceServiceId}/replace`, JSON.stringify({ expectedConfigRevision: check.value?.configRevision, bindings: check.value?.missingWorkspaces, confirmedDestroyedDeployment: true }))
  } catch (cause) { error.value = cause; notice.value = '' }
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
    const previousStep = operation.value.step
    operation.value = result
    if (running.value) {
      if (result.action === 'REPLACE' && result.step === 'RECREATING' && (previousStep !== result.step || !remoteWorkspace.enabled)) {
        await remoteWorkspace.load(); await loadSpaces()
      }
      poll(); return
    }
    await remoteWorkspace.load(); await loadSpaces()
    if (result.state === 'COMPLETED' && result.step !== 'CHECK_FAILED') {
      fillForm()
      check.value = undefined
      notice.value = remoteWorkspace.enabled ? '连接已生效。' : '已关闭。已有空间和文件保留，重新启用后可恢复连接。'
    } else notice.value = result.step === 'CHECK_FAILED' ? '配置已保存，但启用检查未通过。请修正连接配置后再次保存。' : '操作尚未确认完成，请查看诊断并核实远端状态。'
  } catch (cause) { if (alive) error.value = cause }
}
async function checkConnection() {
  if (!current.value || !session.csrfToken) return
  saving.value = true; error.value = undefined; notice.value = ''
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
  <q-page class="admin-page">
    <PageHeader title="远程工作区" subtitle="管理服务连接与用户空间。" />
    <ProblemBanner :error="pendingCommand ? undefined : error || remoteWorkspace.error" />
    <div class="remote-workspace-grid">
      <details class="workspace-service" :open="!current || notApplied || running || blocked || !!pendingCommand || !!check">
        <summary><span class="text-weight-medium">服务连接</span><q-badge :color="notApplied ? 'orange-8' : remoteWorkspace.enabled ? 'positive' : 'grey-7'">{{ status }}</q-badge><span class="text-caption text-grey-7 ellipsis">{{ current?.name ?? '配置服务地址与凭据' }}</span><span class="text-caption text-primary">配置</span></summary>
      <q-card flat>
        <q-card-section>
          <q-toggle v-model="desiredEnabled" label="启用远程工作区" :disable="busy || blocked || !!pendingCommand" />
          <p class="text-caption text-grey-7 q-mb-none">保存后生效；关闭访问会保留空间和文件。</p>
        </q-card-section>
        <template v-if="desiredEnabled || current">
          <q-separator />
          <q-card-section class="workspace-connection-fields">
            <q-input v-model="name" outlined dense label="连接名称" :disable="busy || !!pendingCommand || !desiredEnabled" />
            <q-input v-model="adminOrigin" outlined dense label="管理服务地址" placeholder="https://space.example.com" :disable="busy || !!pendingCommand || !desiredEnabled" />
            <q-input v-model="davOrigin" outlined dense label="文件服务地址（可选）" hint="WebDAV 对外地址；留空不提供文件访问。" :disable="busy || !!pendingCommand || !desiredEnabled" />
            <q-input v-model="bearer" outlined dense type="password" autocomplete="new-password" :label="current ? '管理凭据（留空保留原值）' : '管理凭据'" :disable="busy || !!pendingCommand || !desiredEnabled" />
            <details>
              <summary class="cursor-pointer text-primary">工具服务地址（可选）</summary>
              <q-checkbox v-model="mcpUsesAdmin" dense label="随管理服务地址" class="q-mt-sm" :disable="busy || !!pendingCommand || !desiredEnabled" />
              <q-input v-if="!mcpUsesAdmin" v-model="mcpOrigin" outlined dense label="MCP 服务地址" class="q-mt-sm" :disable="busy || !!pendingCommand || !desiredEnabled" />
            </details>
          </q-card-section>
        </template>
        <q-card-actions class="q-pa-sm q-pt-none">
          <q-btn color="primary" :label="saving ? '正在提交' : running ? '处理中' : pendingCommand ? '核实原操作' : desiredEnabled || !current ? '保存并连接' : '保存并关闭'" :loading="saving || running" :disable="!canSave" @click="requestSave" />
          <q-btn v-if="current" flat label="检查连接" :disable="busy || connectionChanged || !!pendingCommand" @click="checkConnection" />
        </q-card-actions>
        <q-card-section v-if="feedback || operation || check" class="q-pt-none">
          <q-banner v-if="feedback" dense :class="canRepair || blocked || pendingCommand ? 'bg-orange-1' : 'bg-blue-1'" data-cy="workspace-save-notice">{{ feedback }}<template #action><q-btn v-if="canRepair && !pendingCommand" flat dense label="修复连接" :disable="busy" @click="replacementConfirmed = false; confirmReplacement = true" /><q-btn v-if="running || blocked" flat dense label="刷新进度" @click="refreshOperation" /></template></q-banner>
          <details v-if="current" class="text-caption q-mt-sm"><summary>连接诊断</summary>配置版本 {{ current.configRevision }} · 生效版本 {{ current.activeConfigRevision ?? '无' }}<div v-if="check?.diagnosticCode">{{ check.diagnosticCode }}</div><div v-if="operation">{{ operation.operationId }} · {{ operation.step }}<span v-if="operation.diagnosticCode"> · {{ operation.diagnosticCode }}</span></div></details>
        </q-card-section>
      </q-card>
      </details>
      <div>
        <q-card v-if="!selectedUser" flat bordered class="q-mb-sm">
          <q-card-section class="q-pa-sm">
            <div class="row items-center justify-between q-mb-sm"><div class="text-h6">用户工作区</div><q-btn flat round dense icon="refresh" aria-label="刷新空间列表" :disable="busy" @click="loadSpaces()" /></div>
            <p v-if="!remoteWorkspace.enabled" class="text-caption text-grey-7">启用服务后可开通空间。已有空间可继续查看和清理。</p>
            <div v-else class="workspace-user-picker"><PagedEntityPicker v-model="selectedUser" label="选择企业用户" empty-label="选择用户查看或开通" :fetch-page="users" :selected-option="selectedUser && userLabel ? { value: selectedUser, label: userLabel } : undefined" @selected="option => userLabel = option?.label ?? ''" /><q-btn flat dense color="primary" label="管理用户" @click="router.push('/users')" /></div>
            <q-list v-if="!selectedUser || !remoteWorkspace.enabled" separator>
              <q-item v-for="row in workspaces.items" :key="row.userId" clickable @click="selectedUser = row.userId; userLabel = row.displayName"><q-item-section>{{ row.displayName }}<q-item-label caption>{{ workspaceLabel(row.workspace.state) }}</q-item-label></q-item-section><q-item-section side><q-icon name="chevron_right" /></q-item-section></q-item>
              <q-item v-if="!workspaces.items.length && remoteWorkspace.enabled"><q-item-section class="text-grey-7">尚未开通用户空间</q-item-section></q-item>
            </q-list>
            <q-btn v-if="workspaces.nextCursor && !selectedUser" flat label="下一页" @click="loadSpaces(workspaces.nextCursor)" />
          </q-card-section>
        </q-card>
        <q-btn v-if="selectedUser" flat dense icon="arrow_back" label="返回空间列表" class="q-mb-sm" @click="selectedUser = undefined; userLabel = ''; loadSpaces()" />
        <WorkspacePanel v-if="selectedUser" :key="selectedUser" :user-id="selectedUser" :display-name="userLabel" :service-enabled="remoteWorkspace.enabled" class="q-mb-md" />
        <details v-if="current?.davResourceId || remoteWorkspace.enabled || current?.mcpPublished" class="workspace-publication">
         <summary>客户端接入与发布</summary>
         <div class="workspace-resource-links"><q-card v-if="current?.davResourceId" flat bordered data-cy="workspace-dav-resource">
          <q-card-section><div class="text-subtitle2">文件交换 · WebDAV</div><div class="text-caption q-mb-xs">草稿：{{ current.davDraftEnabled ? '启用' : '未启用' }} · 当前发布：{{ current.davPublishedEnabled ? '启用' : '未启用' }}<span v-if="!remoteWorkspace.enabled"> · 服务已关闭，当前无默认指定</span></div><q-btn flat dense color="primary" label="前往 WebDAV 配置" @click="router.push('/resources?section=webdav')" /></q-card-section>
        </q-card>
        <q-card v-if="remoteWorkspace.enabled || current?.mcpPublished" flat bordered>
          <q-card-section><div class="text-subtitle2">助手工具 · MCP</div><div class="text-caption q-mb-xs">{{ current?.mcpPublished ? 'MCP 定义已发布' : '尚未发布 MCP 工具入口' }}<span v-if="!remoteWorkspace.enabled"> · 服务已关闭，工具暂不可用</span></div><q-btn flat dense color="primary" label="前往 MCP 配置" @click="router.push('/resources?section=mcp')" /></q-card-section>
        </q-card></div></details>
      </div>
    </div>
    <q-dialog v-model="confirmDisable"><q-card class="app-dialog app-dialog--sm"><q-card-section class="text-subtitle1 text-weight-medium">关闭远程工作区</q-card-section><q-card-section class="app-dialog__body">将停止用户工作区并撤销 MCP、WebDAV 访问，保留连接配置、空间和文件。远端不可达时，操作会保持待处理。</q-card-section><q-card-actions align="right"><q-btn flat label="取消" v-close-popup /><q-btn color="negative" label="确认关闭" @click="save" /></q-card-actions></q-card></q-dialog>
    <q-dialog v-model="confirmReplacement" :persistent="saving || !!pendingCommand"><q-card class="app-dialog app-dialog--sm"><q-card-section class="text-subtitle1 text-weight-medium">修复服务连接</q-card-section><q-card-section class="app-dialog__body">为原用户重新开通；新空间为空，旧文件不会恢复。Core 用户、配置、发布和用量保留。<details class="q-my-sm"><summary>受影响用户（{{ check?.missingWorkspaces?.length ?? 0 }}）</summary><div v-for="binding in check?.missingWorkspaces" :key="binding.userId">{{ binding.displayName || binding.userId }}</div></details><q-checkbox v-model="replacementConfirmed" dense label="旧部署和旧数据已清理，原管理请求已结束" :disable="saving || !!pendingCommand" /></q-card-section><q-card-actions align="right"><q-btn flat label="取消" :disable="saving || !!pendingCommand" v-close-popup /><q-btn color="primary" :label="pendingCommand ? '核实原操作' : '确认重新开通'" :loading="saving" :disable="!pendingCommand && (!canRepair || !replacementConfirmed)" @click="replaceDeployment" /></q-card-actions></q-card></q-dialog>
  </q-page>
</template>

<style scoped>
.remote-workspace-grid{display:grid;gap:12px;overflow-wrap:anywhere}
.remote-workspace-grid>*{min-width:0}.workspace-connection-fields{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:12px}
.workspace-service{border:1px solid #ddd;border-radius:6px;overflow:hidden}.workspace-service>summary{display:flex;align-items:center;gap:10px;padding:10px 12px;cursor:pointer}.workspace-service>summary::before{content:'▸'}.workspace-service[open]>summary::before{content:'▾'}.workspace-service>summary .ellipsis{flex:1;min-width:0}
.workspace-resource-links{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:12px}.workspace-resource-links :deep(.q-card__section){padding:12px}
.workspace-user-picker{display:flex;align-items:center;gap:8px}.workspace-user-picker>:first-child{flex:1;min-width:0}
.workspace-publication>summary{cursor:pointer;font-size:13px;color:#666;padding:8px 0}.workspace-resource-links :deep(.q-card__section){padding:8px 12px}
@media(max-width:600px){.workspace-connection-fields,.workspace-resource-links{grid-template-columns:minmax(0,1fr)}}
</style>
