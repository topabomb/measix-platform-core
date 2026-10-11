/** Actual previous-binary upgrade and Caddy → Hub/Relay → independent DAV lane.
 * Management is a deterministic test adapter, not real Agent Space qualification.
 * Use a NEW --output directory. --keep leaves only this isolated environment for UI review.
 */
import assert from 'node:assert/strict'
import { createHash, randomUUID, randomBytes } from 'node:crypto'
import { cpSync, existsSync, mkdirSync, readFileSync, writeFileSync, createWriteStream } from 'node:fs'
import { resolve, join } from 'node:path'
import { spawn, execFileSync } from 'node:child_process'
import { once } from 'node:events'
import { DatabaseSync } from 'node:sqlite'
import { createFreshEnvironment, startHubAndRelay, freePort, waitFor, adminBuildHash } from './lib/harness.mjs'

const args = process.argv.slice(2)
const option = name => { const i = args.indexOf(name); return i < 0 ? undefined : args[i + 1] }
const root = resolve(import.meta.dirname, '..'), output = resolve(option('--output') ?? '.artifacts/webdav-acceptance')
const previousBinary = resolve(option('--previous-hub') ?? '.artifacts/webdav-upgrade-baseline-38c4271/control-hub.exe')
const toolsContainer = option('--tools-container')
const upstreamConfig = option('--upstream-config') ? JSON.parse(readFileSync(resolve(option('--upstream-config')), 'utf8')) : undefined
assert.ok(toolsContainer, 'pass --tools-container with Caddy installed and /evidence mounted to the output parent')
assert.ok(!existsSync(output), 'output must be new; preserve prior attempts')
mkdirSync(output, { recursive: true })
const sha = value => createHash('sha256').update(value).digest('hex')
const id = prefix => `${prefix}_${randomUUID()}`
const pause = ms => new Promise(resolve => setTimeout(resolve, ms))
const checks = [], children = []
const evidence = { startedAt: new Date().toISOString(), previousBinarySha256: sha(readFileSync(previousBinary)), checks, upstream: upstreamConfig?.releaseIdentity ?? 'golang.org/x/net/webdav v0.61.0; deterministic management adapter' }
const proof = (name, detail = {}) => { checks.push({ name, ...detail }); writeFileSync(join(output, 'evidence.json'), JSON.stringify(evidence, null, 2)); console.log('PASS ' + name) }
const logProcess = (name, proc) => { const log = createWriteStream(join(output, name + '.log'), { flags: 'a' }); for (const stream of [proc.stdout, proc.stderr]) { stream?.removeAllListeners('data'); stream?.pipe(log) }; children.push(proc); return proc }
async function stop(proc) { if (!proc || proc.exitCode !== null) return; const exited = once(proc, 'exit'); proc.kill(); await exited }
let servers, env, cookie, session, publicOrigin, adapterOrigin, activeBinary
async function stopCore() { if (servers) await Promise.all(Object.values(servers).map(stop)); servers = undefined }
async function startCore(binary, ttl) {
  env.hubBin = binary
  servers = startHubAndRelay(env, { publicOrigin, accessTokenTTL: ttl })
  logProcess(binary === previousBinary ? 'previous-hub' : 'hub', servers.hubProc); logProcess('relay', servers.relayProc)
  await waitFor(env.hubBaseURL + '/live', 'Hub'); await waitFor(env.relayIntBaseURL + '/live', 'Relay')
}
async function login() {
  const r = await fetch(publicOrigin + '/api/admin/v1/session/login', { method: 'POST', headers: { 'Content-Type': 'application/json', Origin: publicOrigin }, body: JSON.stringify({ username: 'admin', password: env.adminPassword }) })
  assert.equal(r.status, 200, 'login')
  session = await r.json(); cookie = r.headers.getSetCookie().map(value => value.split(';')[0]).join('; ')
}
async function raw(path, init = {}) { return fetch(publicOrigin + path, { ...init, headers: { Cookie: cookie, Origin: publicOrigin, 'X-CSRF-Token': session.csrfToken, ...init.headers } }) }
async function api(path, method = 'GET', body, headers = {}) {
  const r = await raw(path, { method, headers: { ...(body === undefined ? {} : { 'Content-Type': 'application/json' }), ...headers }, body: body === undefined ? undefined : JSON.stringify(body) })
  const text = await r.text(); const data = text ? JSON.parse(text) : undefined
  assert.ok(r.ok, `${method} ${path}: ${r.status} ${data?.code ?? ''}`)
  return data
}
async function complete(operation, kind = 'workspace-operations') {
  for (let i = 0; i < 120; i++) {
    const value = await api(`/api/admin/v1/${kind}/${operation.operationId ?? operation.activationId}`)
    if (value.state === 'COMPLETED') { assert.notEqual(value.step, 'CHECK_FAILED'); return value }
    assert.ok(!['FAILED', 'UNKNOWN', 'NEEDS_ATTENTION'].includes(value.state), `operation ${value.state}: ${value.errorCode ?? value.diagnosticCode}`)
    await pause(250)
  }
  throw Error('operation did not complete')
}
const workspacePath = user => `/api/admin/v1/users/${user.userId}/workspace`
async function command(user, action, extra = {}) {
  const view = await api(workspacePath(user))
  return complete(await api(workspacePath(user), 'POST', { action, expectedRevision: view.bindingRevision, ...extra }, { 'Idempotency-Key': id('idem') }))
}
async function publish() {
  const draft = await api('/api/admin/v1/draft')
  const validated = await api('/api/admin/v1/draft:validate', 'POST', { expectedDraftRevision: draft.draftRevision })
  assert.equal(validated.valid, true, JSON.stringify(validated.errors))
  return complete(await api('/api/admin/v1/draft:publish', 'POST', { expectedDraftRevision: draft.draftRevision, acknowledgedWarningCodes: validated.warnings.map(w => w.code) }, { 'Idempotency-Key': id('idem') }), 'activations')
}
async function enroll(user) {
  const enrollment = await api(`/api/admin/v1/users/${user.userId}/enrollments`, 'POST', { expiresInSeconds: 3600 })
  const r = await fetch(publicOrigin + '/api/client/v1/enrollments/exchange', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ code: enrollment.code, installationId: id('ins'), deviceName: 'DAV acceptance', platform: 'ANDROID', appVersion: 'acceptance' }) })
  assert.equal(r.status, 201); Object.assign(user, await r.json())
}
async function refresh(user) {
  const r = await fetch(publicOrigin + '/api/client/v1/sessions/refresh', { method: 'POST', headers: { 'Content-Type': 'application/json', 'Idempotency-Key': id('idem') }, body: JSON.stringify({ refreshToken: user.refreshToken }) })
  assert.equal(r.status, 200); Object.assign(user, await r.json())
}
async function clientGet(user, path) { const r = await fetch(publicOrigin + path, { headers: { Authorization: 'Bearer ' + user.accessToken } }); assert.equal(r.status, 200, path); return r.json() }
function databaseSnapshot(file, original) {
  const db = new DatabaseSync(file, { readOnly: true })
  try {
    assert.equal(db.prepare('PRAGMA integrity_check').get().integrity_check, 'ok')
    assert.deepEqual(db.prepare('PRAGMA foreign_key_check').all(), [])
    const out = {}
    const names = original ? Object.keys(original) : db.prepare("SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' AND name NOT LIKE '%migration%' ORDER BY name").all().map(row => row.name)
    for (const name of names) {
      const columns = original?.[name]?.columns ?? db.prepare(`PRAGMA table_info("${name}")`).all().map(row => row.name)
      const query = db.prepare(`SELECT ${columns.map(name => '"' + name + '"').join(',')} FROM "${name}"`)
      const rows = query.all().map(row => JSON.stringify(row)).sort()
      out[name] = { columns, count: rows.length, hash: sha(rows.join('\n')) }
    }
    return out
  } finally { db.close() }
}

