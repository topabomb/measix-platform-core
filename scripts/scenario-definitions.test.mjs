import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync, readdirSync } from 'node:fs'
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
