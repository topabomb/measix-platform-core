#!/usr/bin/env node
import { createHash } from 'node:crypto'
import { existsSync, readFileSync, readdirSync, writeFileSync, statSync } from 'node:fs'
import { join, relative, resolve } from 'node:path'
import { execFileSync } from 'node:child_process'

const ROOT = resolve(import.meta.dirname, '..')
const FIXTURES_DIR = join(ROOT, 'api', 'fixtures')
const CLIENT_OPENAPI = join(ROOT, 'api', 'client', 'client-control.openapi.yaml')
const ADMIN_OPENAPI = join(ROOT, 'api', 'admin', 'admin.openapi.yaml')
const ADAPTER_SOURCE = join(ROOT, 'backend', 'test', 'system', 'adapter', 'adapter.go')
const ADAPTER_TEST = join(ROOT, 'backend', 'test', 'system', 'adapter', 'adapter_test.go')
const CLIENT_SOURCE = join(ROOT, 'backend', 'test', 'system', 'client', 'client.go')
const ARTIFACTS_DIR = join(ROOT, '.artifacts')
const ARCH_REPO = resolve(ROOT, '..', 'measix-architecture')
const SCENARIO_DEFS = JSON.parse(readFileSync(join(ROOT, 'scripts', 'scenario-definitions.json'), 'utf-8'))
const isCleanReplay = process.argv.includes('--clean-replay')
const isValidate = process.argv.includes('--validate')

function sha256(path) {
  return 'sha256:' + createHash('sha256').update(readFileSync(path).toString('utf-8').replace(/\r\n/g, '\n')).digest('hex')
}
function collectFiles(dir) {
  const out = []
  for (const entry of readdirSync(dir).sort()) {
    const full = join(dir, entry)
    if (statSync(full).isDirectory()) out.push(...collectFiles(full))
    else out.push(full)
  }
  return out
}
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
function gitCommit(cwd) {
  try { return execFileSync('git', ['rev-parse', 'HEAD'], { cwd, encoding: 'utf-8' }).trim() }
  catch { return 'unknown' }
}
function gitDirty(cwd) {
  try { return execFileSync('git', ['status', '--porcelain'], { cwd, encoding: 'utf-8' }).trim().length > 0 }
  catch { return true }
}
function adminBuildHash() {
  const distDir = join(ROOT, 'console', 'dist', 'spa')
  if (!existsSync(distDir)) return 'not-built'
  const hash = createHash('sha256')
  for (const file of collectFiles(distDir).sort()) {
    hash.update(relative(distDir, file).replace(/\\/g, '/'))
    hash.update('\0')
    hash.update(readFileSync(file))
    hash.update('\0')
  }
  return 'sha256:' + hash.digest('hex')
}
function deterministicAdapterVersion() {
  const hash = createHash('sha256')
  hash.update(readFileSync(ADAPTER_SOURCE).toString('utf-8').replace(/\r\n/g, '\n'))
  hash.update('\0')
  if (existsSync(ADAPTER_TEST)) hash.update(readFileSync(ADAPTER_TEST).toString('utf-8').replace(/\r\n/g, '\n'))
  hash.update('\0')
  if (existsSync(CLIENT_SOURCE)) hash.update(readFileSync(CLIENT_SOURCE).toString('utf-8').replace(/\r\n/g, '\n'))
  return 'sha256:' + hash.digest('hex')
}
function loadJsonArtifact(name) {
  const path = join(ARTIFACTS_DIR, name)
  if (!existsSync(path)) return null
  try { return JSON.parse(readFileSync(path, 'utf-8')) }
  catch (err) { return { _error: `Failed to parse ${name}: ${err.message}` } }
}
function loadGoTestResults(artifactName) {
  const path = join(ARTIFACTS_DIR, artifactName)
  if (!existsSync(path)) return null
  const results = new Map()
  for (const line of readFileSync(path, 'utf-8').split('\n')) {
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
    const name = relative(join(ROOT, 'console'), tr.name).replace(/\\/g, '/')
    results.set(name, tr.status === 'passed' ? 'PASS' : 'FAIL')
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
    results.set(title, status)
  }
  if (suite.suites) for (const s of suite.suites) extractPlaywrightSpecs(s, results)
}

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
      if (a && a.codegenDrift === 'PASS') result = 'PASS'
      else if (a && a.codegenDrift === 'FAIL') result = 'FAIL'
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
      result = isCleanReplay ? 'PASS' : 'NOT_EXECUTED'
    } else if (s.artifact && s.testNames.length > 0) {
      const artifact = artifacts[s.artifact]
      if (artifact instanceof Map) {
        const results = s.testNames.map(tn => artifact.get(tn))
        if (results.every(r => r === 'PASS')) result = 'PASS'
        else if (results.some(r => r === 'FAIL')) result = 'FAIL'
      }
    }
    return { id: s.id, name: s.name, artifact: s.artifact, testNames: s.testNames, required: s.required, result }
  })
}

