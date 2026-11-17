import test from 'node:test'
import assert from 'node:assert/strict'
import { mkdirSync,mkdtempSync,readFileSync,rmSync } from 'node:fs'
import { resolve,join } from 'node:path'
import { createHash } from 'node:crypto'
import { captureClientContractCases } from './workspace-thumbnail-api.mjs'

const parent=resolve('.artifacts/media-preview')
mkdirSync(parent,{recursive:true})
test('Client contract capture preserves original bytes and only whitelisted metadata',async()=>{
  const root=mkdtempSync(join(parent,'client-capture-test-'))
  const projection=Buffer.from(' { "agentSpaceId":"spc_test", "schemaVersion":1 }\n')
  const list=Buffer.from('{"entries":[{"path":"image.png","kind":"FILE"}]}\n')
  const requested=[]
  try{
    const manifest=await captureClientContractCases({directory:join(root,'cases'),agentSpaceId:'spc_test',core:{backendSource:'fixed'},provider:{image:'fixed'},fetch:async path=>{
      requested.push(path);return new Response(path.includes('/files?')?list:projection,{status:200,headers:{'Content-Type':'application/json','Set-Cookie':'secret-never-capture'}})
    }})
    assert.equal(requested.length,3)
    assert.equal(requested[0],'/api/client/v1/workspace')
    assert.equal(new URL(requested[2],'http://fixture').searchParams.get('path'),'nested')
    assert.deepEqual(readFileSync(join(root,'cases/workspace.json')),projection)
    assert.equal(manifest.cases[0].responseSHA256,'sha256:'+createHash('sha256').update(projection).digest('hex'))
    assert.equal(JSON.stringify(manifest).includes('secret'),false)
    await assert.rejects(()=>captureClientContractCases({directory:join(root,'cases')}),/exist/i)
  }finally{assert.ok(root.startsWith(parent+'\\'));rmSync(root,{recursive:true})}
})
test('non-200 Client response cannot become a captured compatibility success',async()=>{
  const root=mkdtempSync(join(parent,'client-capture-test-'))
  try{
    await assert.rejects(()=>captureClientContractCases({directory:join(root,'cases'),agentSpaceId:'spc_test',fetch:async()=>new Response('{"code":"login"}',{status:401})}),/200/)
    assert.throws(()=>readFileSync(join(root,'cases/manifest.json')),/ENOENT/)
  }finally{assert.ok(root.startsWith(parent+'\\'));rmSync(root,{recursive:true})}
})
