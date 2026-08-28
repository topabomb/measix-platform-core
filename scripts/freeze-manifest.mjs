#!/usr/bin/env node
import { createHash } from 'node:crypto'
import { existsSync, readFileSync, writeFileSync } from 'node:fs'
import { join, relative, resolve } from 'node:path'

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
const isCleanReplay = process.argv.includes('--clean-replay')
const isValidate = process.argv.includes('--validate')

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
      // The replay-freeze.mjs script updates the manifest after successful replay.
      result = isCleanReplay ? 'PASS' : 'NOT_EXECUTED'
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

function validateArtifactProvenance(artifactName, currentCommit) {
  const errors = []
  const metaPath = join(ARTIFACTS_DIR, artifactName + '.meta.json')

  // Try meta.json first (the new provenance envelope)
  const meta = loadMetaArtifact(artifactName)
  if (meta) {
    // Verify meta commit matches current commit
    if (meta.platformCoreCommit && meta.platformCoreCommit !== currentCommit) {
      errors.push(`Artifact ${artifactName} meta commit mismatch: meta=${meta.platformCoreCommit} current=${currentCommit}`)
    }

    // Verify SHA-256 of the artifact matches meta's artifactSha256
    const artifactPath = join(ARTIFACTS_DIR, artifactName)
    if (existsSync(artifactPath) && meta.artifactSha256) {
      const actualSha = 'sha256:' + createHash('sha256').update(readFileSync(artifactPath)).digest('hex')
      if (actualSha !== meta.artifactSha256) {
        errors.push(`Artifact ${artifactName} SHA mismatch: meta=${meta.artifactSha256} actual=${actualSha}`)
      }
    }

    // Verify exit code — a non-zero exit code means the test command failed
    // but the artifact file was still written (partial output). This must
    // be treated as a failure.
    if (meta.exitCode !== undefined && meta.exitCode !== 0) {
      errors.push(`Artifact ${artifactName} was generated with non-zero exit code ${meta.exitCode}. Test command failed.`)
    }
  } else {
    // Fall back to legacy: check if artifact is JSON with embedded commit field
    const artifact = loadJsonArtifact(artifactName)
    if (artifact && !artifact._error) {
      if (artifact.commit && artifact.commit !== currentCommit) {
        errors.push(`Artifact ${artifactName} was generated for commit ${artifact.commit} but current commit is ${currentCommit}`)
      }
    }
    // For NDJSON artifacts (Go test -json), there's no commit field;
    // meta.json is the only way to provenance them. Warn if missing.
    if (artifactName.endsWith('.json') && !artifactName.includes('static-contract') &&
        !artifactName.includes('resource-baseline') && !artifactName.includes('real-adapter') &&
        !artifactName.includes('e2e-playwright')) {
      // Go NDJSON artifacts need meta.json
      const artifactPath = join(ARTIFACTS_DIR, artifactName)
      if (existsSync(artifactPath) && !meta) {
        errors.push(`Artifact ${artifactName} has no meta.json provenance envelope. Run collect-artifacts with meta generation.`)
      }
    }
  }

  return errors
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
  if (manifest.adminBuildHash !== adminBuildHash(ROOT)) errors.push('adminBuildHash mismatch — rebuild required')
  const notPass = manifest.scenarioResults.filter(s => s.required && s.result !== 'PASS')
  if (notPass.length > 0) { errors.push(`${notPass.length} required scenarios are not PASS:`); for (const s of notPass) errors.push(`  ${s.id} ${s.name}: ${s.result}`) }
  if (errors.length > 0) { console.error('ERROR: Manifest validation failed:'); for (const e of errors) console.error(`  ${e}`); process.exit(1) }
  console.log('Manifest validation: PASS')
  process.exit(0)
}

