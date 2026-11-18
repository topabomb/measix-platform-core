/** Compatibility fixture, not a historical Runcove release. Management is a
 * deterministic substitute; files are served by the existing real x/net DAV.
 * Process/server ownership is supplied by the shared harness caller.
 */
import assert from 'node:assert/strict'
import { createServer,request as httpRequest } from 'node:http'
import { spawn,execFileSync } from 'node:child_process'
import { once } from 'node:events'
import { mkdirSync,writeFileSync,readFileSync } from 'node:fs'
import { randomUUID,createHash } from 'node:crypto'
import { join } from 'node:path'
import { freePort,waitFor } from './lib/harness.mjs'
import { WorkspaceThumbnailSession } from './examples/workspace-thumbnail-session.mjs'
import { openPlatformDAV, publishWorkspaceDAV } from './workspace-thumbnail-relay.mjs'

export function createLegacyPreviewProxy({target,mode,observed}) {
  const origin=new URL(target)
  assert.ok(origin.protocol==='http:'&&origin.hostname==='127.0.0.1'&&!origin.username&&!origin.password&&!origin.search&&!origin.hash&&origin.pathname==='/','Only a fixed loopback fixture target is allowed')
  return createServer((request,response)=>{
    if(!request.url.startsWith('/')||request.url.startsWith('//')){response.writeHead(400);response.end();return}
    const path=request.url.split('?')[0]
    if(path.endsWith('/previews')&&mode.previewStatus){
      const status=mode.previewStatus
      observed.push({method:request.method,path,status,source:'explicit preview absence/forbidden fault injection'})
      if(status===403){response.setHeader('WWW-Authenticate','Bearer realm="must-not-forward"');response.setHeader('Set-Cookie','must-not-forward=value');response.setHeader('Location','http://invalid.example/never-follow')}
      response.writeHead(status);response.end();return
    }
    const forwarded=httpRequest({hostname:origin.hostname,port:origin.port,method:request.method,path:request.url,headers:{...request.headers,host:origin.host}},remote=>{
      observed.push({method:request.method,path,status:remote.statusCode,source:'real x/net/webdav fixture'})
      response.writeHead(remote.statusCode,remote.headers);remote.pipe(response)
    })
    forwarded.on('error',()=>{if(!response.headersSent)response.writeHead(502);response.end()})
    request.on('aborted',()=>forwarded.destroy());response.on('close',()=>forwarded.destroy());request.pipe(forwarded)
  })
}

export async function startLegacyProvider({root,privateDirectory,registerProcess,registerServer,logStream}) {
  mkdirSync(privateDirectory,{mode:0o700})
  const token=randomUUID()+randomUUID(),tokenFile=join(privateDirectory,'management.private'),binary=join(privateDirectory,'standard-dav-fixture.exe')
  writeFileSync(tokenFile,token,{flag:'wx',mode:0o600})
  const fixture=join(root,'scripts/fixtures/workspace-dav-server')
  execFileSync('go',['build','-mod=readonly','-o',binary,'.'],{cwd:fixture,stdio:'pipe',windowsHide:true})
  const port=await freePort(),origin='http://127.0.0.1:'+port
  const process=spawn(binary,['--listen','127.0.0.1:'+port,'--root',join(privateDirectory,'state'),'--token-file',tokenFile],{windowsHide:true,stdio:['ignore','pipe','pipe']})
  registerProcess(process)
  const log=logStream('legacy-standard-dav');process.stdout.pipe(log);process.stderr.pipe(log)
  assert.ok(await waitFor(origin+'/live','standard DAV fixture'))
  assert.equal(process.exitCode,null,'Fixture process did not remain running')
  const mode={previewStatus:0},observed=[]
  const proxy=createLegacyPreviewProxy({target:origin,mode,observed});registerServer(proxy)
  const proxyPort=await freePort();proxy.listen(proxyPort,'127.0.0.1');await once(proxy,'listening')
  return {adminOrigin:origin,mcpOrigin:origin,davOrigin:'http://127.0.0.1:'+proxyPort,managementToken:token,mode,observed,
    identity:{kind:'real standard DAV fixture with deterministic management substitute; not a historical Runcove image',binarySHA256:'sha256:'+createHash('sha256').update(readFileSync(binary)).digest('hex')}}
}

