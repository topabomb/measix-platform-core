#!/usr/bin/env node
/**
 * S0.1 Client Contract Freeze — Evidence Compiler & Validator
 *
 * Per measix-s0-capability-delivery-contract-spec.md §10 and
 * measix-s0-capability-delivery-system-testing-spec.md §18, the freeze
 * manifest must at least record:
 *   - architectureCommit           pinned architecture repository commit
 *   - platformCoreCommit            current Git commit of platform-core
 *   - adminBuildHash                SHA-256 of the Admin Console production build
 *   - clientControlOpenApiHash       SHA-256 of api/client/client-control.openapi.yaml
 *   - canonicalFixtureHash          SHA-256 over the canonical api/fixtures tree
 *   - snapshotSchemaVersion        the frozen Android-visible schemaVersion (1)
 *   - deterministicAdapterVersion  version hash of the deterministic Test Adapter
 *   - realAdapterQualificationRef   reference to the real adapter qualification report
 *   - scenarioResults               ALL CAP-C0 through CAP-C7 scenario pass/fail results
 *   - startedAt                     when the gate execution started
 *   - completedAt                   when the gate execution completed
 *
 * EVIDENCE COMPILER DESIGN:
 *
 * This script does NOT execute tests or claim PASS/FAIL on its own.
 * It consumes machine-readable test result artifacts produced by the
 * test runners and compiles them into the freeze manifest.
 *
 * The correct pipeline is:
 *
 *   test runners (go test -json, vitest --json, playwright --reporter=json)
 *     ↓
 *   machine-readable result artifacts (JSON files in .artifacts/)
 *     ↓
 *   adapter qualification artifact (docs/s0-real-adapter-qualification.json)
 *   resource baseline artifact (docs/s0-resource-baseline.json)
 *   browser T4.1 artifact (Playwright JSON report)
 *     ↓
 *   freeze-manifest validator/compiler (this script)
 *     ↓
 *   docs/s0-freeze-manifest.json
 *
 * The manifest only summarizes evidence that already exists and whose
 * SHA/commit matches. It cannot create PASS results on its own.
 *
 * IMPORTANT: This script MUST NOT be run if:
 *   - Any required scenario is not PASS
 *   - The working tree is dirty
 *   - The Admin build is not present (adminBuildHash must not be "not-built")
 *   - Real adapter qualification has not been executed
 *   - Resource baseline has not been measured
 *   - Test result artifacts do not match the current commit
 */
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
const SNAPSHOT_SCHEMA_VERSION = 1

// Artifact directory — test runners write JSON results here
const ARTIFACTS_DIR = join(ROOT, '.artifacts')

// Architecture repository path (sibling directory)
const ARCH_REPO = resolve(ROOT, '..', 'measix-architecture')

// --- Hashing utilities ---

function sha256(path) {
  const data = readFileSync(path).toString('utf-8').replace(/\r\n/g, '\n')
  return 'sha256:' + createHash('sha256').update(data).digest('hex')
}

function collectFiles(dir) {
  const out = []
  for (const entry of readdirSync(dir).sort()) {
    const full = join(dir, entry)
    const stat = statSync(full)
    if (stat.isDirectory()) {
      out.push(...collectFiles(full))
    } else {
      out.push(full)
    }
  }
  return out
}

function fixturesHash() {
  const files = collectFiles(FIXTURES_DIR).sort()
  const hash = createHash('sha256')
  for (const file of files) {
    hash.update(relative(FIXTURES_DIR, file).replace(/\\/g, '/'))
    hash.update('\0')
    hash.update(readFileSync(file).toString('utf-8').replace(/\r\n/g, '\n'))
    hash.update('\0')
  }
  return 'sha256:' + hash.digest('hex')
}

function gitCommit(cwd) {
  try {
    return execFileSync('git', ['rev-parse', 'HEAD'], { cwd, encoding: 'utf-8' }).trim()
  } catch {
    return 'unknown'
  }
}

function gitDirty(cwd) {
  try {
    const status = execFileSync('git', ['status', '--porcelain'], { cwd, encoding: 'utf-8' }).trim()
    return status.length > 0
  } catch {
    return true
  }
}

