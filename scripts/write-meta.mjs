#!/usr/bin/env node
/**
 * Write a .meta.json provenance envelope for an artifact.
 *
 * Usage:
 *   node scripts/write-meta.mjs <artifact-file> <command> <exit-code>
 *
 * This replaces the inline `node -e "..."` commands in the Makefile
 * that were duplicated 4 times with identical logic.
 */
import { join } from 'node:path'
import { resolveRoot, writeMetaJson } from './lib/harness.mjs'

const ROOT = resolveRoot(import.meta.dirname)
const ARTIFACTS_DIR = join(ROOT, '.artifacts')
const ARCH_REPO = join(ROOT, '..', 'measix-architecture')

const [artifactFile, command, exitCodeStr] = process.argv.slice(2)
const exitCode = parseInt(exitCodeStr, 10)

if (!artifactFile || !command || isNaN(exitCode)) {
  console.error('Usage: node scripts/write-meta.mjs <artifact-file> <command> <exit-code>')
  process.exit(1)
}

writeMetaJson(ARTIFACTS_DIR, artifactFile, ROOT, ARCH_REPO, command, exitCode)