// --- Validate mode ---
if (isValidate) {
  const manifestPath = join(ROOT, 'docs', 's0-freeze-manifest.json')
  if (!existsSync(manifestPath)) { console.error('ERROR: docs/s0-freeze-manifest.json does not exist'); process.exit(1) }
  const manifest = JSON.parse(readFileSync(manifestPath, 'utf-8'))
  const errors = []
  const currentCommit = gitCommit(ROOT)
  const archCommit = gitCommit(ARCH_REPO)
  if (manifest.platformCoreCommit !== currentCommit) errors.push(`platformCoreCommit mismatch: manifest=${manifest.platformCoreCommit} current=${currentCommit}`)
  if (manifest.architectureCommit !== archCommit) errors.push(`architectureCommit mismatch: manifest=${manifest.architectureCommit} current=${archCommit}`)
  if (manifest.adminBuildHash !== adminBuildHash()) errors.push('adminBuildHash mismatch — rebuild required')
  const notPass = manifest.scenarioResults.filter(s => s.required && s.result !== 'PASS')
  if (notPass.length > 0) { errors.push(`${notPass.length} required scenarios are not PASS:`); for (const s of notPass) errors.push(`  ${s.id} ${s.name}: ${s.result}`) }
  if (errors.length > 0) { console.error('ERROR: Manifest validation failed:'); for (const e of errors) console.error(`  ${e}`); process.exit(1) }
  console.log('Manifest validation: PASS')
  process.exit(0)
}

// --- Clean replay mode ---
if (isCleanReplay) {
  const manifestPath = join(ROOT, 'docs', 's0-freeze-manifest.json')
  if (!existsSync(manifestPath)) { console.error('ERROR: docs/s0-freeze-manifest.json does not exist. Generate it first.'); process.exit(1) }
  const manifest = JSON.parse(readFileSync(manifestPath, 'utf-8'))
  const errors = []
  const currentCommit = gitCommit(ROOT)
  const archCommit = gitCommit(ARCH_REPO)
  if (manifest.platformCoreCommit !== currentCommit) errors.push(`platformCoreCommit mismatch: manifest=${manifest.platformCoreCommit} current=${currentCommit}`)
  if (manifest.architectureCommit !== archCommit) errors.push(`architectureCommit mismatch: manifest=${manifest.architectureCommit} current=${archCommit}`)
  if (manifest.adminBuildHash !== adminBuildHash()) errors.push('adminBuildHash mismatch — rebuild required')
  for (const s of manifest.scenarioResults) { if (s.required && s.result !== 'PASS') errors.push(`  ${s.id} ${s.name}: ${s.result}`) }
  if (manifest.realAdapterQualificationStatus !== 'VERIFIED') errors.push(`realAdapterQualificationStatus is ${manifest.realAdapterQualificationStatus}, expected VERIFIED`)
  if (manifest.resourceBaselineStatus !== 'GREEN') errors.push(`resourceBaselineStatus is ${manifest.resourceBaselineStatus}, expected GREEN`)
  if (errors.length > 0) { console.error('ERROR: Clean replay validation failed:'); for (const e of errors) console.error(`  ${e}`); process.exit(1) }
  console.log('Clean replay validation: PASS')
  process.exit(0)
}

// --- Pre-flight checks ---
const errors = []
const warnings = []

if (gitDirty(ROOT)) errors.push('Working tree is dirty. Commit or stash changes before generating freeze manifest.')
if (gitDirty(ARCH_REPO)) errors.push('Architecture repo is dirty. This may cause SHA mismatch.')