function adminBuildHash() {
  const distDir = join(ROOT, 'console', 'dist', 'spa')
  if (!existsSync(distDir)) {
    return 'not-built'
  }
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
  if (existsSync(ADAPTER_TEST)) {
    hash.update(readFileSync(ADAPTER_TEST).toString('utf-8').replace(/\r\n/g, '\n'))
  }
  hash.update('\0')
  if (existsSync(CLIENT_SOURCE)) {
    hash.update(readFileSync(CLIENT_SOURCE).toString('utf-8').replace(/\r\n/g, '\n'))
  }
  return 'sha256:' + hash.digest('hex')
}

// --- Artifact loading ---

function loadJsonArtifact(name) {
  const path = join(ARTIFACTS_DIR, name)
  if (!existsSync(path)) return null
  try {
    return JSON.parse(readFileSync(path, 'utf-8'))
  } catch (err) {
    return { _error: `Failed to parse ${name}: ${err.message}` }
  }
}

/**
 * Load Go test -json output and extract pass/fail per test.
 * Returns a Map of test name -> 'PASS' | 'FAIL'.
 */
function loadGoTestResults(artifactName) {
  const path = join(ARTIFACTS_DIR, artifactName)
  if (!existsSync(path)) return null
  const results = new Map()
  const lines = readFileSync(path, 'utf-8').split('\n')
  for (const line of lines) {
    if (!line.trim()) continue
    try {
      const entry = JSON.parse(line)
      if (entry.Action === 'pass' && entry.Test) {
        results.set(entry.Test, 'PASS')
      } else if (entry.Action === 'fail' && entry.Test) {
        results.set(entry.Test, 'FAIL')
      }
    } catch { /* skip */ }
  }
  return results
}

/**
 * Load vitest JSON output and extract pass/fail per test file.
 */
function loadVitestResults(artifactName) {
  const artifact = loadJsonArtifact(artifactName)
  if (!artifact) return null
  const results = new Map()
  if (artifact.testResults) {
    for (const tr of artifact.testResults) {
      const name = relative(join(ROOT, 'console'), tr.name).replace(/\\/g, '/')
      const status = tr.status === 'passed' ? 'PASS' : 'FAIL'
      results.set(name, status)
    }
  }
  return results
}

// --- CAP scenario definitions ---

