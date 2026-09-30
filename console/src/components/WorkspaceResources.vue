<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { QBtn } from 'quasar'
import { apiFetch } from '../api/client'
import { workspaceBytes as bytes } from '../composables/workspaceFiles'
import type { components } from '../api/generated'

type Resources = components['schemas']['WorkspaceResources']
type Observation = Resources['runtime'] | Resources['memory'] | Resources['disk']
type Allocation = components['schemas']['WorkspaceResourceAllocation']
const props = defineProps<{ baseUrl: string; spaceId: string; revision: number | string }>()
const summary = ref<Resources>(), busy = ref(false), failure = ref('')
let alive = true, serial = 0, controller: AbortController | undefined, timer: ReturnType<typeof setTimeout> | undefined
const runtimeLabels: Record<string, string> = { running: '运行中', stopped: '已停止', paused: '已暂停', transitioning: '状态转换中', notfound: '尚未创建运行环境', unknown: '状态未知' }
const reasons: Record<string, string> = {
  stopped: '空间已停止', paused: '空间已暂停', transitioning: '运行状态正在变化', notfound: '尚未创建运行环境',
  account_inactive: '远端账号未启用', account_changed: '账号状态已变化', not_sampled: '尚未采样', sampling_disabled: '服务未开启采样',
  guest_memory_unavailable: '暂未取得客户机内存样本', stale_msb_snapshot: '内存快照已过期', collection_busy: '采集繁忙，请稍后刷新',
  collection_timeout: '采集超时', workspace_stat_failed: '工作卷采集失败', runtime_changed: '运行实例已变化',
  inspection_failed: '未能核实运行状态', status_unavailable: '运行状态暂不可用', metrics_unavailable: '指标暂不可用',
  invalid_resource_sample: '服务返回的观测无效', refresh_failed: '本次刷新失败', runtime_exited: '运行实例已退出',
}
function time(at: number | null) { return at == null ? '' : new Date(at).toLocaleString() }
function status(o: Observation) { return { current: '最近观测', historical: '历史记录', unavailable: '暂无数据', error: '采集失败' }[o.status] }
function reason(o: Observation) { return o.reason ? reasons[o.reason] ?? '暂时无法取得有效观测' : '' }
function allocation(a: Allocation, cores = false) { return a.value == null ? '未取得配置' : cores ? `${a.value} 核` : bytes(a.value) }
function source(a: Allocation) { return a.source === 'default' ? '创建默认值' : a.source === 'unknown' ? '配置读取失败' : '已配置' }
const cards = computed(() => summary.value ? [
  { key: 'runtime', title: '运行环境', observation: summary.value.runtime, primary: summary.value.runtime.value ? runtimeLabels[summary.value.runtime.value] ?? '状态未知' : '状态未知', secondary: '与企业连接状态分别记录' },
  { key: 'disk', title: '工作区磁盘', observation: summary.value.disk, primary: summary.value.disk.value ? `已用 ${bytes(summary.value.disk.value.usedBytes)}` : '暂无用量', secondary: summary.value.disk.value ? `可用 ${bytes(summary.value.disk.value.availableBytes)}` : '仅统计工作区文件所在卷' },
  { key: 'memory', title: '客户机内存', observation: summary.value.memory, primary: summary.value.memory.value != null ? `已用 ${bytes(summary.value.memory.value)}` : '暂无用量', secondary: `配置上限 ${bytes(summary.value.allocation.memoryLimitBytes.value)}` },
] : [])
function cancel() { serial++; controller?.abort(); controller = undefined; clearTimeout(timer); busy.value = false }
function visible() { return document.visibilityState !== 'hidden' }
function historical() {
  if (!summary.value) return
  for (const key of ['runtime', 'disk', 'memory'] as const) {
    const observation = summary.value[key]
    if (observation.value != null) { observation.status = 'historical'; observation.reason = 'refresh_failed' }
  }
}
async function refresh() {
  if (!alive || !visible() || busy.value) return
  clearTimeout(timer)
  const request = ++serial, space = props.spaceId
  controller = new AbortController(); busy.value = true
  try {
    const next = await apiFetch<Resources>(`${props.baseUrl}/resources?agentSpaceId=${encodeURIComponent(space)}`, { signal: controller.signal })
    if (!alive || request !== serial) return
    if (next.agentSpaceId !== space) { summary.value = undefined; failure.value = '空间或配置已变化，请刷新工作区后重试。'; return }
    summary.value = next; failure.value = ''
  } catch (error) {
    if (!alive || request !== serial) return
    const problem = error as { status?: number; code?: string }
    if ([401, 403, 404, 409].includes(problem.status ?? 0) || ['workspace_space_mismatch', 'workspace_revision_conflict', 'remote_identity_mismatch'].includes(problem.code ?? '')) {
      summary.value = undefined; failure.value = '空间或配置已变化，或当前访问已失效。请刷新工作区后重试。'
    } else { historical(); failure.value = '刷新失败，暂时无法取得最新资源摘要。' + (summary.value ? '已有数值仅供历史参考。' : '') }
  } finally {
    if (alive && request === serial) {
      busy.value = false; controller = undefined
      if (visible()) timer = setTimeout(() => { void refresh() }, 15000)
    }
  }
}
function visibility() { if (document.visibilityState === 'hidden') cancel(); else void refresh() }
watch(() => [props.baseUrl, props.spaceId, props.revision], () => { cancel(); summary.value = undefined; failure.value = ''; void refresh() })
onMounted(() => { document.addEventListener('visibilitychange', visibility); void refresh() })
onBeforeUnmount(() => { alive = false; cancel(); document.removeEventListener('visibilitychange', visibility) })
</script>