try {
  env = await createFreshEnvironment(root, { prefix: 'measix-dav-upgrade', hubBinarySource: previousBinary, deploymentName: 'DAV Upgrade Acceptance', buildStdio: 'pipe', migrateStdio: 'pipe', bootstrapStdio: 'pipe' })
  const previousCopy = env.hubBin
  activeBinary = join(env.envRoot, 'control-hub-current.exe')
  execFileSync('go', ['build', '-o', activeBinary, './cmd/control-hub'], { cwd: join(root, 'backend'), stdio: 'pipe', windowsHide: true })
  evidence.currentBinarySha256 = sha(readFileSync(activeBinary)); evidence.spaHash = adminBuildHash(root)
  cpSync(join(root, 'console/dist/spa'), join(env.envRoot, 'admin-spa'), { recursive: true })
  process.env.HUB_ADMIN_ASSETS_DIR = join(env.envRoot, 'admin-spa')
  const adapterPort = await freePort(); adapterOrigin = `http://127.0.0.1:${adapterPort}`
  const adapterTokenFile = join(env.envRoot, 'adapter-token.private'); const management = upstreamConfig ? readFileSync(resolve(upstreamConfig.managementTokenFile), 'utf8').trim() : randomBytes(32).toString('hex'); writeFileSync(adapterTokenFile, management, { mode: 0o600 })
  const adapterBinary = join(env.envRoot, 'workspace-dav-adapter.exe')
  if (upstreamConfig) {
    adapterOrigin = upstreamConfig.adminOrigin
    const before = await fetch(adapterOrigin + '/admin/v1/users', { headers: { Authorization: 'Bearer ' + management } }); assert.equal(before.status, 200)
    const users = (await before.json()).users
    writeFileSync(join(output, 'upstream-users-before.private.json'), JSON.stringify(users, null, 2), { mode: 0o600 })
    evidence.existingUpstreamUsers = users.length
  } else {
    execFileSync('go', ['build', '-o', adapterBinary, '.'], { cwd: join(root, 'scripts/fixtures/workspace-dav-server'), stdio: 'pipe', windowsHide: true })
    logProcess('adapter', spawn(adapterBinary, ['--listen', `127.0.0.1:${adapterPort}`, '--root', join(env.envRoot, 'adapter-data'), '--token-file', adapterTokenFile], { stdio: ['ignore', 'pipe', 'pipe'], windowsHide: true }))
    await waitFor(adapterOrigin + '/live', 'DAV adapter')
  }
  const mappedPort = execFileSync('docker', ['port', toolsContainer, '9080/tcp'], { encoding: 'utf8', windowsHide: true }).trim().split(':').at(-1)
  publicOrigin = `http://127.0.0.1:${mappedPort}`
  writeFileSync(join(output, 'Caddyfile'), `{\n admin off\n auto_https off\n}\n:9080 {\n @private path /internal /internal/*\n handle @private {\n  respond 404\n }\n @runtime path /runtime/v1 /runtime/v1/*\n handle @runtime {\n  reverse_proxy host.docker.internal:${env.relayPubPort}\n }\n handle {\n  reverse_proxy host.docker.internal:${env.hubPort}\n }\n}\n`)
  logProcess('caddy', spawn('docker', ['exec', toolsContainer, 'caddy', 'run', '--config', '/evidence/' + output.split(/[\\/]/).at(-1) + '/Caddyfile', '--adapter', 'caddyfile'], { stdio: ['ignore', 'pipe', 'pipe'], windowsHide: true }))
  await startCore(previousCopy); await waitFor(publicOrigin + '/live', 'Caddy'); await login()
  const secret = await api('/api/admin/v1/secrets', 'POST', { name: 'Acceptance management', value: management })
  const service = await api('/api/admin/v1/remote-workspace/services', 'POST', { name: 'Workspace acceptance', expectedRevision: 0, config: { adminOrigin: adapterOrigin, mcpOrigin: upstreamConfig?.mcpOrigin ?? adapterOrigin, davOrigin: upstreamConfig?.davOrigin ?? adapterOrigin, managementSecret: { secretId: secret.secretId, secretVersion: secret.secretVersion }, connectTimeoutMs: 90000, idleTimeoutMs: 120000 } }, { 'Idempotency-Key': id('idem') })
  await complete(await api(`/api/admin/v1/remote-workspace/services/${service.workspaceServiceId}/apply`, 'POST', undefined, { 'Idempotency-Key': id('idem') }))
  const users = []
  for (const name of ['authorized', 'revoked']) { const user = await api('/api/admin/v1/users', 'POST', { username: 'dav_' + name, displayName: name === 'authorized' ? 'DAV 验收用户' : '已撤销文件授权用户', role: 'MEMBER' }); users.push(user); await command(user, 'CREATE'); await enroll(user) }
  const [user, revoked] = users
  await command(revoked, 'REVOKE_DAV')
  const view = await api(workspacePath(user))
  const contentPath = workspacePath(user) + '/content?' + new URLSearchParams({ agentSpaceId: view.agentSpaceId, path: 'upgrade-note.txt' })
  let response = await raw(contentPath, { method: 'PUT', headers: { 'If-None-Match': '*' }, body: 'Preserve this workspace across the binary upgrade.\n' }); assert.equal(response.status, 200); await response.body.cancel()
  await publish()
  const oldSnapshot = await clientGet(user, '/api/client/v1/managed/snapshots/1')
  assert.equal(oldSnapshot.webdav, undefined)
  const draft = await api('/api/admin/v1/draft'); draft.content.providers.push({ providerId: id('prv'), displayName: 'Unpublished edit must survive', clientProtocol: 'OPENAI_CHAT_COMPLETIONS', enabled: false })
  await api('/api/admin/v1/draft', 'PUT', { expectedDraftRevision: draft.draftRevision, content: draft.content })
  await stopCore()
  const before = databaseSnapshot(env.hubDB)
  assert.ok(before.agent_spaces.count === 2 && before.secret_versions.count >= 3 && before.managed_releases.count === 1)
  writeFileSync(join(output, 'before-tables.json'), JSON.stringify(before, null, 2))
  const backup = join(output, 'previous-backup.db')
  execFileSync(previousCopy, ['backup', '--db', env.hubDB, '--output', backup], { stdio: 'pipe', windowsHide: true })
  execFileSync(activeBinary, ['migrate', '--db', env.hubDB], { stdio: 'pipe', windowsHide: true })
  assert.deepEqual(databaseSnapshot(env.hubDB, before), before)
  execFileSync(activeBinary, ['migrate', '--db', env.hubDB], { stdio: 'pipe', windowsHide: true })
  assert.deepEqual(databaseSnapshot(env.hubDB, before), before)
  proof('previous executable populated database → additive upgrade twice preserves every original table column', { tables: Object.keys(before).length })
  const upgradedDB = env.hubDB, restoredDB = join(env.envRoot, 'restored-previous.db'); cpSync(backup, restoredDB, { force: false, errorOnExist: true }); env.hubDB = restoredDB
  execFileSync(previousCopy, ['check', '--db', restoredDB], { stdio: 'pipe', windowsHide: true })
  await startCore(previousCopy); await login(); response = await raw(contentPath); assert.equal(await response.text(), 'Preserve this workspace across the binary upgrade.\n'); assert.deepEqual(await clientGet(user, '/api/client/v1/managed/snapshots/1'), oldSnapshot)
  await stopCore(); env.hubDB = upgradedDB
  proof('previous backup restored and opened by previous executable; original Files and Snapshot remain readable')
  await startCore(activeBinary, '15s'); await login()
  await refresh(user); await refresh(revoked)
  assert.deepEqual(await clientGet(user, '/api/client/v1/managed/snapshots/1'), oldSnapshot)
  let staged
  for (let i = 0; i < 80; i++) { staged = await api('/api/admin/v1/draft'); if (staged.content.webdav?.length === 1) break; await pause(250) }
  assert.equal(staged.content.webdav.length, 1); assert.equal(staged.content.webdav[0].enabled, true); assert.ok(staged.content.providers.some(p => p.displayName === 'Unpublished edit must survive'))
  await publish()
  const projected = await api(workspacePath(user)), denied = await api(workspacePath(revoked))
  assert.equal(projected.webdav.available, true); assert.equal(projected.webdav.accessMode, 'FULL'); assert.equal(projected.fileExchangeWebdavResourceId, staged.content.webdav[0].resourceId); assert.equal(denied.filesAvailable, false)
  proof('upgrade stages one default DAV without losing unpublished edits, preserves old snapshot, confirms FULL after publication and retains explicit revoke')
  writeFileSync(join(output, 'environment.private.json'), JSON.stringify({ ...env, publicOrigin, adapterOrigin, user, revoked, service, cookie, session, contentPath, davRoot: `/runtime/v1/resources/${projected.fileExchangeWebdavResourceId}/spaces/${projected.agentSpaceId}/dav/`, pids: children.map(p => p.pid) }, null, 2), { mode: 0o600 })
  evidence.publicOrigin = publicOrigin; evidence.status = 'UPGRADE_PASSED_UI_AND_TRANSFER_PENDING'; writeFileSync(join(output, 'evidence.json'), JSON.stringify(evidence, null, 2))
  console.log('READY ' + publicOrigin + '/admin/; private environment saved to ' + output)
  if (args.includes('--keep')) await new Promise(resolve => { process.once('SIGINT', resolve); process.once('SIGTERM', resolve) })
} catch (error) { evidence.status = 'FAILED'; evidence.failure = error.message; writeFileSync(join(output, 'evidence.json'), JSON.stringify(evidence, null, 2)); throw error }
finally { await Promise.all(children.map(stop)) }