const scenarioDefinitions = [
  // C0 — Contract
  { id: 'CAP-C0-001', name: 'Required profile enum/schema', artifact: 'backend-test.json', testNames: ['TestCAPC0004TTSVoiceRequired', 'TestCAPC0006MCPAuthOwnershipValidation'], required: true },
  { id: 'CAP-C0-002', name: 'Unsupported protocol', artifact: 'backend-test.json', testNames: ['TestHUBCAP004RouteUpstreamValidation'], required: true },
  { id: 'CAP-C0-003', name: 'Model vocabulary', artifact: 'backend-test.json', testNames: ['TestHUBCAP004RouteUpstreamValidation'], required: true },
  { id: 'CAP-C0-004', name: 'TTS voice required', artifact: 'backend-test.json', testNames: ['TestCAPC0004TTSVoiceRequired'], required: true },
  { id: 'CAP-C0-005', name: 'ASR HTTP transcription', artifact: 'backend-test.json', testNames: ['TestHUBCAP004RouteUpstreamValidation'], required: true },
  { id: 'CAP-C0-006', name: 'MCP auth ownership', artifact: 'backend-test.json', testNames: ['TestCAPC0006MCPAuthOwnershipValidation'], required: true },
  { id: 'CAP-C0-007', name: 'Unknown optional response', artifact: 'backend-test.json', testNames: ['TestSYSI0002UnknownOptionalResponseFieldIsTolerated'], required: true },
  { id: 'CAP-C0-008', name: 'Unknown request field rejected', artifact: 'backend-test.json', testNames: ['TestSYSI0003UnknownRequestFieldIsRejected'], required: true },
  { id: 'CAP-C0-009', name: 'Codegen drift', artifact: 'static-contract.json', testNames: ['generated-drift-check'], required: true },
  // C1 — Upstream
  { id: 'CAP-C1-001', name: 'Browser creates NONE-auth Upstream', artifact: 'console-test.json', testNames: ['console/src/pages/UpstreamsPage.test.ts'], required: true },
  { id: 'CAP-C1-002', name: 'BEARER secret', artifact: 'console-test.json', testNames: ['console/src/pages/UpstreamsPage.test.ts'], required: true },
  { id: 'CAP-C1-003', name: 'STATIC_HEADER/BASIC', artifact: 'backend-test.json', testNames: ['TestHUBUPS001CandidateAndActiveConfigRevisionsAreSeparate'], required: true },
  { id: 'CAP-C1-004', name: 'Replace Secret', artifact: 'backend-test.json', testNames: ['TestHUBUPS003SecretVersionsAreAppendOnlyEncryptedAndReferencedPrecisely'], required: true },
  { id: 'CAP-C1-005', name: 'Candidate edit', artifact: 'backend-test.json', testNames: ['TestHUBUPS001CandidateAndActiveConfigRevisionsAreSeparate'], required: true },
  { id: 'CAP-C1-006', name: 'Test connection', artifact: 'backend-test.json', testNames: ['TestHUBUPS002ValidateConfigDoesNotApply'], required: true },
  { id: 'CAP-C1-007', name: 'Apply happy path', artifact: 'backend-test.json', testNames: ['TestI3PublishPersistsIntentBeforeRelayAndFinalizesAfterAck'], required: true },
  { id: 'CAP-C1-008', name: 'Apply refresh recovery', artifact: 'backend-test.json', testNames: ['TestHUBACT008ReconcileFinalizesPending'], required: true },
  { id: 'CAP-C1-009', name: 'Apply failure', artifact: 'backend-test.json', testNames: ['TestHUBUPS005UpdateFailureRetainsOldRevision'], required: true },
  { id: 'CAP-C1-010', name: 'Secret browser safety', artifact: 'console-test.json', testNames: ['console/src/pages/UpstreamsPage.test.ts'], required: true },
  // C2 — Resource Authoring
  { id: 'CAP-C2-001', name: 'Create Provider + Model', artifact: 'console-test.json', testNames: ['console/src/pages/ResourcesPage.test.ts'], required: true },
  { id: 'CAP-C2-002', name: 'Model capabilities', artifact: 'backend-test.json', testNames: ['TestHUBCAP004RouteUpstreamValidation'], required: true },
  { id: 'CAP-C2-003', name: 'Missing Provider', artifact: 'backend-test.json', testNames: ['TestHUBCAP004RouteUpstreamValidation'], required: true },
  { id: 'CAP-C2-004', name: 'Missing Binding', artifact: 'backend-test.json', testNames: ['TestHUBCAP004RouteUpstreamValidation'], required: true },
  { id: 'CAP-C2-010', name: 'Create TTS', artifact: 'console-test.json', testNames: ['console/src/pages/ResourcesPage.test.ts'], required: true },
  { id: 'CAP-C2-011', name: 'Missing voice', artifact: 'backend-test.json', testNames: ['TestCAPC0004TTSVoiceRequired'], required: true },
  { id: 'CAP-C2-020', name: 'Create HTTP ASR', artifact: 'console-test.json', testNames: ['console/src/pages/ResourcesPage.test.ts'], required: true },
  { id: 'CAP-C2-021', name: 'Realtime fields not exposed', artifact: 'console-test.json', testNames: ['console/src/pages/ResourcesPage.test.ts'], required: true },
  { id: 'CAP-C2-030', name: 'Enterprise-managed MCP', artifact: 'console-test.json', testNames: ['console/src/pages/ResourcesPage.test.ts'], required: true },
  { id: 'CAP-C2-031', name: 'NONE auth MCP', artifact: 'backend-test.json', testNames: ['TestCAPC0006MCPAuthOwnershipValidation'], required: true },
  { id: 'CAP-C2-032', name: 'Unsupported auth ownership', artifact: 'backend-test.json', testNames: ['TestCAPC0006MCPAuthOwnershipValidation'], required: true },
  { id: 'CAP-C2-040', name: 'Local coexistence policy', artifact: 'console-test.json', testNames: ['console/src/pages/ResourcesPage.test.ts'], required: true },
  { id: 'CAP-C2-041', name: 'Defaults', artifact: 'console-test.json', testNames: ['console/src/pages/ResourcesPage.test.ts'], required: true },
  { id: 'CAP-C2-042', name: 'Stale revision 409', artifact: 'backend-test.json', testNames: ['TestHUBCAP001DraftOptimisticConcurrencyAndSaveDoesNotActivate'], required: true },
  { id: 'CAP-C2-043', name: 'Route safety', artifact: 'backend-test.json', testNames: ['TestHUBCAP004RouteUpstreamValidation'], required: true },
  // C3 — Snapshot / Review
  { id: 'CAP-C3-001', name: 'Preview canonical projection', artifact: 'backend-test.json', testNames: ['TestHUBCAP006SnapshotDeterministicAndClientSafe'], required: true },
  { id: 'CAP-C3-002', name: 'Preview has no side effect', artifact: 'backend-test.json', testNames: ['TestHUBCAP005SaveDraftDoesNotChangeActiveRelease'], required: true },
  { id: 'CAP-C3-003', name: 'Snapshot no server internals', artifact: 'backend-test.json', testNames: ['TestHUBCAP007SnapshotExcludesSecretsAndUpstreamURLAndRouteID'], required: true },
  { id: 'CAP-C3-004', name: 'Full profile Snapshot', artifact: 'backend-test.json', testNames: ['TestSYSI0001CanonicalFixturesDecodeWithGeneratedWire'], required: true },
  { id: 'CAP-C3-005', name: 'Deterministic order/hash', artifact: 'backend-test.json', testNames: ['TestSnapshotAndRuntimeControlGoldenHashes'], required: true },
  { id: 'CAP-C3-006', name: 'ETag/hash', artifact: 'backend-test.json', testNames: ['TestSnapshotAndRuntimeControlGoldenHashes'], required: true },
  { id: 'CAP-C3-007', name: 'Publish diff', artifact: 'console-test.json', testNames: ['console/src/pages/ResourcesPage.test.ts'], required: true },
  { id: 'CAP-C3-008', name: 'Warning acknowledgement', artifact: 'backend-test.json', testNames: ['TestHUBCAP010WarningsCannotBypassServerValidation'], required: true },
  { id: 'CAP-C3-009', name: 'Invalid Draft Publish', artifact: 'backend-test.json', testNames: ['TestHUBCAP010WarningsCannotBypassServerValidation'], required: true },
  // C4 — Runtime Transport
  { id: 'CAP-C4-001', name: 'Chat request/response', artifact: 'system-test.json', testNames: ['TestCAPC4001ClientChatNonStream'], required: true },
  { id: 'CAP-C4-002', name: 'Chat streaming', artifact: 'backend-test.json', testNames: ['TestRLYTRN001002SSEFirstFlushAndOrderPreserved'], required: true },
  { id: 'CAP-C4-003', name: 'Model cancel', artifact: 'backend-test.json', testNames: ['TestRLYTRN004ClientCancelPropagates'], required: true },
  { id: 'CAP-C4-010', name: 'TTS request', artifact: 'system-test.json', testNames: ['TestCAPC4010ClientTTSBinary'], required: true },
  { id: 'CAP-C4-011', name: 'Binary integrity', artifact: 'backend-test.json', testNames: ['TestRLYTRN006BinaryPayloadExact'], required: true },
  { id: 'CAP-C4-012', name: 'TTS failure', artifact: 'backend-test.json', testNames: ['TestRLYHDRUpstreamErrorPreserved'], required: true },
  { id: 'CAP-C4-020', name: 'Multipart transcription', artifact: 'backend-test.json', testNames: ['TestRLYTRN009MultipartPreserved'], required: true },
  { id: 'CAP-C4-021', name: 'Multipart limits', artifact: 'backend-test.json', testNames: ['TestRLYTRN010LargeUploadStreaming'], required: true },
  { id: 'CAP-C4-022', name: 'ASR cancel', artifact: 'backend-test.json', testNames: ['TestRLYTRN011UploadCancelTerminatesUpstreamRead'], required: true },
  { id: 'CAP-C4-030', name: 'MCP initialize/list/call', artifact: 'system-test.json', testNames: ['TestCAPC4030ClientMCP'], required: true },
  { id: 'CAP-C4-031', name: 'MCP streaming/session', artifact: 'backend-test.json', testNames: ['TestRLYTRN012MCPTransparentForward'], required: true },
  { id: 'CAP-C4-032', name: 'MCP auth injection', artifact: 'backend-test.json', testNames: ['TestRLYTRN014RelayDoesNotRewriteMCP'], required: true },
  { id: 'CAP-C4-040', name: 'Old generation 428 no-forward', artifact: 'backend-test.json', testNames: ['TestRLYADM002GenerationMismatchRejected'], required: true },
  { id: 'CAP-C4-041', name: 'Invalid JWT', artifact: 'backend-test.json', testNames: ['TestRLYAUTH002WrongAlgSignatureKidRejected'], required: true },
  { id: 'CAP-C4-042', name: 'Revoked principal', artifact: 'backend-test.json', testNames: ['TestRLYAUTH007RevokedDeviceSessionRejected'], required: true },
  { id: 'CAP-C4-043', name: 'Unknown resource', artifact: 'backend-test.json', testNames: ['TestRLYADM004UnknownResourceRejected'], required: true },
  { id: 'CAP-C4-044', name: 'Path traversal/absolute URI', artifact: 'backend-test.json', testNames: ['TestRLYROUTE005TraversalVariantsRejected'], required: true },
  { id: 'CAP-C4-045', name: 'Internal header spoof', artifact: 'backend-test.json', testNames: ['TestRLYSecurityClientCannotInjectRequestId'], required: true },
  // C5 — Usage / Pricing
  { id: 'CAP-C5-001', name: 'Model request usage', artifact: 'backend-test.json', testNames: ['TestHUBI5RequestUsageBatchIsIdempotent'], required: true },
  { id: 'CAP-C5-002', name: 'TTS request usage', artifact: 'backend-test.json', testNames: ['TestHUBI5SemanticUsageDedupeAndCompleteness'], required: true },
  { id: 'CAP-C5-003', name: 'ASR request usage', artifact: 'backend-test.json', testNames: ['TestHUBI5SemanticUsageDedupeAndCompleteness'], required: true },
  { id: 'CAP-C5-004', name: 'MCP request usage', artifact: 'backend-test.json', testNames: ['TestHUBI5SemanticUsageDedupeAndCompleteness'], required: true },
  { id: 'CAP-C5-005', name: 'Hub usage outage', artifact: 'backend-test.json', testNames: ['TestRLYSPHubAckDeletesOnlyAcknowledgedBatch'], required: true },
  { id: 'CAP-C5-006', name: 'Duplicate delivery', artifact: 'backend-test.json', testNames: ['TestHUBI5RequestUsageBatchIsIdempotent'], required: true },
  { id: 'CAP-C5-007', name: 'Poison row', artifact: 'backend-test.json', testNames: ['TestRLYSPPoison422IsolatedWithoutDroppingGoodRows'], required: true },
  { id: 'CAP-C5-008', name: 'UNKNOWN semantic', artifact: 'backend-test.json', testNames: ['TestHUBUSG004UnknownPartialDoNotFabricateCost'], required: true },
  { id: 'CAP-C5-009', name: 'PARTIAL semantic', artifact: 'backend-test.json', testNames: ['TestHUBUSG004UnknownPartialDoNotFabricateCost'], required: true },
  { id: 'CAP-C5-010', name: 'Pricing known', artifact: 'backend-test.json', testNames: ['TestHUBI5PricingUsesSpecificEffectiveRuleAndDecimalArithmetic'], required: true },
  { id: 'CAP-C5-011', name: 'Pricing missing', artifact: 'backend-test.json', testNames: ['TestHUBUSG006MissingMeterOrPriceGivesUnknownCost'], required: true },
  { id: 'CAP-C5-012', name: 'Effective pricing', artifact: 'backend-test.json', testNames: ['TestHUBI5PricingUsesSpecificEffectiveRuleAndDecimalArithmetic'], required: true },
  { id: 'CAP-C5-013', name: 'Filter time/user/resource', artifact: 'console-test.json', testNames: ['console/src/pages/UsagePage.test.ts'], required: true },
  { id: 'CAP-C5-014', name: 'Filter kind/upstream/status/completeness', artifact: 'console-test.json', testNames: ['console/src/pages/UsagePage.test.ts'], required: true },
  { id: 'CAP-C5-015', name: 'Request detail safety', artifact: 'backend-test.json', testNames: ['TestHUBUSG004UnknownPartialDoNotFabricateCost'], required: true },
  // C5 — Overview/System
  { id: 'CAP-C5-020', name: 'Hub/Relay converged', artifact: 'console-test.json', testNames: ['console/src/pages/OverviewPage.test.ts'], required: true },
  { id: 'CAP-C5-021', name: 'Relay out of sync', artifact: 'console-test.json', testNames: ['console/src/pages/SystemPage.test.ts'], required: true },
  { id: 'CAP-C5-022', name: 'Activation applying/failed', artifact: 'console-test.json', testNames: ['console/src/pages/OverviewPage.test.ts'], required: true },
  { id: 'CAP-C5-023', name: 'Upstream degraded', artifact: 'console-test.json', testNames: ['console/src/pages/UpstreamsPage.test.ts'], required: true },
  { id: 'CAP-C5-024', name: 'Usage lag/backlog', artifact: 'console-test.json', testNames: ['console/src/pages/SystemPage.test.ts'], required: true },
  { id: 'CAP-C5-025', name: 'Semantic orphan/unknown', artifact: 'console-test.json', testNames: ['console/src/pages/SystemPage.test.ts'], required: true },
  // C6 — Product/System (require candidate build tag execution)
  { id: 'CAP-C6-001', name: 'Golden Path', artifact: 'candidate-test.json', testNames: ['TestCAPC6001GoldenPath'], required: true },
  { id: 'CAP-C6-002', name: 'Test Client Four Capabilities', artifact: 'candidate-test.json', testNames: ['TestCAPC6002TestClientFourCapabilities'], required: true },
  { id: 'CAP-C6-003', name: 'Usage Closure', artifact: 'candidate-test.json', testNames: ['TestCAPC6003UsageClosure'], required: true },
  { id: 'CAP-C6-004', name: 'Publish New Generation', artifact: 'candidate-test.json', testNames: ['TestCAPC6004PublishNewGeneration'], required: true },
  { id: 'CAP-C6-004-ENH', name: 'Enhanced No-Forward + Usage Generation', artifact: 'candidate-test.json', testNames: ['TestCAPC6004EnhancedNoForwardAndUsageGeneration'], required: true },
  { id: 'CAP-C6-010', name: 'Hub Crash Around Publish', artifact: 'candidate-test.json', testNames: ['TestCAPC6010HubCrashAroundPublish'], required: true },
  { id: 'CAP-C6-011', name: 'Relay Restart Recovery', artifact: 'candidate-test.json', testNames: ['TestCAPC6011RelayRestart'], required: true },
  { id: 'CAP-C6-012', name: 'Refresh During Activation', artifact: 'candidate-test.json', testNames: ['TestCAPC6012RefreshDuringActivation'], required: true },
  { id: 'CAP-C6-013', name: 'SQLite Busy/Transient', artifact: 'candidate-test.json', testNames: ['TestCAPC6013SQLiteBusyTransient'], required: true },
  { id: 'CAP-C6-014', name: 'Full Restart Recovery', artifact: 'candidate-test.json', testNames: ['TestCAPC6014FullRestart'], required: true },
  { id: 'CAP-C6-015', name: 'Backup/Restore', artifact: 'candidate-test.json', testNames: ['TestCAPC6015BackupRestore'], required: true },
  // Security (require candidate build tag execution)
  { id: 'CAP-SEC-001', name: 'Unauthenticated Access Denied', artifact: 'candidate-test.json', testNames: ['TestCAPSEC001UnauthenticatedAccessDenied'], required: true },
  { id: 'CAP-SEC-002', name: 'CSRF Enforced', artifact: 'candidate-test.json', testNames: ['TestCAPSEC002CSRFEnforced'], required: true },
  { id: 'CAP-SEC-003', name: 'Session Cookie Attributes', artifact: 'candidate-test.json', testNames: ['TestCAPSEC003SessionCookieAttributes'], required: true },
  { id: 'CAP-SEC-004', name: 'Snapshot No Server Fields Leak', artifact: 'candidate-test.json', testNames: ['TestCAPSEC004SnapshotNoServerFieldsLeak'], required: true },
  { id: 'CAP-SEC-005', name: 'Usage Detail No Content Leak', artifact: 'candidate-test.json', testNames: ['TestCAPSEC005UsageDetailNoContentLeak'], required: true },
  { id: 'CAP-SEC-006', name: 'Invalid Enrollment Rejected', artifact: 'candidate-test.json', testNames: ['TestCAPSEC006InvalidEnrollmentRejected'], required: true },
  { id: 'CAP-SEC-007', name: 'Invalid Access Token Rejected', artifact: 'candidate-test.json', testNames: ['TestCAPSEC007InvalidAccessTokenRejected'], required: true },
  { id: 'CAP-SEC-008', name: 'Strict JSON Validation', artifact: 'candidate-test.json', testNames: ['TestCAPSEC008StrictJSONValidation'], required: true },
  { id: 'CAP-SEC-009', name: 'Client Header Spoof Stripped', artifact: 'candidate-test.json', testNames: ['TestCAPSEC009ClientHeaderSpoofStripped'], required: true },
  { id: 'CAP-SEC-010', name: 'Secret Value Never Returned', artifact: 'candidate-test.json', testNames: ['TestCAPSEC010SecretValueNeverReturned'], required: true },
  { id: 'CAP-SEC-011', name: 'Logout Invalidates Session', artifact: 'candidate-test.json', testNames: ['TestCAPSEC011LogoutInvalidatesSession'], required: true },
  { id: 'CAP-SEC-012', name: 'Client API Auth Enforced', artifact: 'candidate-test.json', testNames: ['TestCAPSEC012ClientAPIAuthEnforced'], required: true },
  { id: 'CAP-SEC-013', name: 'Request Body Size Limit', artifact: 'candidate-test.json', testNames: ['TestCAPSEC013RequestBodySizeLimit'], required: true },
  { id: 'CAP-SEC-014', name: 'System Status No Internal Config', artifact: 'candidate-test.json', testNames: ['TestCAPSEC014SystemStatusNoInternalConfig'], required: true },
  { id: 'CAP-SEC-015', name: 'Idempotent Publish', artifact: 'candidate-test.json', testNames: ['TestCAPSEC015IdempotentPublish'], required: true },
  { id: 'CAP-SEC-016', name: 'Expired/Wrong-Claim JWT Rejected', artifact: 'candidate-test.json', testNames: ['TestCAPSEC016ExpiredWrongClaimJWTRejected'], required: true },
  { id: 'CAP-SEC-017', name: 'Disabled User Rejected', artifact: 'candidate-test.json', testNames: ['TestCAPSEC017DisabledUserRejected'], required: true },
  { id: 'CAP-SEC-018', name: 'Revoked Device Rejected', artifact: 'candidate-test.json', testNames: ['TestCAPSEC018RevokedDeviceRejected'], required: true },
  { id: 'CAP-SEC-019', name: 'Management Endpoint Not Reachable', artifact: 'candidate-test.json', testNames: ['TestCAPSEC019ManagementEndpointNotReachable'], required: true },
  { id: 'CAP-SEC-020', name: 'Upstream Set-Cookie/Redirect Stripped', artifact: 'candidate-test.json', testNames: ['TestCAPSEC020UpstreamSetCookieRedirectStripped'], required: true },
  { id: 'CAP-SEC-021', name: 'Snapshot Preview No Server Fields Leak', artifact: 'candidate-test.json', testNames: ['TestCAPSEC021SnapshotPreviewNoServerFieldsLeak'], required: true },
  // C7 — Freeze Gate (self-referential: excluded from pre-flight required-PASS check)
  { id: 'CAP-C7-001', name: 'Freeze Manifest Generated', artifact: null, testNames: [], required: false },
  { id: 'CAP-C7-002', name: 'Clean Replay', artifact: null, testNames: [], required: false },
  // Baseline
  { id: 'BASELINE', name: 'Resource Baseline', artifact: 'resource-baseline.json', testNames: [], required: false },
]

