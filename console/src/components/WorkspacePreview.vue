<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { getDocument, GlobalWorkerOptions, type PDFDocumentProxy, type PDFDocumentLoadingTask, type RenderTask } from 'pdfjs-dist'
import workerUrl from 'pdfjs-dist/build/pdf.worker.min.mjs?worker&url'
import { apiResponse } from '../api/client'
import { decodeWorkspaceText, previewKind, renderWorkspaceMarkdown, rasterMetadata } from '../composables/workspacePreview'
import ProblemBanner from './ProblemBanner.vue'

GlobalWorkerOptions.workerSrc=workerUrl
const props=defineProps<{baseUrl:string;path:string;etag?:string}>()
const emit=defineEmits<{close:[]}>()
const kind=computed(()=>previewKind(props.path))
const error=ref<unknown>(),loading=ref(true),text=ref(''),html=ref(''),imageUrl=ref('')
const raw=ref(false)
const canvas=ref<HTMLCanvasElement>(),markdown=ref<HTMLElement>(),page=ref(1),pages=ref(0),zoom=ref(1)
const controller=new AbortController(),urls:string[]=[]
let task:PDFDocumentLoadingTask|undefined,pdf:PDFDocumentProxy|undefined,render:RenderTask|undefined,closed=false
const limit=24*1024*1024
async function bytes(path:string,etag?:string,max=limit){
 const response=await apiResponse(props.baseUrl+'/content?path='+encodeURIComponent(path),{signal:controller.signal,headers:etag?{'If-Match':etag}:{}})
 if(Number(response.headers.get('Content-Length'))>max){await response.body?.cancel();throw new Error('文件超过预览大小限制，请下载后打开。')}
 const reader=response.body?.getReader();if(!reader)throw new Error('文件响应为空')
 const chunks:Uint8Array[]=[];let size=0
 try{for(;;){const {done,value}=await reader.read();if(done)break;size+=value.length;if(size>max)throw new Error('文件超过预览大小限制，请下载后打开。');chunks.push(value)}}finally{await reader.cancel()}
 const out=new Uint8Array(size);let offset=0;for(const chunk of chunks){out.set(chunk,offset);offset+=chunk.length}return out
}
async function raster(data:Uint8Array){
 const {type}=rasterMetadata(data)
 const blob=new Blob([data as BlobPart],{type})
 const decoded=await createImageBitmap(blob)
 const pixels=decoded.width*decoded.height;decoded.close()
 if(pixels>16000000)throw new Error('图片超过 1600 万像素预览限制，请下载后打开。')
 if(closed)throw new DOMException('Preview closed','AbortError')
 const url=URL.createObjectURL(blob);urls.push(url);return url
}
async function draw(){
 if(!pdf||!canvas.value||closed)return
 render?.cancel();try{await render?.promise}catch{/* cancellation */}
 const documentPage=await pdf.getPage(page.value);if(closed)return
 const viewport=documentPage.getViewport({scale:zoom.value})
 if(viewport.width*viewport.height>16000000){error.value=new Error('页面尺寸超过预览限制，请降低缩放比例。');return}
 canvas.value.width=viewport.width;canvas.value.height=viewport.height
 render=documentPage.render({canvas:canvas.value,viewport});try{await render.promise}catch(e){if(!closed&&(e as Error).name!=='RenderingCancelledException')error.value=e}
}
async function changePage(delta:number){page.value=Math.max(1,Math.min(pages.value,page.value+delta));await draw()}
onMounted(async()=>{
 try{
  if(kind.value==='download')return
  const data=await bytes(props.path,props.etag,kind.value==='text'||kind.value==='markdown'?2*1024*1024:limit)
  if(closed)return
  if(kind.value==='image')imageUrl.value=await raster(data)
  else if(kind.value==='pdf'){
   task=getDocument({data,enableXfa:false,disableAutoFetch:true,disableFontFace:true})
   controller.signal.addEventListener('abort',()=>{void task?.destroy()},{once:true})
   pdf=await task.promise;if(closed){await task.destroy();return}pages.value=pdf.numPages;loading.value=false;await nextTick();await draw()
  }else{
   text.value=decodeWorkspaceText(data)
   if(kind.value==='markdown'){
    const rendered=renderWorkspaceMarkdown(text.value,props.path);html.value=rendered.html;loading.value=false;await nextTick()
    for(const item of rendered.images.slice(0,20)){
     if(closed)break
     try{const url=await raster(await bytes(item.path,undefined,4*1024*1024));const element=markdown.value?.querySelector<HTMLImageElement>('#'+item.id);if(element)element.src=url}catch{/* unavailable images keep their alt text */}
    }
   }
  }
 }catch(e){if(!closed)error.value=e}finally{loading.value=false}
})
onBeforeUnmount(()=>{closed=true;controller.abort();render?.cancel();void task?.destroy();for(const url of urls)URL.revokeObjectURL(url)})
</script>
<template>
 <q-card class="workspace-preview"><q-card-section class="row items-center no-wrap q-gutter-sm"><div class="text-subtitle1 ellipsis col">{{path}}</div><q-btn flat round icon="close" aria-label="关闭预览" @click="emit('close')"/></q-card-section><q-separator/>
  <q-card-section><ProblemBanner :error="error"/><q-spinner v-if="loading" size="32px"/>
   <p v-if="kind==='download'">此类型不在网页中执行或预览，请下载后使用合适的软件打开。</p>
   <template v-if="kind==='pdf'&&pages"><div class="row items-center q-gutter-sm q-mb-sm"><q-btn flat icon="chevron_left" aria-label="上一页" :disable="page===1" @click="changePage(-1)"/><span>{{page}} / {{pages}}</span><q-btn flat icon="chevron_right" aria-label="下一页" :disable="page===pages" @click="changePage(1)"/><q-select v-model="zoom" dense outlined label="缩放" style="width:120px" :options="[0.5,1,1.5,2]" @update:model-value="draw"/></div><div class="pdf-scroll"><canvas ref="canvas"/></div></template>
   <template v-if="imageUrl"><q-select v-model="zoom" dense outlined label="缩放" :options="[0.5,1,1.5,2]" style="width:120px"/><div class="image-scroll"><img :src="imageUrl" :alt="path" class="image-preview" :style="{width:zoom*100+'%',maxHeight:zoom*70+'vh'}"/></div></template>
   <q-toggle v-if="kind==='markdown'&&!loading" v-model="raw" label="查看原文"/>
   <pre v-if="(kind==='text'||raw)&&!loading" class="text-preview">{{text}}</pre>
   <div v-if="kind==='markdown'" v-show="!raw" ref="markdown" class="markdown-preview" v-html="html"/>
  </q-card-section>
 </q-card>
</template>
<style scoped>
.workspace-preview{width:1000px;max-width:95vw;max-height:92vh;overflow:auto}.image-preview{object-fit:contain}.image-scroll{overflow:auto;max-height:70vh}.text-preview{white-space:pre-wrap;overflow-wrap:anywhere;font-size:13px}.pdf-scroll{overflow:auto;max-height:70vh}.markdown-preview{overflow-wrap:anywhere;line-height:1.65}.markdown-preview :deep(h1){font-size:1.8rem;line-height:1.3;margin:1rem 0}.markdown-preview :deep(h2){font-size:1.5rem;line-height:1.35;margin:1rem 0}.markdown-preview :deep(h3){font-size:1.25rem;line-height:1.4;margin:.8rem 0}.markdown-preview :deep(img){max-width:100%;max-height:600px}.markdown-preview :deep(pre){overflow:auto;background:#f4f4f4;padding:12px}.markdown-preview :deep(table){border-collapse:collapse}.markdown-preview :deep(td),.markdown-preview :deep(th){border:1px solid #ddd;padding:6px}
</style>