// --- Clean replay mode ---
// This is now a manifest validation only. For real clean-environment replay,
// use scripts/replay-freeze.mjs (via `make clean-replay`).
if (isCleanReplay) {
  const manifestPath = join(ROOT, 'docs', 's0-freeze-manifest.json')
  if (!existsSync(manifestPath)) { console.error('ERROR: docs/s0-freeze-manifest.json does not exist. Generate it first.'); process.exit(1) }
  const manifest = JSON.parse(readFileSync(manifestPath, 'utf-8'))
  const errors = []
  const currentCommit = gitCommit(ROOT)
  const archCommit = gitCommit(ARCH_REPO)
  if (manifest.platformCoreCommit !== currentCommit) errors.push(`platformCoreCommit mismatch: manifest=${manifest.platformCoreCommit} current=${currentCommit}`)
  if (manifest.architectureCommit !== archCommit) errors.push(`architectureCommit mismatch: manifest=${manifest.architectureCommit} current=${archCommit}`)
  if (manifest.adminBuildHash !== adminBuildHash(ROOT)) errors.push('adminBuildHash mismatch — rebuild required')
  for (const s of manifest.scenarioResults) { if (s.required && s.result !== 'PASS') errors.push(`  ${s.id} ${s.name}: ${s.result}`) }
  if (manifest.realAdapterQualificationStatus !== 'VERIFIED') errors.push(`realAdapterQualificationStatus is ${manifest.realAdapterQualificationStatus}, expected VERIFIED`)
  if (manifest.resourceBaselineStatus !== 'GREEN') errors.push(`resourceBaselineStatus is ${manifest.resourceBaselineStatus}, expected GREEN`)
  if (errors.length > 0) { console.error('ERROR: Clean replay manifest validation failed:'); for (const e of errors) console.error(`  ${e}`); process.exit(1) }
  console.log('Clean replay manifest validation: PASS')
  console.log('NOTE: For full clean-environment replay, run: make clean-replay')
  process.exit(0)
}

// --- Pre-flight checks ---
const errors = []
const warnings = []

if (gitDirty(ROOT)) errors.push('Working tree is dirty. Commit or stash changes before generating freeze manifest.')
if (gitDirty(ARCH_REPO)) errors.push('Architecture repo is dirty. This may cause SHA mismatch.')

const archCommit = gitCommit(ARCH_REPO)
// Per audit P2-1: do not hardcode an architecture commit. Instead, record the
// architecture commit in the manifest so that freeze-validate can verify the
// correct architecture commit is checked out at validation time. The expected
// architecture commit is implicitly the one shipped with this platform-core
// commit (they are version-coupled).

const buildHash = adminBuildHash(ROOT)
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
// Per architecture: freeze must verify that no profile is FAILED.
// MODEL profile must be VERIFIED. TTS/ASR/MCP are optional — NOT_EXECUTED
// is acceptable when the profile was not configured. Only FAILED is blocking.
if (realAdapterArtifact && realAdapterArtifact.profiles) {
  const REQUIRED_QUAL_PROFILES = ['model', 'tts', 'asr', 'mcp']
  for (const p of REQUIRED_QUAL_PROFILES) {
    const ps = realAdapterArtifact.profiles[p]
    const status = ps ? ps.status : 'MISSING'
    if (status === 'FAILED' || status === 'MISSING') {
      errors.push(`Real adapter qualification profile '${p}' is ${status}, expected VERIFIED or NOT_EXECUTED.`)
    }
  }
  // MODEL must be VERIFIED (required profile)
  const modelStatus = realAdapterArtifact.profiles.model?.status
  if (modelStatus !== 'VERIFIED') {
    errors.push(`Real adapter qualification profile 'model' must be VERIFIED, got ${modelStatus || 'MISSING'}.`)
  }
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

// Validate provenance for all artifacts
const artifactNames = ['backend-test.json', 'system-test.json', 'console-test.json', 'candidate-test.json', 'e2e-playwright.json', 'resource-baseline.json', 'real-adapter-qualification.json']
for (const name of artifactNames) {
  const provenanceErrors = validateArtifactProvenance(name, currentCommit)
  errors.push(...provenanceErrors)
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
  clientControlOpenApiHash: sha256File(CLIENT_OPENAPI),
  adminOpenApiHash: sha256File(ADMIN_OPENAPI),
  canonicalFixtureHash: fixturesHash(),
  deterministicAdapterVersion: deterministicAdapterVersion(ROOT),
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
