import test from 'node:test'
import assert from 'node:assert/strict'
import { maximumConcurrentTransfers, thumbnailReadInterval, holdPreview } from './workspace-thumbnail-browser.mjs'
import { readFileSync } from 'node:fs'
import { createHash } from 'node:crypto'

function heldFixture() {
  let handler
  const page={async route(_pattern,value){handler=value},async unroute(){}}
  const url='http://127.0.0.1/api/admin/v1/users/test/workspace/thumbnail?'+new URLSearchParams({path:'image.png',size:'128',agentSpaceId:'spc_test',expectedEtag:'"source"'})
  const body=readFileSync(new URL('../api/fixtures/workspace/thumbnail.webp',import.meta.url))
  const response={status:()=>200,headers:()=>({'content-type':'image/webp','agent-space-id':'spc_test','preview-source-etag':'"source"',etag:'"'+createHash('sha256').update(body).digest('hex')+'"'}),body:async()=>body}
  const route={request:()=>({method:()=>'GET',url:()=>url}),fetch:async()=>response,fulfill:async()=>{},continue:async()=>{}}
  return {page,route,handle:()=>handler(route),body}
}

test('native transfer intervals count an ending request before a simultaneous new start', () => {
  assert.equal(maximumConcurrentTransfers([{start:0,end:5},{start:1,end:10},{start:5,end:8}]),2)
})
test('native transfer concurrency is independent of callback completion order and detects three real overlaps', () => {
  assert.equal(maximumConcurrentTransfers([{start:5,end:8},{start:1,end:10},{start:0,end:5}]),2)
  assert.equal(maximumConcurrentTransfers([{start:0,end:8},{start:1,end:10},{start:5,end:9}]),3)
  assert.throws(()=>maximumConcurrentTransfers([{start:4,end:2}]))
})
test('cancelled network teardown stays in raw evidence but ends active reading at the observed abort',()=>{
  const native=[{start:0,end:82.2,abortAt:81.7},{start:81.8,end:186.4},{start:81.9,end:251.4}]
  assert.equal(maximumConcurrentTransfers(native),3)
  const reads=native.map(thumbnailReadInterval)
  assert.equal(maximumConcurrentTransfers(reads),2)
  assert.equal(reads[0].networkEnd,82.2)
  assert.equal(reads[0].end,81.7)
  assert.equal(thumbnailReadInterval({start:10,end:20,abortAt:9}).end,10)
})
test('held thumbnail cannot become ready when fetch fails before any real 200',async()=>{
  const fixture=heldFixture(),proofs=[]
  fixture.route.fetch=async()=>{throw Error('synthetic pre-ready failure')}
  const held=await holdPreview(fixture.page,'image.png',128,proofs)
  const rejected=assert.rejects(held.ready,/before.*200/)
  await fixture.handle();await rejected
  assert.equal(proofs.length,0)
  await held.release()
})
test('held real 200 records body/source identity and tolerates only post-ready cancellation',async()=>{
  const fixture=heldFixture(),proofs=[]
  fixture.route.fulfill=async()=>{throw Error('cancelled after ready')}
  const held=await holdPreview(fixture.page,'image.png',128,proofs)
  const handling=fixture.handle();await held.ready
  assert.equal(proofs[0].status,200);assert.equal(proofs[0].byteLength,fixture.body.length)
  assert.equal(proofs[0].sourceETag,'"source"');assert.equal(proofs[0].spaceId,'spc_test')
  await held.release();await handling
  assert.equal(proofs[0].lateFulfillRejected,true)
})
test('busy held response stays unready until a subsequent verified 200',async()=>{
  const fixture=heldFixture(),proofs=[]
  const good=fixture.route.fetch;let count=0,ready=false
  fixture.route.fetch=async()=>++count===1?{status:()=>429}:good()
  const held=await holdPreview(fixture.page,'image.png',128,proofs)
  held.ready.then(()=>{ready=true})
  await fixture.handle();await Promise.resolve();assert.equal(ready,false);assert.equal(proofs.length,0)
  const handling=fixture.handle();await held.ready
  assert.deepEqual(proofs[0].attemptStatuses,[429,200])
  await held.release();await handling
})
test('a verified held response does not hold a later recovery request for the same file',async()=>{
  const fixture=heldFixture(),proofs=[]
  const realFetch=fixture.route.fetch
  let fetches=0,continued=0
  fixture.route.fetch=async()=>{if(++fetches>1)throw Error('recovery must pass through');return realFetch()}
  fixture.route.continue=async()=>{continued++}
  const held=await holdPreview(fixture.page,'image.png',128,proofs)
  const first=fixture.handle();await held.ready
  try{
    await fixture.handle()
    assert.equal(fetches,1)
    assert.equal(continued,1)
    assert.equal(proofs.length,1)
  }finally{await held.release();await first}
})
