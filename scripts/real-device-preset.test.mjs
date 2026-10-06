import { test } from 'node:test'
import assert from 'node:assert/strict'
import { createServer } from 'node:http'
import { spawn, spawnSync } from 'node:child_process'
import { mkdtempSync, readFileSync, writeFileSync, rmSync, mkdirSync, copyFileSync, existsSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'

const root = resolve(import.meta.dirname, '..')

test('Windows preset launches native process with spaces and quotes intact', { skip: process.platform !== 'win32' }, t => {
  const directory = mkdtempSync(join(tmpdir(), 'measix launch test '))
  t.after(() => rmSync(directory, { recursive: true, force: true }))
  const probe = join(directory, 'argv probe.cjs')
  writeFileSync(probe, 'console.log(JSON.stringify(process.argv.slice(2)))')
  const expected = ['--db', 'J:\\Go Projects\\hub.db', '--assets', 'J:\\Go Projects\\dist\\', 'quoted "value"', '中文目录']
  const psString = value => "'" + value.replaceAll("'", "''") + "'"
  const launch = readFileSync(join(root, 'scripts/start-real-device-preset.ps1'), 'utf8')
    .split(/\r?\n/).filter(line => /^    \$(quotedArgs|process) = /.test(line)).join('\n')
  assert.ok(launch.includes('Start-Process'))
  writeFileSync(join(directory, 'harness.ps1'), '\uFEFF' + [
    "$ErrorActionPreference = 'Stop'",
    '$binaryPath = ' + psString(process.execPath),
    '$repoRoot = ' + psString(directory),
    '$logRoot = $repoRoot',
    '$args = @(' + [probe, ...expected].map(psString).join(', ') + ')',
    launch.replace(' -PassThru', ' -Wait -PassThru'),
    '$process.WaitForExit()',
    'exit $process.ExitCode',
  ].join('\n'), 'utf8')
  const result = spawnSync('powershell.exe', ['-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', join(directory, 'harness.ps1')], { encoding: 'utf8', timeout: 15000 })
  assert.ifError(result.error)
  const stderr = existsSync(join(directory, 'device-demo.err.log')) ? readFileSync(join(directory, 'device-demo.err.log'), 'utf8') : ''
  assert.equal(result.status, 0, result.stderr + stderr)
  assert.deepEqual(JSON.parse(readFileSync(join(directory, 'device-demo.out.log'), 'utf8')), expected)
})

test('Windows preset accepts internal listen overrides without changing the public origin', { skip: process.platform !== 'win32' }, t => {
  const directory = mkdtempSync(join(tmpdir(), 'measix port test '))
  t.after(() => rmSync(directory, { recursive: true, force: true }))
  const source = readFileSync(join(root, 'scripts/start-real-device-preset.ps1'), 'utf8').replaceAll('\r\n', '\n')
  const start = source.lastIndexOf('    } finally { Pop-Location }') + '    } finally { Pop-Location }'.length
  const construction = source.slice(start, source.indexOf('    $portalUpstream =', start))
  const script = [
    "$env:MEASIX_REAL_DEVICE_HUB_INTERNAL_LISTEN = '127.0.0.1:19101'",
    "$env:MEASIX_REAL_DEVICE_RELAY_INTERNAL_LISTEN = '127.0.0.1:19103'",
    "$origin = 'http://192.0.2.20:9100'",
    '$uri = [Uri]$origin',
    "$repoRoot = $portalRoot = $dbPath = $masterKeyPath = $jwtKeyPath = $relayTokenPath = $spoolPath = 'C:\\fixture'",
    construction,
    'ConvertTo-Json -InputObject @($args) -Compress',
  ].join('\n')
  const path = join(directory, 'ports.ps1')
  writeFileSync(path, script)
  const result = spawnSync('powershell.exe', ['-NoProfile', '-File', path], { encoding: 'utf8', timeout: 15000 })
  assert.ifError(result.error)
  assert.equal(result.status, 0, result.stderr)
  const args = JSON.parse(result.stdout)
  assert.equal(args[args.indexOf('--hub-internal-listen') + 1], '127.0.0.1:19101')
  assert.equal(args[args.indexOf('--relay-internal-listen') + 1], '127.0.0.1:19103')
  assert.equal(args[args.indexOf('--public-origin') + 1], 'http://192.0.2.20:9100')
})

test('Windows preset selects a bindable loopback port when its default is occupied', { skip: process.platform !== 'win32' }, t => {
  const directory = mkdtempSync(join(tmpdir(), 'measix busy port test '))
  t.after(() => rmSync(directory, { recursive: true, force: true }))
  const source = readFileSync(join(root, 'scripts/start-real-device-preset.ps1'), 'utf8').replaceAll('\r\n', '\n')
  const helper = source.match(/^function Get-AvailableLoopbackAddress[\s\S]*?^}/m)?.[0] || ''
  const start = source.lastIndexOf('    } finally { Pop-Location }') + '    } finally { Pop-Location }'.length
  const construction = source.slice(start, source.indexOf('    $portalUpstream =', start))
    .replaceAll("'127.0.0.1:9101'", "('127.0.0.1:' + $busyPort)")
    .replaceAll('Get-AvailableLoopbackAddress 9101', 'Get-AvailableLoopbackAddress $busyPort')
  const script = [
    "$ErrorActionPreference = 'Stop'",
    helper,
    '$env:MEASIX_REAL_DEVICE_HUB_INTERNAL_LISTEN = $null',
    "$env:MEASIX_REAL_DEVICE_RELAY_INTERNAL_LISTEN = '127.0.0.1:19103'",
    "$origin = 'http://192.0.2.20:9100'",
    '$uri = [Uri]$origin',
    "$repoRoot = $portalRoot = $dbPath = $masterKeyPath = $jwtKeyPath = $relayTokenPath = $spoolPath = 'C:\\fixture'",
    '$busy = [System.Net.Sockets.TcpListener]::new([System.Net.IPAddress]::Loopback, 0)',
    '$busy.ExclusiveAddressUse = $true',
    '$busy.Start()',
    '$busyPort = $busy.LocalEndpoint.Port',
    'try {',
    construction,
    '$selected = $args[([Array]::IndexOf($args, "--hub-internal-listen") + 1)]',
    '$probe = [System.Net.Sockets.TcpListener]::new([System.Net.IPAddress]::Loopback, [int]($selected.Split(":")[-1]))',
    '$probe.ExclusiveAddressUse = $true',
    'try { $probe.Start() } finally { $probe.Stop() }',
    'ConvertTo-Json -InputObject @{ busy = $busyPort; args = @($args) } -Compress',
    '} finally { $busy.Stop() }',
  ].join('\n')
  const path = join(directory, 'ports.ps1')
  writeFileSync(path, script)
  const result = spawnSync('powershell.exe', ['-NoProfile', '-File', path], { encoding: 'utf8', timeout: 15000 })
  assert.ifError(result.error)
  assert.equal(result.status, 0, result.stderr)
  const { busy, args } = JSON.parse(result.stdout)
  const address = args[args.indexOf('--hub-internal-listen') + 1]
  assert.match(address, /^127\.0\.0\.1:\d+$/)
  assert.notEqual(Number(address.split(':')[1]), busy, 'must not select the occupied default')
})

