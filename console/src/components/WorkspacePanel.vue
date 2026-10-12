<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { copyToClipboard, useQuasar } from 'quasar'
import { apiFetch, createIdempotencyKey } from '../api/client'
import type { components } from '../api/generated'
import { useSessionStore } from '../stores/session'
import ProblemBanner from './ProblemBanner.vue'
import LoadingState from './LoadingState.vue'
import WorkspaceFiles from './WorkspaceFiles.vue'
import WorkspaceResources from './WorkspaceResources.vue'
import WorkspaceDAVAccess from './WorkspaceDAVAccess.vue'
import { workspaceLabel } from '../composables/workspaceLabels'

const props=withDefaults(defineProps<{userId:string;displayName?:string;serviceEnabled?:boolean}>(),{serviceEnabled:false})
const session=useSessionStore()
const quasar=useQuasar()
type Projection=components['schemas']['WorkspaceProjection']
type Command=components['schemas']['WorkspaceCommand']
const view=ref<Projection>(),error=ref<unknown>(),busy=ref(false)
const refreshError=ref<unknown>()
const operation=ref<components['schemas']['WorkspaceOperation']>()
const confirm=ref<Command['action']>(),confirmation=ref(''),evidence=ref(''),remoteUsername=ref(''),spaceId=ref('')
type ConnectionKind='dav'|'mcp'
const connectionKind=ref<ConnectionKind>('dav')
const connectionOpen=ref(false),connection=ref<components['schemas']['WorkspaceDAVConnection']|components['schemas']['WorkspaceMCPConnection']>(),revealing=ref(false)
const davConnection=computed(()=>connection.value&&'davUrl' in connection.value?connection.value:undefined)
const mcpConnection=computed(()=>connection.value&&'mcpUrl' in connection.value?connection.value:undefined)
const showFiles=ref(false),showToken=ref(false),showResources=ref(false)
const recoveryBearer=ref(''),recoverySecret=ref<components['schemas']['SecretRef']>()
const url=computed(()=>`/api/admin/v1/users/${encodeURIComponent(props.userId)}/workspace`)
const labels:Record<string,string>={UNPROVISIONED:'未开通',CONNECTING:'开通中',CONNECTED:'已开通',DISCONNECTING:'断开中',DISCONNECTED:'已断开',RESTORING:'恢复中',DELETING:'删除中',DELETED:'已删除',NEEDS_ATTENTION:'需要处理'}
const actionLabels:Record<string,string>={CREATE:'开通远程工作区',DISCONNECT:'断开工作区',RESTORE:'恢复连接',DELETE:'删除远程工作区',TAKEOVER:'显式接管',RESET_MCP:'重新签发 MCP 凭据',SET_DAV:'签发 / 重设 WebDAV Token',SET_DAV_ACCESS:'应用 WebDAV 权限',REVOKE_DAV:'撤销 WebDAV Token',CONTINUE:'核实后继续'}
const pendingOperation=computed(()=>operation.value&&operation.value.state!=='COMPLETED')
const davPending=computed(()=>!!operation.value&&['UNKNOWN','NEEDS_ATTENTION'].includes(operation.value.state)&&operation.value.step==='DAV_SENT')
const canVerifyDisconnect=computed(()=>!!operation.value&&['UNKNOWN','NEEDS_ATTENTION'].includes(operation.value.state)&&['DAV_SENT','DAV_REVOKE_SENT'].includes(operation.value.step))
const needsEvidence=computed(()=>['TAKEOVER','CONTINUE'].includes(confirm.value??'')||confirm.value==='DISCONNECT'&&canVerifyDisconnect.value)
let alive=true,timer:ReturnType<typeof setTimeout>|undefined,controller=new AbortController(),refreshEpoch=0
let pending:{payload:string;key:string}|undefined
let revealEpoch=0,revealController:AbortController|undefined
function invalidateConnection(){revealEpoch++;revealController?.abort();revealController=undefined;revealing.value=false;connection.value=undefined;showToken.value=false}
function canRevealMCP(value:Projection|undefined){return props.serviceEnabled&&value?.state==='CONNECTED'&&value.serviceState!=='DISABLED'&&['available','mcp_not_published'].includes(value.mcpReason??'')}
function openConnection(kind:ConnectionKind){invalidateConnection();error.value=undefined;connectionKind.value=kind;connectionOpen.value=true;void reveal()}
async function refresh(){
 const epoch=++refreshEpoch,signal=controller.signal
 try{const next=await apiFetch<Projection>(url.value,{signal});if(!alive||signal.aborted||epoch!==refreshEpoch)return
 const nextOperation=next.operationId?await apiFetch<components['schemas']['WorkspaceOperation']>(`/api/admin/v1/workspace-operations/${next.operationId}`,{signal}):undefined
 if(!alive||signal.aborted||epoch!==refreshEpoch)return
 if((connectionKind.value==='dav'&&!next.filesAvailable)||(connectionKind.value==='mcp'&&!canRevealMCP(next))||next.agentSpaceId!==view.value?.agentSpaceId||next.bindingRevision!==view.value?.bindingRevision||connectionKind.value==='dav'&&next.webdav?.desiredBindingRevision!==view.value?.webdav?.desiredBindingRevision)invalidateConnection()
 view.value=next;operation.value=nextOperation
 refreshError.value=undefined
 }catch(e){if(alive&&!signal.aborted&&epoch===refreshEpoch)refreshError.value=e}
}
function schedule(){if(timer)clearTimeout(timer);if(document.hidden)return;timer=setTimeout(async()=>{await refresh();if(alive&&!document.hidden)schedule()},3000)}
function visibility(){if(document.hidden){if(timer)clearTimeout(timer);connectionOpen.value=false;invalidateConnection()}else{void refresh().finally(()=>{if(alive)schedule()})}}
function ask(action:Command['action']){confirm.value=action;confirmation.value='';evidence.value='';invalidateConnection();recoveryBearer.value='';recoverySecret.value=undefined}
async function command(action:Command['action']){
 if(!view.value||!session.csrfToken)return
 busy.value=true;error.value=undefined
 const signal=controller.signal,commandUrl=url.value
 const body:Command={action,expectedRevision:view.value.bindingRevision,...(confirmation.value?{confirmation:confirmation.value}:{}),...(evidence.value?{evidence:evidence.value}:{}),...(action==='TAKEOVER'||action==='CONTINUE'&&operation.value?.step==='CREATE_SENT'?{remoteUsername:remoteUsername.value,agentSpaceId:spaceId.value}:{})}
 try{
  if(action==='CONTINUE'){
   if(recoveryBearer.value){const secret=await apiFetch<components['schemas']['Secret']>('/api/admin/v1/secrets',{method:'POST',body:JSON.stringify({name:'Remote workspace recovery management',value:recoveryBearer.value}),signal},session.csrfToken);if(!alive||signal.aborted)return;recoverySecret.value={secretId:secret.secretId,secretVersion:secret.secretVersion};recoveryBearer.value=''}
   if(recoverySecret.value)body.managementSecret=recoverySecret.value
  }
  const payload=JSON.stringify(body);if(pending?.payload!==payload)pending={payload,key:createIdempotencyKey()}
  const result=await apiFetch<components['schemas']['WorkspaceOperation']>(commandUrl,{method:'POST',body:payload,headers:{'Idempotency-Key':pending.key},signal},session.csrfToken);if(!alive||signal.aborted)return;operation.value=result;pending=undefined;confirm.value=undefined;invalidateConnection();recoverySecret.value=undefined;await refresh()
 }catch(e){if(alive&&!signal.aborted)error.value=e}finally{if(!signal.aborted)busy.value=false}
}
async function reveal(){
 if(!session.csrfToken||!view.value||(connectionKind.value==='dav'?!view.value.filesAvailable:!canRevealMCP(view.value)))return
 invalidateConnection();revealing.value=true;error.value=undefined
 const epoch=revealEpoch;revealController=new AbortController();const signal=revealController.signal
 const path=connectionKind.value==='dav'?url.value+'/dav-connection':url.value+`/mcp-connection?agentSpaceId=${encodeURIComponent(view.value.agentSpaceId??'')}&expectedRevision=${view.value.bindingRevision}`
 try{const value=await apiFetch<NonNullable<typeof connection.value>>(path,{method:'POST',signal,cache:'no-store'},session.csrfToken);if(alive&&!signal.aborted&&epoch===revealEpoch&&connectionOpen.value&&!document.hidden)connection.value=value}catch(e){if(alive&&!signal.aborted&&epoch===revealEpoch)error.value=e}finally{if(epoch===revealEpoch){revealing.value=false;revealController=undefined}}
}
async function copy(value:string){try{await copyToClipboard(value);quasar.notify({type:'positive',message:'已复制',timeout:1200})}catch(e){error.value=e}}
watch(connectionOpen,open=>{if(!open)invalidateConnection()},{flush:'sync'})
watch(()=>props.serviceEnabled,enabled=>{if(!enabled){connectionOpen.value=false;invalidateConnection()}})
watch(()=>props.userId,async()=>{controller.abort();controller=new AbortController();refreshEpoch++;invalidateConnection();view.value=undefined;operation.value=undefined;connectionOpen.value=false;showFiles.value=false;showResources.value=false;confirm.value=undefined;recoveryBearer.value='';recoverySecret.value=undefined;pending=undefined;busy.value=false;error.value=undefined;refreshError.value=undefined;await refresh();if(alive)schedule()})
onMounted(async()=>{document.addEventListener('visibilitychange',visibility);await refresh();if(alive)schedule()})
onBeforeUnmount(()=>{alive=false;controller.abort();invalidateConnection();document.removeEventListener('visibilitychange',visibility);if(timer)clearTimeout(timer);recoveryBearer.value='';recoverySecret.value=undefined})
</script>
<template>
 <q-card flat bordered data-cy="workspace-panel">
  <q-card-section><div class="workspace-panel-heading"><div><div class="text-subtitle1 text-weight-medium">{{displayName || '远程工作区'}}</div><span class="text-caption text-grey-7">{{view?labels[view.state]:'正在加载'}}</span></div><q-btn flat round dense icon="refresh" aria-label="刷新工作区" :disable="busy" @click="error=undefined;refresh()"/></div></q-card-section>
  <q-card-section class="q-pa-sm q-pt-none"><ProblemBanner :error="error ?? refreshError"/>
   <template v-if="view"><q-banner v-if="view.serviceState==='NOT_CONFIGURED'" dense class="bg-grey-2 q-mb-sm">企业尚未配置远程工作区服务。</q-banner><q-banner v-else-if="view.serviceState==='DISABLED'" dense class="bg-grey-2 q-mb-sm">远程工作区服务未启用，不能开通或恢复连接。已有空间仍可核查和删除。</q-banner>
    <div v-if="(!view.mcpAvailable && view.mcpReason!=='mcp_not_published')||!view.filesAvailable" class="text-caption text-grey-7 q-mb-sm">MCP：{{workspaceLabel(view.mcpReason)}} · 文件：{{davPending?'凭据签发待核实，请先处理下方操作':workspaceLabel(view.filesReason)}}</div>
    <q-banner v-if="serviceEnabled && view.state==='CONNECTED' && view.filesReason==='dav_not_configured'" dense class="bg-grey-2 q-mb-sm">请先在远程工作区服务配置中填写文件服务地址，再签发 WebDAV Token。</q-banner>
    <q-banner v-if="serviceEnabled && view.state==='CONNECTED' && view.filesReason==='dav_credential_unavailable' && !pendingOperation" dense class="bg-orange-1 q-mb-sm">文件凭据不可用，请核实上游配置及原请求结果后重新签发。<template #action><q-btn flat label="签发文件凭据" :disable="busy" @click="ask('SET_DAV')"/></template></q-banner>

    <div class="row items-start q-gutter-xs">
     <q-btn v-if="serviceEnabled && view.state==='UNPROVISIONED' && view.mcpReason!=='user_unavailable'" color="primary" label="开通工作区" :disable="busy" @click="ask('CREATE')"/>
     <q-btn v-if="serviceEnabled && (view.state==='UNPROVISIONED'||operation?.step==='CREATE_SENT'&&operation.state==='UNKNOWN') && view.mcpReason!=='user_unavailable'" outline label="接管已有空间" :disable="busy" @click="ask('TAKEOVER')"/>
     <q-btn v-if="serviceEnabled && view.filesAvailable" dense color="primary" :label="showFiles?'收起文件':'浏览文件'" @click="showFiles=!showFiles"/>
     <details class="workspace-maintenance"><summary class="text-primary">连接维护</summary><div class="row q-gutter-xs q-mt-sm"><q-btn v-if="view.state==='CONNECTED'" outline label="断开" :disable="busy||!!pendingOperation&&!canVerifyDisconnect" @click="ask('DISCONNECT')"/>
     <q-btn v-if="serviceEnabled && view.state==='DISCONNECTED' && view.mcpReason!=='user_unavailable'" color="primary" label="恢复连接" :disable="busy||!!pendingOperation" @click="ask('RESTORE')"/>
     <q-btn v-if="serviceEnabled && view.state==='CONNECTED' && view.filesAvailable" outline label="WebDAV 连接信息" :disable="busy" @click="openConnection('dav')"/>
     <q-btn v-if="canRevealMCP(view)" outline label="MCP / 云桌面凭据" :disable="busy" @click="openConnection('mcp')"/>
     <q-btn v-else-if="serviceEnabled && view.state==='CONNECTED'" outline label="重新签发 MCP 凭据" :disable="busy||!!pendingOperation" @click="ask('RESET_MCP')"/>
     <q-btn v-if="view.agentSpaceId&&['CONNECTED','DISCONNECTED'].includes(view.state)" flat color="negative" label="删除空间" :disable="busy||!!pendingOperation" @click="ask('DELETE')"/></div></details>
    </div>
    <details v-if="operation?.state==='COMPLETED'" class="text-caption text-grey-7 q-mt-sm"><summary>最近操作：{{actionLabels[operation.action]}} · 已完成</summary>{{operation.operationId}} · {{operation.step}}</details>
    <q-banner v-if="operation && operation.state!=='COMPLETED'" dense :class="['UNKNOWN','NEEDS_ATTENTION'].includes(operation.state)?'bg-orange-1 q-mt-sm':'bg-blue-1 q-mt-sm'" data-cy="workspace-operation"><div class="text-weight-medium">{{davPending?'文件凭据签发结果待核实':`${actionLabels[operation.action]??'工作区操作'} · ${workspaceLabel(operation.state)}`}}</div><div v-if="davPending" class="q-mt-xs">{{view.mcpAvailable?'MCP 可继续使用；':''}}文件访问暂不可用。原签发请求可能已在远端执行，不能直接重复签发。</div><div v-if="operation.diagnosticCode">{{workspaceLabel(operation.diagnosticCode)}}</div><details class="text-caption"><summary>操作详情</summary>{{operation.operationId}} · {{operation.step}}</details><div v-if="['UNKNOWN','NEEDS_ATTENTION'].includes(operation.state)" class="q-mt-xs"><div>{{davPending?'先查询远端状态，再确认原请求已经结束及账号、空间匹配。查询不会自动恢复文件访问。':'请先核实远端目标及此前在途写入。'}}</div><q-btn flat dense label="重新查询" :disable="busy" @click="command('REQUERY')"/><q-btn v-if="operation.step!=='CREATE_SENT'||!serviceEnabled||view.mcpReason==='user_unavailable'" flat dense label="核实后继续" :disable="busy" @click="ask('CONTINUE')"/></div></q-banner>
    <details v-if="view.agentSpaceId" class="text-caption text-grey-7 q-mt-sm"><summary>空间标识与最近观测</summary><div class="text-break">{{view.agentSpaceId}}</div><div>{{view.observedAt?new Date(view.observedAt).toLocaleString():'未观测'}} · 绑定版本 {{view.bindingRevision}}</div></details>
   </template>
  </q-card-section>
  <WorkspaceFiles v-if="showFiles&&view?.agentSpaceId" :key="view.agentSpaceId" :base-url="url" :space-id="view.agentSpaceId" :available="serviceEnabled&&view.filesAvailable"/>
  <details v-if="view?.webdav" class="workspace-details"><summary>客户端 WebDAV <span class="text-grey-7">{{view.webdav.available ? (view.webdav.accessMode==='READ_ONLY'?'只读':'完整') : workspaceLabel(view.webdav.reason)}}</span><q-chip v-if="view.fileExchangeWebdavResourceId" dense size="sm" color="blue-1">默认文件交换空间</q-chip></summary>
    <WorkspaceDAVAccess :key="userId" :workspace="view" :base-url="url" :disabled="busy || !!pendingOperation || !serviceEnabled || view.state !== 'CONNECTED'" @changed="refresh" />
  </details>
  <details v-if="view?.agentSpaceId" :open="showResources" class="workspace-details" data-cy="workspace-resource-details" @toggle="showResources=($event.target as HTMLDetailsElement).open"><summary>资源摘要与容量</summary><WorkspaceResources v-if="showResources" :key="view.agentSpaceId" :base-url="url" :space-id="view.agentSpaceId" :revision="`${view.bindingRevision}:${view.state}:${view.serviceState}`"/></details>
  <q-dialog :model-value="!!confirm" :persistent="busy" @update:model-value="v=>{if(!v){confirm=undefined;recoveryBearer='';recoverySecret=undefined}}"><q-card class="app-dialog app-dialog--sm"><q-card-section class="text-subtitle1 text-weight-medium">{{actionLabels[confirm??'']}}</q-card-section><q-card-section class="app-dialog__body workspace-form-fields"><ProblemBanner :error="error"/>
   <div v-if="confirm==='CREATE'">为此用户创建独立空间。首次工具或文件访问时启动运行环境。</div>
   <div v-if="confirm==='DISCONNECT'">停止工作区并撤销 MCP 和 WebDAV 访问。账号和文件保留。</div>
   <div v-if="confirm==='RESTORE'">恢复原空间与 MCP 访问。此前已授权的文件访问会自动恢复；明确撤销的文件授权保持撤销。</div>
   <template v-if="confirm==='DELETE'"><q-banner dense class="bg-red-1">将永久删除该空间及其中全部文件。此操作不可撤销。</q-banner><div class="text-break">{{view?.agentSpaceId}}</div><q-input v-model="confirmation" outlined label="输入完整空间 ID 确认"/></template>
   <div v-if="confirm==='SET_DAV'">新的文件凭据将替换旧值，Files 和平台 WebDAV 会自动使用新凭据。使用原始连接信息的外部客户端需手工更新 Token。</div>
   <div v-if="confirm==='RESET_MCP'">更换 MCP 凭据并关闭旧工具连接，原云桌面会话也会失效。外部 MCP 客户端和云桌面登录需使用新 Token；空间文件与 DAV 配置保留。</div>
   <div v-if="confirm==='REVOKE_DAV'">撤销全部文件访问，包括 Console Files、平台 WebDAV 和原始外部连接；不会自动恢复。MCP 保持原状态。若只需关闭平台 WebDAV，请在企业配置中关闭该资源并发布。</div>
   <template v-if="confirm==='TAKEOVER'||confirm==='CONTINUE'&&operation?.step==='CREATE_SENT'"><q-input v-model="remoteUsername" outlined label="现有远端账号"/><q-input v-model="spaceId" outlined label="原 agentSpaceId"/><div>{{confirm==='TAKEOVER'?'确认原控制方停止编排且旧管理请求已结束。接管会撤销原凭据，保留原空间和文件。':'确认原创建请求已结束，并核对原账号与空间。仅继续当前停用或删除清理，不会恢复访问。'}}</div></template>
   <p v-if="confirm==='CONTINUE'&&davPending">核实原请求已结束后，将重新设置当前空间的文件凭据。已有外部 WebDAV 客户端可能需要更新 Token；不会删除空间文件。</p>
   <q-input v-if="confirm==='CONTINUE'" v-model="recoveryBearer" type="password" autocomplete="new-password" outlined label="新的管理凭据（仅原凭据失效时填写）" hint="只用于原目标的当前操作。完成后请同步服务连接配置。"/>
   <p v-if="confirm==='DISCONNECT'&&canVerifyDisconnect">请先核实原 DAV 请求已经结束及原账号、空间身份。断开将结束该待处理操作并撤销访问，保留空间和文件；之后可以修正连接配置再恢复。</p>
   <q-input v-if="needsEvidence" v-model="evidence" type="textarea" outlined label="核实依据（必填）"/>
  </q-card-section><q-card-actions align="right"><q-btn flat label="取消" v-close-popup :disable="busy"/><q-btn :color="confirm==='DELETE'?'negative':'primary'" label="确认" :loading="busy" :disable="confirm==='DELETE'&&confirmation!==view?.agentSpaceId||needsEvidence&&!evidence.trim()" @click="confirm&&command(confirm)"/></q-card-actions></q-card></q-dialog>
  <q-dialog v-model="connectionOpen">
   <q-card class="app-dialog workspace-connection-dialog" data-cy="workspace-connection-dialog">
    <q-card-section class="text-subtitle1 text-weight-medium">{{connectionKind==='dav'?'WebDAV 连接信息':'MCP / 云桌面凭据'}}</q-card-section>
    <q-card-section class="app-dialog__body">
     <p class="text-caption text-grey-7 q-mb-sm">{{connectionKind==='dav'?'原始直连具有完整文件权限，不受平台只读限制。':'完整 Token 用于 MCP Bearer 认证，也用于云桌面登录。'}}</p>
     <ProblemBanner :error="error"/>
     <LoadingState v-if="revealing" label="正在读取当前凭据…"/>
     <div v-else-if="!connection" class="q-mb-sm"><q-btn outline dense label="重新读取" :disable="connectionKind==='dav'?!view?.filesAvailable:!canRevealMCP(view)" @click="reveal"/></div>
     <div v-if="davConnection" class="workspace-form-fields">
      <q-input :model-value="davConnection.davUrl" readonly outlined dense label="DAV 地址"><template #append><q-btn flat round dense icon="content_copy" aria-label="复制 DAV 地址" @click="copy(davConnection.davUrl)"/></template></q-input>
      <q-input :model-value="davConnection.username" readonly outlined dense label="用户名"/>
      <q-input :model-value="davConnection.token" :type="showToken?'text':'password'" readonly outlined dense label="Token"><template #append><q-btn flat round dense :icon="showToken?'visibility_off':'visibility'" :aria-label="showToken?'隐藏 Token':'显示 Token'" @click="showToken=!showToken"/><q-btn flat round dense icon="content_copy" aria-label="复制完整 Token" @click="copy(davConnection.token)"/></template></q-input>
     </div>
     <div v-if="mcpConnection" class="workspace-form-fields">
      <q-input :model-value="mcpConnection.mcpUrl" readonly outlined dense label="MCP 地址"><template #append><q-btn flat round dense icon="content_copy" aria-label="复制 MCP 地址" @click="copy(mcpConnection.mcpUrl)"/></template></q-input>
      <q-input :model-value="mcpConnection.username" readonly outlined dense label="用户名"/>
      <q-input :model-value="mcpConnection.token" :type="showToken?'text':'password'" readonly outlined dense label="MCP Token"><template #append><q-btn flat round dense :icon="showToken?'visibility_off':'visibility'" :aria-label="showToken?'隐藏 MCP Token':'显示 MCP Token'" @click="showToken=!showToken"/><q-btn flat round dense icon="content_copy" aria-label="复制完整 MCP Token" @click="copy(mcpConnection.token)"/></template></q-input>
     </div>
     <details class="text-caption text-grey-7 q-mt-sm"><summary>直连说明</summary><p class="q-mt-xs q-mb-none">直连不受 Core 发布、预算和用量统计约束。{{connectionKind==='dav'?'普通客户端请使用平台下发的 WebDAV 配置，外部客户端使用此处用户名和完整 Token。':'查看不更换凭据。云桌面是否可用取决于空间配置。'}}</p></details>
    </q-card-section>
    <q-card-actions align="between" class="workspace-connection-actions">
     <div v-if="connectionKind==='dav'" class="row q-gutter-xs"><q-btn flat dense label="签发 / 重设 Token" :disable="revealing||busy||!!pendingOperation" @click="connectionOpen=false;ask('SET_DAV')"/><q-btn flat dense color="negative" label="撤销 Token" :disable="revealing||busy||!!pendingOperation||!view?.filesAvailable" @click="connectionOpen=false;ask('REVOKE_DAV')"/></div>
     <q-btn v-else flat dense label="重新签发 MCP 凭据" :disable="revealing||busy||!!pendingOperation||!canRevealMCP(view)" @click="connectionOpen=false;ask('RESET_MCP')"/>
     <q-btn flat dense color="primary" label="关闭" v-close-popup/>
    </q-card-actions>
   </q-card>
  </q-dialog>
 </q-card>
</template>

<style scoped>
.workspace-details{border-top:1px solid #e0e4eb;padding:8px 12px}.workspace-details>summary{cursor:pointer;display:list-item;font-size:13px}.workspace-details>summary span{margin-left:8px}.workspace-maintenance{align-self:center;padding:6px 4px}.workspace-maintenance>summary{cursor:pointer;font-size:13px}.workspace-details :deep(.workspace-resources){border:0;margin:8px 0 0;padding:0}
.workspace-panel-heading{display:flex;align-items:center;justify-content:space-between;gap:8px}.workspace-panel-heading>:first-child{min-width:0;overflow-wrap:anywhere}.workspace-panel-heading>:last-child{flex-shrink:0}
.workspace-form-fields{display:grid;gap:8px;min-width:0}.workspace-connection-dialog{overflow-wrap:anywhere}.workspace-connection-dialog :deep(.q-field){min-width:0}.workspace-connection-actions{border-top:1px solid #e1ddea}
.workspace-connection-dialog :deep(.q-btn--dense){min-height:36px}.workspace-connection-actions>div{max-width:100%}
</style>
