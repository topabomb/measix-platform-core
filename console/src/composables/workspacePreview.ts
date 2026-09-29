import { Marked } from 'marked'
import DOMPurify from 'dompurify'

export function previewKind(name: string): 'markdown' | 'text' | 'image' | 'pdf' | 'download' {
 const ext = name.split('.').pop()?.toLowerCase() ?? ''
 if (['md','markdown'].includes(ext)) return 'markdown'
 if (['txt','log','json','yaml','yml','toml','csv','xml','js','ts','py','go','rs','sh','css','sql'].includes(ext)) return 'text'
 if (['jpg','jpeg','png','webp','gif','bmp'].includes(ext)) return 'image'
 return ext === 'pdf' ? 'pdf' : 'download'
}
export function decodeWorkspaceText(bytes: Uint8Array): string {
 const text = new TextDecoder('utf-8', { fatal: true }).decode(bytes)
 if (text.includes('\0')) throw new Error('文件不是可预览的 UTF-8 文本，请下载后打开。')
 return text
}
const escape = (s: string) => s.replaceAll('&','&amp;').replaceAll('<','&lt;').replaceAll('>','&gt;').replaceAll('"','&quot;')
export function renderWorkspaceMarkdown(text: string, documentPath: string) {
 const images: { id: string; path: string }[] = []
 const markdown = new Marked({ gfm: true, renderer: {
  html: ({text}) => escape(text),
  image: ({href,text}) => {
   if (/^(?:[a-z][a-z0-9+.-]*:|\/|\\)/i.test(href)) return escape(text)
   const parts = documentPath.split('/').slice(0,-1)
   for (const part of href.split('/')) {
    if (part === '..' || /[%?#\\]/.test(part)) return escape(text)
    if (part && part !== '.') parts.push(part)
   }
   const id = `workspace-image-${images.length}`
   images.push({id,path:parts.join('/')})
   return `<img id="${id}" alt="${escape(text)}">`
  },
  link: ({href,text}) => /^(https?:|mailto:)/i.test(href) ? `<a href="${escape(href)}" target="_blank" rel="noopener noreferrer">${escape(text)}</a>` : escape(text),
 } })
 const html = DOMPurify.sanitize(markdown.parse(text,{async:false}) as string, {
  ALLOWED_TAGS:['h1','h2','h3','h4','p','br','strong','em','ul','ol','li','pre','code','a','blockquote','hr','table','thead','tbody','tr','th','td','img'],
  ALLOWED_ATTR:['href','title','start','id','alt','target','rel'], ALLOW_DATA_ATTR:false,
 })
 return {html,images}
}

// Inspect bounded format headers before asking a browser decoder to allocate pixels.
export function rasterMetadata(data:Uint8Array):{type:string;width:number;height:number}{
 const view=new DataView(data.buffer,data.byteOffset,data.byteLength)
 const ascii=(at:number,n:number)=>String.fromCharCode(...data.subarray(at,at+n))
 let type='',width=0,height=0
 if(data.length>=24&&ascii(1,3)==='PNG'&&ascii(12,4)==='IHDR'){
  type='image/png';width=view.getUint32(16);height=view.getUint32(20)
 }else if(data.length>=10&&/^GIF8[79]a$/.test(ascii(0,6))){
  type='image/gif';width=view.getUint16(6,true);height=view.getUint16(8,true)
 }else if(data.length>=26&&ascii(0,2)==='BM'){
  type='image/bmp';const header=view.getUint32(14,true)
  if(header===12){width=view.getUint16(18,true);height=view.getUint16(20,true)}
  else if(header>=40){width=view.getInt32(18,true);height=Math.abs(view.getInt32(22,true))}
 }else if(data.length>=30&&ascii(0,4)==='RIFF'&&ascii(8,4)==='WEBP'){
  type='image/webp';const chunk=ascii(12,4)
  if(chunk==='VP8X'){width=1+data[24]!+(data[25]!<<8)+(data[26]!<<16);height=1+data[27]!+(data[28]!<<8)+(data[29]!<<16)}
  else if(chunk==='VP8L'&&data[20]===47){const bits=view.getUint32(21,true);width=(bits&16383)+1;height=((bits>>>14)&16383)+1}
  else if(chunk==='VP8 '&&data[23]===157&&data[24]===1&&data[25]===42){width=view.getUint16(26,true)&16383;height=view.getUint16(28,true)&16383}
 }else if(data.length>=4&&data[0]===255&&data[1]===216){
  type='image/jpeg'
  for(let at=2;at+3<data.length;){
   if(data[at++]!==255)break
   while(data[at]===255)at++
   const marker=data[at++]!;if(marker===217||marker===218)break
   if(marker===1||(marker>=208&&marker<=215))continue
   if(at+2>data.length)break
   const length=view.getUint16(at);if(length<2||at+length>data.length)break
   if([192,193,194,195,197,198,199,201,202,203,205,206,207].includes(marker)&&length>=7){height=view.getUint16(at+3);width=view.getUint16(at+5);break}
   at+=length
  }
 }
 if(!type||width<1||height<1)throw new Error('图片格式或尺寸无效，请下载后打开。')
 if(width*height>16000000)throw new Error('图片超过 1600 万像素预览限制，请下载后打开。')
 return{type,width,height}
}