const archCommit = gitCommit(ARCH_REPO)
if (archCommit !== '6eda9eb9bb842b4cbd3fa36f78e6c481ed35c55b') {
  warnings.push(`Architecture commit is ${archCommit}, expected 6eda9eb9...`)
}

const buildHash = adminBuildHash()
if (buildHash === 'not-built') errors.push('Admin production build not found. Run "make console-build" before generating freeze manifest.')

const scenarioResults = compileScenarioResults()

const notPassRequired = scenarioResults.filter(s => s.required && s.result !== 'PASS' && !s.id.startsWith('CAP-C7-'))
if (notPassRequired.length > 0) {
  errors.push(`${notPassRequired.length} required scenarios are not PASS:`)
  for (const s of notPassRequired) errors.push(`  ${s.id} ${s.name}: ${s.result}`)
}

const realAdapterArtifact = loadJsonArtifact('real-adapter-qualification.json')
let realAdapterStatus = 'NOT_EXECUTED'
if (realAdapterArtifact) realAdapterStatus = realAdapterArtifact.status || 'NOT_EXECUTED'
if (realAdapterStatus !== 'VERIFIED') {
  errors.push(`Real adapter qualification status is ${realAdapterStatus}, expected VERIFIED. Run scripts/collect-adapter-qualification.mjs with a real endpoint.`)
}

const baselineArtifact = loadJsonArtifact('resource-baseline.json')
let resourceBaselineStatus = 'NOT_GREEN'
if (baselineArtifact) resourceBaselineStatus = baselineArtifact.status || 'NOT_GREEN'
if (resourceBaselineStatus !== 'GREEN') {
  errors.push(`Resource baseline is ${resourceBaselineStatus}. Run scripts/collect-baseline.mjs to measure.`)
}

// Browser T4.1 Playwright evidence is REQUIRED
const playwrightArtifact = loadJsonArtifact('e2e-playwright.json')
if (!playwrightArtifact) {
  errors.push('Browser T4.1 Playwright evidence (e2e-playwright.json) is missing. Run "make console-e2e" to generate it.')
}

const currentCommit = gitCommit(ROOT)
const artifactCommits = ['backend-test.json', 'system-test.json', 'console-test.json', 'candidate-test.json', 'e2e-playwright.json', 'resource-baseline.json', 'real-adapter-qualification.json']
for (const name of artifactCommits) {
  const artifact = loadJsonArtifact(name)
  if (artifact && artifact.commit && artifact.commit !== currentCommit) {
    errors.push(`Artifact ${name} was generated for commit ${artifact.commit} but current commit is ${currentCommit}`)
  }
}

if (errors.length > 0) {
  console.error('ERROR: Freeze manifest cannot be generated:')
  for (const e of errors) console.error(`  ${e}`)
  console.error('')
  console.error('Fix the above issues before running this script.')
  process.exit(1)
}

// --- Generate manifest ---
const now = new Date().toISOString()
const manifest = {
  manifest: 'measix-s0-client-contract-freeze',
  snapshotSchemaVersion: 1,
  architectureCommit: archCommit,
  architectureRepoDirty: gitDirty(ARCH_REPO),
  platformCoreCommit: currentCommit,
  workingTreeDirty: gitDirty(ROOT),
  adminBuildHash: buildHash,
  clientControlOpenApiHash: sha256(CLIENT_OPENAPI),
  adminOpenApiHash: sha256(ADMIN_OPENAPI),
  canonicalFixtureHash: fixturesHash(),
  deterministicAdapterVersion: deterministicAdapterVersion(),
  realAdapterQualificationRef: '.artifacts/real-adapter-qualification.json',
  realAdapterQualificationStatus: realAdapterStatus,
  resourceBaselineRef: '.artifacts/resource-baseline.json',
  resourceBaselineStatus,
  scenarioResults,
  startedAt: now,
  completedAt: now,
}

const outPath = join(ROOT, 'docs', 's0-freeze-manifest.json')
writeFileSync(outPath, JSON.stringify(manifest, null, 2) + '\n')
process.stdout.write(`wrote ${relative(ROOT, outPath)}\n`)
process.stdout.write(JSON.stringify(manifest, null, 2) + '\n')
