/** Real local remote workspace acceptance. Requires a separately deployed pinned service.
 * Never accepts a non-loopback management origin. Credentials stay in input files.
 * Example: node scripts/workspace-integration.mjs --config .artifacts/workspace-test.json
 * Config: {adminOrigin,mcpOrigin,davOrigin,managementTokenFile,releaseIdentity,imageIdentity}
 */
import assert from 'node:assert/strict'
import { createHash, randomUUID } from 'node:crypto'
import { readFileSync, writeFileSync, mkdirSync, createWriteStream } from 'node:fs'
import { resolve, join } from 'node:path'
import { once } from 'node:events'
import { createFreshEnvironment, startHubAndRelay, waitFor, adminBuildHash } from './lib/harness.mjs'

const args=process.argv.slice(2), configPath=args[args.indexOf('--config')+1]
if(!args.includes('--config')||!configPath)throw Error('Pass --config with an isolated local Agent Space configuration')
const config=JSON.parse(readFileSync(resolve(configPath),'utf8'))
for(const field of ['adminOrigin','mcpOrigin','davOrigin'])assert.ok(['127.0.0.1','localhost','[::1]'].includes(new URL(config[field]).hostname),'Only isolated loopback services are accepted')
assert.ok(typeof config.releaseIdentity === 'string' && config.releaseIdentity.trim(), 'Record the tested Agent Space source identity')
const managementToken=readFileSync(resolve(config.managementTokenFile),'utf8').trim()
const root=resolve('.'),out=resolve(args.includes('--output')?args[args.indexOf('--output')+1]:'.artifacts/workspace-acceptance')
mkdirSync(out,{recursive:true})
const env=await createFreshEnvironment(root,{prefix:'measix-workspace-acceptance',deploymentName:'Workspace acceptance',displayName:'Acceptance admin',bootstrapStdio:'pipe',buildStdio:'pipe',migrateStdio:'pipe'})
// Private recovery material for a failed run; never include it in evidence/commits.
writeFileSync(join(out,'environment.json'),JSON.stringify(env,null,2),{mode:0o600})
process.env.HUB_ADMIN_ASSETS_DIR=join(root,'console/dist/spa')
let processes,session,cookie
const checks=[],key=()=> 'idem_'+randomUUID(),pause=()=>new Promise(r=>setTimeout(r,500))
const proof=(name)=>{checks.push(name);console.log('PASS '+name)}
async function start(){processes=startHubAndRelay(env);for(const [name,p]of Object.entries(processes)){const log=createWriteStream(join(out,name+'.log'),{flags:'a'});for(const stream of[p.stdout,p.stderr]){stream.removeAllListeners('data');stream.pipe(log)}}await waitFor(env.hubBaseURL+'/live','hub');await waitFor(env.relayIntBaseURL+'/live','relay')}
async function stop(){await Promise.all(Object.values(processes??{}).map(async p=>{if(p.exitCode!==null)return;const exited=once(p,'exit');p.kill();await exited}))}
async function login(){const r=await fetch(env.hubBaseURL+'/api/admin/v1/session/login',{method:'POST',headers:{'Content-Type':'application/json',Origin:env.hubBaseURL},body:JSON.stringify({username:'admin',password:env.adminPassword})});assert.equal(r.status,200);session=await r.json();cookie=r.headers.getSetCookie().map(x=>x.split(';')[0]).join('; ')}
async function raw(path,init={}){
 const match=path.match(/^(\/api\/admin\/v1\/users\/[^/]+\/workspace)\/(?:files|content)(?:\?|$)/)
 if(match&&!new URL(path,env.hubBaseURL).searchParams.has('agentSpaceId')){const view=await api(match[1]);path+=(path.includes('?')?'&':'?')+'agentSpaceId='+encodeURIComponent(view.agentSpaceId)}
 return fetch(env.hubBaseURL+path,{...init,headers:{cookie,Origin:env.hubBaseURL,'X-CSRF-Token':session.csrfToken,...init.headers}})}
