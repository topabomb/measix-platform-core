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
