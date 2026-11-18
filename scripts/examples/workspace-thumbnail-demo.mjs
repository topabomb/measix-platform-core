import { readFileSync, writeFileSync, mkdirSync } from 'node:fs'
import { resolve, join, dirname } from 'node:path'
import { pathToFileURL } from 'node:url'
import { createHash } from 'node:crypto'
import { WorkspaceThumbnailSession, PreviewError } from './workspace-thumbnail-session.mjs'

// Explicitly selected files, fetched sequentially. A UI should supply its
// existing visible-read queue rather than enqueue an entire directory.
export async function runDemo(config, output, fetch = globalThis.fetch) {
  if (!Array.isArray(config.items) || typeof config.accessTokenFile !== 'string') throw new TypeError('Expected items and an accessTokenFile')
  const client = new WorkspaceThumbnailSession({ endpoint: config.endpoint, agentSpaceId: config.agentSpaceId, fetch: (url, options) => {
    // The existing session owner may refresh the token file. That does not
    // cause another discovery while this connection/principal stays the same.
    const token = readFileSync(config.accessTokenFile, 'utf8').trim()
    if (!token || token.length > 16 * 1024 || !/^[\x21-\x7e]+$/.test(token)) throw new TypeError('Expected one access token')
    return fetch(url, { ...options, headers: { Authorization: 'Bearer ' + token } })
  } })
  try {
    mkdirSync(output, { recursive: false }) // Never overwrite a prior run.
    const capabilities = await client.capabilities()
    const items = []
    for (const entry of config.items) {
      const item = { path: entry.path, expectedEtag: entry.expectedEtag, size: entry.size ?? 128 }
      try {
        const thumbnail = await client.thumbnail(item)
        if (!thumbnail) { items.push({ ...item, unsupported: true }); continue }
        const file = String(items.length).padStart(4, '0') + '.webp'
        writeFileSync(join(output, file), thumbnail.bytes, { flag: 'wx' })
        items.push({ ...item, file, etag: thumbnail.etag, bytes: thumbnail.bytes.length, sha256: createHash('sha256').update(thumbnail.bytes).digest('hex') })
      } catch (error) {
        items.push({ ...item, error: error instanceof PreviewError ? error.code : 'request_failed', status: error instanceof PreviewError ? error.status : undefined })
      }
    }
    const report = { agentSpaceId: config.agentSpaceId, capabilities, items }
    writeFileSync(join(output, 'result.json'), JSON.stringify(report, null, 2) + '\n', { flag: 'wx' })
    return report
  } finally { client.dispose() }
}

if (process.argv[1] && pathToFileURL(resolve(process.argv[1])).href === import.meta.url) {
  try {
    if (process.argv.length !== 4) throw new TypeError('Usage: node scripts/examples/workspace-thumbnail-demo.mjs <private-config.json> <new-output-directory>')
    const configFile = resolve(process.argv[2]), config = JSON.parse(readFileSync(configFile, 'utf8'))
    config.accessTokenFile = resolve(dirname(configFile), config.accessTokenFile)
    const result = await runDemo(config, resolve(process.argv[3]))
    const failed = result.items.filter(item => item.error).length
    console.log(JSON.stringify({ written: result.items.filter(item => item.file).length, unsupported: !result.capabilities.supported, failed }))
    if (failed) process.exitCode = 1
  } catch (error) {
    const code = error instanceof PreviewError ? error.code : /^[A-Z][A-Z0-9_]{1,40}$/.test(error.code ?? '') ? error.code : 'invalid_demo_request'
    console.error(process.argv.length !== 4 ? 'Usage: node scripts/examples/workspace-thumbnail-demo.mjs <private-config.json> <new-output-directory>' : code)
    process.exitCode = 1
  }
}