test('Windows preset stop preserves ownership and only clears confirmed stopped records', { skip: process.platform !== 'win32' }, async t => {
  for (const scenario of ['stale', 'record-mismatch', 'actual-mismatch', 'normal', 'forced', 'failure', 'query-failure']) {
    await t.test(scenario, t => {
      const directory = mkdtempSync(join(tmpdir(), 'measix-stop-test-'))
      t.after(() => rmSync(directory, { recursive: true, force: true }))
      mkdirSync(join(directory, 'scripts'))
      const data = join(directory, '.data', 'device-real')
      mkdirSync(data, { recursive: true })
      copyFileSync(join(root, 'scripts/stop-real-device-preset.ps1'), join(directory, 'scripts/stop-real-device-preset.ps1'))
      const expected = join(data, 'bin', 'measix-device-demo.exe')
      const statePath = join(data, 'process.json')
      const original = JSON.stringify({ pid: 12345, executable: ['stale', 'record-mismatch'].includes(scenario) ? `${expected}.temporary` : expected })
      writeFileSync(statePath, original)
      writeFileSync(join(directory, 'harness.ps1'), `
$ErrorActionPreference = 'Stop'
$global:presetTestAlive = '${scenario}' -ne 'stale'
$global:presetTestStops = 0
$global:presetTestActual = Join-Path $PSScriptRoot '.data\\device-real\\bin\\measix-device-demo.exe'
if ('${scenario}' -eq 'actual-mismatch') { $global:presetTestActual += '.unrelated' }
function Get-CimInstance {
    param($ClassName, $Filter, $ErrorAction)
    if ('${scenario}' -eq 'query-failure') { throw 'Synthetic query failure' }
    if ($global:presetTestAlive) { [pscustomobject]@{ ExecutablePath = $global:presetTestActual } }
}
function Stop-Process {
    param($Id, [switch]$Force, $ErrorAction)
    $global:presetTestStops++
    if ('${scenario}' -eq 'normal' -or ('${scenario}' -eq 'forced' -and $Force)) { $global:presetTestAlive = $false }
}
function Wait-Process { param($Id, $Timeout, $ErrorAction) }
try { & (Join-Path $PSScriptRoot 'scripts\\stop-real-device-preset.ps1') }
catch { Write-Output ('FAILURE: ' + $_.Exception.Message) }
finally { Write-Output ('STOPS=' + $global:presetTestStops) }
`)
      const result = spawnSync('powershell.exe', ['-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', join(directory, 'harness.ps1')], { encoding: 'utf8', timeout: 15000 })
      assert.ifError(result.error)
      assert.equal(result.status, 0, result.stderr)
      const preserve = ['record-mismatch', 'actual-mismatch', 'failure', 'query-failure'].includes(scenario)
      assert.equal(existsSync(statePath), preserve, result.stdout)
      if (preserve) {
        assert.equal(readFileSync(statePath, 'utf8'), original)
        assert.match(result.stdout, /FAILURE:/)
      } else assert.doesNotMatch(result.stdout, /FAILURE:/)
      const stops = scenario === 'normal' ? 1 : ['forced', 'failure'].includes(scenario) ? 2 : 0
      assert.match(result.stdout, new RegExp('STOPS=' + stops))
      if (scenario.endsWith('mismatch')) {
        assert.match(result.stdout, /PID 12345/)
        assert.match(result.stdout, /Recorded:.*Expected:.*Actual:/)
      }
    })
  }
})

