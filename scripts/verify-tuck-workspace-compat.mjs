/** Replay captured Client bytes through a fixed, unmodified Tuck consumer. */
import assert from 'node:assert/strict'
import { createHash } from 'node:crypto'
import { execFileSync, spawn } from 'node:child_process'
import { createWriteStream, existsSync, lstatSync, mkdirSync, readFileSync, realpathSync, writeFileSync } from 'node:fs'
import { basename, dirname, join, resolve, sep } from 'node:path'
import { finished } from 'node:stream/promises'
import { pathToFileURL } from 'node:url'

const root = resolve(import.meta.dirname, '..')
const evidenceRoot = join(root, '.artifacts/media-preview')
const commit = 'dc9f10996717fc135b96ae00bc9bd0d0cc5cfd78'
const consumerFiles = ['internal/tuck/contract/provenance.json', 'frontend/src/shared/enterprise-workspace/EnterpriseWorkspace.vue', 'internal/tuck/contract/client-schemas.json', 'internal/tuck/enterprise_workspace.go']
const fixture = join(root, 'scripts/fixtures/tuck-workspace-compat_test.go')
const tests = ['TestCurrentCoreWorkspaceCaptureReplay', 'TestCurrentCoreWorkspacePathGuards', 'TestEnterpriseWorkspaceStreamingConditionsZIPAndPaths', 'TestParityWorkspaceConditionsErrorsAndMutations', 'TestEnterpriseWorkspaceTruncationEarlyErrorsAndCancellation']
const hash = bytes => createHash('sha256').update(bytes).digest('hex')
const git = (repo, args) => execFileSync('git', ['--no-optional-locks', '-C', repo, ...args], { encoding: 'utf8', windowsHide: true }).trim()
const json = path => JSON.parse(readFileSync(path, 'utf8'))

function consumerState(tuck, command = git) {
  return { commit: command(tuck, ['rev-parse', 'HEAD']), workingTreeStatus: command(tuck, ['status', '--porcelain=v1', '--untracked-files=all']), files: Object.fromEntries(consumerFiles.map(name => [name, hash(readFileSync(join(tuck, name)))])) }
}

export function validateInputs(options, dependencies = {}) {
  for (const key of ['tuck', 'baseline', 'manifest', 'output']) assert.equal(typeof options[key], 'string', `Missing ${key}`)
  const value = Object.fromEntries(Object.entries(options).map(([key, path]) => [key, resolve(path)]))
  assert.ok(value.output.startsWith(evidenceRoot + sep) && value.output.slice(0, 2).toUpperCase() === 'D:', 'Output must stay in the Core D: private evidence directory')
  assert.ok(!existsSync(value.output), 'Occupied evidence is never reused')
  assert.ok(!value.output.startsWith(value.tuck + sep), 'Evidence cannot be in the consumer repository')
  const baseline = json(value.baseline)
  assert.equal(baseline.commit, commit, 'Consumer baseline commit must be pinned')
  assert.equal(baseline.workingTreeStatus, '', 'Consumer baseline must be clean')
  assert.deepEqual(Object.keys(baseline.files).sort(), [...consumerFiles].sort(), 'Consumer baseline must cover all four files')
  const before = consumerState(value.tuck, dependencies.git)
  assert.deepEqual(before, { commit: baseline.commit, workingTreeStatus: baseline.workingTreeStatus, files: baseline.files }, 'Consumer does not match the fixed baseline')
  const manifest = json(value.manifest)
  assert.equal(manifest.schemaVersion, 1, 'Capture manifest version')
  assert.ok(manifest.core?.backendSource && manifest.core?.binaries?.hub && manifest.core?.binaries?.relay && manifest.core?.adminBuildHash && manifest.provider?.image && manifest.provider?.expectedSource && manifest.provider?.expectedHelper && manifest.provider?.expectedWorkspace, 'Capture producer identity is required')
  assert.deepEqual(manifest.cases.map(c => c.id).sort(), ['files-nested', 'files-root', 'workspace'], 'Three distinct Client captures are required')
  const captureFiles = {}
  for (const capture of manifest.cases) {
    assert.equal(capture.method, 'GET', 'Capture must be a Client GET')
    assert.equal(capture.status, 200, 'Capture must be a real successful response')
    assert.equal(capture.contentType, 'application/json', 'Capture response MIME')
    assert.equal(capture.bodyFile, capture.id + '.json', 'Capture body file must be local and named by case')
    const request = new URL(capture.path, 'http://captured.invalid')
    assert.equal(request.origin, 'http://captured.invalid', 'Capture must use a Client path, not a URL')
    assert.equal(request.hash, '', 'Capture Client path fragment')
    assert.equal(request.pathname, '/api/client/v1/workspace' + (capture.id === 'workspace' ? '' : '/files'), 'Admin or other paths are not Client captures')
    if (capture.id === 'workspace') assert.equal(request.search, '', 'Workspace Client projection takes no query')
    else {
      assert.deepEqual([...request.searchParams.keys()].sort(), ['agentSpaceId', 'path'], 'Capture query must retain exactly space and directory')
      assert.equal(request.searchParams.get('agentSpaceId'), capture.agentSpaceId, 'Capture space metadata')
      assert.equal(request.searchParams.get('path'), capture.directory, 'Capture directory metadata')
      assert.equal(capture.directory === '', capture.id === 'files-root', 'Capture root/nested directories')
    }
    const path = join(dirname(value.manifest), basename(capture.bodyFile))
    assert.ok(lstatSync(path).isFile() && realpathSync(path).startsWith(realpathSync(dirname(value.manifest)) + sep), 'Capture body must be a regular file in the capture directory')
    const body = readFileSync(path)
    assert.ok(body.length > 0 && body.length <= 4 * 1024 * 1024, 'Capture response must fit the old consumer limit')
    assert.equal(capture.byteLength, body.length, 'Capture response length differs')
    assert.equal(capture.responseSHA256, 'sha256:' + hash(body), 'Capture response hash differs')
    captureFiles[capture.bodyFile] = hash(body)
  }
  return { ...value, before, manifestValue: manifest, manifestSHA256: hash(readFileSync(value.manifest)), captureFiles }
}

