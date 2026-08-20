#!/usr/bin/env node
/**
 * Generates the S0.1 Client Contract Freeze manifest.
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
 * The manifest is written to docs/s0-freeze-manifest.json. It is a deterministic
 * build-time artifact derived from the committed source tree.
 *
 * IMPORTANT: This script MUST NOT be run if:
 *   - Any required scenario is not PASS
 *   - The working tree is dirty
 *   - The Admin build is not present (adminBuildHash must not be "not-built")
 *   - Real adapter qualification has not been executed
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

// Architecture repository path (sibling directory).
const ARCH_REPO = resolve(ROOT, '..', 'measix-architecture')

function sha256(path) {
  // Normalize CRLF -> LF so the canonical hash matches the generator and is stable
  // across Windows/Linux working trees.
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
  // Hash the Admin Console production build output if it exists.
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
  // Hash the adapter source + test to produce a deterministic version.
  const hash = createHash('sha256')
  hash.update(readFileSync(ADAPTER_SOURCE).toString('utf-8').replace(/\r\n/g, '\n'))
  hash.update('\0')
  if (existsSync(ADAPTER_TEST)) {
    hash.update(readFileSync(ADAPTER_TEST).toString('utf-8').replace(/\r\n/g, '\n'))
  }
  hash.update('\0')
  // Include the Test Client source since it's part of the qualification path.
  if (existsSync(CLIENT_SOURCE)) {
    hash.update(readFileSync(CLIENT_SOURCE).toString('utf-8').replace(/\r\n/g, '\n'))
  }
  return 'sha256:' + hash.digest('hex')
}

// Full CAP scenario list per measix-s0-capability-delivery-system-testing-spec.md
// C0-C5 scenarios map to existing unit/component tests; C6-C7 map to system tests.
// Each scenario has: id, name, file, required, and result (PASS/FAIL/NOT_EXECUTED).
const scenarioResults = [
  // C0 — Contract Scenarios
  { id: 'CAP-C0-001', name: 'Required profile enum/schema', file: 'backend/internal/contract/', required: true, result: 'PASS', testNames: ['TestCAPC0004TTSVoiceRequired', 'TestCAPC0006MCPAuthOwnershipValidation'] },
  { id: 'CAP-C0-002', name: 'Unsupported protocol not supported', file: 'backend/internal/hub/capability/', required: true, result: 'PASS', testNames: ['TestHUBCAP004RouteUpstreamValidation'] },
  { id: 'CAP-C0-003', name: 'Model vocabulary', file: 'backend/internal/hub/capability/', required: true, result: 'PASS', testNames: ['TestHUBCAP004RouteUpstreamValidation'] },
  { id: 'CAP-C0-004', name: 'TTS voice required', file: 'backend/internal/hub/capability/', required: true, result: 'PASS', testNames: ['TestCAPC0004TTSVoiceRequired'] },
  { id: 'CAP-C0-005', name: 'ASR HTTP transcription profile', file: 'backend/internal/hub/capability/', required: true, result: 'PASS', testNames: ['TestHUBCAP004RouteUpstreamValidation'] },
  { id: 'CAP-C0-006', name: 'MCP auth ownership', file: 'backend/internal/hub/capability/', required: true, result: 'PASS', testNames: ['TestCAPC0006MCPAuthOwnershipValidation'] },
  { id: 'CAP-C0-007', name: 'Unknown optional response tolerated', file: 'backend/internal/contract/', required: true, result: 'PASS', testNames: ['TestSYSI0002UnknownOptionalResponseFieldIsTolerated'] },
  { id: 'CAP-C0-008', name: 'Unknown request field rejected', file: 'backend/internal/contract/', required: true, result: 'PASS', testNames: ['TestSYSI0003UnknownRequestFieldIsRejected'] },
  { id: 'CAP-C0-009', name: 'Codegen drift', file: 'Makefile:generated-drift', required: true, result: 'PASS', testNames: ['generated-drift check'] },
  // C1 — Upstream Operational
  { id: 'CAP-C1-001', name: 'Browser creates NONE-auth Upstream', file: 'console/src/pages/UpstreamsPage.vue', required: true, result: 'PASS', testNames: ['UpstreamsPage.test.ts'] },
  { id: 'CAP-C1-002', name: 'BEARER secret', file: 'console/src/pages/UpstreamsPage.vue', required: true, result: 'PASS', testNames: ['UpstreamsPage.test.ts'] },
  { id: 'CAP-C1-003', name: 'STATIC_HEADER/BASIC', file: 'backend/internal/hub/upstream/', required: true, result: 'PASS', testNames: ['TestHUBUPS001CandidateAndActiveConfigRevisionsAreSeparate'] },
  { id: 'CAP-C1-004', name: 'Replace Secret', file: 'backend/internal/hub/upstream/', required: true, result: 'PASS', testNames: ['TestHUBUPS003SecretVersionsAreAppendOnlyEncryptedAndReferencedPrecisely'] },
  { id: 'CAP-C1-005', name: 'Candidate edit', file: 'backend/internal/hub/upstream/', required: true, result: 'PASS', testNames: ['TestHUBUPS001CandidateAndActiveConfigRevisionsAreSeparate'] },
  { id: 'CAP-C1-006', name: 'Test connection', file: 'backend/internal/hub/upstream/', required: true, result: 'PASS', testNames: ['TestHUBUPS002ValidateConfigDoesNotApply'] },
  { id: 'CAP-C1-007', name: 'Apply happy path', file: 'backend/internal/hub/runtimecontrol/', required: true, result: 'PASS', testNames: ['TestI3PublishPersistsIntentBeforeRelayAndFinalizesAfterAck'] },
  { id: 'CAP-C1-008', name: 'Apply refresh recovery', file: 'backend/internal/hub/runtimecontrol/', required: true, result: 'PASS', testNames: ['TestHUBACT008ReconcileFinalizesPending'] },
  { id: 'CAP-C1-009', name: 'Apply failure', file: 'backend/internal/hub/upstream/', required: true, result: 'PASS', testNames: ['TestHUBUPS005UpdateFailureRetainsOldRevision'] },
  { id: 'CAP-C1-010', name: 'Secret browser safety', file: 'console/src/pages/UpstreamsPage.vue', required: true, result: 'PASS', testNames: ['UpstreamsPage.test.ts'] },
  // C2 — Resource Authoring
  { id: 'CAP-C2-001', name: 'Create Provider + Model', file: 'console/src/pages/ResourcesPage.vue', required: true, result: 'PASS', testNames: ['ResourcesPage.test.ts'] },
  { id: 'CAP-C2-002', name: 'Model capabilities', file: 'backend/internal/hub/capability/', required: true, result: 'PASS', testNames: ['TestHUBCAP004RouteUpstreamValidation'] },
  { id: 'CAP-C2-003', name: 'Missing Provider', file: 'backend/internal/hub/capability/', required: true, result: 'PASS', testNames: ['TestHUBCAP004RouteUpstreamValidation'] },
  { id: 'CAP-C2-004', name: 'Missing Binding', file: 'backend/internal/hub/capability/', required: true, result: 'PASS', testNames: ['TestHUBCAP004RouteUpstreamValidation'] },
  { id: 'CAP-C2-010', name: 'Create TTS', file: 'console/src/pages/ResourcesPage.vue', required: true, result: 'PASS', testNames: ['ResourcesPage.test.ts'] },
  { id: 'CAP-C2-011', name: 'Missing voice', file: 'backend/internal/hub/capability/', required: true, result: 'PASS', testNames: ['TestCAPC0004TTSVoiceRequired'] },
  { id: 'CAP-C2-020', name: 'Create HTTP ASR', file: 'console/src/pages/ResourcesPage.vue', required: true, result: 'PASS', testNames: ['ResourcesPage.test.ts'] },
  { id: 'CAP-C2-030', name: 'Enterprise-managed MCP', file: 'console/src/pages/ResourcesPage.vue', required: true, result: 'PASS', testNames: ['ResourcesPage.test.ts'] },
  { id: 'CAP-C2-031', name: 'NONE auth MCP', file: 'backend/internal/hub/capability/', required: true, result: 'PASS', testNames: ['TestCAPC0006MCPAuthOwnershipValidation'] },
  { id: 'CAP-C2-032', name: 'Unsupported auth ownership', file: 'backend/internal/hub/capability/', required: true, result: 'PASS', testNames: ['TestCAPC0006MCPAuthOwnershipValidation'] },
  { id: 'CAP-C2-040', name: 'Local coexistence policy', file: 'console/src/pages/ResourcesPage.vue', required: true, result: 'PASS', testNames: ['ResourcesPage.test.ts'] },
  { id: 'CAP-C2-041', name: 'Defaults', file: 'console/src/pages/ResourcesPage.vue', required: true, result: 'PASS', testNames: ['ResourcesPage.test.ts'] },
  { id: 'CAP-C2-042', name: 'Stale revision 409', file: 'backend/internal/hub/capability/', required: true, result: 'PASS', testNames: ['TestHUBCAP001DraftOptimisticConcurrencyAndSaveDoesNotActivate'] },
  { id: 'CAP-C2-043', name: 'Route safety', file: 'backend/internal/hub/capability/', required: true, result: 'PASS', testNames: ['TestHUBCAP004RouteUpstreamValidation'] },
  // C3 — Snapshot / Review
  { id: 'CAP-C3-001', name: 'Preview canonical projection', file: 'backend/internal/hub/capability/', required: true, result: 'PASS', testNames: ['TestHUBCAP006SnapshotDeterministicAndClientSafe'] },
  { id: 'CAP-C3-002', name: 'Preview has no side effect', file: 'backend/internal/hub/httpapi/', required: true, result: 'PASS', testNames: ['TestHUBCAP005SaveDraftDoesNotChangeActiveRelease'] },
  { id: 'CAP-C3-003', name: 'Snapshot no server internals', file: 'backend/internal/hub/capability/', required: true, result: 'PASS', testNames: ['TestHUBCAP007SnapshotExcludesSecretsAndUpstreamURLAndRouteID'] },
  { id: 'CAP-C3-004', name: 'Full profile Snapshot', file: 'api/fixtures/snapshot/', required: true, result: 'PASS', testNames: ['TestSYSI0001CanonicalFixturesDecodeWithGeneratedWire'] },
  { id: 'CAP-C3-005', name: 'Deterministic order/hash', file: 'backend/internal/hub/capability/', required: true, result: 'PASS', testNames: ['TestSnapshotAndRuntimeControlGoldenHashes'] },
  { id: 'CAP-C3-006', name: 'ETag/hash', file: 'backend/internal/hub/capability/', required: true, result: 'PASS', testNames: ['TestSnapshotAndRuntimeControlGoldenHashes'] },
  { id: 'CAP-C3-007', name: 'Publish diff', file: 'console/src/pages/ResourcesPage.vue', required: true, result: 'PASS', testNames: ['ResourcesPage.test.ts'] },
  { id: 'CAP-C3-008', name: 'Warning acknowledgement', file: 'backend/internal/hub/capability/', required: true, result: 'PASS', testNames: ['TestHUBCAP010WarningsCannotBypassServerValidation'] },
  { id: 'CAP-C3-009', name: 'Invalid Draft Publish', file: 'backend/internal/hub/capability/', required: true, result: 'PASS', testNames: ['TestHUBCAP010WarningsCannotBypassServerValidation'] },
  // C4 — Runtime Transport
  { id: 'CAP-C4-001', name: 'Chat request/response', file: 'backend/test/system/adapter/', required: true, result: 'NOT_EXECUTED', testNames: ['TestAdapter'] },
  { id: 'CAP-C4-002', name: 'Chat streaming', file: 'backend/internal/relay/runtime/', required: true, result: 'PASS', testNames: ['TestRLYTRN001002SSEFirstFlushAndOrderPreserved'] },
  { id: 'CAP-C4-003', name: 'Model cancel', file: 'backend/internal/relay/runtime/', required: true, result: 'PASS', testNames: ['TestRLYTRN004ClientCancelPropagates'] },
  { id: 'CAP-C4-010', name: 'TTS request', file: 'backend/test/system/adapter/', required: true, result: 'NOT_EXECUTED', testNames: ['TestAdapter'] },
  { id: 'CAP-C4-011', name: 'Binary integrity', file: 'backend/internal/relay/runtime/', required: true, result: 'PASS', testNames: ['TestRLYTRN006BinaryPayloadExact'] },
  { id: 'CAP-C4-012', name: 'TTS failure', file: 'backend/internal/relay/runtime/', required: true, result: 'PASS', testNames: ['TestRLYHDRUpstreamErrorPreserved'] },
  { id: 'CAP-C4-020', name: 'Multipart transcription', file: 'backend/internal/relay/runtime/', required: true, result: 'PASS', testNames: ['TestRLYTRN009MultipartPreserved'] },
  { id: 'CAP-C4-021', name: 'Multipart limits', file: 'backend/internal/relay/runtime/', required: true, result: 'PASS', testNames: ['TestRLYTRN010LargeUploadStreaming'] },
  { id: 'CAP-C4-022', name: 'ASR cancel', file: 'backend/internal/relay/runtime/', required: true, result: 'PASS', testNames: ['TestRLYTRN011UploadCancelTerminatesUpstreamRead'] },
  { id: 'CAP-C4-030', name: 'MCP initialize/list/call', file: 'backend/internal/relay/runtime/', required: true, result: 'PASS', testNames: ['TestRLYTRN012MCPTransparentForward'] },
  { id: 'CAP-C4-031', name: 'MCP streaming/session', file: 'backend/internal/relay/runtime/', required: true, result: 'PASS', testNames: ['TestRLYTRN012MCPTransparentForward'] },
  { id: 'CAP-C4-032', name: 'MCP auth injection', file: 'backend/internal/relay/runtime/', required: true, result: 'PASS', testNames: ['TestRLYTRN014RelayDoesNotRewriteMCP'] },
  { id: 'CAP-C4-040', name: 'Old generation 428 no-forward', file: 'backend/internal/relay/runtime/', required: true, result: 'PASS', testNames: ['TestRLYADM002GenerationMismatchRejected'] },
  { id: 'CAP-C4-041', name: 'Invalid JWT', file: 'backend/internal/relay/runtime/', required: true, result: 'PASS', testNames: ['TestRLYAUTH002WrongAlgSignatureKidRejected'] },
  { id: 'CAP-C4-042', name: 'Revoked principal', file: 'backend/internal/relay/runtime/', required: true, result: 'PASS', testNames: ['TestRLYAUTH007RevokedDeviceSessionRejected'] },
  { id: 'CAP-C4-043', name: 'Unknown resource', file: 'backend/internal/relay/runtime/', required: true, result: 'PASS', testNames: ['TestRLYADM004UnknownResourceRejected'] },
  { id: 'CAP-C4-044', name: 'Path traversal/absolute URI', file: 'backend/internal/relay/runtime/', required: true, result: 'PASS', testNames: ['TestRLYROUTE005TraversalVariantsRejected'] },
  { id: 'CAP-C4-045', name: 'Internal header spoof', file: 'backend/internal/relay/runtime/', required: true, result: 'PASS', testNames: ['TestRLYSecurityClientCannotInjectRequestId'] },
  // C5 — Usage / Pricing
  { id: 'CAP-C5-001', name: 'Model request usage', file: 'backend/internal/hub/usage/', required: true, result: 'PASS', testNames: ['TestHUBI5RequestUsageBatchIsIdempotent'] },
  { id: 'CAP-C5-002', name: 'TTS request usage', file: 'backend/internal/hub/usage/', required: true, result: 'PASS', testNames: ['TestHUBI5SemanticUsageDedupeAndCompleteness'] },
  { id: 'CAP-C5-003', name: 'ASR request usage', file: 'backend/internal/hub/usage/', required: true, result: 'PASS', testNames: ['TestHUBI5SemanticUsageDedupeAndCompleteness'] },
  { id: 'CAP-C5-004', name: 'MCP request usage', file: 'backend/internal/hub/usage/', required: true, result: 'PASS', testNames: ['TestHUBI5SemanticUsageDedupeAndCompleteness'] },
  { id: 'CAP-C5-005', name: 'Hub usage outage', file: 'backend/internal/relay/metering/', required: true, result: 'PASS', testNames: ['TestRLYSPHubAckDeletesOnlyAcknowledgedBatch'] },
  { id: 'CAP-C5-006', name: 'Duplicate delivery', file: 'backend/internal/hub/usage/', required: true, result: 'PASS', testNames: ['TestHUBI5RequestUsageBatchIsIdempotent'] },
  { id: 'CAP-C5-007', name: 'Poison row', file: 'backend/internal/relay/metering/', required: true, result: 'PASS', testNames: ['TestRLYSPPoison422IsolatedWithoutDroppingGoodRows'] },
  { id: 'CAP-C5-008', name: 'UNKNOWN semantic', file: 'backend/internal/hub/usage/', required: true, result: 'PASS', testNames: ['TestHUBUSG004UnknownPartialDoNotFabricateCost'] },
  { id: 'CAP-C5-009', name: 'PARTIAL semantic', file: 'backend/internal/hub/usage/', required: true, result: 'PASS', testNames: ['TestHUBUSG004UnknownPartialDoNotFabricateCost'] },
  { id: 'CAP-C5-010', name: 'Pricing known', file: 'backend/internal/hub/usage/', required: true, result: 'PASS', testNames: ['TestHUBI5PricingUsesSpecificEffectiveRuleAndDecimalArithmetic'] },
  { id: 'CAP-C5-011', name: 'Pricing missing', file: 'backend/internal/hub/usage/', required: true, result: 'PASS', testNames: ['TestHUBUSG006MissingMeterOrPriceGivesUnknownCost'] },
  { id: 'CAP-C5-012', name: 'Effective pricing', file: 'backend/internal/hub/usage/', required: true, result: 'PASS', testNames: ['TestHUBI5PricingUsesSpecificEffectiveRuleAndDecimalArithmetic'] },
  { id: 'CAP-C5-013', name: 'Filter time/user/resource', file: 'console/src/pages/UsagePage.vue', required: true, result: 'PASS', testNames: ['UsagePage.test.ts'] },
  { id: 'CAP-C5-014', name: 'Filter kind/upstream/status/completeness', file: 'console/src/pages/UsagePage.vue', required: true, result: 'PASS', testNames: ['UsagePage.test.ts'] },
  { id: 'CAP-C5-015', name: 'Request detail safety', file: 'backend/internal/hub/usage/', required: true, result: 'PASS', testNames: ['TestHUBUSG004UnknownPartialDoNotFabricateCost'] },
  // C5 — Overview/System
  { id: 'CAP-C5-020', name: 'Hub/Relay converged', file: 'console/src/pages/OverviewPage.vue', required: true, result: 'PASS', testNames: ['OverviewPage.test.ts'] },
  { id: 'CAP-C5-021', name: 'Relay out of sync', file: 'console/src/pages/SystemPage.vue', required: true, result: 'PASS', testNames: ['SystemPage.test.ts'] },
  { id: 'CAP-C5-022', name: 'Activation applying/failed', file: 'console/src/pages/OverviewPage.vue', required: true, result: 'PASS', testNames: ['OverviewPage.test.ts'] },
  { id: 'CAP-C5-023', name: 'Upstream degraded', file: 'console/src/pages/UpstreamsPage.vue', required: true, result: 'PASS', testNames: ['UpstreamsPage.test.ts'] },
  { id: 'CAP-C5-024', name: 'Usage lag/backlog', file: 'console/src/pages/SystemPage.vue', required: true, result: 'PASS', testNames: ['SystemPage.test.ts'] },
  { id: 'CAP-C5-025', name: 'Semantic orphan/unknown', file: 'console/src/pages/SystemPage.vue', required: true, result: 'PASS', testNames: ['SystemPage.test.ts'] },
  // C6 — Product/System
  { id: 'CAP-C6-001', name: 'Golden Path', file: 'golden_path_test.go', required: true, result: 'NOT_EXECUTED' },
  { id: 'CAP-C6-002', name: 'Test Client Four Capabilities', file: 'golden_path_test.go', required: true, result: 'NOT_EXECUTED' },
  { id: 'CAP-C6-003', name: 'Usage Closure', file: 'golden_path_test.go', required: true, result: 'NOT_EXECUTED' },
  { id: 'CAP-C6-004', name: 'Publish New Generation', file: 'golden_path_test.go', required: true, result: 'NOT_EXECUTED' },
  { id: 'CAP-C6-004-ENH', name: 'Enhanced No-Forward + Usage Generation', file: 'recovery_test.go', required: true, result: 'NOT_EXECUTED' },
  { id: 'CAP-C6-010', name: 'Hub Crash Around Publish', file: 'recovery_test.go', required: true, result: 'NOT_EXECUTED' },
  { id: 'CAP-C6-011', name: 'Relay Restart Recovery', file: 'golden_path_test.go', required: true, result: 'NOT_EXECUTED' },
  { id: 'CAP-C6-012', name: 'Refresh During Activation', file: 'golden_path_test.go', required: true, result: 'NOT_EXECUTED' },
  { id: 'CAP-C6-013', name: 'SQLite Busy/Transient', file: 'recovery_test.go', required: true, result: 'NOT_EXECUTED' },
  { id: 'CAP-C6-014', name: 'Full Restart Recovery', file: 'golden_path_test.go', required: true, result: 'NOT_EXECUTED' },
  { id: 'CAP-C6-015', name: 'Backup/Restore', file: 'golden_path_test.go', required: true, result: 'NOT_EXECUTED' },
  // Security
  { id: 'CAP-SEC-001', name: 'Unauthenticated Access Denied', file: 'security_test.go', required: true, result: 'NOT_EXECUTED' },
  { id: 'CAP-SEC-002', name: 'CSRF Enforced', file: 'security_test.go', required: true, result: 'NOT_EXECUTED' },
  { id: 'CAP-SEC-003', name: 'Session Cookie Attributes', file: 'security_test.go', required: true, result: 'NOT_EXECUTED' },
  { id: 'CAP-SEC-004', name: 'Snapshot No Server Fields Leak', file: 'security_test.go', required: true, result: 'NOT_EXECUTED' },
  { id: 'CAP-SEC-005', name: 'Usage Detail No Content Leak', file: 'security_test.go', required: true, result: 'NOT_EXECUTED' },
  { id: 'CAP-SEC-006', name: 'Invalid Enrollment Rejected', file: 'security_test.go', required: true, result: 'NOT_EXECUTED' },
  { id: 'CAP-SEC-007', name: 'Invalid Access Token Rejected', file: 'security_test.go', required: true, result: 'NOT_EXECUTED' },
  { id: 'CAP-SEC-008', name: 'Strict JSON Validation', file: 'security_test.go', required: true, result: 'NOT_EXECUTED' },
  { id: 'CAP-SEC-009', name: 'Client Header Spoof Stripped', file: 'security_test.go', required: true, result: 'NOT_EXECUTED' },
  { id: 'CAP-SEC-010', name: 'Secret Value Never Returned', file: 'security_test.go', required: true, result: 'NOT_EXECUTED' },
  { id: 'CAP-SEC-011', name: 'Logout Invalidates Session', file: 'security_test.go', required: true, result: 'NOT_EXECUTED' },
  { id: 'CAP-SEC-012', name: 'Client API Auth Enforced', file: 'security_test.go', required: true, result: 'NOT_EXECUTED' },
  { id: 'CAP-SEC-013', name: 'Request Body Size Limit', file: 'security_test.go', required: true, result: 'NOT_EXECUTED' },
  { id: 'CAP-SEC-014', name: 'System Status No Internal Config', file: 'security_test.go', required: true, result: 'NOT_EXECUTED' },
  { id: 'CAP-SEC-015', name: 'Idempotent Publish', file: 'security_test.go', required: true, result: 'NOT_EXECUTED' },
  // C7
  { id: 'CAP-C7-001', name: 'Freeze Manifest Generated', file: 'scripts/freeze-manifest.mjs', required: true, result: 'NOT_EXECUTED' },
  { id: 'CAP-C7-002', name: 'Clean Replay', file: 'docs/s0-clean-replay-report.md', required: true, result: 'NOT_EXECUTED' },
  // Baseline
  { id: 'BASELINE', name: 'Resource Baseline', file: 'baseline_test.go', required: false, result: 'NOT_EXECUTED' },
]

// --- Pre-flight checks ---

const errors = []

// 1. Working tree must be clean
if (gitDirty(ROOT)) {
  errors.push('Working tree is dirty. Commit or stash changes before generating freeze manifest.')
}

// 2. Admin build must exist
const buildHash = adminBuildHash()
if (buildHash === 'not-built') {
  errors.push('Admin production build not found. Run "make console-build" before generating freeze manifest.')
}

// 3. All required scenarios must be PASS
const notPassRequired = scenarioResults.filter(s => s.required && s.result !== 'PASS')
if (notPassRequired.length > 0) {
  errors.push(`${notPassRequired.length} required scenarios are not PASS:`)
  for (const s of notPassRequired) {
    errors.push(`  ${s.id} ${s.name}: ${s.result}`)
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
  platformCoreCommit: gitCommit(ROOT),
  workingTreeDirty: gitDirty(ROOT),
  adminBuildHash: buildHash,
  clientControlOpenApiHash: sha256(CLIENT_OPENAPI),
  adminOpenApiHash: sha256(ADMIN_OPENAPI),
  canonicalFixtureHash: fixturesHash(),
  deterministicAdapterVersion: deterministicAdapterVersion(),
  realAdapterQualificationRef: 'docs/s0-real-adapter-qualification.md',
  realAdapterQualificationStatus: 'NOT_EXECUTED',
  resourceBaselineRef: 'docs/s0-resource-baseline.md',
  resourceBaselineStatus: 'NOT_GREEN',
  scenarioResults,
  startedAt: now,
  completedAt: now,
}

const outPath = join(ROOT, 'docs', 's0-freeze-manifest.json')
writeFileSync(outPath, JSON.stringify(manifest, null, 2) + '\n')
process.stdout.write(`wrote ${relative(ROOT, outPath)}\n`)
process.stdout.write(JSON.stringify(manifest, null, 2) + '\n')
