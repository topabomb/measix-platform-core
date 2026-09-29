<script setup lang="ts">
import { computed, defineAsyncComponent, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { apiFetch } from '../api/client'
import type { components } from '../api/generated'
import { useSessionStore } from '../stores/session'
import ProblemBanner from './ProblemBanner.vue'
const WorkspacePreview=defineAsyncComponent(()=>import('./WorkspacePreview.vue'))
type Entry=components['schemas']['WorkspaceFileEntry']
type Mutation=components['schemas']['WorkspaceFileMutation']
const props=defineProps<{baseUrl:string;spaceId:string}>(),session=useSessionStore()
const directory=ref(''),list=ref<components['schemas']['WorkspaceFileList']>(),error=ref<unknown>(),busy=ref(false)
const preview=ref<Entry>(),selected=ref<Entry>(),action=ref<Mutation['action']>(),destination=ref(''),recursive=ref(false),overwrite=ref(false),uploadOverwrite=ref(false)
const result=ref<components['schemas']['WorkspaceFileResult']>(),uploadFile=ref<File>(),progress=ref<number>(),transfer=ref('')
const controller=new AbortController();let transferController:AbortController|undefined,xhr:XMLHttpRequest|undefined,alive=true,listRevision=0
const entries=computed(()=>[...(list.value?.entries??[])].sort((a,b)=>a.kind===b.kind?a.path.localeCompare(b.path):a.kind==='DIRECTORY'?-1:1))
const name=(path:string)=>path.split('/').filter(Boolean).pop()??path
const join=(value:string)=>directory.value?directory.value+'/'+value:value
watch([uploadFile,directory,()=>list.value?.entries.find(e=>e.path===join(uploadFile.value?.name??''))?.etag], () => { uploadOverwrite.value=false })
function validDestination(value:string){return !!value&&!value.startsWith('/')&&!value.split('/').some(part=>!part||part==='.'||part==='..'||/[\\\r\n\0]/.test(part))}
async function load(path=directory.value){
 const revision=++listRevision;busy.value=true;error.value=undefined
 try{const value=await apiFetch<components['schemas']['WorkspaceFileList']>(props.baseUrl+'/files?path='+encodeURIComponent(path),{signal:controller.signal});if(alive&&revision===listRevision){directory.value=path;list.value=value}}catch(e){if(alive)error.value=e}finally{if(revision===listRevision)busy.value=false}
}
function ask(kind:Mutation['action'],entry?:Entry){action.value=kind;selected.value=entry;destination.value=kind==='MOVE'&&entry?entry.path:'';recursive.value=false;overwrite.value=false;result.value=undefined}
async function mutate(){
 if(!action.value||!session.csrfToken||busy.value)return
 busy.value=true;error.value=undefined
 const body:Mutation={action:action.value,path:action.value==='MKCOL'?join(destination.value):selected.value!.path}
 if(body.action==='MOVE'||body.action==='COPY'){
  body.destination=destination.value;body.overwrite=overwrite.value
  if(overwrite.value){
   const parent=destination.value.split('/').slice(0,-1).join('/')
   try{const targetList=await apiFetch<components['schemas']['WorkspaceFileList']>(props.baseUrl+'/files?path='+encodeURIComponent(parent),{signal:controller.signal});const target=targetList.entries.find(e=>e.path===destination.value);if(!target?.etag||target.kind!=='FILE')throw new Error('仅支持覆盖已确认版本的普通文件。');body.targetEtag=target.etag}catch(e){error.value=e;busy.value=false;return}
  }
 }
 if(selected.value?.etag)body.sourceEtag=selected.value.etag
 if(body.action==='DELETE')body.recursiveConfirmed=recursive.value
 busy.value=true;error.value=undefined
 try{result.value=await apiFetch(props.baseUrl+'/files',{method:'POST',body:JSON.stringify(body),signal:controller.signal},session.csrfToken);action.value=undefined;await load()}catch(e){if(alive)error.value=e}finally{busy.value=false}
}
function cancel(){xhr?.abort();transferController?.abort()}
async function upload(){
 const file=uploadFile.value;if(!file||!session.csrfToken)return
 const existing=list.value?.entries.find(e=>e.path===join(file.name))
 if(existing&&(!uploadOverwrite.value||!existing.etag||existing.kind!=='FILE')){error.value=new Error('同名文件已存在。确认覆盖后再上传，目录不能覆盖。');return}
 transfer.value='上传 '+file.name;progress.value=0;error.value=undefined
 const request=new XMLHttpRequest();xhr=request
 try{await new Promise<void>((resolve,reject)=>{
  request.open('PUT',props.baseUrl+'/content?path='+encodeURIComponent(join(file.name)))
  request.setRequestHeader('X-CSRF-Token',session.csrfToken!)
  request.setRequestHeader('Content-Type','application/octet-stream')
  request.setRequestHeader(existing?'If-Match':'If-None-Match',existing?.etag??'*')
  request.upload.onprogress=e=>{if(e.lengthComputable)progress.value=e.loaded/e.total}
  request.onload=()=>{if(request.status>=200&&request.status<300)resolve();else reject(new Error(`上传失败 (${request.status})；请刷新目录确认远端状态，再决定是否重试。`))}
  request.onerror=()=>reject(new Error('连接中断；上传结果未知，请刷新目录核实。'))
  request.onabort=()=>reject(new Error('已取消上传；请刷新目录核实远端结果。'))
  request.send(file)
 });uploadFile.value=undefined;await load()}catch(e){if(alive)error.value=e}finally{xhr=undefined;transfer.value='';progress.value=undefined}
}
async function download(entry:Entry){
 // A native download streams through the authenticated proxy without buffering the file in JS.
 const anchor=document.createElement('a');anchor.href=props.baseUrl+'/content?path='+encodeURIComponent(entry.path);anchor.download=name(entry.path);anchor.rel='noopener';anchor.click()
}
onMounted(()=>load())
onBeforeUnmount(()=>{alive=false;controller.abort();cancel();preview.value=undefined})
</script>
<template>
 <q-separator/><q-card-section data-cy="workspace-files"><div class="row items-center q-gutter-sm"><div class="text-subtitle1 col">文件</div><q-btn flat dense icon="refresh" label="刷新" :disable="busy" @click="load()"/><q-btn outline dense icon="create_new_folder" label="新建目录" :disable="busy||!!transfer" @click="ask('MKCOL')"/></div>
  <ProblemBanner :error="error"/>
  <div class="row items-center q-my-sm"><q-btn flat dense label="根目录" @click="load('')"/><q-btn v-if="directory" flat dense icon="arrow_upward" label="上一级" @click="load(directory.split('/').slice(0,-1).join('/'))"/><span class="text-caption text-break">/{{directory}}</span></div>
  <div v-if="list?.usedBytes!==undefined" class="text-caption text-grey-7">已用 {{(list.usedBytes/1048576).toFixed(1)}} MiB<span v-if="list.availableBytes!==undefined"> · 可用 {{(list.availableBytes/1048576).toFixed(1)}} MiB</span></div>
  <q-linear-progress v-if="busy" indeterminate/>
  <q-list bordered separator class="rounded-borders q-my-sm">
   <q-item v-for="entry in entries" :key="entry.path"><q-item-section avatar><q-icon :name="entry.kind==='DIRECTORY'?'folder':'description'"/></q-item-section><q-item-section role="button" tabindex="0" :aria-label="(entry.kind==='DIRECTORY'?'打开目录 ':'预览 ')+name(entry.path)" @keydown.enter="entry.kind==='DIRECTORY'?load(entry.path):preview=entry" @keydown.space.prevent="entry.kind==='DIRECTORY'?load(entry.path):preview=entry" @click="entry.kind==='DIRECTORY'?load(entry.path):preview=entry"><q-item-label class="cursor-pointer text-break">{{name(entry.path)}}</q-item-label><q-item-label caption>{{entry.kind==='DIRECTORY'?'目录':entry.size===undefined?'大小未知':(entry.size/1024).toFixed(1)+' KiB'}}<span v-if="entry.modifiedAt"> · {{new Date(entry.modifiedAt).toLocaleString()}}</span></q-item-label></q-item-section><q-item-section side><q-btn flat round dense icon="more_vert" :aria-label="name(entry.path)+' 操作'"><q-menu><q-list style="min-width:150px"><q-item v-if="entry.kind==='FILE'" clickable v-close-popup @click="preview=entry"><q-item-section>预览</q-item-section></q-item><q-item v-if="entry.kind==='FILE'" clickable v-close-popup @click="download(entry)"><q-item-section>下载</q-item-section></q-item><q-item clickable v-close-popup @click="ask('MOVE',entry)"><q-item-section>重命名 / 移动</q-item-section></q-item><q-item clickable v-close-popup @click="ask('COPY',entry)"><q-item-section>复制</q-item-section></q-item><q-item clickable v-close-popup @click="ask('DELETE',entry)"><q-item-section class="text-negative">删除</q-item-section></q-item></q-list></q-menu></q-btn></q-item-section></q-item>
   <q-item v-if="list&&!entries.length&&!busy"><q-item-section class="text-grey-7">目录为空</q-item-section></q-item>
  </q-list>
  <div class="row items-center q-gutter-sm"><q-file v-model="uploadFile" dense outlined label="选择上传文件" class="col" :disable="!!transfer"/><q-btn color="primary" label="上传" :disable="!uploadFile||!!transfer||busy" @click="upload"/></div><q-checkbox v-if="uploadFile&&list?.entries.some(e=>e.path===join(uploadFile!.name))" v-model="uploadOverwrite" label="确认覆盖当前同名文件版本"/>
  <div v-if="transfer" class="q-mt-sm">{{transfer}}<q-linear-progress :value="progress"/><q-btn flat label="取消传输" @click="cancel"/></div>
  <q-banner v-if="result&&result.outcome!=='SUCCEEDED'" class="bg-orange-1 q-mt-sm">操作{{result.outcome==='PARTIAL'?'部分完成':'结果未知'}}，请刷新后核实。<div v-for="failure in result.failures" :key="failure.path">{{failure.path}} · {{failure.status}}</div><div v-if="result.truncated">仅显示部分失败项。</div></q-banner>
 </q-card-section>
 <q-dialog :model-value="!!preview" @update:model-value="v=>{if(!v)preview=undefined}"><WorkspacePreview v-if="preview" :key="spaceId+'/'+preview.path" :base-url="baseUrl" :path="preview.path" :etag="preview.etag" @close="preview=undefined"/></q-dialog>
 <q-dialog :model-value="!!action" @update:model-value="v=>{if(!v)action=undefined}"><q-card style="width:500px;max-width:95vw"><q-card-section class="text-h6">{{action==='MKCOL'?'新建目录':action==='DELETE'?'删除文件或目录':action==='COPY'?'复制':'重命名 / 移动'}}</q-card-section><q-card-section class="q-gutter-sm"><ProblemBanner :error="error"/><div class="text-break">{{selected?.path}}</div><q-input v-if="action!=='DELETE'" v-model="destination" outlined :label="action==='MKCOL'?'当前目录下的新目录名':'目标完整相对路径（从根目录起）'"/><q-checkbox v-if="['MOVE','COPY'].includes(action??'')&&selected?.kind==='FILE'" v-model="overwrite" label="确认覆盖目标普通文件的当前版本"/><q-banner v-if="action==='DELETE'" class="bg-red-1">删除不可撤销。</q-banner><q-checkbox v-if="action==='DELETE'&&selected?.kind==='DIRECTORY'" v-model="recursive" label="确认递归删除目录及其所有内容"/></q-card-section><q-card-actions align="right"><q-btn flat label="取消" v-close-popup/><q-btn :color="action==='DELETE'?'negative':'primary'" label="确认" :loading="busy" :disable="action==='DELETE'?selected?.kind==='DIRECTORY'&&!recursive:!validDestination(destination)" @click="mutate"/></q-card-actions></q-card></q-dialog>
</template>
