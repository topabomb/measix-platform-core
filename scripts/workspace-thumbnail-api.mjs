/** Validation-only real Client protocol lane; lifecycle stays in the shared harness. */
import assert from 'node:assert/strict'
import { createHash,randomUUID } from 'node:crypto'
import { mkdirSync,writeFileSync,readFileSync } from 'node:fs'
import { join } from 'node:path'
import { WorkspaceThumbnailSession } from './examples/workspace-thumbnail-session.mjs'

const sha = bytes=>'sha256:'+createHash('sha256').update(bytes).digest('hex')

export async function captureClientContractCases({directory,agentSpaceId,core,provider,fetch:clientFetch}) {
  mkdirSync(directory,{recursive:false})
  const cases=[]
  for(const item of [{id:'workspace',path:'/api/client/v1/workspace',directory:''},{id:'files-root',directory:''},{id:'files-nested',directory:'nested'}]) {
    const path=item.path??'/api/client/v1/workspace/files?'+new URLSearchParams({agentSpaceId,path:item.directory})
    const response=await clientFetch(path)
    assert.equal(response.status,200,'Client capture requires a real 200 response')
    assert.equal(response.headers.get('Content-Type')?.split(';')[0],'application/json')
    const bytes=Buffer.from(await response.arrayBuffer())
    assert.ok(bytes.length>0&&bytes.length<=4*1024*1024)
    const body=JSON.parse(bytes.toString('utf8'))
    if(item.id==='workspace')assert.equal(body.agentSpaceId,agentSpaceId)
    else assert.ok(Array.isArray(body.entries)&&body.entries.length>0)
    const bodyFile=item.id+'.json'
    writeFileSync(join(directory,bodyFile),bytes,{flag:'wx'})
    cases.push({id:item.id,method:'GET',path,agentSpaceId,directory:item.directory,status:200,contentType:'application/json',bodyFile,responseSHA256:sha(bytes),byteLength:bytes.length})
  }
  const manifest={schemaVersion:1,core,provider,cases}
  writeFileSync(join(directory,'manifest.json'),JSON.stringify(manifest,null,2)+'\n',{flag:'wx'})
  return manifest
}

