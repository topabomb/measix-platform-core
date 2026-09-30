import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync, readdirSync, existsSync } from 'node:fs'
import { join, resolve } from 'node:path'

const ROOT = resolve(import.meta.dirname, '..')

// Which Go package registers the flags for each command. Two commands delegate
// flag parsing to a config package, and control-hub's subcommands register their
// own flags, so the source of truth differs per invocation.
const FLAG_SOURCES = {
  'control-hub:run': ['backend/internal/hub/config'],
  'control-hub:bootstrap-admin': ['backend/cmd/control-hub'],
  'control-hub': ['backend/cmd/control-hub', 'backend/internal/hub/config'],
  'runtime-relay': ['backend/cmd/runtime-relay', 'backend/internal/relay/config'],
  'device-demo': ['backend/cmd/device-demo'],
  'devmigrate': ['backend/cmd/devmigrate'],
}

const FLAG_STATEMENTS = [
  /\.(?:String|Bool|Int|Int64|Uint|Duration|Float64)Var\(\s*&[\w.]+,\s*"([^"]+)"/g,
  /\.(?:String|Bool|Int|Int64|Uint|Duration|Float64)\(\s*"([^"]+)"/g,
]

function registeredFlags(key) {
  const dirs = FLAG_SOURCES[key]
  assert.ok(dirs, `no flag source declared for ${key}; add it to FLAG_SOURCES`)
  const flags = new Set()
  for (const dir of dirs) {
    for (const file of readdirSync(join(ROOT, dir)).filter(name => name.endsWith('.go') && !name.endsWith('_test.go'))) {
      const text = readFileSync(join(ROOT, dir, file), 'utf8')
      for (const pattern of FLAG_STATEMENTS) {
        for (const match of text.matchAll(pattern)) flags.add(match[1])
      }
    }
  }
  return flags
}

/** Split "pkg subcommand --flag" into its parts. */
function parseInvocation(text, where) {
  const command = /go run \.\/cmd\/([\w-]+)/.exec(text)
  if (!command) return null
  const rest = text.slice(command.index + command[0].length)
  const sub = /^\s+([a-z][\w-]*)(?=\s|$)/.exec(rest)
  const subcommand = sub ? sub[1] : null
  const flags = [...rest.matchAll(/--([a-z][\w-]*)/g)].map(m => m[1])
  return { pkg: command[1], subcommand, flags, where }
}

function invocations() {
  const found = []

  const scripts = JSON.parse(readFileSync(join(ROOT, 'package.json'), 'utf8')).scripts ?? {}
  for (const [name, value] of Object.entries(scripts)) {
    const parsed = parseInvocation(value, `package.json script ${name}`)
    if (parsed) found.push(parsed)
  }

  for (const line of readFileSync(join(ROOT, 'Makefile'), 'utf8').split(/\r?\n/)) {
    const parsed = parseInvocation(line, 'Makefile')
    if (parsed) found.push(parsed)
  }

  for (const file of readdirSync(join(ROOT, 'scripts')).filter(name => name.endsWith('.ps1'))) {
    const lines = readFileSync(join(ROOT, 'scripts', file), 'utf8').split(/\r?\n/)
    lines.forEach((line, index) => {
      const parsed = parseInvocation(line, `scripts/${file}:${index + 1}`)
      if (parsed) found.push(parsed)
    })
    // Arguments handed to a built binary through an `$args = @( ... )` block.
    const start = lines.findIndex(line => /\$args\s*=\s*@\(/.test(line))
    if (start >= 0) {
      let text = lines[start]
      for (let i = start + 1; i < lines.length && !/^\s*\)\s*$/.test(lines[i]); i++) text += ' ' + lines[i]
      found.push({
        pkg: 'device-demo',
        subcommand: null,
        flags: [...text.matchAll(/--([a-z][\w-]*)/g)].map(m => m[1]),
        where: `scripts/${file}:${start + 1} device-demo arguments`,
      })
    }
  }
  return found
}

const found = invocations()

test('the checker actually sees the repository command lines', () => {
  // Without this, a broken extractor would make the contract test pass while
  // checking nothing at all.
  assert.ok(found.length >= 5, `expected several command invocations, found ${found.length}`)
  const packages = new Set(found.map(item => item.pkg))
  for (const expected of ['control-hub', 'runtime-relay', 'devmigrate', 'device-demo']) {
    assert.ok(packages.has(expected), `no invocation found for ./cmd/${expected}`)
  }
  const totalFlags = found.reduce((sum, item) => sum + item.flags.length, 0)
  assert.ok(totalFlags >= 15, `expected many flags across invocations, found ${totalFlags}`)
})

test('every command referenced by repository tooling exists', () => {
  const missing = [...new Set(found.map(item => item.pkg))]
    .filter(pkg => !existsSync(join(ROOT, 'backend', 'cmd', pkg)))
  assert.deepEqual(missing, [])
})

test('every flag passed by repository tooling is registered by its command', () => {
  const problems = []
  for (const { pkg, subcommand, flags, where } of found) {
    const key = subcommand && FLAG_SOURCES[`${pkg}:${subcommand}`] ? `${pkg}:${subcommand}` : pkg
    const registered = registeredFlags(key)
    for (const flag of flags) {
      if (!registered.has(flag)) problems.push(`${where}: ./cmd/${pkg} does not define --${flag}`)
    }
  }
  assert.deepEqual(problems, [])
})