// --- Compile scenario results from artifacts ---

function compileScenarioResults() {
  const artifacts = {
    'backend-test.json': loadGoTestResults('backend-test.json'),
    'system-test.json': loadGoTestResults('system-test.json'),
    'console-test.json': loadVitestResults('console-test.json'),
    'candidate-test.json': loadGoTestResults('candidate-test.json'),
    'static-contract.json': loadJsonArtifact('static-contract.json'),
    'resource-baseline.json': loadJsonArtifact('resource-baseline.json'),
  }

  return scenarioDefinitions.map(scenario => {
    let result = 'NOT_EXECUTED'

    if (scenario.id === 'CAP-C0-009') {
      const a = artifacts['static-contract.json']
      if (a && a.codegenDrift === 'PASS') result = 'PASS'
      else if (a && a.codegenDrift === 'FAIL') result = 'FAIL'
    } else if (scenario.id === 'BASELINE') {
      const a = artifacts['resource-baseline.json']
      if (a && a.status === 'GREEN') result = 'PASS'
      else if (a && a.status === 'NOT_GREEN') result = 'FAIL'
    } else if (scenario.artifact && scenario.testNames.length > 0) {
      const artifact = artifacts[scenario.artifact]
      if (artifact instanceof Map) {
        // Go test or vitest results
        const results = scenario.testNames.map(tn => artifact.get(tn))
        if (results.every(r => r === 'PASS')) result = 'PASS'
        else if (results.some(r => r === 'FAIL')) result = 'FAIL'
        // else NOT_EXECUTED (artifact missing or test not found)
      }
    }

    return {
      id: scenario.id,
      name: scenario.name,
      artifact: scenario.artifact,
      testNames: scenario.testNames,
      required: scenario.required,
      result,
    }
  })
}

