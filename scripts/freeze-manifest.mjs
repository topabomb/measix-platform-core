#!/usr/bin/env node
import { createHash } from 'node:crypto'
import { existsSync, readFileSync, writeFileSync, mkdirSync } from 'node:fs'
import { join, relative, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import {
  resolveRoot,
  collectFiles,
  gitCommit,
  gitDirty,
  adminBuildHash,
  deterministicAdapterVersion,
  sha256File,
} from './lib/harness.mjs'

const ROOT = resolveRoot(import.meta.dirname)
const FIXTURES_DIR = join(ROOT, 'api', 'fixtures')
const CLIENT_OPENAPI = join(ROOT, 'api', 'client', 'client-control.openapi.yaml')
const ADMIN_OPENAPI = join(ROOT, 'api', 'admin', 'admin.openapi.yaml')
const ARTIFACTS_DIR = join(ROOT, '.artifacts')
const ARCH_REPO = resolve(ROOT, '..', 'measix-architecture')
const SCENARIO_DEFS = JSON.parse(readFileSync(join(ROOT, 'scripts', 'scenario-definitions.json'), 'utf-8'))


// Local-only: fixtures hash (not shared with harness.mjs)
function fixturesHash() {
  const hash = createHash('sha256')
  for (const file of collectFiles(FIXTURES_DIR).sort()) {
    hash.update(relative(FIXTURES_DIR, file).replace(/\\/g, '/'))
    hash.update('\0')
    hash.update(readFileSync(file).toString('utf-8').replace(/\r\n/g, '\n'))
    hash.update('\0')
  }
  return 'sha256:' + hash.digest('hex')
}

// --- Artifact loaders ---

function loadJsonArtifact(name) {
  const path = join(ARTIFACTS_DIR, name)
  if (!existsSync(path)) return null
  try {
    let content = readFileSync(path, 'utf-8')
    // Strip UTF-8 BOM if present (PowerShell Out-File -Encoding utf8 adds BOM)
    if (content.charCodeAt(0) === 0xFEFF) content = content.slice(1)
    return JSON.parse(content)
  }
  catch (err) { return { _error: `Failed to parse ${name}: ${err.message}` } }
}

function loadMetaArtifact(name) {
  const metaPath = join(ARTIFACTS_DIR, name + '.meta.json')
  if (!existsSync(metaPath)) return null
  try { return JSON.parse(readFileSync(metaPath, 'utf-8')) }
  catch { return null }
}

function loadGoTestResults(artifactName) {
  const path = join(ARTIFACTS_DIR, artifactName)
  if (!existsSync(path)) return null
  const results = new Map()
  let content = readFileSync(path, 'utf-8')
  // Strip UTF-8 BOM if present (PowerShell Out-File -Encoding utf8 adds BOM)
  if (content.charCodeAt(0) === 0xFEFF) content = content.slice(1)
  for (const line of content.split('\n')) {
    if (!line.trim()) continue
    try {
      const e = JSON.parse(line)
      if (e.Action === 'pass' && e.Test) results.set(e.Test, 'PASS')
      else if (e.Action === 'fail' && e.Test) results.set(e.Test, 'FAIL')
    } catch {}
  }
  return results
}

function loadVitestResults(artifactName) {
  const a = loadJsonArtifact(artifactName)
  if (!a) return null
  const results = new Map()
  if (a.testResults) for (const tr of a.testResults) {
    const name = 'console/' + relative(join(ROOT, 'console'), tr.name).replace(/\\/g, '/')
    // For vitest, we need both file-level and per-test granularity.
    // Store individual test names too.
    const fileStatus = tr.status === 'passed' ? 'PASS' : 'FAIL'
    results.set(name, fileStatus)
    // Also store individual test cases with their full path
    if (tr.assertionResults) {
      for (const ar of tr.assertionResults) {
        // Vitest's fullName uses space as separator between ancestorTitles and title.
        // We need to use ' > ' separator to match scenario-definitions.json format.
        // ancestorTitles is an array like ["ResourcesPage"], title is the test name.
        // ar.fullName is "ResourcesPage test name" (space-separated).
        // We want "ResourcesPage > test name".
        const parts = ar.ancestorTitles || []
        const title = ar.title || ar.name || ar.fullName || 'unknown'
        const fullNameWithArrow = parts.length > 0
          ? name + ' > ' + [...parts, title].join(' > ')
          : name + ' > ' + title
        results.set(fullNameWithArrow, ar.status === 'passed' ? 'PASS' : 'FAIL')
      }
    }
  }
  return results
}

function loadPlaywrightResults(artifactName) {
  const a = loadJsonArtifact(artifactName)
  if (!a) return null
  const results = new Map()
  if (a.suites) for (const suite of a.suites) extractPlaywrightSpecs(suite, results)
  return results
}

function extractPlaywrightSpecs(suite, results) {
  if (suite.specs) for (const spec of suite.specs) {
    const title = spec.title || spec.name || 'unknown'
    let status = 'NOT_EXECUTED'
    if (spec.tests) for (const test of spec.tests) {
      if (test.results) {
        const passed = test.results.some(r => r.status === 'passed')
        const failed = test.results.some(r => r.status === 'failed')
        if (passed && !failed) status = 'PASS'
        else if (failed) status = 'FAIL'
      }
    }
    // Store by exact title (the Playwright spec title)
    results.set(title, status)
    // Also extract any CAP-XX-NNN stable ID from the title or annotations
    const capIdMatch = title.match(/CAP-[A-Z0-9-]+-\d+/g)
    if (capIdMatch) {
      for (const capId of capIdMatch) {
        results.set(capId, status)
      }
    }
    // Check for annotations (Playwright supports test annotations)
    if (spec.annotations) {
      for (const ann of spec.annotations) {
        if (ann.type && ann.type.startsWith('CAP-')) {
          results.set(ann.type, status)
        }
      }
    }
  }
  if (suite.suites) for (const s of suite.suites) extractPlaywrightSpecs(s, results)
}

// --- Scenario result compilation ---

function compileScenarioResults() {
  const artifacts = {
    'backend-test.json': loadGoTestResults('backend-test.json'),
    'system-test.json': loadGoTestResults('system-test.json'),
    'console-test.json': loadVitestResults('console-test.json'),
    'candidate-test.json': loadGoTestResults('candidate-test.json'),
    'e2e-playwright.json': loadPlaywrightResults('e2e-playwright.json'),
    'static-contract.json': loadJsonArtifact('static-contract.json'),
    'resource-baseline.json': loadJsonArtifact('resource-baseline.json'),
    'real-adapter-qualification.json': loadJsonArtifact('real-adapter-qualification.json'),
  }

  return SCENARIO_DEFS.map(s => {
    let result = 'NOT_EXECUTED'

    if (s.id === 'CAP-C0-009') {
      const a = artifacts['static-contract.json']
      // Per audit P0-4: static-contract.json now stores {status, outputHash}
      // for each check. Top-level PASS requires all three sub-checks to PASS.
      if (a && a.codegenDrift?.status === 'PASS' && a.gofmt?.status === 'PASS' && a.goVet?.status === 'PASS') result = 'PASS'
      else if (a && (a.codegenDrift?.status === 'FAIL' || a.gofmt?.status === 'FAIL' || a.goVet?.status === 'FAIL')) result = 'FAIL'
      else if (a && a._error) result = 'FAIL'
    } else if (s.id === 'BASELINE') {
      const a = artifacts['resource-baseline.json']
      if (a && a.status === 'GREEN') result = 'PASS'
      else if (a && a.status === 'NOT_GREEN') result = 'FAIL'
    } else if (s.id === 'ADAPTER-QUAL') {
      const a = artifacts['real-adapter-qualification.json']
      if (a && a.status === 'VERIFIED') result = 'PASS'
      else if (a && a.status && a.status !== 'NOT_EXECUTED') result = 'FAIL'
    } else if (s.id === 'CAP-C7-001') {
      result = 'PASS'
    } else if (s.id === 'CAP-C7-002') {
      // Two-phase freeze: candidate manifest writes NOT_EXECUTED.
      // After clean replay passes, the final manifest writes PASS.
      // Current runtime-only replay cannot finalize this scenario.
      result = 'NOT_EXECUTED'
    } else if (s.artifact && s.testNames.length > 0) {
      const artifact = artifacts[s.artifact]
      if (artifact instanceof Map) {
        // Use exact match for stable scenario IDs / test names
        const results = s.testNames.map(tn => artifact.get(tn))
        if (results.every(r => r === 'PASS')) result = 'PASS'
        else if (results.some(r => r === 'FAIL')) result = 'FAIL'
      }
    }

    return {
      id: s.id,
      name: s.name,
      artifact: s.artifact,
      testNames: s.testNames,
      required: s.required,
      result,
    }
  })
}

// --- Provenance validation ---


export function qualificationVerified(artifact) {
  return artifact?.status === 'VERIFIED' && ['model','tts','asr','mcp'].every(p => {
    const row = artifact.profiles?.[p]
    return row?.status === 'VERIFIED' && row.adapterName && row.adapterVersion && row.adapterVersion !== 'unknown'
      && row.upstreamId && Number.isInteger(row.configRevision) && row.configRevision > 0
      && row.usageRecordsCount > 0 && Array.isArray(row.transport) && row.transport.length > 0
  })
}

export function validatePins(manifest, facts, definitions = SCENARIO_DEFS, allowPendingReplay = false) {
  const errors = []
  for (const [field, expected] of Object.entries(facts)) {
    if (manifest[field] !== expected) errors.push(field + ' mismatch')
  }
  if (manifest.workingTreeDirty !== false || manifest.architectureRepoDirty !== false) errors.push('Manifest source was dirty or unknown')
  const rows = manifest.scenarioResults
  if (!Array.isArray(rows)) return [...errors, 'Missing scenarioResults']
  if (rows.length !== definitions.length || new Set(rows.map(s => s.id)).size !== rows.length) errors.push('Scenario set mismatch')
  for (const definition of definitions) {
    const row = rows.find(s => s.id === definition.id)
    if (!row || row.required !== definition.required) { errors.push('Missing/altered scenario ' + definition.id); continue }
    if (definition.required && row.result !== 'PASS' && !(allowPendingReplay && row.id === 'CAP-C7-002' && row.result === 'NOT_EXECUTED')) errors.push('Scenario not PASS: ' + definition.id)
  }
  return errors
}

const artifactNames = ['backend-test.json','system-test.json','console-test.json','candidate-test.json','e2e-playwright.json','static-contract.json','resource-baseline.json','real-adapter-qualification.json']
function byteHash(path) { return 'sha256:' + createHash('sha256').update(readFileSync(path)).digest('hex') }
function sourceFacts() {
  const source = readFileSync(join(ROOT,'backend/internal/hub/capability/snapshot.go'),'utf8')
  const version = source.match(/const CurrentSnapshotSchemaVersion = (\d+)/)?.[1]
  if (!version) throw new Error('Cannot determine live Snapshot compiler schema')
  return {
    platformCoreCommit: gitCommit(ROOT), architectureCommit: gitCommit(ARCH_REPO),
    snapshotSchemaVersion: Number(version), adminBuildHash: adminBuildHash(ROOT),
    clientControlOpenApiHash: sha256File(CLIENT_OPENAPI), adminOpenApiHash: sha256File(ADMIN_OPENAPI),
    relayControlOpenApiHash: sha256File(join(ROOT,'api/internal/relay-control.openapi.yaml')),
    usageIngestOpenApiHash: sha256File(join(ROOT,'api/internal/usage-ingest.openapi.yaml')),
    canonicalFixtureHash: fixturesHash(), deterministicAdapterVersion: deterministicAdapterVersion(ROOT),
  }
}

function evidenceChecks(facts) {
  const errors = [], pins = {}
  for (const name of artifactNames) {
    const path = join(ARTIFACTS_DIR,name)
    const meta = loadMetaArtifact(name)
    if (!existsSync(path) || !meta) { errors.push(name + ': missing artifact/meta'); continue }
    if (meta.platformCoreCommit !== facts.platformCoreCommit || meta.architectureCommit !== facts.architectureCommit) errors.push(name + ': source mismatch')
    if (meta.workingTreeDirty !== false || meta.architectureRepoDirty !== false) errors.push(name + ': dirty/unknown source')
    if (meta.exitCode !== 0) errors.push(name + ': command did not succeed')
    if (meta.artifactSha256 !== byteHash(path)) errors.push(name + ': artifact hash mismatch')
    pins[name] = { artifactSha256: byteHash(path), metaSha256: byteHash(path + '.meta.json') }
  }
  if (!qualificationVerified(loadJsonArtifact('real-adapter-qualification.json'))) errors.push('All four real adapter profiles must be VERIFIED')
  if (loadJsonArtifact('resource-baseline.json')?.status !== 'GREEN') errors.push('Resource baseline must be GREEN')
  return { errors, pins }
}

export function validateCandidate(manifest, { allowPendingReplay = false } = {}) {
  const facts = sourceFacts()
  const evidence = evidenceChecks(facts)
  const errors = [...validatePins(manifest,facts,SCENARIO_DEFS,allowPendingReplay), ...evidence.errors]
  if (gitDirty(ROOT) || gitDirty(ARCH_REPO)) errors.push('Current source checkout is dirty')
  if (facts.adminBuildHash === 'not-built') errors.push('Admin production build missing')
  if (facts.snapshotSchemaVersion !== 1) errors.push('This CAP runner is S0.1-only; live Snapshot v2 requires the ERX/S0.2 gate and cannot be relabeled v1')
  for (const name of artifactNames) {
    const pin = manifest.artifactPins?.[name], current = evidence.pins[name]
    if (!pin || !current || pin.artifactSha256 !== current.artifactSha256 || pin.metaSha256 !== current.metaSha256) errors.push(name + ': manifest evidence pin mismatch')
  }
  const actual = compileScenarioResults()
  for (const row of actual) {
    if (row.required && !row.id.startsWith('CAP-C7-') && row.result !== 'PASS') errors.push('Artifact scenario not PASS: ' + row.id)
  }
  if (!allowPendingReplay) {
    // Runtime-only replay is deliberately not accepted as clean-source C7 proof.
    if (manifest.replayKind !== 'CLEAN_SOURCE_AND_RUNTIME') errors.push('Missing independently rebuilt clean-source replay')
    const path = join(ARTIFACTS_DIR,'replay-artifact.json')
    if (!existsSync(path) || manifest.replayArtifactHash !== byteHash(path)) errors.push('Missing/changed replay artifact')
    const replay = loadJsonArtifact('replay-artifact.json')
    if (replay?.status !== 'PASS' || replay?.replayKind !== 'CLEAN_SOURCE_AND_RUNTIME' || replay?.platformCoreCommit !== facts.platformCoreCommit || replay?.architectureCommit !== facts.architectureCommit) errors.push('Replay provenance mismatch')
  }
  return errors
}

function main() {
  const startedAt = new Date().toISOString()
  if (process.argv.includes('--clean-replay')) throw new Error('--clean-replay is not validation. An independent clean-source rebuild/replay is required; runtime replay alone cannot finalize C7.')
  const index = process.argv.indexOf('--manifest')
  const path = index < 0 ? join(ARTIFACTS_DIR,'s0-freeze-candidate.json') : resolve(process.argv[index + 1] ?? '')
  if (path === join(ROOT,'docs','s0-freeze-manifest.json')) throw new Error('Historical tracked manifest is immutable; use an artifact candidate path')
  if (process.argv.includes('--validate')) {
    const manifest = JSON.parse(readFileSync(path,'utf8'))
    const errors = validateCandidate(manifest,{allowPendingReplay:process.argv.includes('--candidate')})
    if (errors.length) throw new Error(errors.join('\n'))
    console.log('Manifest ' + (process.argv.includes('--candidate') ? 'candidate' : 'final') + ' validation: PASS')
    return
  }
  const facts = sourceFacts(), evidence = evidenceChecks(facts)
  const manifest = {
    manifest:'measix-s0-client-contract-freeze', stage:'S0.1', ...facts,
    workingTreeDirty:gitDirty(ROOT), architectureRepoDirty:gitDirty(ARCH_REPO),
    realAdapterQualificationRef:'.artifacts/real-adapter-qualification.json', realAdapterQualificationStatus:loadJsonArtifact('real-adapter-qualification.json')?.status,
    resourceBaselineRef:'.artifacts/resource-baseline.json', resourceBaselineStatus:loadJsonArtifact('resource-baseline.json')?.status,
    artifactPins:evidence.pins, scenarioResults:compileScenarioResults(), startedAt, completedAt:new Date().toISOString(),
  }
  const errors = validateCandidate(manifest,{allowPendingReplay:true})
  if (errors.length) throw new Error(errors.join('\n'))
  mkdirSync(ARTIFACTS_DIR,{recursive:true})
  writeFileSync(path,JSON.stringify(manifest,null,2)+'\n',{flag:'wx'})
  console.log('Wrote candidate (not a Freeze): ' + relative(ROOT,path))
}
if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try { main() } catch (error) { console.error(error.message); process.exitCode = 1 }
}