async function runPreset(t, { validationErrors = [], changed = true } = {}) {
  const directory = mkdtempSync(join(tmpdir(), 'measix-preset-test-'))
  t.after(() => rmSync(directory, { recursive: true, force: true }))
  const password = join(directory, 'password.txt')
  const keys = join(directory, 'keys.env')
  const state = join(directory, 'state.json')
  writeFileSync(password, 'synthetic-password')
  writeFileSync(keys, ['DEEPSEEK_API_KEY', 'MIMO_API_KEY', 'FIRECRAWL_API_KEY', 'DASHSCOPE_API_KEY']
    .map(key => `${key}=synthetic-key`).concat('ALIBABA_TOKEN_PLAN_BASE_URL=https://example.invalid/compatible-mode/v1').join('\n'))
  const originalState = JSON.stringify({ upstreams: Object.fromEntries(
    ['deepseek', 'mimo', 'firecrawl', 'alibaba'].map(kind => [kind, { upstreamId: `up_${kind}` }]),
  ) })
  writeFileSync(state, originalState)
  const requests = []
  let content
  const server = createServer(async (request, response) => {
    const chunks = []
    for await (const chunk of request) chunks.push(chunk)
    const body = chunks.length ? JSON.parse(Buffer.concat(chunks).toString()) : undefined
    const path = request.url.replace('/api/admin/v1', '')
    requests.push({ method: request.method, path, body })
    let result
    if (path === '/session/login') {
      response.setHeader('Set-Cookie', 'measix_admin_session=synthetic; HttpOnly')
      result = { csrfToken: 'synthetic-csrf' }
    } else if (path.startsWith('/upstreams/') && request.method === 'GET') {
      result = { upstreamId: path.split('/').at(-1), status: 'ACTIVE' }
    } else if (path === '/draft' && request.method === 'GET') {
      result = { draftRevision: 41, content: { policy: { retainedPolicy: true } } }
    } else if (path === '/draft' && request.method === 'PUT') {
      content = body.content
      result = { draftRevision: 42 }
    } else if (path === '/draft:validate') {
      const missing = content.starters.flatMap((starter, i) => starter.openingSnapshot ? [] : [
        { code: 'missing_starter_opening', path: `starters[${i}].openingSnapshot` },
      ])
      const errors = [...missing, ...validationErrors]
      result = { valid: errors.length === 0, errors, warnings: [] }
    } else if (path === '/draft:preview') {
      result = { publishedGeneration: 7, diffSummary: { added: 0, changed: changed ? 3 : 0, removed: 0 } }
    } else if (path === '/draft:publish') {
      result = { activationId: 'activation-test' }
    } else if (path === '/activations/activation-test') {
      result = { state: 'COMPLETED' }
    } else {
      response.statusCode = 500
      result = { code: 'unexpected_test_request' }
    }
    response.setHeader('Content-Type', 'application/json')
    response.end(JSON.stringify(result))
  })
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve))
  t.after(() => new Promise(resolve => server.close(resolve)))
  const child = spawn(process.execPath, [join(root, 'scripts/real-device-preset.mjs')], {
    env: { ...process.env, MEASIX_REAL_DEVICE_ORIGIN: `http://127.0.0.1:${server.address().port}`,
      MEASIX_REAL_DEVICE_ADMIN_PASSWORD_FILE: password, MEASIX_REAL_DEVICE_SUPPLIER_KEYS: keys,
      MEASIX_REAL_DEVICE_STATE: state },
    stdio: ['ignore', 'pipe', 'pipe'],
  })
  let output = ''
  child.stdout.on('data', chunk => { output += chunk })
  child.stderr.on('data', chunk => { output += chunk })
  const code = await new Promise((resolve, reject) => { child.on('error', reject); child.on('close', resolve) })
  assert.equal(readFileSync(state, 'utf8'), originalState, 'existing upstream identities must not be rewritten')
  assert.ok(!requests.some(item => ['/secrets', '/upstreams'].includes(item.path)), 'existing credentials must be reused')
  assert.ok(!output.includes('synthetic-password') && !output.includes('synthetic-key'), 'do not print secrets')
  return { code, output, content, requests }
}

