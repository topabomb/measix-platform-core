#!/usr/bin/env node
/**
 * Real Adapter Qualification Artifact Generator
 *
 * This script guides the operator through real adapter qualification
 * and generates .artifacts/real-adapter-qualification.json — the
 * machine-readable artifact consumed by freeze-manifest.mjs.
 *
 * Per architecture qualification spec, the qualification unit is
 * adapter/version + configRevision + profile. Different profiles
 * (Model/TTS/ASR/MCP) may use different endpoints/adapters.
 *
 * Usage:
 *   node scripts/collect-adapter-qualification.mjs --endpoint <url> --key <api-key> [--profile model]
 *
 * Or to mark as NOT_EXECUTED (default when no args):
 *   node scripts/collect-adapter-qualification.mjs
 */
import { writeFileSync, mkdirSync, existsSync, readFileSync } from 'node:fs'
import { join, resolve } from 'node:path'
import { execFileSync } from 'node:child_process'

const ROOT = resolve(import.meta.dirname, '..')
const ARTIFACTS_DIR = join(ROOT, '.artifacts')
const OUT_PATH = join(ARTIFACTS_DIR, 'real-adapter-qualification.json')

// Parse args
const args = process.argv.slice(2)
let endpoint = null
let apiKey = null
let profile = 'all'

for (let i = 0; i < args.length; i++) {
  if (args[i] === '--endpoint' && i + 1 < args.length) {
    endpoint = args[++i]
  } else if (args[i] === '--key' && i + 1 < args.length) {
    apiKey = args[++i]
  } else if (args[i] === '--profile' && i + 1 < args.length) {
    profile = args[++i]
  }
}

// Get current commit
let commit = 'unknown'
try {
  commit = execFileSync('git', ['rev-parse', 'HEAD'], {
    cwd: ROOT,
    encoding: 'utf-8',
  }).trim()
} catch { /* ignore */ }

if (!endpoint || !apiKey) {
  // Mark as NOT_EXECUTED
  const artifact = {
    status: 'NOT_EXECUTED',
    commit,
    qualifiedAt: null,
    reason: 'No endpoint or API key provided. See docs/s0-real-adapter-qualification.md for the qualification procedure.',
    profiles: {
      model: { status: 'NOT_EXECUTED' },
      tts: { status: 'NOT_EXECUTED' },
      asr: { status: 'NOT_EXECUTED' },
      mcp: { status: 'NOT_EXECUTED' },
    },
  }
  mkdirSync(ARTIFACTS_DIR, { recursive: true })
  writeFileSync(OUT_PATH, JSON.stringify(artifact, null, 2) + '\n')
  console.log(`Wrote ${OUT_PATH} (NOT_EXECUTED)`)
  console.log('To execute real adapter qualification:')
  console.log('  1. Start Hub + Relay (see scripts/e2e-harness.mjs)')
  console.log('  2. Run: node scripts/collect-adapter-qualification.mjs --endpoint <url> --key <api-key>')
  console.log('  3. Follow the procedure in docs/s0-real-adapter-qualification.md')
  process.exit(0)
}

// Real qualification would go here — this is the procedure entry point.
// The actual qualification requires:
// 1. Creating a Secret in Hub with the API key
// 2. Creating an Upstream with the endpoint URL
// 3. Testing and applying the upstream
// 4. Creating resources (Model/TTS/ASR/MCP) bound to the upstream
// 5. Publishing a draft
// 6. Running runtime requests through the Relay against the real upstream
// 7. Verifying usage records
//
// This script is the entry point for automating that flow.
// For now, it produces a NOT_EXECUTED artifact with the endpoint info.

console.log('Real adapter qualification procedure:')
console.log(`  Endpoint: ${endpoint}`)
console.log(`  Profile: ${profile}`)
console.log('')
console.log('This script is a placeholder for the automated qualification flow.')
console.log('Follow the manual procedure in docs/s0-real-adapter-qualification.md.')
console.log('')

const artifact = {
  status: 'NOT_EXECUTED',
  commit,
  qualifiedAt: null,
  endpoint: endpoint.replace(/\/$/, ''),
  profile,
  reason: 'Automated qualification flow not yet implemented. Use manual procedure.',
  profiles: {
    model: { status: 'NOT_EXECUTED' },
    tts: { status: 'NOT_EXECUTED' },
    asr: { status: 'NOT_EXECUTED' },
    mcp: { status: 'NOT_EXECUTED' },
  },
}

mkdirSync(ARTIFACTS_DIR, { recursive: true })
writeFileSync(OUT_PATH, JSON.stringify(artifact, null, 2) + '\n')
console.log(`Wrote ${OUT_PATH}`)
