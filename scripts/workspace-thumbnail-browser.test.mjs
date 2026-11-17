import test from 'node:test'
import assert from 'node:assert/strict'
import { maximumConcurrentTransfers, thumbnailReadInterval, holdPreview, writeAcceptanceEvidence, validateInputs, validatePerformanceViewport } from './workspace-thumbnail-browser.mjs'
import { readFileSync, mkdtempSync, rmSync } from 'node:fs'
import { createHash } from 'node:crypto'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

const performanceInputs={image:'sha256:'+'1'.repeat(64),adminBuild:'sha256:'+'2'.repeat(64),expectedSource:'3'.repeat(64),expectedHelper:'4'.repeat(64),agentSpaceSource:'/source',output:'unused'}
test('performance lane cannot be combined with another acceptance lane',()=>{
  for(const flag of ['apiOnly','legacyOnly','setupOnly'])assert.throws(()=>validateInputs({...performanceInputs,performanceOnly:true,[flag]:true}),/Choose exactly one/)
})
test('performance lane requires an independently pinned Workspace image before startup',()=>{
  assert.throws(()=>validateInputs({...performanceInputs,performanceOnly:true}),/Independent Workspace identity required/)
})

test('performance evidence requires positive native overlap rather than callback overlap alone',()=>{
  const adminBuild=performanceInputs.adminBuild
  const valid={passed:true,servedAdminBuild:adminBuild,summary:{capabilityQueries:1,phases:Array.from({length:8},()=>({visible:1,coverage:1}))},io:[{status:206,overlappingThumbnailRequests:1,overlapMs:2.5}]}
  validatePerformanceViewport(valid,adminBuild)
  for(const overlapMs of [0,-1,NaN,undefined])assert.throws(()=>validatePerformanceViewport({...valid,io:[{...valid.io[0],overlapMs}]},adminBuild),/positive native/)
})

test('low disk cleanup still persists the original failed acceptance evidence',t=>{
  const out=mkdtempSync(join(tmpdir(),'thumbnail-low-disk-'))
  t.after(()=>rmSync(out,{recursive:true,force:true}))
  const report={passed:false,failure:'Browser stopped by disk guard',diskFailure:'low disk',cleanupPassed:true}
  writeAcceptanceEvidence(out,report,()=>({bavail:1n,bsize:1024n}))
  const saved=JSON.parse(readFileSync(join(out,'evidence.json'),'utf8'))
  assert.equal(saved.failure,report.failure);assert.equal(saved.diskFailure,'low disk')
  assert.equal(saved.passed,false);assert.equal(saved.cleanupPassed,true);assert.equal(saved.diskFreeEnd,1024)
})

test('a final low disk reading cannot mark an otherwise successful acceptance passed',t=>{
  const out=mkdtempSync(join(tmpdir(),'thumbnail-final-disk-'))
  t.after(()=>rmSync(out,{recursive:true,force:true}))
  writeAcceptanceEvidence(out,{passed:true,cleanupPassed:true},()=>({bavail:1n,bsize:1024n}))
  const saved=JSON.parse(readFileSync(join(out,'evidence.json'),'utf8'))
  assert.equal(saved.passed,false);assert.match(saved.diskFailure,/5 GiB/)
})

test('a failed final disk measurement is recorded without losing acceptance evidence',t=>{
  const out=mkdtempSync(join(tmpdir(),'thumbnail-disk-read-'))
  t.after(()=>rmSync(out,{recursive:true,force:true}))
  writeAcceptanceEvidence(out,{passed:false,failure:'original failure',cleanupPassed:true},()=>{throw Object.assign(Error('disk read failed'),{code:'EIO'})})
  const saved=JSON.parse(readFileSync(join(out,'evidence.json'),'utf8'))
  assert.equal(saved.failure,'original failure');assert.equal(saved.passed,false)
  assert.equal(saved.diskFreeEnd,null);assert.equal(saved.diskReadFailure,'EIO')
})

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