export async function runThumbnailAPI(context) {
  const {out,env,options,users,api,raw,command,put,workspace,fixtureReady,managementToken,core}=context
  const checks=[],user=users[0],base='/api/client/v1/workspace'
  const proof=name=>{checks.push(name);console.log('PASS API '+name)}
  const enrollment=await api('/api/admin/v1/users/'+user.userId+'/enrollments','POST',{expiresInSeconds:3600})
  const exchanged=await fetch(env.hubBaseURL+'/api/client/v1/enrollments/exchange',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({platform:'ANDROID',deviceName:'Isolated thumbnail API acceptance',code:enrollment.code,installationId:'ins_'+randomUUID(),appVersion:'thumbnail-api-acceptance'})})
  assert.equal(exchanged.status,201)
  const {accessToken}=await exchanged.json()
  assert.ok(accessToken)
  const clientFetch=(path,init={})=>fetch(new URL(path,env.hubBaseURL),{...init,headers:{Authorization:'Bearer '+accessToken,...init.headers}})
  const query=(path,extra={})=>new URLSearchParams({agentSpaceId:user.spaceId,path,...extra})
  const content=path=>base+'/content?'+query(path)
  const endpoint=base+'/thumbnail'
  const source=async path=>{const response=await clientFetch(content(path),{method:'HEAD'});assert.equal(response.status,200);return response.headers.get('ETag')}
  const preview=(path,etag,size=128)=>endpoint+'?'+query(path,{expectedEtag:etag,size:String(size)})
  const error=async(response,status,code)=>{assert.equal(response.status,status);assert.equal((await response.json()).code,code)}
  const available=async()=>{const response=await clientFetch(base);assert.equal(response.status,200);return response.json()}
  async function inspect(response,expectedSource){
    assert.equal(response.status,200);assert.equal(response.headers.get('Content-Type'),'image/webp')
    assert.equal(response.headers.get('Agent-Space-Id'),user.spaceId);assert.equal(response.headers.get('Preview-Source-ETag'),expectedSource)
    const bytes=Buffer.from(await response.arrayBuffer()),etag='"'+sha(bytes).slice(7)+'"'
    assert.equal(response.headers.get('ETag'),etag);assert.equal(Number(response.headers.get('Content-Length')),bytes.length)
    assert.ok(bytes.length>0&&bytes.length<=1024*1024);assert.equal(bytes.subarray(0,4).toString(),'RIFF');assert.equal(bytes.subarray(8,12).toString(),'WEBP')
    return {bytes,etag}
  }

  const cases=join(out,'client-contract-cases')
  const manifest=await captureClientContractCases({directory:cases,agentSpaceId:user.spaceId,core,provider:{image:options.image,expectedSource:options.expectedSource,expectedHelper:options.expectedHelper,expectedWorkspace:options.expectedWorkspace},fetch:clientFetch})
  for(const id of ['files-root','files-nested']) {
    const data=JSON.parse(readFileSync(join(cases,id+'.json'),'utf8'))
    assert.ok(data.entries.some(entry=>entry.kind==='DIRECTORY'))
    assert.ok(data.entries.some(entry=>entry.path.endsWith('.png')))
    assert.ok(data.entries.some(entry=>entry.path.endsWith('.mp4')))
  }
  console.log('CLIENT_CONTRACT_CASES '+join(cases,'manifest.json'))
  proof('real Client Bearer workspace/root/nested 200 bodies captured byte-for-byte for fixed-consumer replay')

  const capabilityPath=endpoint+'?'+new URLSearchParams({agentSpaceId:user.spaceId})
  for(const fetcher of [clientFetch,path=>raw(workspace(user)+path.slice(base.length),{method:'OPTIONS'})]) {
    const response=await fetcher(capabilityPath,{method:'OPTIONS'})
    assert.equal(response.status,200);assert.equal(response.headers.get('Agent-Space-Id'),user.spaceId)
    assert.equal((await response.json()).supported,true)
  }
  let etag=await source('00-image.png')
  const image=await inspect(await clientFetch(preview('00-image.png',etag),{headers:{Range:'bytes=0-7'}}),etag)
  writeFileSync(join(out,'client-image.webp'),image.bytes,{flag:'wx'})
  const head=await clientFetch(preview('00-image.png',etag),{method:'HEAD'})
  assert.equal(head.status,200);assert.equal(head.headers.get('ETag'),image.etag);assert.equal(Number(head.headers.get('Content-Length')),image.bytes.length);assert.equal((await head.arrayBuffer()).byteLength,0)
  for(const method of ['GET','HEAD']) {
    const response=await clientFetch(preview('00-image.png',etag),{method,headers:{'If-None-Match':image.etag}})
    assert.equal(response.status,304);assert.equal((await response.arrayBuffer()).byteLength,0)
  }
  const admin=await inspect(await raw(workspace(user)+'/thumbnail?'+query('00-image.png',{expectedEtag:etag,size:'128'})),etag)
  assert.deepEqual(admin.bytes,image.bytes)
  const adminURL=workspace(user)+'/thumbnail?'+query('00-image.png',{expectedEtag:etag,size:'128'})
  const adminHead=await raw(adminURL,{method:'HEAD'})
  assert.equal(adminHead.status,200);assert.equal(adminHead.headers.get('ETag'),image.etag);assert.equal(Number(adminHead.headers.get('Content-Length')),image.bytes.length)
  assert.equal(adminHead.headers.get('Preview-Source-ETag'),etag);assert.equal((await adminHead.arrayBuffer()).byteLength,0)
  for(const method of ['GET','HEAD']) {
    const conditional=await raw(adminURL,{method,headers:{'If-None-Match':image.etag}})
    assert.equal(conditional.status,304);assert.equal(conditional.headers.get('ETag'),image.etag);assert.equal((await conditional.arrayBuffer()).byteLength,0)
  }
  proof('real Admin Cookie and Client Bearer OPTIONS/GET/HEAD/304; exact output/source identity and ignored thumbnail Range')

  let discoveries=0
  const session=new WorkspaceThumbnailSession({endpoint:env.hubBaseURL+endpoint,agentSpaceId:user.spaceId,fetch:(url,init)=>{if(init?.method==='OPTIONS')discoveries++;return clientFetch(url,init)}})
  try {
    assert.equal(session.capabilities(),session.capabilities())
    const first=await session.thumbnail({path:'00-image.png',expectedEtag:etag})
    const second=await session.thumbnail({path:'01-video.mp4',expectedEtag:await source('01-video.mp4')})
    assert.deepEqual(Buffer.from(first.bytes),image.bytes);assert.ok(second.bytes.length>0);assert.equal(discoveries,1)
    writeFileSync(join(out,'client-video.webp'),second.bytes,{flag:'wx'})
  }finally{session.dispose()}
  proof('the real Node connection example reuses one OPTIONS promise for sequential image/video thumbnails')

  const oldSource=etag
  let response=await raw(workspace(user)+'/content?'+query('00-image.png'),{method:'PUT',headers:{'If-Match':etag},body:readFileSync(join(out,'agent-space/media/changed.png'))})
  assert.equal(response.status,200);await response.body.cancel()
  await error(await clientFetch(preview('00-image.png',oldSource),{headers:{'If-None-Match':image.etag}}),412,'file_version_conflict')
  etag=await source('00-image.png')
  assert.notEqual(etag,oldSource);assert.notEqual((await inspect(await clientFetch(preview('00-image.png',etag)),etag)).etag,image.etag)
  await put(user,'deleted.png',readFileSync(join(out,'agent-space/media/image.png')))
  const deletedETag=await source('deleted.png')
  await api(workspace(user)+'/files?'+query(''),'POST',{action:'DELETE',path:'deleted.png',sourceEtag:deletedETag})
  await error(await clientFetch(preview('deleted.png',deletedETag)),404,'file_not_found')
  response=await clientFetch(capabilityPath,{method:'OPTIONS'});assert.equal((await response.json()).supported,true)
  await error(await clientFetch(preview('unsafe.html',await source('unsafe.html'))),415,'preview_unsupported_media')
  assert.equal((await available()).filesAvailable,true)
  proof('source ETag replacement wins over derived 304; deleted source is 404; unsupported media is 415 and Files remains available')

  const original=Buffer.from('原始文件 Range and download\r\n')
  await put(user,'compat-origin.txt',original)
  const originalETag=await source('compat-origin.txt')
  response=await clientFetch(content('compat-origin.txt'),{headers:{Range:'bytes=0-3','If-Match':originalETag}})
  assert.equal(response.status,206);assert.deepEqual(Buffer.from(await response.arrayBuffer()),original.subarray(0,4))
  response=await clientFetch(content('compat-origin.txt'));assert.deepEqual(Buffer.from(await response.arrayBuffer()),original)
  await api(workspace(user)+'/files?'+query(''),'POST',{action:'COPY',path:'compat-origin.txt',destination:'nested/copied.txt',sourceEtag:originalETag,overwrite:false})
  await api(workspace(user)+'/files?'+query(''),'POST',{action:'MOVE',path:'nested/copied.txt',destination:'compat-moved.txt',sourceEtag:await source('nested/copied.txt'),overwrite:false})
  response=await clientFetch(content('compat-moved.txt'));assert.deepEqual(Buffer.from(await response.arrayBuffer()),original)
  await api(workspace(user)+'/files?'+query(''),'POST',{action:'DELETE',path:'compat-moved.txt',sourceEtag:await source('compat-moved.txt')})
  proof('original conditional write/read/COPY/MOVE/DELETE, download bytes and byte Range remain compatible')

  const before=await available(),connection=await api(workspace(user)+'/dav-connection','POST')
  response=await fetch(fixtureReady.adminOrigin+'/admin/v1/users/'+connection.username+'/dav-token',{method:'POST',headers:{Authorization:'Bearer '+managementToken,'Content-Type':'application/json'},body:JSON.stringify({agentSpaceId:user.spaceId,token:randomUUID()+randomUUID()})})
  assert.equal(response.status,200);await response.body.cancel()
  await error(await clientFetch(preview('00-image.png',etag)),503,'dav_credential_unavailable')
  let view=await available();assert.equal(view.filesAvailable,false);assert.equal(view.filesReason,'dav_credential_unavailable');assert.equal(view.agentSpaceId,before.agentSpaceId);assert.equal(view.mcpAvailable,before.mcpAvailable)
  assert.equal((await api(workspace(users[1]))).filesAvailable,true)
  await command(user,'SET_DAV');view=await available();assert.equal(view.filesAvailable,true)
  await inspect(await clientFetch(preview('00-image.png',etag)),etag)
  await command(user,'REVOKE_DAV');assert.equal((await available()).filesAvailable,false)
  response=await clientFetch(preview('00-image.png',etag));assert.equal(response.status,503);await response.body.cancel()
  await command(user,'SET_DAV');await inspect(await clientFetch(preview('00-image.png',etag)),etag)
  proof('real remote DAV key change yields sanitized 503 and withdraws only that account; Core reissue and explicit revoke/reissue recover correctly')
  return {checks,clientContractManifest:manifest,reusedEvidence:[{scope:'same fixed provider active generation cancellation/revocation and Range',path:'build-qualification-20261117-ce8fb36c/protocol/server-d592-final.json'},{scope:'same Core/UI source, real API 429 and aborted-body scope/permission cancellation',path:'stage3-browser-final-20261117-0fd58a/evidence.json'}]}
}