// --- Pre-flight checks ---

const errors = []
const warnings = []

// 1. Working tree must be clean
if (gitDirty(ROOT)) {
  errors.push('Working tree is dirty. Commit or stash changes before generating freeze manifest.')
}

// 2. Admin build must exist
const buildHash = adminBuildHash()
if (buildHash === 'not-built') {
  errors.push('Admin production build not found. Run "make console-build" before generating freeze manifest.')
}

// 3. Compile scenario results from artifacts
const scenarioResults = compileScenarioResults()

// 4. All required scenarios (excluding C7 self-referential and BASELINE) must be PASS
const notPassRequired = scenarioResults.filter(s =>
  s.required && s.result !== 'PASS' && !s.id.startsWith('CAP-C7-'))
if (notPassRequired.length > 0) {
  errors.push(`${notPassRequired.length} required scenarios are not PASS:`)
  for (const s of notPassRequired) {
    errors.push(`  ${s.id} ${s.name}: ${s.result}`)
  }
}

// 5. Real adapter qualification must be executed
const realAdapterArtifact = loadJsonArtifact('real-adapter-qualification.json')
let realAdapterStatus = 'NOT_EXECUTED'
if (realAdapterArtifact) {
  realAdapterStatus = realAdapterArtifact.status || 'NOT_EXECUTED'
}
if (realAdapterStatus === 'NOT_EXECUTED') {
  errors.push('Real adapter qualification has not been executed. See docs/s0-real-adapter-qualification.md')
}

