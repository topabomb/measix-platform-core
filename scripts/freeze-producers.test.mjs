import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync, readdirSync } from 'node:fs'
import { join, resolve } from 'node:path'
import { artifactNames } from './freeze-manifest.mjs'

const ROOT = resolve(import.meta.dirname, '..')
const META_WRITERS = ['write-meta.mjs', 'writeMetaJson']

/** A place in the repository tooling that can write an artifact. */
function producerSites() {
  const sites = []

  const makefile = readFileSync(join(ROOT, 'Makefile'), 'utf8')
  let current = null
  for (const line of makefile.split(/\r?\n/)) {
    const target = /^([A-Za-z][\w-]*):/.exec(line)
    if (target) {
      current = { where: `Makefile target ${target[1]}`, body: line }
      sites.push(current)
      continue
    }
    if (line.startsWith('\t')) { if (current) current.body += '\n' + line; continue }
    if (line.trim() === '') continue
    current = null
  }

  for (const file of readdirSync(join(ROOT, 'scripts'))) {
    if (!file.endsWith('.mjs')) continue
    // freeze-manifest.mjs names every artifact because it consumes them, and
    // tests only describe them; neither produces evidence.
    if (file === 'freeze-manifest.mjs' || file.endsWith('.test.mjs')) continue
    sites.push({ where: `scripts/${file}`, body: readFileSync(join(ROOT, 'scripts', file), 'utf8') })
  }

  return sites
}

const sites = producerSites()

test('every required freeze artifact is named by some producer', () => {
  assert.ok(artifactNames.length >= 8, `expected the freeze to require several artifacts, found ${artifactNames.length}`)
  const missing = artifactNames.filter(name => !sites.some(site => site.body.includes(name)))
  assert.deepEqual(missing, [])
})

test('every required freeze artifact has a producer that also writes its metadata', () => {
  // freeze-manifest rejects an artifact without a matching *.meta.json, so a
  // producer that writes only the artifact leaves the freeze unsatisfiable.
  const problems = []
  for (const name of artifactNames) {
    const producers = sites.filter(site => site.body.includes(name))
    if (producers.length === 0) { problems.push(`${name}: no producer`); continue }
    if (!producers.some(site => META_WRITERS.some(writer => site.body.includes(writer)))) {
      problems.push(`${name}: produced by ${producers.map(p => p.where).join(', ')} but no metadata writer`)
    }
  }
  assert.deepEqual(problems, [])
})
