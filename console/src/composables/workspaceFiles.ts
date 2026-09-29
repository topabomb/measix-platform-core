import { ApiProblem, apiResponse } from '../api/client'

export const textLimit = 2 * 1024 * 1024
export function workspaceBytes(value: number | null | undefined): string {
 if (value == null) return '—'
 const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB']; let n = value, i = 0
 while (n >= 1024 && i < units.length - 1) { n /= 1024; i++ }
 return `${new Intl.NumberFormat('zh-CN', { maximumFractionDigits: i ? 1 : 0 }).format(n)} ${units[i]}`
}
export function workspaceFileUrl(base: string, spaceId: string, resource: 'files'|'content', path='') {
 return `${base}/${resource}?${new URLSearchParams({agentSpaceId:spaceId,path})}`
}
export async function readWorkspaceContent(url: string, signal: AbortSignal, max: number, etag?: string) {
 const response=await apiResponse(url,{signal,headers:etag?{'If-Match':etag}:{}})
 if(Number(response.headers.get('Content-Length'))>max){await response.body?.cancel();throw new Error('文件超过预览或编辑大小限制，请下载后打开。')}
 const reader=response.body?.getReader();if(!reader)throw new Error('文件响应为空')
 const chunks:Uint8Array[]=[];let size=0
 try{for(;;){const {done,value}=await reader.read();if(done)break;size+=value.length;if(size>max)throw new Error('文件超过预览或编辑大小限制，请下载后打开。');chunks.push(value)}}finally{await reader.cancel()}
 const data=new Uint8Array(size);let offset=0;for(const chunk of chunks){data.set(chunk,offset);offset+=chunk.length}
 return {data,etag:response.headers.get('ETag')??undefined}
}
export function downloadWorkspaceFile(base:string,spaceId:string,path:string) {
 const anchor=document.createElement('a');anchor.href=workspaceFileUrl(base,spaceId,'content',path);anchor.download=path.split('/').pop()??path;anchor.rel='noopener';anchor.click()
}
export function workspaceFileError(error:unknown): string {
 const code=error instanceof ApiProblem?error.code:''
 const messages:Record<string,string>={
  file_version_conflict:'文件版本已变化。编辑内容已保留，请重新读取核实或另存为新文件。',
  workspace_space_mismatch:'工作区已被替换。请退出并重新打开工作区，不能把当前操作提交到新空间。',
  file_conflict:'同名目标已存在或目标目录不可用，请刷新目录后重新选择。',
  file_storage_full:'工作区空间不足，请清理文件后再操作。',
  file_transfer_limit:'并发文件操作过多，请等待当前传输完成后再操作。',
  file_listing_limit:'目录内容超过当前支持的读取范围，请使用其他文件工具整理；这不表示目录为空。',
  dav_credential_unavailable:'文件访问凭据不可用，请联系管理员重新签发 WebDAV Token。',
  workspace_unavailable:'工作区当前不可用，请刷新工作区状态。',
  workspace_result_unknown:'操作结果未知，请重新读取远端文件核实；不要直接重复保存或上传。',
  file_not_found:'文件已不存在，请刷新目录。',file_locked:'文件被锁定，请稍后核实。',
  file_transport_unavailable:'文件服务连接中断，请稍后重新读取。',
 }
 return messages[code]??(error instanceof Error?error.message:'文件操作失败，请刷新核实。')
}