// 6. Resource baseline must be GREEN
const baselineArtifact = loadJsonArtifact('resource-baseline.json')
let resourceBaselineStatus = 'NOT_GREEN'
if (baselineArtifact) {
  resourceBaselineStatus = baselineArtifact.status || 'NOT_GREEN'
}
if (resourceBaselineStatus !== 'GREEN') {
  errors.push(`Resource baseline is ${resourceBaselineStatus}. See docs/s0-resource-baseline.md`)
}

// 7. Check for artifact/commit mismatch
const currentCommit = gitCommit(ROOT)
for (const artifactName of ['backend-test.json', 'system-test.json', 'console-test.json', 'candidate-test.json']) {
  const artifact = loadJsonArtifact(artifactName)
  if (artifact && artifact.commit && artifact.commit !== currentCommit) {
    errors.push(`Artifact ${artifactName} was generated for commit ${artifact.commit} but current commit is ${currentCommit}`)
  }
}

if (errors.length > 0) {
  console.error('ERROR: Freeze manifest cannot be generated:')
  for (const e of errors) {
    console.error(`  ${e}`)
  }
  console.error('')
  console.error('Fix the above issues before running this script.')
  process.exit(1)
}

// --- Generate manifest ---

const now = new Date().toISOString()

const manifest = {
  manifest: 'measix-s0-client-contract-freeze',
  snapshotSchemaVersion: SNAPSHOT_SCHEMA_VERSION,
  architectureCommit: gitCommit(ARCH_REPO),
  architectureRepoDirty: gitDirty(ARCH_REPO),
  platformCoreCommit: currentCommit,
  workingTreeDirty: gitDirty(ROOT),
  adminBuildHash: buildHash,
  clientControlOpenApiHash: sha256(CLIENT_OPENAPI),
  adminOpenApiHash: sha256(ADMIN_OPENAPI),
  canonicalFixtureHash: fixturesHash(),
  deterministicAdapterVersion: deterministicAdapterVersion(),
  realAdapterQualificationRef: 'docs/s0-real-adapter-qualification.md',
  realAdapterQualificationStatus: realAdapterStatus,
  resourceBaselineRef: 'docs/s0-resource-baseline.md',
  resourceBaselineStatus,
  scenarioResults,
  startedAt: now,
  completedAt: now,
}

const outPath = join(ROOT, 'docs', 's0-freeze-manifest.json')
writeFileSync(outPath, JSON.stringify(manifest, null, 2) + '\n')
process.stdout.write(`wrote ${relative(ROOT, outPath)}\n`)
process.stdout.write(JSON.stringify(manifest, null, 2) + '\n')
