<script setup lang="ts">
import { computed, defineAsyncComponent, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { ApiProblem, apiFetch } from '../api/client'
import type { components } from '../api/generated'
import { useSessionStore } from '../stores/session'
import { downloadWorkspaceFile, workspaceFileError, workspaceFileUrl, workspaceBytes } from '../composables/workspaceFiles'
import { previewKind } from '../composables/workspacePreview'
import { thumbnailMediaKind, useWorkspaceThumbnails } from '../composables/workspaceThumbnails'
import WorkspaceTextEditor from './WorkspaceTextEditor.vue'
const WorkspacePreview=defineAsyncComponent(()=>import('./WorkspacePreview.vue'))
type Entry=components['schemas']['WorkspaceFileEntry']
type Mutation=components['schemas']['WorkspaceFileMutation']
const props=withDefaults(defineProps<{baseUrl:string;spaceId:string;available?:boolean}>(),{available:true}),session=useSessionStore()
const fileScope=()=>JSON.stringify([props.baseUrl,props.spaceId,session.user?.userId??''])
const fileUrl=(resource:'files'|'content',path='')=>workspaceFileUrl(props.baseUrl,props.spaceId,resource,path)
const editor=ref<{path:string;create:boolean;baseUrl:string;spaceId:string;identity:string}>()
const editorScopeMatches=()=>!!editor.value&&editor.value.baseUrl===props.baseUrl&&editor.value.spaceId===props.spaceId&&editor.value.identity===(session.user?.userId??'')
const editorAvailable=computed(()=>props.available&&editorScopeMatches())
function openEditor(path:string,create:boolean){editor.value={path,create,baseUrl:props.baseUrl,spaceId:props.spaceId,identity:session.user?.userId??''}}
function editorSaved(){if(editorScopeMatches()&&props.available)void load()}
const editable=(path:string)=>['text','markdown'].includes(previewKind(path))
const search=ref('')
const viewMode=ref<'list'|'thumbnails'>('list')
const directory=ref(''),list=ref<components['schemas']['WorkspaceFileList']>(),error=ref<unknown>(),busy=ref(false)
const preview=ref<Entry>(),selected=ref<Entry>(),action=ref<Mutation['action']>(),destination=ref(''),recursive=ref(false),overwrite=ref(false),uploadOverwrite=ref(false)
const result=ref<components['schemas']['WorkspaceFileResult']>(),uploadFile=ref<File>(),progress=ref<number>(),transfer=ref('')
let controller=new AbortController();let transferController:AbortController|undefined,xhr:XMLHttpRequest|undefined,alive=true,listRevision=0
const entries=computed(()=>[...(list.value?.entries??[])].filter(e=>e.path.split('/').pop()!.toLocaleLowerCase().includes((search.value??'').toLocaleLowerCase())).sort((a,b)=>a.kind===b.kind?a.path.localeCompare(b.path):a.kind==='DIRECTORY'?-1:1))
const thumbnails=useWorkspaceThumbnails({identity:()=>session.user?.userId??'',baseUrl:()=>props.baseUrl,spaceId:()=>props.spaceId,available:()=>props.available,entries:()=>entries.value,size:()=>viewMode.value==='list'?128:256})
const name=(path:string)=>path.split('/').filter(Boolean).pop()??path
const join=(value:string)=>directory.value?directory.value+'/'+value:value
watch([uploadFile,directory,()=>list.value?.entries.find(e=>e.path===join(uploadFile.value?.name??''))?.etag], () => { uploadOverwrite.value=false })
function validDestination(value:string){return !!value&&!value.startsWith('/')&&!value.split('/').some(part=>!part||part==='.'||part==='..'||/[\\\r\n\0]/.test(part))}
async function load(path=directory.value){
 if(!props.available||!props.spaceId)return
 if(path!==directory.value)thumbnails.clearDirectory()
 const revision=++listRevision,signal=controller.signal;busy.value=true;error.value=undefined
 try{const value=await apiFetch<components['schemas']['WorkspaceFileList']>(fileUrl('files',path),{signal});if(alive&&!signal.aborted&&revision===listRevision){if(directory.value!==path)search.value='';directory.value=path;list.value=value}}catch(e){if(alive&&!signal.aborted&&revision===listRevision){error.value=e;thumbnails.restoreDirectory()}}finally{if(alive&&!signal.aborted&&revision===listRevision)busy.value=false}
}
function ask(kind:Mutation['action'],entry?:Entry){action.value=kind;selected.value=entry;destination.value=kind==='MOVE'&&entry?entry.path:'';recursive.value=false;overwrite.value=false;result.value=undefined}
async function mutate(){
 if(!props.available||!action.value||!session.csrfToken||busy.value)return
 const signal=controller.signal,scope=fileScope(),csrf=session.csrfToken,url=fileUrl('files'),sourceEtag=selected.value?.etag,current=()=>alive&&props.available&&!signal.aborted&&scope===fileScope()
 busy.value=true;error.value=undefined
 const body:Mutation={action:action.value,path:action.value==='MKCOL'?join(destination.value):selected.value!.path}
 if(body.action==='MOVE'||body.action==='COPY'){
  body.destination=destination.value;body.overwrite=overwrite.value
  if(overwrite.value){
   const parent=destination.value.split('/').slice(0,-1).join('/')
   try{const targetList=await apiFetch<components['schemas']['WorkspaceFileList']>(fileUrl('files',parent),{signal});if(!current())return;const target=targetList.entries.find(e=>e.path===body.destination);if(!target?.etag||target.kind!=='FILE')throw new Error('仅支持覆盖已确认版本的普通文件。');body.targetEtag=target.etag}catch(e){if(current()){error.value=e;busy.value=false}return}
  }
 }
 if(sourceEtag)body.sourceEtag=sourceEtag
 if(body.action==='DELETE')body.recursiveConfirmed=recursive.value
 busy.value=true;error.value=undefined
 try{const value=await apiFetch<components['schemas']['WorkspaceFileResult']>(url,{method:'POST',body:JSON.stringify(body),signal},csrf);if(current()){result.value=value;action.value=undefined;await load()}}catch(e){if(current())error.value=e}finally{if(current())busy.value=false}
}
function cancel(){xhr?.abort();transferController?.abort()}
async function upload(){
 const file=uploadFile.value;if(!props.available||!file||!session.csrfToken)return
 const signal=controller.signal,scope=fileScope(),csrf=session.csrfToken,current=()=>alive&&props.available&&!signal.aborted&&scope===fileScope()
 const existing=list.value?.entries.find(e=>e.path===join(file.name))
 if(existing&&(!uploadOverwrite.value||!existing.etag||existing.kind!=='FILE')){error.value=new Error('同名文件已存在。确认覆盖后再上传，目录不能覆盖。');return}
 transfer.value='上传 '+file.name;progress.value=0;error.value=undefined
 const request=new XMLHttpRequest();xhr=request
 try{await new Promise<void>((resolve,reject)=>{
  request.open('PUT',fileUrl('content',join(file.name)))
  request.setRequestHeader('X-CSRF-Token',csrf)
  request.setRequestHeader('Content-Type','application/octet-stream')
  request.setRequestHeader(existing?'If-Match':'If-None-Match',existing?.etag??'*')
  request.upload.onprogress=e=>{if(e.lengthComputable)progress.value=e.loaded/e.total}
  request.onload=()=>{if(request.status>=200&&request.status<300)resolve();else {let body:Record<string,unknown>={};try{body=JSON.parse(request.responseText)}catch{/* proxy error */}reject(new ApiProblem(request.status,String(body.code??'http_error'),String(body.detail??'上传失败，请刷新核实。')))}}
  request.onerror=()=>reject(new Error('连接中断；上传结果未知，请刷新目录核实。'))
  request.onabort=()=>reject(new Error('已取消上传；请刷新目录核实远端结果。'))
  request.send(file)
 });if(current()){uploadFile.value=undefined;await load()}}catch(e){if(current())error.value=e}finally{if(xhr===request)xhr=undefined;if(current()){transfer.value='';progress.value=undefined}}
}
async function download(entry:Entry){
 downloadWorkspaceFile(props.baseUrl,props.spaceId,entry.path)
}
watch(()=>props.available,available=>{
 if(!available){
  const interrupted=!!transfer.value
  listRevision++;controller.abort();busy.value=false;cancel();preview.value=undefined;transfer.value='';progress.value=undefined
  if(interrupted){uploadFile.value=undefined;uploadOverwrite.value=false;result.value={outcome:'UNKNOWN',failures:[],truncated:false}}
 }else{controller=new AbortController();void load()}
})
watch([()=>props.baseUrl,()=>props.spaceId,()=>session.user?.userId],()=>{
 listRevision++;controller.abort();controller=new AbortController();cancel();preview.value=undefined
 action.value=undefined;selected.value=undefined;destination.value='';recursive.value=false;overwrite.value=false;result.value=undefined
 uploadFile.value=undefined;uploadOverwrite.value=false;transfer.value='';progress.value=undefined
 list.value=undefined;directory.value='';search.value='';error.value=undefined;busy.value=false;void load()
},{flush:'sync'})
onMounted(()=>load())
onBeforeUnmount(()=>{alive=false;controller.abort();cancel();preview.value=undefined})
</script>
<template>
 <q-separator/><q-card-section class="workspace-files" data-cy="workspace-files"><q-banner v-if="!available" class="bg-orange-1">文件访问已失效，请刷新工作区。未保存的文本仍可复制。</q-banner>
  <div class="workspace-file-toolbar"><div class="text-subtitle2">文件 <span v-if="list" class="text-caption text-grey-7">{{entries.length}} 项</span></div><div class="workspace-file-actions">
   <div role="group" aria-label="文件显示方式" class="workspace-view-mode"><q-btn flat dense label="列表" icon="view_list" data-cy="workspace-view-list" :aria-pressed="viewMode==='list'" :color="viewMode==='list'?'primary':undefined" @click="viewMode='list'"/><q-btn flat dense label="缩略图" icon="grid_view" data-cy="workspace-view-thumbnails" :aria-pressed="viewMode==='thumbnails'" :color="viewMode==='thumbnails'?'primary':undefined" @click="viewMode='thumbnails'"/></div>
   <q-btn flat round dense icon="refresh" aria-label="刷新文件列表" :disable="busy||!available" @click="load()"/><q-btn outline dense label="新建文本" :disable="busy||!!transfer||!available" @click="openEditor(join('新文件.txt'),true)"/><q-btn outline dense icon="create_new_folder" label="新建目录" :disable="busy||!!transfer||!available" @click="ask('MKCOL')"/>
  </div></div>
  <q-banner v-if="error" class="bg-orange-1" role="alert">{{workspaceFileError(error)}}</q-banner>
  <div class="workspace-file-navigation"><div class="row items-center"><q-btn flat dense label="根目录" @click="load('')"/><q-btn v-if="directory" flat dense icon="arrow_upward" label="上一级" @click="load(directory.split('/').slice(0,-1).join('/'))"/><span class="text-caption text-break">/{{directory}}</span></div><q-input v-model="search" outlined dense clearable label="筛选当前目录" :disable="!available" @clear="search=''"/></div>
  <div v-if="list?.usedBytes!==undefined" class="text-caption text-grey-7">已用 {{workspaceBytes(list.usedBytes)}}<span v-if="list.availableBytes!==undefined"> · 可用 {{workspaceBytes(list.availableBytes)}}</span></div>
  <q-linear-progress v-if="busy" indeterminate/>

  <q-list bordered :separator="viewMode==='list'" class="workspace-file-list rounded-borders q-my-sm" :class="{'workspace-files-grid':viewMode==='thumbnails'}" aria-label="当前目录文件" data-cy="workspace-file-collection" :data-view="viewMode">
   <q-item v-for="entry in entries" :key="entry.path" dense data-cy="workspace-file-entry" :data-file-path="entry.path">
    <q-item-section class="workspace-file-open" role="button" tabindex="0" :aria-label="(entry.kind==='DIRECTORY'?'打开目录 ':'预览 ')+name(entry.path)" @keydown.enter="available && (entry.kind==='DIRECTORY'?load(entry.path):preview=entry)" @keydown.space.prevent="available && (entry.kind==='DIRECTORY'?load(entry.path):preview=entry)" @click="available && (entry.kind==='DIRECTORY'?load(entry.path):preview=entry)">
     <div class="workspace-file-summary">
      <span :ref="element=>thumbnails.bind(entry,element)" :data-thumbnail-path="entry.path" class="workspace-thumbnail-slot"><img v-if="thumbnails.url(entry)" :key="thumbnails.url(entry)" :src="thumbnails.url(entry)" alt="" class="workspace-thumbnail" @error="thumbnails.failed(entry,$event)"/><q-icon v-else :name="entry.kind==='DIRECTORY'?'folder':'description'"/><span v-if="viewMode==='thumbnails'&&entry.kind==='FILE'&&thumbnailMediaKind(entry.path)==='VIDEO'" class="workspace-video-badge" data-cy="workspace-video-badge"><q-icon name="play_arrow"/>视频</span></span>
      <div class="workspace-file-labels"><q-item-label class="cursor-pointer text-break">{{name(entry.path)}}</q-item-label><q-item-label caption><span v-if="viewMode==='thumbnails'&&entry.kind==='FILE'">{{thumbnailMediaKind(entry.path)==='VIDEO'?'视频':thumbnailMediaKind(entry.path)==='IMAGE'?'图片':'文件'}} · </span>{{entry.kind==='DIRECTORY'?'目录':entry.size===undefined?'大小未知':workspaceBytes(entry.size)}}<span v-if="entry.modifiedAt"> · {{new Date(entry.modifiedAt).toLocaleString()}}</span></q-item-label></div>
     </div>
    </q-item-section>
    <q-item-section side><q-btn flat round dense icon="more_vert" :disable="!available" :aria-label="name(entry.path)+' 操作'"><q-menu><q-list style="min-width:150px"><q-item v-if="entry.kind==='FILE'" clickable v-close-popup @click="preview=entry"><q-item-section>预览</q-item-section></q-item><q-item v-if="entry.kind==='FILE'&&editable(entry.path)" clickable v-close-popup @click="openEditor(entry.path,false)"><q-item-section>编辑文本</q-item-section></q-item><q-item v-if="entry.kind==='FILE'" clickable v-close-popup @click="download(entry)"><q-item-section>下载</q-item-section></q-item><q-item clickable v-close-popup @click="ask('MOVE',entry)"><q-item-section>重命名 / 移动</q-item-section></q-item><q-item clickable v-close-popup @click="ask('COPY',entry)"><q-item-section>复制</q-item-section></q-item><q-item clickable v-close-popup @click="ask('DELETE',entry)"><q-item-section class="text-negative">删除</q-item-section></q-item></q-list></q-menu></q-btn></q-item-section>
   </q-item>
   <q-item v-if="list&&!entries.length&&!busy"><q-item-section class="text-grey-7">{{search ? '没有匹配的文件' : '目录为空'}}</q-item-section></q-item>
  </q-list>
  <div class="row items-center q-gutter-sm"><q-file v-model="uploadFile" dense outlined label="选择上传文件" class="col" :disable="!!transfer||!available"/><q-btn color="primary" label="上传" :disable="!uploadFile||!!transfer||busy||!available" @click="upload"/></div><q-checkbox v-if="uploadFile&&list?.entries.some(e=>e.path===join(uploadFile!.name))" v-model="uploadOverwrite" label="确认覆盖当前同名文件版本"/>
  <div v-if="transfer" class="q-mt-sm">{{transfer}}<q-linear-progress :value="progress"/><q-btn flat label="取消传输" @click="cancel"/></div>
  <q-banner v-if="result&&result.outcome!=='SUCCEEDED'" class="bg-orange-1 q-mt-sm">操作{{result.outcome==='PARTIAL'?'部分完成':'结果未知'}}，请刷新后核实。<div v-for="failure in result.failures" :key="failure.path">{{failure.path}} · {{failure.status}}</div><div v-if="result.truncated">仅显示部分失败项。</div></q-banner>
 </q-card-section>
 <q-dialog :model-value="!!preview" @update:model-value="v=>{if(!v)preview=undefined}"><WorkspacePreview v-if="preview" :key="spaceId+'/'+preview.path" :base-url="baseUrl" :space-id="spaceId" :path="preview.path" :etag="preview.etag" @close="preview=undefined"/></q-dialog>
 <q-dialog :model-value="!!editor" persistent><WorkspaceTextEditor v-if="editor" :base-url="editor.baseUrl" :space-id="editor.spaceId" :path="editor.path" :create="editor.create" :available="editorAvailable" @close="editor=undefined" @saved="editorSaved"/></q-dialog>
 <q-dialog :model-value="!!action" @update:model-value="v=>{if(!v)action=undefined}"><q-card class="app-dialog app-dialog--sm"><q-card-section class="text-h6">{{action==='MKCOL'?'新建目录':action==='DELETE'?'删除文件或目录':action==='COPY'?'复制':'重命名 / 移动'}}</q-card-section><q-card-section class="app-dialog__body workspace-file-form"><q-banner v-if="error" class="bg-orange-1" role="alert">{{workspaceFileError(error)}}</q-banner><div class="text-break">{{selected?.path}}</div><q-input v-if="action!=='DELETE'" v-model="destination" outlined dense :label="action==='MKCOL'?'当前目录下的新目录名':'目标完整相对路径（从根目录起）'"/><q-checkbox v-if="['MOVE','COPY'].includes(action??'')&&selected?.kind==='FILE'" v-model="overwrite" label="确认覆盖目标普通文件的当前版本"/><q-banner v-if="action==='DELETE'" class="bg-red-1">删除不可撤销。</q-banner><q-checkbox v-if="action==='DELETE'&&selected?.kind==='DIRECTORY'" v-model="recursive" label="确认递归删除目录及其所有内容"/></q-card-section><q-card-actions align="right"><q-btn flat label="取消" v-close-popup/><q-btn :color="action==='DELETE'?'negative':'primary'" label="确认" :loading="busy" :disable="action==='DELETE'?selected?.kind==='DIRECTORY'&&!recursive:!validDestination(destination)" @click="mutate"/></q-card-actions></q-card></q-dialog>
</template>

<style scoped>
.workspace-file-navigation{display:flex;align-items:center;justify-content:space-between;gap:8px;margin:8px 0}.workspace-file-navigation>:first-child{min-width:0;flex:1}.workspace-file-navigation>:last-child{width:240px;max-width:55%}@media(max-width:600px){.workspace-file-navigation{flex-wrap:wrap}.workspace-file-navigation>:last-child{width:100%;max-width:100%}}
.workspace-file-list{max-height:clamp(220px,52vh,620px);overflow:auto;overscroll-behavior:contain}.workspace-file-list :deep(.q-item){min-height:44px;padding:6px 10px}.workspace-files :deep(.q-field){min-width:0}
.workspace-file-summary{display:flex;align-items:center;gap:8px;min-width:0;width:100%}.workspace-file-labels{min-width:0;flex:1;overflow-wrap:anywhere}.workspace-file-open{cursor:pointer}.workspace-file-open:focus-visible{outline:2px solid var(--q-primary);outline-offset:2px;border-radius:3px}.workspace-view-mode{display:flex;align-items:center;border:1px solid rgba(127,127,127,.25);border-radius:4px}
.workspace-thumbnail-slot{display:flex;align-items:center;justify-content:center;position:relative;width:28px;height:28px;flex-shrink:0}.workspace-thumbnail{width:28px;height:28px;object-fit:contain}
.workspace-files-grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(min(160px,100%),1fr));gap:8px;padding:8px}.workspace-files-grid :deep(.q-item){display:block;position:relative;padding:8px;border:1px solid rgba(127,127,127,.25);border-radius:4px}.workspace-files-grid :deep(.q-item__section--main){margin:0}.workspace-files-grid :deep(.q-item__section--side){position:absolute;bottom:8px;right:4px;padding:0}.workspace-files-grid .workspace-file-summary{flex-direction:column;align-items:stretch}.workspace-files-grid .workspace-file-labels{padding-right:32px}.workspace-files-grid .workspace-thumbnail-slot{width:100%;height:128px;background:rgba(127,127,127,.08);border-radius:3px}.workspace-files-grid .workspace-thumbnail{width:100%;height:100%}.workspace-files-grid .workspace-thumbnail-slot>.q-icon{font-size:48px}.workspace-video-badge{position:absolute;left:4px;bottom:4px;display:flex;align-items:center;padding:2px 4px;gap:2px;border-radius:3px;background:rgba(0,0,0,.7);color:white;font-size:12px;line-height:16px}
.workspace-file-toolbar,.workspace-file-actions{display:flex;align-items:center;gap:8px;flex-wrap:wrap}.workspace-file-toolbar{justify-content:space-between}.workspace-file-form{display:grid;gap:8px;overflow-wrap:anywhere}
</style>