test('real-device preset publishes complete authored v5 openings with existing upstreams', async t => {
  const result = await runPreset(t)
  assert.equal(result.code, 0, result.output)
  assert.equal(result.content.starters.length, 3)
  for (const starter of result.content.starters) {
    const opening = starter.openingSnapshot
    assert.equal(opening.format, 1)
    assert.ok(opening.systemPrompt.trim())
    assert.ok(Array.isArray(opening.initialContexts))
    assert.equal(new Set(opening.initialContexts.map(item => item.id)).size, opening.initialContexts.length)
    for (const block of opening.initialContexts) {
      assert.ok(block.id.trim())
      assert.equal(Object.hasOwn(block, 'title'), false)
      assert.equal(typeof block.content, 'string')
    }
  }
  assert.ok(result.content.starters[0].openingSnapshot.initialContexts.length >= 2, 'exercise ordered opening backgrounds on device')
  assert.deepEqual(result.content.models.map(model => model.upstreamModelKey), ['deepseek-flash', 'qwen3.8-flash'])
  assert.equal(result.content.policy.retainedPolicy, true)
  const mutations = result.requests.filter(item => item.path === '/draft' && item.method === 'PUT' || item.path === '/draft:publish')
  assert.equal(mutations[0].body.expectedDraftRevision, 41)
  assert.equal(mutations[1].body.expectedDraftRevision, 42)
})

test('unchanged preset does not publish another release', async t => {
  const result = await runPreset(t, { changed: false })
  assert.equal(result.code, 0, result.output)
  assert.ok(!result.requests.some(item => item.path === '/draft:publish'))
})

test('real-device preset explicitly authors v5 MCP access and assistant bindings', async t => {
  const result = await runPreset(t)
  assert.equal(result.code, 0, result.output)
  const server = result.content.mcp[0]
  assert.equal(server.toolAccessMode, 'ALL')
  assert.deepEqual(server.allowedTools, [])
  for (const assistant of result.content.assistants) {
    assert.deepEqual(assistant.mcpBindings, [{ mcpServerId: server.mcpServerId, toolSelection: 'ALL', toolNames: [] }])
    assert.equal(Object.hasOwn(assistant, 'mcpServerIds'), false)
  }
})

test('validation failure reports authoritative field path and prevents preview/publish', async t => {
  const result = await runPreset(t, { validationErrors: [
    { code: 'invalid_starter_opening', path: 'starters[1].openingSnapshot.initialContexts[0].content' },
  ] })
  assert.notEqual(result.code, 0)
  assert.match(result.output, /invalid_starter_opening:starters\[1\]\.openingSnapshot\.initialContexts\[0\]\.content/)
  assert.ok(!result.requests.some(item => ['/draft:preview', '/draft:publish'].includes(item.path)))
})

test('database upgrade failure guidance preserves existing real-device data', () => {
  const source = readFileSync(join(root, 'scripts/start-real-device-preset.ps1'), 'utf8')
  const failure = source.split(/\r?\n/).find(line => line.includes('throw') && /database .*failed/i.test(line))
  assert.ok(failure, 'launcher must diagnose database preparation failure')
  assert.doesNotMatch(failure, /device:real:reset|delete and recreate/i)
  assert.match(failure, /preserv|backup/i)
})
