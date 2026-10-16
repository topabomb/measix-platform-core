/** Runs only against the dedicated environment created by webdav-acceptance.mjs. */
import assert from 'node:assert/strict'
import {readFileSync,writeFileSync} from 'node:fs'
import {resolve,dirname,join} from 'node:path'
import {randomUUID} from 'node:crypto'
import {setTimeout as pause} from 'node:timers/promises'
const file=resolve(process.argv[2]),env=JSON.parse(readFileSync(file,'utf8'))
const checks=[],evidence={startedAt:new Date().toISOString(),checks},out=join(dirname(file),'lifecycle-'+Date.now()+'.json')
const save=()=>writeFileSync(out,JSON.stringify(evidence,null,2))
const proof=name=>{checks.push(name);save();console.log('PASS '+name)}
async function admin(path,method='GET',body){const r=await fetch(env.publicOrigin+path,{method,headers:{Cookie:env.cookie,Origin:env.publicOrigin,'X-CSRF-Token':env.session.csrfToken,'Content-Type':'application/json','Idempotency-Key':'idem_'+randomUUID()},body:body===undefined?undefined:JSON.stringify(body)});assert.ok(r.ok,`${method} ${path}: ${r.status}`);return r.status===204?undefined:r.json()}
async function refresh(){const r=await fetch(env.publicOrigin+'/api/client/v1/sessions/refresh',{method:'POST',headers:{'Content-Type':'application/json','Idempotency-Key':'idem_'+randomUUID()},body:JSON.stringify({refreshToken:env.user.refreshToken})});assert.equal(r.status,200);Object.assign(env.user,await r.json());writeFileSync(file,JSON.stringify(env,null,2),{mode:0o600})}
const wp=`/api/admin/v1/users/${env.user.userId}/workspace`
async function complete(op,kind='workspace-operations'){for(let i=0;i<160;i++){const r=await admin('/api/admin/v1/'+kind+'/'+(op.operationId??op.activationId));if(r.state==='COMPLETED')return;assert.ok(!['UNKNOWN','NEEDS_ATTENTION','FAILED'].includes(r.state),JSON.stringify(r));await pause(250)}throw Error('operation timeout')}
async function command(action,extra={}){const v=await admin(wp);await complete(await admin(wp,'POST',{action,expectedRevision:v.bindingRevision,...extra}));return admin(wp)}
async function toggle(enabled){const d=await admin('/api/admin/v1/draft');assert.equal(d.content.webdav.length,1);d.content.webdav[0].enabled=enabled;const saved=await admin('/api/admin/v1/draft','PUT',{expectedDraftRevision:d.draftRevision,content:d.content});const revision=saved.draftRevision;const v=await admin('/api/admin/v1/draft:validate','POST',{expectedDraftRevision:revision});assert.equal(v.valid,true);await complete(await admin('/api/admin/v1/draft:publish','POST',{expectedDraftRevision:revision,acknowledgedWarningCodes:v.warnings.map(w=>w.code)}),'activations')}
async function dav(method='PROPFIND'){await refresh();const r=await fetch(env.publicOrigin+env.davRoot,{method,headers:{Authorization:'Bearer '+env.user.accessToken,Depth:'0'}});await r.arrayBuffer();return r.status}
async function files(){const r=await fetch(env.publicOrigin+env.contentPath,{headers:{Cookie:env.cookie}});assert.equal(r.status,200);assert.match(await r.text(),/Preserve this workspace/)}
try{
 const initial=await admin(wp),resource=initial.fileExchangeWebdavResourceId;assert.ok(resource)
 await toggle(false);let v=await admin(wp);assert.equal(v.fileExchangeWebdavResourceId,undefined);assert.equal(v.webdav.available,false);assert.ok([403,404,503].includes(await dav()));await files()
 await toggle(true);v=await admin(wp);assert.equal(v.fileExchangeWebdavResourceId,resource);assert.equal(v.webdav.available,true);assert.equal(await dav(),207)
 proof('published DAV off removes unique designation and denies DAV while Files works; on restores the same resource')
 await command('SET_DAV');v=await admin(wp);assert.equal(v.webdav.accessMode,initial.webdav.accessMode);assert.equal(await dav(),207);await files()
 proof('credential reset through Activation preserves chosen permission and restores DAV/Files')
 await command('REVOKE_DAV');v=await admin(wp);assert.equal(v.filesAvailable,false);assert.equal(v.webdav.available,false);assert.ok([403,404,503].includes(await dav()));await pause(1500);assert.equal((await admin(wp)).filesAvailable,false)
 await command('SET_DAV');assert.equal(await dav(),207);await files()
 proof('explicit revoke remains revoked until explicit reissue; original file and DAV permission survive')
 await command('DISCONNECT');v=await admin(wp);assert.equal(v.webdav.available,false);await command('RESTORE');v=await admin(wp);assert.equal(v.filesAvailable,true);assert.equal(v.webdav.available,true);assert.equal(v.webdav.accessMode,initial.webdav.accessMode);assert.equal(await dav(),207);await files()
 proof('authorized disconnect/restore automatically prepares file access without manual key renewal')
 evidence.status='PASSED';evidence.completedAt=new Date().toISOString();save()
}catch(e){evidence.status='FAILED';evidence.failure=e.message;save();throw e}