async function api(path,method='GET',body,headers={}){const r=await raw(path,{method,headers:{...(body===undefined?{}:{'Content-Type':'application/json'}),...headers},body:body===undefined?undefined:JSON.stringify(body)});assert.ok(r.ok,path+' HTTP '+r.status);const text=await r.text();return text?JSON.parse(text):undefined}
async function complete(op){for(let i=0;i<180;i++){const v=await api('/api/admin/v1/workspace-operations/'+op.operationId);if(v.state==='COMPLETED'){assert.notEqual(v.step,'CHECK_FAILED');return v}assert.ok(!['UNKNOWN','NEEDS_ATTENTION'].includes(v.state),v.diagnosticCode);await pause()}throw Error('Operation did not complete')}
const base=u=>'/api/admin/v1/users/'+u.userId+'/workspace'
async function command(u,action){const v=await api(base(u));return complete(await api(base(u),'POST',{action,expectedRevision:v.bindingRevision,...(action==='DELETE'?{confirmation:v.agentSpaceId}:{})},{'Idempotency-Key':key()}))}
async function state(u,expected){for(let i=0;i<180;i++){const v=await api(base(u));if(v.state===expected)return v;await pause()}throw Error('Expected '+expected)}
async function enroll(u){const e=await api('/api/admin/v1/users/'+u.userId+'/enrollments','POST',{expiresInSeconds:3600});const r=await fetch(env.hubBaseURL+'/api/client/v1/enrollments/exchange',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({platform:'ANDROID',deviceName:'Workspace acceptance',code:e.code,installationId:'ins_'+randomUUID(),appVersion:'workspace-acceptance'})});assert.equal(r.status,201);return {...u,...await r.json()}}
let mcpId
async function mcp(u,method,params,id=1,extra={}){const r=await fetch(env.relayPubBaseURL+'/runtime/v1/resources/'+mcpId+'/mcp',{method:'POST',headers:{Authorization:'Bearer '+u.accessToken,'X-Measix-Managed-Generation':'1','X-Measix-Interaction-Id':'int_'+randomUUID(),'Content-Type':'application/json',Accept:'application/json, text/event-stream',...(u.mcpSession?{'Mcp-Session-Id':u.mcpSession}:{}),...extra},body:JSON.stringify({jsonrpc:'2.0',...(id===null?{}:{id}),method,params})});if(method==='initialize')u.mcpSession=r.headers.get('mcp-session-id');const text=await r.text();let body;for(const line of text.split('\n'))if(line.startsWith('data: {'))body=JSON.parse(line.slice(6));return{status:r.status,body}}
async function initMCP(u){const r=await mcp(u,'initialize',{protocolVersion:'2024-11-05',capabilities:{},clientInfo:{name:'acceptance',version:'1'}});assert.equal(r.status,200);assert.ok(u.mcpSession);await mcp(u,'notifications/initialized',{},null)}
async function tool(u,name,args){const r=await mcp(u,'tools/call',{name,arguments:args});assert.equal(r.status,200);assert.ok(r.body?.result&&!r.body.result.isError,JSON.stringify(r.body));return r.body.result}
const users=[]
try{
 await start();await login()
 if(args.includes('--ui-only')){
  writeFileSync(join(out,'ui-env.json'),JSON.stringify(env,null,2),{mode:0o600})
  console.log('UI review ready: '+env.hubBaseURL+'/admin/ (credentials in private ui-env.json)')
  await new Promise(resolve=>{process.once('SIGINT',resolve);process.once('SIGTERM',resolve)})
  process.exitCode=0
 }else{
 const noService=await api(base(session.user));assert.equal(noService.serviceState,'NOT_CONFIGURED');assert.equal(noService.state,'UNPROVISIONED')
 const secret=await api('/api/admin/v1/secrets','POST',{name:'Isolated Agent Space management',value:managementToken})
 const workspaceService=await api('/api/admin/v1/remote-workspace/services','POST',{name:'Agent Space acceptance',expectedRevision:0,config:{adminOrigin:config.adminOrigin,mcpOrigin:config.mcpOrigin,davOrigin:config.davOrigin,managementSecret:{secretId:secret.secretId,secretVersion:secret.secretVersion},connectTimeoutMs:90000,idleTimeoutMs:120000}},{'Idempotency-Key':key()});mcpId=workspaceService.mcpServerId
 assert.equal((await api(base(session.user))).serviceState,'DISABLED')
 await api('/api/admin/v1/remote-workspace/services/'+workspaceService.workspaceServiceId+'/check','POST');await complete(await api('/api/admin/v1/remote-workspace/services/'+workspaceService.workspaceServiceId+'/apply','POST',undefined,{'Idempotency-Key':key()}))
 assert.equal((await api(base(session.user))).serviceState,'ENABLED');proof('unprovisioned user distinguishes unconfigured, disabled and enabled service')
 for(const n of['a','b']){const u=await api('/api/admin/v1/users','POST',{username:'acceptance_'+n,displayName:'Acceptance '+n,role:'MEMBER'});users.push(u);await command(u,'CREATE')}
 const beforeMcp=await api(base(users[0]));assert.equal(beforeMcp.filesAvailable,true);assert.equal(beforeMcp.mcpAvailable,false)
 const resourcePath=(u,spc)=>base(u)+'/resources?agentSpaceId='+encodeURIComponent(spc)
 const cold=await api(resourcePath(users[0],beforeMcp.agentSpaceId))
 assert.equal(cold.agentSpaceId,beforeMcp.agentSpaceId);assert.equal(cold.runtime.value,'notfound');assert.equal(cold.disk.status,'unavailable');assert.equal(cold.allocation.cpuCores.source,'default')
 await new Promise(resolve=>setTimeout(resolve,5500))
 assert.equal((await api(resourcePath(users[0],beforeMcp.agentSpaceId))).runtime.value,'notfound')
 let unauthorized=await fetch(env.hubBaseURL+resourcePath(users[0],beforeMcp.agentSpaceId));assert.equal(unauthorized.status,401);await unauthorized.body.cancel()
 let mismatch=await raw(resourcePath(users[1],beforeMcp.agentSpaceId));assert.equal(mismatch.status,409);await mismatch.body.cancel()
 proof('admin resource authorization, space isolation and repeated cold reads never start a VM')
 let pre=await raw(base(users[0])+'/content?path=before-mcp.txt',{method:'PUT',headers:{'If-None-Match':'*'},body:'Files work before optional MCP publication'});assert.equal(pre.status,200);await pre.body.cancel()
 pre=await raw(base(users[0])+'/content?path=before-mcp.txt');assert.equal(await pre.text(),'Files work before optional MCP publication')
 const initialDraft=await api('/api/admin/v1/draft');assert.ok(!initialDraft.content.mcp.some(x=>x.mcpServerId===mcpId))
 proof('provision and file management before optional MCP publication; saving service leaves draft unchanged')
 const activeResources=await api(resourcePath(users[0],beforeMcp.agentSpaceId))
 assert.equal(activeResources.runtime.value,'running');assert.equal(activeResources.disk.status,'current');assert.ok(activeResources.disk.value.usedBytes>0);assert.equal(activeResources.allocation.cpuCores.source,'sandbox')
 assert.equal(activeResources.memory.status,'current');assert.ok(activeResources.memory.value>0)
 proof('real running disk and guest memory observations before optional MCP publication')
 await api('/api/admin/v1/remote-workspace/services/'+workspaceService.workspaceServiceId+'/mcp-draft','POST',{expectedDraftRevision:initialDraft.draftRevision},{'Idempotency-Key':key()})
 const draft=await api('/api/admin/v1/draft'),validation=await api('/api/admin/v1/draft:validate','POST',{expectedDraftRevision:draft.draftRevision})
 const activation=await api('/api/admin/v1/draft:publish','POST',{expectedDraftRevision:draft.draftRevision,acknowledgedWarningCodes:(validation.warnings??[]).map(w=>w.code)},{'Idempotency-Key':key()})
 for(let i=0;i<60;i++){const v=await api('/api/admin/v1/activations/'+activation.activationId);if(v.state==='COMPLETED')break;assert.notEqual(v.state,'FAILED');await pause()}
 proof('explicit optional MCP staging and normal draft publish')
 assert.equal((await api(base(users[0]))).mcpAvailable,true)
 const a=await enroll(users[0]),b=await enroll(users[1]);await initMCP(a);await initMCP(b)
 await tool(a,'write',{path:'/workspace/跨入口.txt',content:'MCP and DAV share this original space\n'})
 let r=await raw(base(a)+'/content?path='+encodeURIComponent('跨入口.txt'));assert.equal(await r.text(),'MCP and DAV share this original space\n')
 const clientView=await fetch(env.hubBaseURL+'/api/client/v1/workspace',{headers:{Authorization:'Bearer '+a.accessToken}}).then(r=>r.json())
 assert.equal(clientView.serviceState,'ENABLED')
 const clientURL=env.hubBaseURL+'/api/client/v1/workspace/content?'+new URLSearchParams({path:'client-edit.txt',agentSpaceId:clientView.agentSpaceId})
 const clientHeaders={Authorization:'Bearer '+a.accessToken}
 const initialText=Buffer.from('\uFEFFfirst\r\n')
 let cr=await fetch(clientURL,{method:'PUT',headers:{...clientHeaders,'If-None-Match':'*'},body:initialText});assert.equal(cr.status,200);await cr.body.cancel()
 cr=await fetch(clientURL,{headers:clientHeaders});assert.equal(cr.status,200);const editETag=cr.headers.get('etag');assert.deepEqual(Buffer.from(await cr.arrayBuffer()),initialText)
 cr=await fetch(clientURL,{method:'PUT',headers:{...clientHeaders,'If-Match':editETag},body:Buffer.from('\uFEFFedited text\r\n')});assert.equal(cr.status,200);await cr.body.cancel()
 cr=await fetch(clientURL,{method:'PUT',headers:{...clientHeaders,'If-Match':editETag},body:'stale overwrite'});assert.equal(cr.status,409);assert.equal((await cr.json()).code,'file_version_conflict')
 const wrong=new URL(clientURL);wrong.searchParams.set('agentSpaceId','spc_'+randomUUID())
 cr=await fetch(wrong,{method:'PUT',headers:{...clientHeaders,'If-None-Match':'*'},body:'wrong space'});assert.equal(cr.status,409);assert.equal((await cr.json()).code,'workspace_space_mismatch')
 const missing=new URL(clientURL);missing.searchParams.delete('agentSpaceId');cr=await fetch(missing,{headers:clientHeaders});assert.equal(cr.status,400);await cr.body.cancel()
 cr=await fetch(clientURL,{headers:clientHeaders});assert.deepEqual(Buffer.from(await cr.arrayBuffer()),Buffer.from('\uFEFFedited text\r\n'))
 proof('native client file authentication, exact UTF-8 bytes, conditional editing and required original space')
 const other=await mcp(b,'tools/call',{name:'read',arguments:{path:'/workspace/跨入口.txt'}});assert.ok(other.body?.result?.isError)
 const cross=await mcp(b,'tools/list',{},3,{'Mcp-Session-Id':a.mcpSession,'X-Measix-User-Id':a.userId});assert.ok([401,404].includes(cross.status))
 const snapshots=[];for(const u of[a,b]){const q=await fetch(env.hubBaseURL+'/api/client/v1/managed/snapshots/1',{headers:{Authorization:'Bearer '+u.accessToken}});assert.equal(q.status,200);snapshots.push(await q.text())}assert.equal(snapshots[0],snapshots[1]);proof('two-user real MCP isolation, DAV interoperability and byte-identical Snapshot')
 const budgetPath='/api/admin/v1/users/'+b.userId+'/budgets/MCP'
 const budget=await api(budgetPath,'PUT',{expectedRevision:0,mode:'LIMITED',limits:[{period:'LIFETIME',meter:'REQUESTS',limit:'0'}],reason:'Workspace quota rejection acceptance'})
 const blocked=await mcp(b,'tools/list',{},4);assert.equal(blocked.status,429)
 await api(budgetPath,'PUT',{expectedRevision:budget.revision,mode:'UNLIMITED',limits:[],reason:'Finish acceptance quota case'})
 proof('remote workspace MCP budget rejects before forwarding')
 const content=base(a)+'/content?path=large.bin',expected=createHash('sha256')
 async function* upload(){for(let offset=0;offset<64*1024*1024;offset+=65536){const chunk=Buffer.alloc(65536);for(let j=0;j<chunk.length;j+=4)chunk.writeUInt32LE(offset+j,j);expected.update(chunk);yield chunk}}
 r=await raw(content,{method:'PUT',headers:{'If-None-Match':'*','Content-Type':'application/octet-stream'},body:upload(),duplex:'half'});assert.equal(r.status,200,r.ok?'':await r.text());await r.body.cancel()
 r=await raw(content,{method:'HEAD'});assert.equal(r.status,200);assert.equal(Number(r.headers.get('Content-Length')),64*1024*1024);const etag=r.headers.get('etag')
 r=await raw(content,{headers:{Range:'bytes=0-1023','If-Match':etag}});assert.equal(r.status,206);assert.equal((await r.arrayBuffer()).byteLength,1024)
 r=await raw(content);const actual=createHash('sha256');let total=0;for await(const chunk of r.body){actual.update(chunk);total+=chunk.length}assert.equal(total,64*1024*1024);assert.equal(actual.digest('hex'),expected.digest('hex'))
 for(const [headers,status]of [[{'If-Match':'"stale"'},409],[{'If-None-Match':etag},304],[{Range:'bytes=999999999-'},416]]){r=await raw(content,{headers});assert.equal(r.status,status);await r.body?.cancel()}
 proof('64 MiB bounded streaming, digest, HEAD, Range, conditions and invalid Range')
 await new Promise(resolve=>setTimeout(resolve,5500))
 const grownResources=await api(resourcePath(a,beforeMcp.agentSpaceId))
 assert.equal(grownResources.disk.status,'current');assert.ok(grownResources.disk.value.usedBytes>=activeResources.disk.value.usedBytes+64*1024*1024)
 proof('resource disk usage increases after real 64 MiB file transfer')
 const textPath=base(a)+'/content?path='+encodeURIComponent('跨入口.txt')
 r=await raw(textPath,{method:'HEAD'});const textEtag=r.headers.get('etag')
 const abort=new AbortController(),timer=setTimeout(()=>abort.abort(),150)
 async function* slowUpload(){while(!abort.signal.aborted){yield Buffer.alloc(65536,88);await new Promise(resolve=>setTimeout(resolve,40))}}
 try{await raw(textPath,{method:'PUT',headers:{'If-Match':textEtag},body:slowUpload(),duplex:'half',signal:abort.signal});assert.fail('cancelled upload unexpectedly completed')}catch(e){assert.ok(abort.signal.aborted)}finally{clearTimeout(timer)}
 await pause();r=await raw(textPath);assert.equal(await r.text(),'MCP and DAV share this original space\n');proof('cancelled slow overwrite preserves original file')

 const original=await api(base(a)),dav=await api(base(a)+'/dav-connection','POST')
 r=await fetch(dav.davUrl+encodeURIComponent('跨入口.txt'),{headers:{Authorization:'Basic '+Buffer.from(dav.username+':'+dav.token).toString('base64')}});assert.equal(r.status,200);await r.body.cancel()
 await command(a,'DISCONNECT');await state(a,'DISCONNECTED');r=await fetch(dav.davUrl,{headers:{Authorization:'Bearer '+dav.token},method:'PROPFIND'});assert.ok([401,403].includes(r.status));await r.body.cancel()
 const stoppedResources=await api(resourcePath(a,original.agentSpaceId))
 assert.equal(stoppedResources.runtime.value,'stopped');assert.equal(stoppedResources.memory.status,'unavailable');assert.equal(stoppedResources.disk.status,'historical');assert.ok(stoppedResources.disk.observedAt>=grownResources.disk.observedAt)
 assert.equal((await api(resourcePath(a,original.agentSpaceId))).disk.observedAt,stoppedResources.disk.observedAt)
 proof('disconnected space stays observable with original disk timestamp and no fake zero memory')
 await command(a,'RESTORE');let v=await api(base(a));assert.equal(v.agentSpaceId,original.agentSpaceId);assert.equal(v.filesAvailable,false);await command(a,'SET_DAV');proof('external DAV Basic/Bearer, disconnect, original-space restore and explicit DAV reissue')
 await command(b,'DISCONNECT');await complete(await api('/api/admin/v1/remote-workspace/services/'+workspaceService.workspaceServiceId+'/disable','POST',undefined,{'Idempotency-Key':key()}));await state(a,'DISCONNECTED')
 assert.equal((await api(resourcePath(a,original.agentSpaceId))).runtime.value,'stopped');proof('disabled enterprise service retains admin resource inspection')
 await complete(await api('/api/admin/v1/remote-workspace/services/'+workspaceService.workspaceServiceId+'/apply','POST',undefined,{'Idempotency-Key':key()}));v=await state(a,'CONNECTED');assert.equal(v.filesAvailable,false);assert.equal((await api(base(b))).state,'DISCONNECTED');await command(a,'SET_DAV');proof('service re-enable restores prior intent without auto DAV or manual-disconnect revival')
 await stop();await start();await login();v=await api(base(a));assert.equal(v.agentSpaceId,original.agentSpaceId);r=await raw(base(a)+'/content?path='+encodeURIComponent('跨入口.txt'));assert.equal(r.status,200);await r.body.cancel();proof('Hub/Relay restart preserves original space, credentials and files')
 const usage=await api('/api/admin/v1/usage/requests?pageSize=100');assert.ok(usage.items.some(x=>x.workspaceTarget?.agentSpaceId===original.agentSpaceId&&x.targetVersion===2&&!x.upstreamId&&x.settlementState==='SETTLED'));proof('real admission and settled usage retain immutable workspace attribution')
 for(const u of users){await command(u,'DELETE');assert.equal((await api(base(u))).state,'UNPROVISIONED')}
 proof('remote deletion completes and active local bindings are removed')
 await command(a,'CREATE');const recreated=await api(base(a));assert.notEqual(recreated.agentSpaceId,original.agentSpaceId)
 mismatch=await raw(resourcePath(a,original.agentSpaceId));assert.equal(mismatch.status,409);await mismatch.body.cancel()
 const recreatedResources=await api(resourcePath(a,recreated.agentSpaceId));assert.equal(recreatedResources.runtime.value,'notfound');assert.equal(recreatedResources.disk.status,'unavailable');assert.equal(recreatedResources.disk.value,null)
 await command(a,'DELETE');proof('same-user recreation rejects old resource identity and never reuses prior disk history')
 const sha=p=>'sha256:'+createHash('sha256').update(readFileSync(p)).digest('hex')
 writeFileSync(join(out,'evidence.json'),JSON.stringify({verifiedAt:new Date().toISOString(),checks,agentSpace:{release:config.releaseIdentity,image:config.imageIdentity},binaries:{hub:sha(env.hubBin),relay:sha(env.relayBin)},adminBuild:adminBuildHash(root),contracts:Object.fromEntries(['admin/admin.openapi.yaml','client/client-control.openapi.yaml','internal/relay-control.openapi.yaml','internal/usage-ingest.openapi.yaml'].map(p=>[p,sha(join(root,'api',p))]))},null,2)+'\n')
 if(args.includes('--keep-ui')){
  const u=await api('/api/admin/v1/users','POST',{username:'workspace_review',displayName:'Workspace browser review',role:'MEMBER'});await command(u,'CREATE')
  writeFileSync(join(out,'ui-env.json'),JSON.stringify({...env,user:u,workspaceServiceId:workspaceService.workspaceServiceId},null,2),{mode:0o600})
  console.log('UI review ready: '+env.hubBaseURL+'/admin/ (credentials in private ui-env.json)')
  await new Promise(resolve=>{process.once('SIGINT',resolve);process.once('SIGTERM',resolve)})
 }
 }
}finally{await stop();console.log('Evidence directory: '+out)}
