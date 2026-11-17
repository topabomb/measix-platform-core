import test from 'node:test'
import assert from 'node:assert/strict'
import { createServer } from 'node:http'
import { once } from 'node:events'
import { createLegacyPreviewProxy } from './workspace-thumbnail-legacy.mjs'

test('legacy preview override never replaces DAV root authentication and records no credentials',async()=>{
  const observed=[],mode={previewStatus:405}
  const backend=createServer((request,response)=>{
    if(request.headers.authorization!=='Bearer private-test-key'){response.writeHead(401);response.end();return}
    response.writeHead(207);response.end('real backend marker')
  })
  backend.listen(0,'127.0.0.1');await once(backend,'listening')
  const proxy=createLegacyPreviewProxy({target:'http://127.0.0.1:'+backend.address().port,mode,observed})
  proxy.listen(0,'127.0.0.1');await once(proxy,'listening')
  try{
    const origin='http://127.0.0.1:'+proxy.address().port
    assert.equal((await fetch(origin+'/u/alice/previews',{method:'OPTIONS'})).status,405)
    assert.equal((await fetch(origin+'/u/alice/dav/',{method:'PROPFIND'})).status,401)
    const response=await fetch(origin+'/u/alice/dav/',{method:'PROPFIND',headers:{Authorization:'Bearer private-test-key'}})
    assert.equal(response.status,207);assert.equal(await response.text(),'real backend marker')
    assert.equal(JSON.stringify(observed).includes('private-test-key'),false)
  }finally{await Promise.all([new Promise(resolve=>proxy.close(resolve)),new Promise(resolve=>backend.close(resolve))])}
})
test('legacy validation proxy refuses a non-loopback target before listening',()=>{
  assert.throws(()=>createLegacyPreviewProxy({target:'http://outside.invalid',mode:{},observed:[]}),/loopback/)
})
