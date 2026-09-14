import { test } from 'node:test'
import assert from 'node:assert/strict'
import { validatePins, qualificationVerified } from './freeze-manifest.mjs'

test('qualification needs all four profiles, not an unexecuted optional profile', () => {
  const profiles = Object.fromEntries(['model','tts','asr','mcp'].map(p => [p, {status:'VERIFIED',adapterName:'test',adapterVersion:'1',upstreamId:'ups_test',configRevision:1,usageRecordsCount:1,transport:['HTTP_REQUEST_RESPONSE']}]))
  assert.equal(qualificationVerified({status:'VERIFIED',profiles}), true)
  const unknownIdentity = structuredClone(profiles)
  unknownIdentity.mcp.adapterVersion = 'unknown'
  assert.equal(qualificationVerified({status:'VERIFIED',profiles:unknownIdentity}), false)
  const noUsage = structuredClone(profiles)
  noUsage.asr.usageRecordsCount = 0
  assert.equal(qualificationVerified({status:'VERIFIED',profiles:noUsage}), false)
  profiles.mcp.status = 'NOT_EXECUTED'
  assert.equal(qualificationVerified({status:'VERIFIED',profiles}), false)
})

test('manifest pins reject missing scenarios, dirty source and contract drift', () => {
  const facts = { platformCoreCommit:'core', architectureCommit:'arch', snapshotSchemaVersion:1, adminBuildHash:'build', clientControlOpenApiHash:'client', adminOpenApiHash:'admin', relayControlOpenApiHash:'relay', usageIngestOpenApiHash:'usage', canonicalFixtureHash:'fixtures', deterministicAdapterVersion:'adapter' }
  const defs = [{id:'CAP-X',required:true}]
  const good = {...facts, workingTreeDirty:false, architectureRepoDirty:false, scenarioResults:[{id:'CAP-X',required:true,result:'PASS'}]}
  assert.deepEqual(validatePins(good, facts, defs), [])
  for (const bad of [{...good,scenarioResults:[]},{...good,workingTreeDirty:true},{...good,clientControlOpenApiHash:'stale'},{...good,snapshotSchemaVersion:2}]) {
    assert.ok(validatePins(bad,facts,defs).length > 0)
  }
})
