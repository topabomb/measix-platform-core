/** Real thumbnail browser acceptance. Uses the existing Hub/Relay harness and
 * Agent Space fixture owner. No existing service, DB, volume or user is input.
 * Private keys/passwords stay in this run's private temporary environment.
 */
import assert from 'node:assert/strict'
import { createHash, randomUUID } from 'node:crypto'
import { spawn, execFileSync } from 'node:child_process'
import { once } from 'node:events'
import { copyFileSync, createWriteStream, existsSync, mkdirSync, readFileSync, statfsSync, writeFileSync } from 'node:fs'
import { resolve, join, relative, sep } from 'node:path'
import { createInterface } from 'node:readline'
import { pathToFileURL } from 'node:url'
import { createFreshEnvironment, startHubAndRelay, snapshotAdminBuild, waitFor, cleanupEnvironment, collectFiles } from './lib/harness.mjs'

const root = resolve(import.meta.dirname, '..')
const digest = bytes => 'sha256:'+createHash('sha256').update(bytes).digest('hex')
const pause = ms => new Promise(resolveP => setTimeout(resolveP, ms))
const floor = 5n*1024n*1024n*1024n
// Native resource timing intervals, not ordering of asynchronous Playwright
// request/finished callbacks. Equal endpoints do not represent overlap.
export function maximumConcurrentTransfers(transfers) {
  const events=[]
  for(const {start,end} of transfers) {
    assert.ok(Number.isFinite(start)&&Number.isFinite(end)&&end>=start,'Invalid native transfer timing')
    if(end>start)events.push([start,1],[end,-1])
  }
  events.sort((a,b)=>a[0]-b[0]||a[1]-b[1])
  let active=0,maximum=0
  for(const [,change]of events){active+=change;maximum=Math.max(maximum,active)}
  return maximum
}
export function thumbnailReadInterval(value) {
  const {start,end,abortAt}=value
  assert.ok(Number.isFinite(start)&&Number.isFinite(end)&&end>=start,'Invalid native transfer timing')
  if(abortAt!==undefined)assert.ok(Number.isFinite(abortAt),'Invalid observed abort time')
  return {...value,networkEnd:end,end:abortAt===undefined?end:Math.max(start,Math.min(end,abortAt))}
}
// The held response must be a verified real success before cancellation tests
// can proceed. Transport failure before that point is a failed precondition.
export async function holdPreview(page,path,size,proofs) {
  let resolveReady,rejectReady,releaseGate,verified=false,proof
  const gate=new Promise(resolveP=>{releaseGate=resolveP}),attemptStatuses=[]
  let timer
  const ready=new Promise((resolveP,reject)=>{
    resolveReady=resolveP;rejectReady=reject
    timer=setTimeout(()=>reject(Error('Held preview failed before verified 200: timeout')),15000)
  })
  const handler=async route=>{
    const query=new URL(route.request().url()).searchParams
    if(route.request().method()!=='GET'||query.get('path')!==path||size!==undefined&&Number(query.get('size'))!==size){await route.continue();return}
    if(verified){await route.continue();return}
    try{
      const response=await route.fetch(),status=response.status()
      attemptStatuses.push(status)
      if(status!==200){await route.fulfill({response});return}
      const headers=response.headers(),body=await response.body(),hash=digest(body).slice(7)
      assert.ok(body.length>0&&body.length<=1024*1024)
      assert.equal(headers['content-type'],'image/webp')
      assert.equal(headers['agent-space-id'],query.get('agentSpaceId'))
      assert.equal(headers['preview-source-etag'],query.get('expectedEtag'))
      assert.equal(headers.etag,'"'+hash+'"')
      assert.equal(body.subarray(0,4).toString(),'RIFF');assert.equal(body.subarray(8,12).toString(),'WEBP')
      proof={path,size:Number(query.get('size')),status,byteLength:body.length,bodySHA256:hash,sourceETag:headers['preview-source-etag'],spaceId:headers['agent-space-id'],representationETag:headers.etag,attemptStatuses:[...attemptStatuses],verifiedAt:Date.now()}
      proofs.push(proof);verified=true;clearTimeout(timer);resolveReady()
      await gate;await route.fulfill({response})
    }catch{
      if(!verified){clearTimeout(timer);rejectReady(Error('Held preview failed before verified 200'))}
      else proof.lateFulfillRejected=true
    }
  }
  await page.route('**/workspace/thumbnail?**',handler)
  return {ready,async release(){clearTimeout(timer);releaseGate();await page.unroute('**/workspace/thumbnail?**',handler)}}
}
export function validateInputs(options) {
  for (const key of (options.legacyOnly?['adminBuild']:['image','adminBuild'])) assert.match(options[key]??'', /^sha256:[0-9a-f]{64}$/, `Exact ${key} required`)
  if(!options.legacyOnly)for (const key of ['expectedSource','expectedHelper']) assert.match(options[key]??'', /^[0-9a-f]{64}$/, `Independent ${key} required`)
  if(options.apiOnly||options.expectedWorkspace)assert.match(options.expectedWorkspace??'',/^sha256:[0-9a-f]{64}$/,'Independent Workspace identity required')
  assert.ok(!(options.apiOnly&&options.setupOnly),'Choose API-only or setup-only')
  assert.ok(!(options.legacyOnly&&(options.apiOnly||options.setupOnly)),'Choose exactly one validation lane')
  if(!options.legacyOnly)assert.match(options.agentSpaceSource??'', /^\/(?!\/)[^\r\n]+$/, 'Pass an explicit WSL Agent Space source directory')
  const parent = resolve(root,'.artifacts/media-preview'), output = resolve(options.output??'')
  assert.equal(output.slice(0,2).toUpperCase(),'D:','Evidence must stay on D:')
  assert.ok(output.startsWith(parent+sep),'Evidence must be below .artifacts/media-preview')
  assert.ok(!existsSync(output),'Occupied evidence directory is never reused')
  return {...options, output}
}
function checkDisk() {
  const state=statfsSync(root,{bigint:true}), free=state.bavail*state.bsize
  assert.ok(free>=floor,`D: free space below the 5 GiB acceptance floor (${free} bytes)`)
  return Number(free)
}
function wslPath(path) {
  const absolute=resolve(path)
  assert.match(absolute,/^D:\\/i,'Only the D: evidence/workspace path is mapped')
  return '/mnt/d/'+absolute.slice(3).replaceAll('\\','/')
}
function docker(...args) { return execFileSync('wsl.exe',['-d','Ubuntu','--','docker',...args],{encoding:'utf8',windowsHide:true}).trim() }
function inventory() {
  return {containers:docker('ps','-aq').split(/\s+/).filter(Boolean),volumes:docker('volume','ls','-q').split(/\s+/).filter(Boolean)}
}
function backendSource() {
  const hash=createHash('sha256')
  for(const path of collectFiles(join(root,'backend')).filter(path=>/\.(go|sql)$/.test(path)||/go\.(mod|sum)$/.test(path)).sort()) {
    hash.update(relative(root,path).replaceAll('\\','/'));hash.update('\0');hash.update(readFileSync(path));hash.update('\0')
  }
  return 'sha256:'+hash.digest('hex')
}
async function stopProcess(child, timeout=10000) {
  if (!child || child.exitCode!==null || child.signalCode!==null) return
  let stopped=exitWithin(child,timeout)
  child.kill('SIGTERM')
  if(!await stopped) {stopped=exitWithin(child,2000);child.kill('SIGKILL');await stopped}
  assert.ok(child.exitCode!==null||child.signalCode!==null,'Owned process survived cleanup')
}
function exitWithin(child, timeout) {
  if(child.exitCode!==null||child.signalCode!==null)return Promise.resolve(true)
  return new Promise(resolveP=>{
    const done=()=>{clearTimeout(timer);resolveP(true)}
    const timer=setTimeout(()=>{child.off('exit',done);resolveP(false)},timeout)
    child.once('exit',done)
  })
}
export async function run(input) {
  const options=validateInputs(input), out=options.output
  checkDisk();mkdirSync(out)
  const privateTemp=join(out,'private-temp');mkdirSync(privateTemp)
  process.env.TEMP=privateTemp;process.env.TMP=privateTemp;process.env.TMPDIR=privateTemp;process.env.GOTMPDIR=privateTemp
  const report={passed:false,cleanupPassed:false,startedAt:new Date().toISOString(),expected:options,checks:[],diskFreeStart:checkDisk()}
  const sourcePaths=['scripts/workspace-thumbnail-browser.mjs','scripts/fixtures/workspace-preview-service.py','console/e2e/workspace-thumbnails.spec.ts',...(options.apiOnly?['scripts/workspace-thumbnail-api.mjs','scripts/examples/workspace-thumbnail-session.mjs']:[]),...(options.legacyOnly?['scripts/workspace-thumbnail-legacy.mjs','scripts/examples/workspace-thumbnail-session.mjs','scripts/fixtures/workspace-dav-server/main.go','scripts/fixtures/workspace-dav-server/go.mod','scripts/fixtures/workspace-dav-server/go.sum']:[])]
  const sourceSnapshot=join(out,'validation-source');mkdirSync(sourceSnapshot)
  report.sources=Object.fromEntries(sourcePaths.map(path=>{
    const source=join(root,path),copy=join(sourceSnapshot,path.replaceAll('/','__'))
    copyFileSync(source,copy);return [path,digest(readFileSync(copy))]
  }))
  const sourcesUnchanged=()=>sourcePaths.every(path=>digest(readFileSync(join(root,path)))===report.sources[path])
  const baseline=inventory();report.dockerBefore=baseline
  let fixture,fixtureReady,legacyProvider,legacyDirectory,env,processes={},browser,diskTimer,interrupted=false,session,cookie='',baseURL
  const servers=[]
  const users=[]
  const logs=[]
  const logStream=name=>{const stream=createWriteStream(join(out,name+'.log'),{flags:'wx'});logs.push(stream);return stream}
  const key=()=> 'idem_'+randomUUID()
  async function raw(path,init={}) {
    return fetch(baseURL+path,{...init,headers:{cookie,Origin:baseURL,'X-CSRF-Token':session?.csrfToken??'',...init.headers}})
  }
  async function api(path,method='GET',body,headers={}) {
    const response=await raw(path,{method,headers:{...(body===undefined?{}:{'Content-Type':'application/json'}),...headers},body:body===undefined?undefined:JSON.stringify(body)})
    assert.ok(response.ok,`${path}: HTTP ${response.status} ${(await response.clone().text()).slice(0,300)}`)
    const text=await response.text();return text?JSON.parse(text):undefined
  }
  const workspace=user=>'/api/admin/v1/users/'+user.userId+'/workspace'
  async function complete(operation) {
    for(let i=0;i<240;i++) {
      const value=await api('/api/admin/v1/workspace-operations/'+operation.operationId)
      if(value.state==='COMPLETED'){assert.notEqual(value.step,'CHECK_FAILED');return value}
      assert.ok(!['UNKNOWN','NEEDS_ATTENTION'].includes(value.state),value.diagnosticCode);await pause(500)
    }
    throw Error('Workspace operation did not complete')
  }
  async function command(user,action) {
    const view=await api(workspace(user))
    return complete(await api(workspace(user),'POST',{action,expectedRevision:view.bindingRevision,...(action==='DELETE'?{confirmation:view.agentSpaceId}:{})},{'Idempotency-Key':key()}))
  }
  async function put(user,path,bytes) {
    const response=await raw(workspace(user)+'/content?'+new URLSearchParams({agentSpaceId:user.spaceId,path}),{method:'PUT',headers:{'If-None-Match':'*','Content-Type':'application/octet-stream'},body:bytes})
    assert.equal(response.status,200,`${path} seed: ${await response.text()}`)
  }
  function minimumPDF() {
    const objects=['<< /Type /Catalog /Pages 2 0 R >>','<< /Type /Pages /Kids [3 0 R] /Count 1 >>','<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 100] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>','<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>']
    const content='BT /F1 18 Tf 20 50 Td (Preview acceptance) Tj ET'
    objects.push(`<< /Length ${content.length} >>\nstream\n${content}\nendstream`)
    let text='%PDF-1.4\n',offsets=[0]
    for(const [index,object] of objects.entries()){offsets.push(Buffer.byteLength(text));text+=`${index+1} 0 obj\n${object}\nendobj\n`}
    const start=Buffer.byteLength(text)
    text+=`xref\n0 ${objects.length+1}\n0000000000 65535 f \n`+offsets.slice(1).map(offset=>String(offset).padStart(10,'0')+' 00000 n \n').join('')+`trailer\n<< /Size ${objects.length+1} /Root 1 0 R >>\nstartxref\n${start}\n%%EOF\n`
    return Buffer.from(text)
  }
  const onSignal=()=>{interrupted=true;browser?.kill('SIGTERM');fixture?.stdin.end()}
  process.on('SIGINT',onSignal);process.on('SIGTERM',onSignal)
  try {
    if(options.legacyOnly){
      legacyDirectory=join(privateTemp,'legacy-provider')
      const {startLegacyProvider}=await import('./workspace-thumbnail-legacy.mjs')
      legacyProvider=await startLegacyProvider({root,privateDirectory:legacyDirectory,registerProcess:child=>{processes.legacyDAVProc=child},registerServer:server=>servers.push(server),logStream})
      fixtureReady=legacyProvider;report.fixture=legacyProvider.identity
    }else{
    fixture=spawn('wsl.exe',['-d','Ubuntu','--','python3','-B','-u',wslPath(join(root,'scripts/fixtures/workspace-preview-service.py')),
      '--source',options.agentSpaceSource,'--output',wslPath(join(out,'agent-space')),'--image',options.image,
      '--expected-source',options.expectedSource,'--expected-helper',options.expectedHelper,...(options.expectedWorkspace?['--expected-workspace',options.expectedWorkspace]:[])],{windowsHide:true,stdio:['pipe','pipe','pipe']})
    fixture.stderr.pipe(logStream('fixture-bridge'))
    fixtureReady=await new Promise((resolveP,reject)=>{
      const lines=createInterface({input:fixture.stdout})
      const timer=setTimeout(()=>reject(Error('Agent Space fixture startup timed out')),240000)
      lines.on('line',line=>{try{const value=JSON.parse(line);if(value.ready){clearTimeout(timer);resolveP(value)}}catch{reject(Error('Unexpected fixture bridge output'))}})
      fixture.once('error',reject);fixture.once('exit',code=>{clearTimeout(timer);reject(Error('Fixture exited before readiness: '+code))})
    })
    report.fixture={container:fixtureReady.container,owner:fixtureReady.owner,image:options.image}
    }
    checkDisk()
    report.backendSource=backendSource()
    env=await createFreshEnvironment(root,{prefix:'thumbnail-browser',deploymentName:'Thumbnail browser acceptance',displayName:'Browser admin',buildStdio:'pipe',bootstrapStdio:'pipe',migrateStdio:'pipe'})
    assert.equal(backendSource(),report.backendSource,'Backend source changed while building')
    assert.ok(resolve(env.envRoot).startsWith(resolve(privateTemp)+sep),'Harness temp escaped the run-owned D: directory')
    const build=snapshotAdminBuild(root,out)
    assert.equal(build.buildHash,options.adminBuild,'Admin dist differs from the explicitly approved production build')
    report.servedAdmin=build
    process.env.HUB_ADMIN_ASSETS_DIR=build.directory
    const coreProcesses=startHubAndRelay(env);Object.assign(processes,coreProcesses)
    for(const [name,child] of Object.entries(coreProcesses)) {const log=logStream(name);for(const stream of[child.stdout,child.stderr]){stream.removeAllListeners('data');stream.pipe(log)}}
    baseURL=env.hubBaseURL
    assert.ok(await waitFor(baseURL+'/live','Hub'));assert.ok(await waitFor(env.relayIntBaseURL+'/live','Relay'))
    const indexResponse=await fetch(baseURL+'/admin/');assert.equal(indexResponse.status,200)
    const html=await indexResponse.arrayBuffer()
    assert.equal(digest(Buffer.from(html)),digest(readFileSync(join(build.directory,'index.html'))),'Served Admin index differs from pinned SPA')
    report.binaries={hub:digest(readFileSync(env.hubBin)),relay:digest(readFileSync(env.relayBin))}
    const login=await fetch(baseURL+'/api/admin/v1/session/login',{method:'POST',headers:{'Content-Type':'application/json',Origin:baseURL},body:JSON.stringify({username:'admin',password:env.adminPassword})})
    assert.equal(login.status,200);session=await login.json();cookie=login.headers.getSetCookie().map(value=>value.split(';')[0]).join('; ')
    const token=legacyProvider?.managementToken??readFileSync(join(out,'agent-space/management-token.private'),'utf8').trim()
    const secret=await api('/api/admin/v1/secrets','POST',{name:'Isolated browser fixture management',value:token})
    const service=await api('/api/admin/v1/remote-workspace/services','POST',{name:'Thumbnail browser fixture',expectedRevision:0,config:{adminOrigin:fixtureReady.adminOrigin,mcpOrigin:fixtureReady.mcpOrigin,davOrigin:fixtureReady.davOrigin,managementSecret:{secretId:secret.secretId,secretVersion:secret.secretVersion},connectTimeoutMs:90000,idleTimeoutMs:120000}},{'Idempotency-Key':key()})
    await api('/api/admin/v1/remote-workspace/services/'+service.workspaceServiceId+'/check','POST')
    await complete(await api('/api/admin/v1/remote-workspace/services/'+service.workspaceServiceId+'/apply','POST',{expectedConfigRevision:service.configRevision},{'Idempotency-Key':key()}))
    for(const name of ['A','B']) {
      const user=await api('/api/admin/v1/users','POST',{username:'thumbnail_'+name.toLowerCase()+'_'+randomUUID().slice(0,8),displayName:'Thumbnail Browser '+name,role:'MEMBER'})
      users.push(user);await command(user,'CREATE');user.spaceId=(await api(workspace(user))).agentSpaceId
    }
    const media=join(out,'agent-space/media')
    if(!options.legacyOnly){
    const image=readFileSync(join(media,'image.png'))
    for(const user of users) {
      await api(workspace(user)+'/files?'+new URLSearchParams({agentSpaceId:user.spaceId}),'POST',{action:'MKCOL',path:'nested'})
      if(!options.apiOnly)await api(workspace(user)+'/files?'+new URLSearchParams({agentSpaceId:user.spaceId}),'POST',{action:'MKCOL',path:'vanishing'})
      await put(user,'nested/inside.png',image)
      if(options.apiOnly){
        await api(workspace(user)+'/files?'+new URLSearchParams({agentSpaceId:user.spaceId}),'POST',{action:'MKCOL',path:'nested/deeper'})
        await put(user,'nested/video.mp4',readFileSync(join(media,'video.mp4')))
      }
      await put(user,'00-image.png',image);await put(user,'01-video.mp4',readFileSync(join(media,'video.mp4')))
      await put(user,'01-video.webm',readFileSync(join(media,'video.webm')))
      if(!options.apiOnly)for(let index=2;index<30;index++)await put(user,`${String(index).padStart(2,'0')}-image.png`,image)
      await put(user,'notes.txt',Buffer.from('\uFEFForiginal text\r\n'))
      await put(user,'readme.md',Buffer.from('# Real Markdown\n\n![local image](00-image.png)\n\n<script>globalThis.__workspaceExecuted=true</script>\n'))
      await put(user,'sample.pdf',minimumPDF())
      await put(user,'unsafe.html',Buffer.from('<script>globalThis.__workspaceExecuted=true</script><h1>unsafe</h1>'))
      await put(user,'unsafe.svg',Buffer.from('<svg xmlns="http://www.w3.org/2000/svg" onload="globalThis.__workspaceExecuted=true"/>'))
    }
    }
    report.checks.push(options.legacyOnly?'real Hub/Relay with standard DAV fixture and explicit management substitute; all business objects use Admin API':'real Hub/Relay and approved SPA; service/users/spaces/files created through Admin API')
    if(!options.legacyOnly)writeFileSync(join(out,'browser-input.private.json'),JSON.stringify({baseURL,users,mediaDirectory:media,output:out,adminBuild:build.buildHash}),{flag:'wx',mode:0o600})
    if(options.legacyOnly){
      assert.ok(sourcesUnchanged(),'Validation source changed after its pre-execution snapshot')
      const {runLegacyAPI}=await import('./workspace-thumbnail-legacy.mjs')
      report.api=await runLegacyAPI({root,env,users,api,raw,put,workspace,provider:legacyProvider})
      assert.ok(sourcesUnchanged(),'Validation source changed during legacy API execution')
    }else if(options.apiOnly){
      assert.ok(sourcesUnchanged(),'Validation source changed after its pre-execution snapshot')
      const {runThumbnailAPI}=await import('./workspace-thumbnail-api.mjs')
      report.api=await runThumbnailAPI({out,env,options,users,api,raw,command,put,workspace,fixtureReady,managementToken:token,core:{backendSource:report.backendSource,binaries:report.binaries,adminBuildHash:build.buildHash}})
      assert.ok(sourcesUnchanged(),'Validation source changed during API execution')
      report.checks.push('real Client/Admin API lane passed; see api checks and client-contract-cases/manifest.json')
    }else if(!options.setupOnly) {
      assert.ok(sourcesUnchanged(),'Validation source changed after its pre-execution snapshot')
      browser=spawn(process.execPath,[join(root,'console/node_modules/@playwright/test/cli.js'),'test','workspace-thumbnails.spec.ts','--output='+join(out,'playwright-results')],{cwd:join(root,'console'),windowsHide:true,stdio:['ignore','pipe','pipe'],env:{...process.env,MEASIX_E2E_BASE_URL:baseURL,MEASIX_E2E_ADMIN_PASSWORD:env.adminPassword,MEASIX_THUMBNAIL_BROWSER_INPUT:join(out,'browser-input.private.json'),PLAYWRIGHT_JSON_OUTPUT_FILE:join(out,'playwright.json'),PLAYWRIGHT_HTML_REPORT:join(out,'playwright-report')}})
      const log=logStream('playwright');browser.stdout.pipe(log);browser.stderr.pipe(log)
      diskTimer=setInterval(()=>{try{checkDisk()}catch(error){report.diskFailure=error.message;browser.kill('SIGTERM')}},1000)
      const [code]=await once(browser,'exit');assert.equal(code,0,'Browser acceptance failed; inspect playwright.log/report')
      const result=JSON.parse(readFileSync(join(out,'playwright.json'),'utf8'))
      assert.equal(result.stats.expected,2);assert.equal(result.stats.skipped,0);assert.equal(result.stats.unexpected,0);assert.equal(result.stats.flaky,0)
      report.browser={stats:result.stats,viewports:[1280,390].map(width=>({width,...JSON.parse(readFileSync(join(out,String(width),'browser-network.json'),'utf8'))}))}
      for(const viewport of report.browser.viewports){assert.equal(viewport.heldResponses.length,5);assert.ok(viewport.heldResponses.every(proof=>proof.status===200&&proof.byteLength>0&&proof.sourceETag&&proof.spaceId&&proof.bodySHA256))}
      assert.ok(!report.diskFailure&&!interrupted,'Acceptance interrupted')
      assert.ok(sourcesUnchanged(),'Validation source changed during browser execution')
      report.checks.push('production Admin browser acceptance passed; see playwright.json and browser-network.json')
    }
    assert.ok(!interrupted,'Acceptance interrupted')
    report.passed=true
  } catch(error) {
    report.failure=error.message;throw error
  } finally {
    if(diskTimer)clearInterval(diskTimer)
    const cleanupErrors=[]
    if(baseURL&&session) for(const user of users) {try {const view=await api(workspace(user));if(view.agentSpaceId)await command(user,'DELETE')}catch(error){cleanupErrors.push('Core workspace cleanup: '+error.message)}}
    try{await stopProcess(browser);await Promise.all(Object.values(processes).map(child=>stopProcess(child)))}catch(error){cleanupErrors.push(error.message)}
    for(const server of servers)try{await new Promise(resolveP=>server.close(()=>resolveP()))}catch(error){cleanupErrors.push(error.message)}
    if(legacyDirectory){assert.ok(resolve(legacyDirectory).startsWith(resolve(privateTemp)+sep));cleanupEnvironment([],[],legacyDirectory,false);if(existsSync(legacyDirectory))cleanupErrors.push('Legacy private fixture state survived cleanup')}
    if(env) {
      assert.ok(resolve(env.envRoot).startsWith(resolve(privateTemp)+sep),'Refuse recursive cleanup outside owned D: temp')
      cleanupEnvironment([],[],env.envRoot,false)
      if(existsSync(env.envRoot))cleanupErrors.push('Harness temporary environment survived cleanup')
    }
    if(fixture&&fixture.exitCode===null&&fixture.signalCode===null) {
      const ended=exitWithin(fixture,100000)
      fixture.stdin.end('{"action":"stop"}\n')
      await ended
      if(fixture.exitCode===null&&fixture.signalCode===null)cleanupErrors.push('Agent Space fixture did not terminate')
    }
    if(fixture&&fixture.exitCode!==0)cleanupErrors.push('Agent Space fixture exit was not clean')
    const fixtureReport=join(out,'agent-space/fixture.json')
    if(existsSync(fixtureReport)&&!JSON.parse(readFileSync(fixtureReport,'utf8')).cleanupPassed)cleanupErrors.push('Agent Space fixture resource cleanup failed')
    const after=inventory();report.dockerAfter=after
    for(const kind of['containers','volumes'])for(const item of baseline[kind])if(!after[kind].includes(item))cleanupErrors.push('Existing Docker resource lost: '+kind)
    if(fixtureReady?.owner&&docker('ps','-aq','--filter','label=io.runcove.preview-qualification='+fixtureReady.owner))cleanupErrors.push('Owned Docker container survived')
    if(fixtureReady?.owner&&docker('volume','ls','-q','--filter','label=io.runcove.preview-qualification='+fixtureReady.owner))cleanupErrors.push('Owned Docker volume survived')
    report.cleanupErrors=cleanupErrors;report.cleanupPassed=cleanupErrors.length===0;report.diskFreeEnd=checkDisk();report.finishedAt=new Date().toISOString()
    report.interrupted=interrupted;if(interrupted)report.passed=false
    report.validationSourcesUnchanged=sourcesUnchanged()
    writeFileSync(join(out,'evidence.json'),JSON.stringify(report,null,2)+'\n',{flag:'wx'})
    for(const stream of logs)stream.end()
    process.off('SIGINT',onSignal);process.off('SIGTERM',onSignal)
    console.log('Evidence: '+out)
    if(!report.cleanupPassed)throw Error('Acceptance cleanup failed; inspect evidence.json')
    assert.ok(!interrupted,'Acceptance interrupted')
  }
}

if(process.argv[1]&&pathToFileURL(resolve(process.argv[1])).href===import.meta.url) {
  const options={}
  const names={'--output':'output','--image':'image','--expected-source':'expectedSource','--expected-helper':'expectedHelper','--expected-workspace':'expectedWorkspace','--admin-build':'adminBuild','--agent-space-source':'agentSpaceSource'}
  for(let index=2;index<process.argv.length;index++){const name=process.argv[index];if(name==='--setup-only'){options.setupOnly=true;continue}if(name==='--api-only'){options.apiOnly=true;continue}if(name==='--legacy-only'){options.legacyOnly=true;continue}assert.ok(names[name],'Unknown argument '+name);options[names[name]]=process.argv[++index]}
  await run(options)
}