<template>
  <section class="workspace-resources q-mx-md q-mb-md" aria-label="工作区资源摘要" data-cy="workspace-resources">
    <div class="row items-center justify-between q-mb-sm">
      <div><div class="text-subtitle1 text-weight-medium">资源摘要</div><div class="text-caption text-grey-7">查看不会启动运行环境 · 页面可见时每 15 秒刷新</div></div>
      <q-btn flat dense icon="refresh" label="刷新摘要" data-cy="refresh-resources" :loading="busy" :disable="busy" @click="refresh" />
    </div>
    <div v-if="failure" role="status" class="resource-notice q-pa-sm q-mb-sm">{{ failure }}</div>
    <div v-if="!summary" class="text-grey-7 q-py-sm" aria-live="polite">{{ busy ? '正在读取资源摘要…' : '暂时没有可显示的资源摘要。' }}</div>
    <template v-else>
      <div class="resource-grid">
        <div v-for="card in cards" :key="card.key" class="resource-card">
          <div class="row items-center justify-between q-gutter-xs"><span class="text-grey-8">{{ card.title }}</span><span class="resource-state" :class="`state-${card.observation.status}`">{{ status(card.observation) }}</span></div>
          <div class="resource-value q-mt-sm">{{ card.primary }}</div>
          <div class="text-caption text-grey-8 q-mt-xs">{{ card.secondary }}</div>
          <div v-if="card.observation.observedAt != null" class="text-caption text-grey-7 q-mt-sm">{{ time(card.observation.observedAt) }}</div>
          <div v-if="reason(card.observation)" class="text-caption text-grey-8 q-mt-xs">{{ reason(card.observation) }}</div>
        </div>
      </div>
      <div class="resource-allocation q-mt-sm text-caption">
        <span>CPU：{{ allocation(summary.allocation.cpuCores, true) }}（{{ source(summary.allocation.cpuCores) }}）</span>
        <span>内存：{{ allocation(summary.allocation.memoryLimitBytes) }}（{{ source(summary.allocation.memoryLimitBytes) }}）</span>
        <span>工作卷：{{ allocation(summary.allocation.workspaceCapacityBytes) }}（{{ source(summary.allocation.workspaceCapacityBytes) }}）</span>
      </div>
      <details class="text-caption text-grey-7 q-mt-sm"><summary class="cursor-pointer">数据口径</summary><p class="q-mt-xs q-mb-none">磁盘仅统计工作卷，配置容量与文件系统已用、可用之和可能不同。内存是客户机最近快照，不是宿主物理内存占用。暂停不表示宿主资源已释放。历史数据保留原观测时间，不能代表停机瞬间用量。</p></details>
    </template>
  </section>
</template>

<style scoped>
.workspace-resources{border-top:1px solid #e0e4eb;padding-top:16px;min-width:0}
.resource-grid{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:10px}
.resource-card{border:1px solid #e0e4eb;border-radius:8px;padding:12px;min-width:0;overflow-wrap:anywhere}
.resource-value{font-size:1.15rem;font-weight:600;line-height:1.5}
.resource-state{font-size:11px;padding:2px 6px;border-radius:4px;background:#edf2f7;color:#475569;white-space:nowrap}
.state-current{background:#e8f5e9;color:#25632c}.state-historical,.state-error{background:#fff3e0;color:#855100}
.resource-notice{background:#fff5e5;color:#795000;border-radius:6px}
.resource-allocation{display:flex;gap:6px 18px;flex-wrap:wrap;color:#475569}
@media(max-width:700px){.resource-grid{grid-template-columns:minmax(0,1fr)}.resource-value{font-size:1.1rem}}
</style>
