import { test } from 'node:test'
import assert from 'node:assert/strict'
import fs, { readFileSync, readdirSync } from 'node:fs'
import { syncBuiltinESMExports } from 'node:module'
import { compileScenarioResults, scenarioResultErrors } from './freeze-manifest.mjs'
import { resolve } from 'node:path'

test('required backend evidence selectors name existing tests', () => {
  const root = resolve(import.meta.dirname, '..')
  const walk = path => readdirSync(path, {withFileTypes:true}).flatMap(entry => {
    const child=resolve(path,entry.name)
    return entry.isDirectory() ? walk(child) : entry.name.endsWith('_test.go') ? [child] : []
  })
  const tests=new Set(walk(resolve(root,'backend')).flatMap(path => [...readFileSync(path,'utf8').matchAll(/func (Test\w+)\(/g)].map(m=>m[1])))
  const scenarios=JSON.parse(readFileSync(resolve(root,'scripts/scenario-definitions.json'),'utf8'))
  const missing=scenarios.filter(s=>s.required && s.artifact==='backend-test.json').flatMap(s=>(s.testNames??[]).filter(name=>!tests.has(name.split('/')[0])).map(name=>s.id+': '+name))
  assert.deepEqual(missing,[])
})

test('CAP usage evidence requires both current browser scenarios and rejects missing or failed results', t => {
  const root = resolve(import.meta.dirname, '..')
  const artifact = resolve(root, '.artifacts/e2e-playwright.json')
  const files = ['golden-path-authoring.spec.ts', 'golden-path-usage.spec.ts']
  const titles = files.map(file => {
    const source = readFileSync(resolve(root, 'console/e2e', file), 'utf8')
    const title = [...source.matchAll(/test\('([^']+)'/g)].map(match => match[1]).find(title => title.startsWith('CAP-C6-001-'))
    assert.ok(title, file)
    return title
  })
  let results = new Map(titles.map(title => [title, 'passed']))
  const originalRead = fs.readFileSync
  const originalExists = fs.existsSync
  t.mock.method(fs, 'existsSync', path => resolve(String(path)) === artifact || originalExists(path))
  t.mock.method(fs, 'readFileSync', (path, ...args) => resolve(String(path)) === artifact
    ? JSON.stringify({ suites: [{ specs: [...results].map(([title, status]) => ({ title, tests: [{ results: [{ status }] }] })) }] })
    : originalRead(path, ...args))
  syncBuiltinESMExports()
  const evaluate = () => compileScenarioResults().find(row => row.id === 'CAP-C6-001')
  try {
    assert.equal(evaluate().result, 'PASS')
    for (const title of titles) {
      results = new Map(titles.map(title => [title, 'passed']))
      results.delete(title)
      assert.equal(evaluate().result, 'NOT_EXECUTED', title)
      assert.ok(scenarioResultErrors([evaluate()]).length > 0)
      results.set(title, 'failed')
      assert.equal(evaluate().result, 'FAIL', title)
      assert.ok(scenarioResultErrors([evaluate()]).length > 0)
    }
  } finally {
    t.mock.restoreAll()
    syncBuiltinESMExports()
  }
})

test('CAP current wire gate requires every backend result including v5 opening isolation', t => {
  const root = resolve(import.meta.dirname, '..')
  const scenarios = JSON.parse(readFileSync(resolve(root, 'scripts/scenario-definitions.json'), 'utf8'))
  const definition = scenarios.find(s => s.id === 'CAP-C0-010')
  const artifact = resolve(root, '.artifacts/backend-test.json')
  const originalRead = fs.readFileSync
  const originalExists = fs.existsSync
  let results = new Map(definition.testNames.map(name => [name, 'pass']))
  t.mock.method(fs, 'existsSync', path => resolve(String(path)) === artifact || originalExists(path))
  t.mock.method(fs, 'readFileSync', (path, ...args) => resolve(String(path)) === artifact
    ? [...results].map(([Test, Action]) => JSON.stringify({ Test, Action })).join('\n')
    : originalRead(path, ...args))
  syncBuiltinESMExports()
  const evaluate = () => compileScenarioResults().find(row => row.id === definition.id)
  try {
    assert.equal(evaluate().result, 'PASS')
    assert.deepEqual(scenarioResultErrors([evaluate()]), [])
    // The explicit v5 case also detects accidentally removing its evidence selector.
    for (const missing of new Set([...definition.testNames, 'TestStarterV5StrictWireAndV4Isolation'])) {
      results = new Map(definition.testNames.map(name => [name, 'pass']))
      results.delete(missing)
      assert.equal(evaluate().result, 'NOT_EXECUTED', missing)
      assert.ok(scenarioResultErrors([evaluate()]).length > 0, missing)
      results.set(missing, 'fail')
      assert.equal(evaluate().result, 'FAIL', missing)
      assert.ok(scenarioResultErrors([evaluate()]).length > 0, missing)
    }
  } finally {
    t.mock.restoreAll()
    syncBuiltinESMExports()
  }
})
