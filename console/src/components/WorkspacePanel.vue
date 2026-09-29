<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { copyToClipboard } from 'quasar'
import { apiFetch, createIdempotencyKey } from '../api/client'
import type { components } from '../api/generated'
import { useSessionStore } from '../stores/session'
import ProblemBanner from './ProblemBanner.vue'
import WorkspaceFiles from './WorkspaceFiles.vue'
import { workspaceLabel } from '../composables/workspaceLabels'

const props=withDefaults(defineProps<{userId:string;displayName?:string;serviceEnabled?:boolean}>(),{serviceEnabled:false})
const session=useSessionStore()
type Projection=components['schemas']['WorkspaceProjection']
type Command=components['schemas']['WorkspaceCommand']
const view=ref<Projection>(),error=ref<unknown>(),busy=ref(false)
const operation=ref<components['schemas']['WorkspaceOperation']>()
const confirm=ref<Command['action']>(),confirmation=ref(''),evidence=ref(''),remoteUsername=ref(''),spaceId=ref('')
const connectionOpen=ref(false),connection=ref<components['schemas']['WorkspaceDAVConnection']>(),revealing=ref(false)
const showFiles=ref(false),showToken=ref(false)
const recoveryBearer=ref(''),recoverySecret=ref<components['schemas']['SecretRef']>()
const url=`/api/admin/v1/users/${encodeURIComponent(props.userId)}/workspace`
const labels:Record<string,string>={UNPROVISIONED:'未开通',CONNECTING:'开通中',CONNECTED:'已连接',DISCONNECTING:'断开中',DISCONNECTED:'已断开',RESTORING:'恢复中',DELETING:'删除中',DELETED:'已删除',NEEDS_ATTENTION:'需要处理'}
const actionLabels:Record<string,string>={CREATE:'开通远程工作区',DISCONNECT:'断开工作区',RESTORE:'恢复连接',DELETE:'删除远程工作区',TAKEOVER:'显式接管',RESET_MCP:'重新签发 MCP 凭据',SET_DAV:'签发 / 重设 WebDAV Token',REVOKE_DAV:'撤销 WebDAV Token',CONTINUE:'核实后继续'}
const pendingOperation=computed(()=>operation.value&&operation.value.state!=='COMPLETED')
let alive=true,timer:ReturnType<typeof setTimeout>|undefined,controller=new AbortController()
let pending:{payload:string;key:string}|undefined
async function refresh(){
 try{const next=await apiFetch<Projection>(url,{signal:controller.signal});if(!alive)return;if(!next.filesAvailable||next.bindingRevision!==view.value?.bindingRevision){connection.value=undefined;showToken.value=false}view.value=next
 if(next.operationId)operation.value=await apiFetch(`${'/api/admin/v1/workspace-operations/'}${next.operationId}`,{signal:controller.signal})
 else if(operation.value?.state!=='COMPLETED')operation.value=undefined
 }catch(e){if(alive)error.value=e}
}
function schedule(){timer=setTimeout(async()=>{await refresh();if(alive)schedule()},3000)}
function ask(action:Command['action']){confirm.value=action;confirmation.value='';evidence.value='';connection.value=undefined;recoveryBearer.value='';recoverySecret.value=undefined}
async function command(action:Command['action']){
 if(!view.value||!session.csrfToken)return
 busy.value=true;error.value=undefined
 const body:Command={action,expectedRevision:view.value.bindingRevision,...(confirmation.value?{confirmation:confirmation.value}:{}),...(evidence.value?{evidence:evidence.value}:{}),...(action==='TAKEOVER'||action==='CONTINUE'&&operation.value?.step==='CREATE_SENT'?{remoteUsername:remoteUsername.value,agentSpaceId:spaceId.value}:{})}
 try{
  if(action==='CONTINUE'){
   if(recoveryBearer.value){const secret=await apiFetch<components['schemas']['Secret']>('/api/admin/v1/secrets',{method:'POST',body:JSON.stringify({name:'Remote workspace recovery management',value:recoveryBearer.value}),signal:controller.signal},session.csrfToken);recoverySecret.value={secretId:secret.secretId,secretVersion:secret.secretVersion};recoveryBearer.value=''}
   if(recoverySecret.value)body.managementSecret=recoverySecret.value
  }
  const payload=JSON.stringify(body);if(pending?.payload!==payload)pending={payload,key:createIdempotencyKey()}
  const result=await apiFetch<components['schemas']['WorkspaceOperation']>(url,{method:'POST',body:payload,headers:{'Idempotency-Key':pending.key},signal:controller.signal},session.csrfToken);if(!alive)return;operation.value=result;pending=undefined;confirm.value=undefined;connection.value=undefined;recoverySecret.value=undefined;await refresh()
 }catch(e){if(alive)error.value=e}finally{busy.value=false}
}
async function reveal(){
 if(!session.csrfToken)return;revealing.value=true;error.value=undefined
 try{const value=await apiFetch<components['schemas']['WorkspaceDAVConnection']>(url+'/dav-connection',{method:'POST',signal:controller.signal},session.csrfToken);if(alive&&connectionOpen.value)connection.value=value}catch(e){if(alive)error.value=e}finally{revealing.value=false}
}
async function copy(value:string){try{await copyToClipboard(value)}catch(e){error.value=e}}
onMounted(async()=>{await refresh();if(alive)schedule()})
onBeforeUnmount(()=>{alive=false;controller.abort();if(timer)clearTimeout(timer);connection.value=undefined;recoveryBearer.value='';recoverySecret.value=undefined})
</script>
<template>
 <q-card flat bordered data-cy="workspace-panel">
  <q-card-section><div class="row items-center justify-between"><div><div class="text-h6">远程工作区<span v-if="displayName"> · {{displayName}}</span></div><div class="text-caption text-grey-7">{{view?labels[view.state]:'正在加载'}}</div></div><q-btn flat round icon="refresh" aria-label="刷新工作区" :disable="busy" @click="refresh"/></div></q-card-section>
  <q-card-section class="q-pt-none"><ProblemBanner :error="error"/>
   <template v-if="view"><q-banner v-if="!serviceEnabled" dense class="bg-grey-2 q-mb-sm">远程工作区服务未启用，不能开通或恢复连接。已有空间仍可核查和删除。</q-banner>
    <div class="row q-gutter-sm q-my-sm"><q-chip dense :color="view.mcpAvailable?'green-1':'grey-2'">{{view.mcpAvailable?'MCP 可用':view.mcpReason==='mcp_not_published'?'MCP 未发布（可选）':'MCP 不可用'}}</q-chip><q-chip dense :color="view.filesAvailable?'green-1':'grey-2'">文件 {{view.filesAvailable?'可用':'不可用'}}</q-chip></div>
    <div v-if="!view.mcpAvailable||!view.filesAvailable" class="text-caption text-grey-7 q-mb-sm">MCP：{{workspaceLabel(view.mcpReason)}} · 文件：{{workspaceLabel(view.filesReason)}}</div>
    <q-banner v-if="serviceEnabled && view.state==='CONNECTED' && view.filesReason==='dav_not_configured'" dense class="bg-grey-2 q-mb-sm">请先在远程工作区服务配置中填写文件服务地址，再签发 WebDAV Token。</q-banner>
    <div class="row q-gutter-xs">
     <q-btn v-if="serviceEnabled && view.state==='UNPROVISIONED' && view.mcpReason!=='user_unavailable'" color="primary" label="开通工作区" :disable="busy" @click="ask('CREATE')"/>
     <q-btn v-if="serviceEnabled && (view.state==='UNPROVISIONED'||operation?.step==='CREATE_SENT'&&operation.state==='UNKNOWN') && view.mcpReason!=='user_unavailable'" outline label="接管已有空间" :disable="busy" @click="ask('TAKEOVER')"/>
     <q-btn v-if="serviceEnabled && view.filesAvailable" color="primary" :label="showFiles?'收起文件':'浏览文件'" @click="showFiles=!showFiles"/>
     <q-btn v-if="view.state==='CONNECTED'" outline label="断开" :disable="busy||!!pendingOperation" @click="ask('DISCONNECT')"/>
     <q-btn v-if="serviceEnabled && view.state==='DISCONNECTED' && view.mcpReason!=='user_unavailable'" color="primary" label="恢复连接" :disable="busy||!!pendingOperation" @click="ask('RESTORE')"/>
     <q-btn v-if="serviceEnabled && view.state==='CONNECTED' && view.filesReason!=='dav_not_configured'" outline label="WebDAV 连接信息" :disable="busy" @click="connectionOpen=true"/>
     <q-btn v-if="view.agentSpaceId&&['CONNECTED','DISCONNECTED'].includes(view.state)" flat color="negative" label="删除空间" :disable="busy||!!pendingOperation" @click="ask('DELETE')"/>
    </div>
    <q-banner v-if="operation" dense class="bg-blue-1 q-mt-sm">{{workspaceLabel(operation.state)}}<div v-if="operation.diagnosticCode">{{workspaceLabel(operation.diagnosticCode)}}</div><details class="text-caption"><summary>操作详情</summary>{{operation.operationId}} · {{operation.step}}</details><div v-if="['UNKNOWN','NEEDS_ATTENTION'].includes(operation.state)" class="q-mt-xs">请先核实远端目标及此前在途写入。<q-btn flat dense label="重新查询" @click="command('REQUERY')"/><q-btn v-if="operation.step!=='CREATE_SENT'||!serviceEnabled||view.mcpReason==='user_unavailable'" flat dense label="核实后继续" @click="ask('CONTINUE')"/></div></q-banner>
    <details v-if="serviceEnabled && view.state==='CONNECTED' && view.mcpReason!=='mcp_not_published'" class="text-caption q-mt-sm"><summary>工具连接维护</summary><p>工具凭据失效时可重新签发；此操作会关闭旧工具连接，不改变文件授权。</p><q-btn flat label="重新签发 MCP 凭据" :disable="busy||!!pendingOperation" @click="ask('RESET_MCP')"/></details>
    <details v-if="view.agentSpaceId" class="text-caption text-grey-7 q-mt-sm"><summary>空间标识与最近观测</summary><div class="text-break">{{view.agentSpaceId}}</div><div>{{view.observedAt?new Date(view.observedAt).toLocaleString():'未观测'}} · 绑定版本 {{view.bindingRevision}}</div></details>
   </template>
  </q-card-section>
  <WorkspaceFiles v-if="serviceEnabled&&showFiles&&view?.filesAvailable&&view.agentSpaceId" :key="view.agentSpaceId+'/'+view.bindingRevision" :base-url="url" :space-id="view.agentSpaceId"/>
  <q-dialog :model-value="!!confirm" :persistent="busy" @update:model-value="v=>{if(!v){confirm=undefined;recoveryBearer='';recoverySecret=undefined}}"><q-card style="width:520px;max-width:95vw"><q-card-section class="text-h6">{{actionLabels[confirm??'']}}</q-card-section><q-card-section class="q-gutter-sm"><ProblemBanner :error="error"/>
   <div v-if="confirm==='CREATE'">为此用户创建独立空间。首次工具或文件访问时启动运行环境。</div>
   <div v-if="confirm==='DISCONNECT'">停止工作区并撤销 MCP 和 WebDAV 访问。账号和文件保留。</div>
   <div v-if="confirm==='RESTORE'">恢复原空间与 MCP 访问。WebDAV 需要另行签发新 Token。</div>
   <template v-if="confirm==='DELETE'"><q-banner dense class="bg-red-1">将永久删除该空间及其中全部文件。此操作不可撤销。</q-banner><div class="text-break">{{view?.agentSpaceId}}</div><q-input v-model="confirmation" outlined label="输入完整空间 ID 确认"/></template>
   <div v-if="confirm==='SET_DAV'">新的 Token 将替换旧值。外部 WebDAV 客户端必须更新配置。</div>
   <div v-if="confirm==='RESET_MCP'">更换 MCP 凭据并关闭旧工具连接，保留空间文件与 DAV 配置。</div>
   <div v-if="confirm==='REVOKE_DAV'">撤销文件访问及外部 WebDAV 客户端凭据；MCP 保持原状态。</div>
   <template v-if="confirm==='TAKEOVER'||confirm==='CONTINUE'&&operation?.step==='CREATE_SENT'"><q-input v-model="remoteUsername" outlined label="现有远端账号"/><q-input v-model="spaceId" outlined label="原 agentSpaceId"/><div>{{confirm==='TAKEOVER'?'确认原控制方停止编排且旧管理请求已结束。接管会撤销原凭据，保留原空间和文件。':'确认原创建请求已结束，并核对原账号与空间。仅继续当前停用或删除清理，不会恢复访问。'}}</div></template>
   <q-input v-if="confirm==='CONTINUE'" v-model="recoveryBearer" type="password" autocomplete="new-password" outlined label="新的管理凭据（仅原凭据失效时填写）" hint="只用于原目标的当前操作。完成后请同步服务连接配置。"/>
   <q-input v-if="confirm==='TAKEOVER'||confirm==='CONTINUE'" v-model="evidence" type="textarea" outlined label="核实依据（必填）"/>
  </q-card-section><q-card-actions align="right"><q-btn flat label="取消" v-close-popup :disable="busy"/><q-btn :color="confirm==='DELETE'?'negative':'primary'" label="确认" :loading="busy" :disable="confirm==='DELETE'&&confirmation!==view?.agentSpaceId||['TAKEOVER','CONTINUE'].includes(confirm??'')&&!evidence.trim()" @click="confirm&&command(confirm)"/></q-card-actions></q-card></q-dialog>
  <q-dialog v-model="connectionOpen" @hide="connection=undefined;showToken=false"><q-card style="width:600px;max-width:95vw"><q-card-section class="text-h6">WebDAV 连接信息</q-card-section><q-card-section>
   <ProblemBanner :error="error"/><p>管理员可将连接信息交付给用户。外部客户端使用用户名和完整 Token，不使用企业登录密码。</p>
   <q-btn v-if="!connection" outline label="查看当前连接信息" :loading="revealing" :disable="!view?.filesAvailable" @click="reveal"/>
   <div v-else class="q-gutter-sm"><q-input :model-value="connection.davUrl" readonly outlined label="DAV 地址"><template #append><q-btn flat round icon="content_copy" aria-label="复制 DAV 地址" @click="copy(connection.davUrl)"/></template></q-input><q-input :model-value="connection.username" readonly outlined label="用户名"/><q-input :model-value="connection.token" :type="showToken?'text':'password'" readonly outlined label="Token"><template #append><q-btn flat round :icon="showToken?'visibility_off':'visibility'" :aria-label="showToken?'隐藏 Token':'显示 Token'" @click="showToken=!showToken"/><q-btn flat round icon="content_copy" aria-label="复制完整 Token" @click="copy(connection.token)"/></template></q-input></div>
   <div class="row q-gutter-xs q-mt-md"><q-btn outline label="签发 / 重设 Token" :disable="busy||!!pendingOperation" @click="connectionOpen=false;ask('SET_DAV')"/><q-btn flat color="negative" label="撤销 Token" :disable="busy||!!pendingOperation||!view?.filesAvailable" @click="connectionOpen=false;ask('REVOKE_DAV')"/></div>
  </q-card-section><q-card-actions align="right"><q-btn flat label="关闭" v-close-popup/></q-card-actions></q-card></q-dialog>
 </q-card>
</template>