export async function runLegacyAPI({root,env,users,api,raw,put,workspace,provider}) {
  const user=users[0],checks=[]
  const bytes=readFileSync(join(root,'api/fixtures/workspace/thumbnail.webp'))
  await put(user,'image.webp',bytes)
  const enrollment=await api('/api/admin/v1/users/'+user.userId+'/enrollments','POST',{expiresInSeconds:3600})
  const exchange=await fetch(env.hubBaseURL+'/api/client/v1/enrollments/exchange',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({platform:'ANDROID',deviceName:'Standard DAV compatibility',code:enrollment.code,installationId:'ins_'+randomUUID(),appVersion:'legacy-thumbnail-validation'})})
  assert.equal(exchange.status,201);const {accessToken}=await exchange.json()
  const clientFetch=(path,init={})=>fetch(new URL(path,env.hubBaseURL),{...init,headers:{Authorization:'Bearer '+accessToken,...init.headers}})
  const base='/api/client/v1/workspace',query=new URLSearchParams({agentSpaceId:user.spaceId})
  const file=base+'/content?'+new URLSearchParams({agentSpaceId:user.spaceId,path:'image.webp'})
  let response=await clientFetch(file,{method:'HEAD'});assert.equal(response.status,200)
  const etag=response.headers.get('ETag'),previewQuery=new URLSearchParams({agentSpaceId:user.spaceId,path:'image.webp',expectedEtag:etag,size:'128'})
  const absent={schemaVersion:1,supported:false,sizes:[],contentType:'image/webp',mediaKinds:[]}
  for(const status of [404,405]) {
    provider.mode.previewStatus=status===404?0:405
    const before=provider.observed.length
    response=await clientFetch(base+'/thumbnail?'+query,{method:'OPTIONS'});assert.equal(response.status,200);assert.deepEqual(await response.json(),absent)
    const calls=provider.observed.slice(before)
    assert.ok(calls.some(value=>value.method==='OPTIONS'&&value.status===status&&value.path.endsWith('/previews')))
    assert.ok(calls.some(value=>value.method==='PROPFIND'&&value.status===207&&value.path.endsWith('/dav/')))
    for(const method of ['GET','HEAD']){
      response=await clientFetch(base+'/thumbnail?'+previewQuery,{method});assert.equal(response.status,501)
      if(method==='GET')assert.equal((await response.json()).code,'preview_not_supported')
      else assert.equal((await response.arrayBuffer()).byteLength,0)
    }
    response=await raw(workspace(user)+'/thumbnail?'+previewQuery);assert.equal(response.status,501);assert.equal((await response.json()).code,'preview_not_supported')
    checks.push(`OPTIONS ${status} plus same-key authenticated real DAV root normalizes false; Client GET/HEAD and Admin GET return 501`)
    console.log('PASS LEGACY '+checks.at(-1))
  }
  provider.mode.previewStatus=0
  let options=0,gets=0
  const session=new WorkspaceThumbnailSession({endpoint:env.hubBaseURL+base+'/thumbnail',agentSpaceId:user.spaceId,fetch:(url,init)=>{if(init?.method==='OPTIONS')options++;else gets++;return clientFetch(url,init)}})
  try{assert.equal(await session.thumbnail({path:'image.webp',expectedEtag:etag}),undefined);assert.equal(await session.thumbnail({path:'image.webp',expectedEtag:etag}),undefined);assert.equal(options,1);assert.equal(gets,0)}finally{session.dispose()}
  checks.push('real Node connection example memoizes legacy unsupported discovery and never downloads a preview')
  provider.mode.previewStatus=403
  response=await clientFetch(base+'/thumbnail?'+previewQuery);assert.equal(response.status,403);assert.equal((await response.json()).code,'file_forbidden')
  for(const name of ['WWW-Authenticate','Set-Cookie','Location'])assert.equal(response.headers.get(name),null)
  response=await clientFetch(base);assert.equal((await response.json()).filesAvailable,true)
  response=await clientFetch(file);assert.equal(response.status,200);assert.deepEqual(Buffer.from(await response.arrayBuffer()),bytes)
  checks.push('explicitly injected preview 403 stays file_forbidden without withdrawing DAV, leaking headers or breaking original files')
  console.log('PASS LEGACY '+checks.at(-1))
  await publishWorkspaceDAV(api, true)
  const dav = await openPlatformDAV({ api, env, user, accessToken })
  for (const status of [404, 405, 403]) {
    provider.mode.previewStatus = status === 404 ? 0 : status
    const before = provider.observed.length
    response = await dav.send(dav.preview, { method: 'OPTIONS' })
    assert.equal(response.status, status)
    await response.arrayBuffer()
    assert.equal(provider.observed.length, before + 1, 'Relay passes the capability status without another upstream probe')
    assert.equal(provider.observed.at(-1).status, status)
    if (status === 403) continue
    response = await dav.send(dav.file(), { method: 'PROPFIND', headers: { Depth: '0' } })
    assert.equal(response.status, 207)
    await response.arrayBuffer()
    assert.equal(provider.observed.at(-1).status, 207)
  }
  checks.push('Relay preserves legacy 404/405 and explicit 403; the client verifies the same authenticated DAV root only for 404/405')
  console.log('PASS LEGACY '+checks.at(-1))
  provider.mode.previewStatus=0
  return {checks,provider:provider.identity,requests:provider.observed,limitations:['Management is a deterministic substitute; this is not a tested old Runcove release.','404 is returned by the existing standard DAV fixture; 405 and 403 are explicit local preview-response injections.']}
}
