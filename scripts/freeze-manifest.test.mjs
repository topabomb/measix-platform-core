import { test } from 'node:test'
import assert from 'node:assert/strict'
import {
  validatePins,
  qualificationVerified,
  validateReplay,
  manifestSelfEvidence,
  compileScenarioResults,
  scenarioResultErrors,
  artifactNames,
  REPLAY_STAGES,
} from './freeze-manifest.mjs'

const SHA = 'sha256:' + 'a'.repeat(64)
function selfEvidenceManifest() {
  return {
    manifest: 'measix-s0-client-contract-freeze',
    platformCoreCommit: 'a'.repeat(40),
    architectureCommit: 'b'.repeat(40),
    workingTreeDirty: false,
    architectureRepoDirty: false,
    snapshotSchemaVersion: 4,
    adminBuildHash: SHA,
    clientControlOpenApiHash: SHA,
    adminOpenApiHash: SHA,
    relayControlOpenApiHash: SHA,
    usageIngestOpenApiHash: SHA,
    canonicalFixtureHash: SHA,
    deterministicAdapterVersion: '1.0.0',
    artifactPins: Object.fromEntries(artifactNames.map(name => [name, { artifactSha256: SHA, metaSha256: SHA }])),
  }
}

test('CAP-C7-001 is proven by manifest identity and evidence pins, not by the file existing', () => {
  const good = selfEvidenceManifest()
  assert.deepEqual(manifestSelfEvidence(good), [])
  assert.equal(compileScenarioResults(good).find(row => row.id === 'CAP-C7-001').result, 'PASS')
  for (const bad of [
    { ...good, workingTreeDirty: true },
    { ...good, architectureRepoDirty: true },
    { ...good, platformCoreCommit: 'not-a-sha' },
    { ...good, adminBuildHash: 'not-built' },
    { ...good, clientControlOpenApiHash: 'stale' },
    { ...good, artifactPins: {} },
    { ...good, artifactPins: { 'backend-test.json': { artifactSha256: SHA, metaSha256: SHA } } },
    { ...good, snapshotSchemaVersion: 3 },
    { ...good, manifest: 'something-else' },
  ]) {
    assert.ok(manifestSelfEvidence(bad).length > 0, 'expected evidence failure')
    assert.equal(compileScenarioResults(bad).find(row => row.id === 'CAP-C7-001').result, 'FAIL')
  }
  assert.equal(compileScenarioResults(undefined).find(row => row.id === 'CAP-C7-001').result, 'FAIL')
})

test('required CAP-C7 rows are not exempt from artifact evidence', () => {
  const rows = [
    { id: 'CAP-C7-001', required: true, result: 'FAIL' },
    { id: 'CAP-C7-002', required: true, result: 'NOT_EXECUTED' },
    { id: 'CAP-C0-001', required: true, result: 'PASS' },
    { id: 'CAP-OPT', required: false, result: 'NOT_EXECUTED' },
  ]
  assert.deepEqual(scenarioResultErrors(rows, {}), ['Artifact scenario not PASS: CAP-C7-001', 'Artifact scenario not PASS: CAP-C7-002'])
  // Only a pending clean-source replay may leave CAP-C7-002 unresolved.
  assert.deepEqual(scenarioResultErrors(rows, { allowPendingReplay: true }), ['Artifact scenario not PASS: CAP-C7-001'])
  assert.deepEqual(scenarioResultErrors([{ id: 'CAP-C7-002', required: true, result: 'PASS' }], {}), [])
})

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

test('replay PASS cannot substitute for rebuilt pins and complete successful commands', () => {
  const facts = { platformCoreCommit: 'core', architectureCommit: 'arch', adminBuildHash: 'build' }
  const stages = REPLAY_STAGES.map(id => ({ id, exitCode: 0, outputHash: 'sha256:' + 'a'.repeat(64) }))
  const valid = { status: 'PASS', replayKind: 'CLEAN_SOURCE_AND_RUNTIME', ...facts, rebuiltFacts: facts, candidateManifestHash: 'sha256:' + 'b'.repeat(64), stages }
  assert.deepEqual(validateReplay(valid, facts, valid.candidateManifestHash), [])
  for (const bad of [
    { ...valid, stages: stages.slice(1) },
    { ...valid, stages: [...stages, stages[0]] },
    { ...valid, stages: stages.map(s => s.id === 'browser' ? { ...s, exitCode: 1 } : s) },
    { ...valid, rebuiltFacts: { ...facts, adminBuildHash: 'other' } },
    { ...valid, candidateManifestHash: 'sha256:' + 'c'.repeat(64) },
  ]) assert.ok(validateReplay(bad, facts, valid.candidateManifestHash).length > 0)
})