export async function runCompatibility(options) {
  const input = validateInputs(options)
  const virtual = join(input.tuck, 'internal/tuck/current_core_workspace_compat_test.go')
  assert.ok(!existsSync(virtual), 'Overlay must not shadow a real consumer test')
  assert.ok(existsSync(fixture), 'External compatibility test fixture is missing')
  mkdirSync(input.output, { recursive: true })
  const temporary = join(input.output, 'temp'), goTemporary = join(input.output, 'go-temp')
  mkdirSync(temporary); mkdirSync(goTemporary)
  const overlay = join(input.output, 'overlay.json')
  writeFileSync(overlay, JSON.stringify({ Replace: { [virtual]: fixture } }, null, 2) + '\n', { flag: 'wx' })
  const stdoutPath = join(input.output, 'go-test.jsonl'), stderrPath = join(input.output, 'go-test.stderr.log')
  const stdout = createWriteStream(stdoutPath, { flags: 'wx' }), stderr = createWriteStream(stderrPath, { flags: 'wx' })
  let exitCode, failure
  const args = ['test', '-mod=readonly', '-overlay', overlay, './internal/tuck', '-run', '^(' + tests.join('|') + ')$', '-count=1', '-timeout=4m', '-json']
  try {
    const child = spawn('go', args, { cwd: input.tuck, windowsHide: true, env: { ...process.env, GOFLAGS: '', GOWORK: 'off', TEMP: temporary, TMP: temporary, TMPDIR: temporary, GOTMPDIR: goTemporary, MEASIX_TUCK_COMPAT_MANIFEST: input.manifest } })
    child.stdout.pipe(stdout); child.stderr.pipe(stderr)
    exitCode = await new Promise((resolveP, reject) => { child.once('error', reject); child.once('close', resolveP) })
  } catch (error) { failure = String(error) }
  finally { stdout.end(); stderr.end(); await Promise.all([finished(stdout), finished(stderr)]) }
  const after = consumerState(input.tuck)
  const passedTests = readFileSync(stdoutPath, 'utf8').split('\n').flatMap(line => { try { const event = JSON.parse(line); return event.Action === 'pass' && event.Test ? [event.Test] : [] } catch { return [] } })
  const preserved = JSON.stringify(after) === JSON.stringify(input.before)
  const capturesPreserved = input.manifestSHA256 === hash(readFileSync(input.manifest)) && Object.entries(input.captureFiles).every(([name, digest]) => hash(readFileSync(join(dirname(input.manifest), name))) === digest)
  const passed = exitCode === 0 && preserved && capturesPreserved && tests.every(name => passedTests.includes(name))
  const report = { passed, scope: 'Current Core raw Client responses -> fixed old Tuck parser and EnterpriseWorkspace.Handle replay; not Tuck desktop networking or thumbnail UI', consumer: { before: input.before, after, preserved }, producer: { manifestSHA256: input.manifestSHA256, core: input.manifestValue.core, provider: input.manifestValue.provider, captureFiles: input.captureFiles, preserved: capturesPreserved }, overlaySourceSHA256: hash(readFileSync(fixture)), goArgs: args, exitCode, ...(failure ? { failure } : {}), passedTests, logs: { stdout: basename(stdoutPath), stderr: basename(stderrPath) } }
  writeFileSync(join(input.output, 'result.json'), JSON.stringify(report, null, 2) + '\n', { flag: 'wx' })
  assert.ok(passed, 'Fixed consumer compatibility failed; see preserved evidence/result.json')
  return report
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  const options = {}
  for (let index = 2; index < process.argv.length; index += 2) {
    const flag = process.argv[index], value = process.argv[index + 1]
    assert.ok(['--tuck', '--baseline', '--manifest', '--output'].includes(flag) && value && !Object.hasOwn(options, flag.slice(2)), 'Usage: --tuck DIR --baseline FILE --manifest FILE --output NEW_DIR')
    options[flag.slice(2)] = value
  }
  runCompatibility(options).then(report => console.log(JSON.stringify({ passed: report.passed, consumer: report.consumer.after.commit, output: resolve(options.output) }))).catch(error => { console.error(error.message); process.exitCode = 1 })
}
