import assert from 'node:assert/strict'
import {readFileSync,writeFileSync} from 'node:fs'
import {resolve,dirname,join} from 'node:path'
import {randomUUID} from 'node:crypto'
import {createClient,AuthType} from 'webdav'

const file=resolve(process.argv[2]),env=JSON.parse(readFileSync(file,'utf8'))
const response=await fetch(env.publicOrigin+'/api/client/v1/sessions/refresh',{method:'POST',headers:{'Content-Type':'application/json','Idempotency-Key':'idem_'+randomUUID()},body:JSON.stringify({refreshToken:env.user.refreshToken})})
assert.equal(response.status,200);Object.assign(env.user,await response.json());writeFileSync(file,JSON.stringify(env,null,2),{mode:0o600})
const client=createClient(env.publicOrigin+env.davRoot,{authType:AuthType.Token,token:{access_token:env.user.accessToken,token_type:'Bearer'}})
const folder='/sdk-'+randomUUID(),source=folder+'/中文 %.txt',copied=folder+'/copy.txt',moved=folder+'/moved.txt'
assert.ok(Array.isArray(await client.getDirectoryContents('/')))
await client.createDirectory(folder)
assert.equal(await client.putFileContents(source,'SDK byte round trip\n',{overwrite:false}),true)
assert.equal(await client.getFileContents(source,{format:'text'}),'SDK byte round trip\n')
await client.copyFile(source,copied);await client.moveFile(copied,moved)
assert.equal((await client.stat(moved)).size,20)
assert.ok((await client.getDirectoryContents(folder)).some(item=>item.basename==='中文 %.txt'))
await client.deleteFile(folder)
assert.equal(await client.exists(folder),false)
writeFileSync(join(dirname(file),'standard-client-'+Date.now()+'.json'),JSON.stringify({status:'PASSED',client:'webdav@5.11.0',at:new Date().toISOString(),operations:['PROPFIND','MKCOL','PUT','GET','COPY','MOVE','HEAD/stat','DELETE'],auth:'platform Bearer',unicodePercentRoundTrip:true},null,2))
console.log('PASS webdav@5.11.0 standard client through platform DAV: listing, create, Unicode/percent PUT/GET, COPY/MOVE/stat, DELETE')
