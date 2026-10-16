<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { onBeforeRouteLeave } from 'vue-router'
import { ApiProblem, apiResponse } from '../api/client'
import { useSessionStore } from '../stores/session'
import { decodeEditableText, encodeEditableText, previewKind } from '../composables/workspacePreview'
import { readWorkspaceContent, textLimit, workspaceFileError, workspaceFileUrl } from '../composables/workspaceFiles'

const props=withDefaults(defineProps<{baseUrl:string;spaceId:string;path:string;create?:boolean;available?:boolean}>(),{create:false,available:true})
const emit=defineEmits<{close:[];saved:[]}>(),session=useSessionStore()
const text=ref(''),original=ref(''),etag=ref<string>(),loading=ref(false),saving=ref(false),loaded=ref(props.create)
const format=ref({bom:false,newline:'\n'}),error=ref(''),blocked=ref(false),saveAs=ref(props.create),destination=ref(props.path)
const failedTarget=ref<string>()
const confirm=ref(false),confirmation=ref('')
let controller=new AbortController(),alive=true,pending:(()=>void)|undefined,leaveResolve:((v:boolean)=>void)|undefined
const dirty=computed(()=>text.value!==original.value || props.create && !!text.value)
const validPath=computed(()=>!!destination.value && !destination.value.startsWith('/') && !destination.value.split('/').some(v=>!v||v==='.'||v==='..'||/[\\\r\n\0]/.test(v)) && ['text','markdown'].includes(previewKind(destination.value)))
const canSave=computed(()=>props.available && loaded.value && !loading.value && !saving.value && (saveAs.value?validPath.value && destination.value!==failedTarget.value && (props.create||destination.value!==props.path):!!etag.value && dirty.value && !blocked.value))

async function load() {
 controller.abort();controller=new AbortController();loading.value=true;error.value=''
 try{
  const content=await readWorkspaceContent(workspaceFileUrl(props.baseUrl,props.spaceId,'content',props.path),controller.signal,textLimit)
  const decoded=decodeEditableText(content.data)
  if(!alive)return
  text.value=decoded.text;original.value=decoded.text;format.value=decoded;etag.value=content.etag
  loaded.value=true;blocked.value=false;failedTarget.value=undefined
  if(!content.etag || content.etag.startsWith('W/')) {etag.value=undefined;error.value='文件没有可用于条件保存的版本标识，可另存为新文件。'}
 }catch(e){if(alive)error.value=workspaceFileError(e)}finally{loading.value=false}
}
function request(action:()=>void,message:string) {
 if(dirty.value){pending=action;confirmation.value=message;confirm.value=true}else action()
}
function close(){request(()=>emit('close'),'当前编辑尚未保存，确定放弃并关闭？')}
function reload(){request(()=>{saveAs.value=false;void load()},'重新读取将放弃当前编辑，确定继续？')}
function accept(){confirm.value=false;pending?.();pending=undefined}
function cancelConfirm(){confirm.value=false;pending=undefined;leaveResolve?.(false);leaveResolve=undefined}
function beginSaveAs(){saveAs.value=true;destination.value=''}
async function save(){
 if(!canSave.value||!session.csrfToken)return
 const body=encodeEditableText(text.value,format.value)
 if(body.byteLength>textLimit){error.value='文本超过 2 MiB 编辑限制，请缩小内容或通过上传功能保存。';return}
 const target=saveAs.value?destination.value:props.path
 saving.value=true;error.value=''
 try{
  const response=await apiResponse(workspaceFileUrl(props.baseUrl,props.spaceId,'content',target),{method:'PUT',body:body as BodyInit,signal:controller.signal,headers:{'Content-Type':'application/octet-stream',...(saveAs.value?{'If-None-Match':'*'}:{'If-Match':etag.value!})}},session.csrfToken)
  const result=await response.json() as {outcome:string}
  if(result.outcome!=='SUCCEEDED')throw new ApiProblem(503,'workspace_result_unknown','保存结果待核实')
  if(alive){original.value=text.value;emit('saved');emit('close')}
 }catch(e){
  if(alive){error.value=workspaceFileError(e);blocked.value=true;failedTarget.value=target;if(!(e instanceof ApiProblem))error.value+='；编辑内容已保留。若请求已发出，请重新读取核实保存结果。'}
 }finally{saving.value=false}
}
function beforeUnload(e:BeforeUnloadEvent){if(dirty.value){e.preventDefault();e.returnValue=''}}
watch(()=>props.available,value=>{
 if(!value){controller.abort();blocked.value=true;error.value='工作区文件访问已失效。编辑内容保留在此，无法继续保存。'}
 else{controller=new AbortController();error.value=props.create?'文件访问已恢复，请核实目标后保存。':'文件访问已恢复，请重新读取核实原文件，或另存为新文件。'}
})
onBeforeRouteLeave(()=>{
 if(!dirty.value)return true
 return new Promise<boolean>(resolve=>{leaveResolve=resolve;pending=()=>{resolve(true);leaveResolve=undefined};confirmation.value='离开此页将放弃尚未保存的编辑，确定继续？';confirm.value=true})
})
onMounted(()=>{window.addEventListener('beforeunload',beforeUnload);if(!props.create)void load()})
onBeforeUnmount(()=>{alive=false;controller.abort();window.removeEventListener('beforeunload',beforeUnload);leaveResolve?.(false)})
</script>
<template>
 <q-card class="workspace-text-editor">
  <q-card-section><div class="text-h6">{{create?'新建文本文件':'编辑文本文件'}}</div><div class="text-caption text-break">{{path}}</div></q-card-section>
  <q-card-section class="q-gutter-md">
   <q-banner v-if="error" class="bg-orange-1" role="alert">{{error}}</q-banner>
   <q-spinner v-if="loading"/>
   <q-input v-if="saveAs" v-model="destination" outlined label="新文件路径（从根目录起）" hint="支持 UTF-8 文本和 Markdown；只创建新文件，不覆盖同名文件。" :disable="saving"/>
   <q-input v-model="text" outlined type="textarea" label="文本内容" :readonly="!loaded||loading||saving" :input-style="{minHeight:'40vh',fontFamily:'monospace'}"/>
   <div class="text-caption text-grey-7">UTF-8{{format.bom?' · BOM':''}} · {{format.newline==='\r\n'?'CRLF':format.newline==='\r'?'CR':'LF'}} · 最大 2 MiB · {{dirty?'有未保存的修改':'未修改'}}。保存成功后关闭编辑器。</div>
  </q-card-section>
  <q-card-actions align="right" class="q-pa-md">
   <q-btn v-if="!create" flat label="重新读取" :disable="loading||saving||!available" @click="reload"/>
   <q-btn v-if="!create&&!saveAs" outline label="另存为" :disable="!loaded||saving||!available" @click="beginSaveAs"/>
   <q-btn flat label="关闭" :disable="saving" @click="close"/>
   <q-btn color="primary" label="保存并关闭" :loading="saving" :disable="!canSave" @click="save"/>
  </q-card-actions>
  <q-dialog v-model="confirm" persistent><q-card><q-card-section>{{confirmation}}</q-card-section><q-card-actions align="right"><q-btn flat label="继续编辑" @click="cancelConfirm"/><q-btn color="negative" label="放弃修改" @click="accept"/></q-card-actions></q-card></q-dialog>
 </q-card>
</template>
<style scoped>.workspace-text-editor{width:900px;max-width:95vw;max-height:92vh;overflow:auto}.text-break{overflow-wrap:anywhere}</style>
