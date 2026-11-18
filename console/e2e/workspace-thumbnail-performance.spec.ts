import { test, expect, type Page, type Request } from '@playwright/test'
import { mkdirSync, readFileSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'
import { maximumConcurrentTransfers, thumbnailReadInterval } from '../../scripts/workspace-thumbnail-browser.mjs'
import { positiveTransferOverlap, runSampledPhase, summarizePerformance, validatePerformanceFixture } from '../../scripts/workspace-thumbnail-performance.mjs'

test.use({ trace: 'off', video: 'off' })
const inputFile = process.env.MEASIX_THUMBNAIL_BROWSER_INPUT
const input = inputFile ? JSON.parse(readFileSync(inputFile, 'utf8')) : undefined
test.skip(!input?.performance, 'Run the existing thumbnail runner with --performance-only')

type Phase = { name: string; startedAt: number; endedAt: number; firstImageAt?: number; allVisibleAt?: number; visiblePaths: string[]; renderedPaths: string[] }
type Network = { method: string; phase: string; path: string; size: number; startedAt: number; endedAt?: number; status?: number; failed?: string; cancelled?: boolean }
type Native = { start: number; end: number; abortAt?: number; path: string; size: number; fetchObserved: boolean }
type RangeNative = { url: string; start: number; end: number }
type Visible = { visible: string[]; rendered: string[] }

async function visibleMedia(page: Page) {
  return page.locator('[data-cy="workspace-file-collection"]').evaluate(collection => {
    const bounds = collection.getBoundingClientRect()
    const top = Math.max(0, bounds.top), bottom = Math.min(innerHeight, bounds.bottom)
    const left = Math.max(0, bounds.left), right = Math.min(innerWidth, bounds.right)
    const visible: string[] = [], rendered: string[] = []
    const size = collection.getAttribute('data-view') === 'thumbnails' ? 256 : 128
    const directory = collection.closest('[data-cy="workspace-files"]')?.querySelector('.workspace-file-navigation .text-caption')?.textContent?.trim().replace(/^\//, '') ?? ''
    for (const slot of collection.querySelectorAll<HTMLElement>('[data-thumbnail-path]')) {
      const path = slot.dataset.thumbnailPath!
      if (!/\.(png|jpg|mp4|webm)$/.test(path)) continue
      const box = slot.getBoundingClientRect()
      if (box.bottom <= top || box.top >= bottom || box.right <= left || box.left >= right) continue
      visible.push(path)
      const image = slot.querySelector('img')
      if (image?.complete && Math.max(image.naturalWidth, image.naturalHeight) === size) rendered.push(path)
    }
    return { visible, rendered, size, directory }
  })
}

for (const [index, viewport] of [{ width: 1280, height: 900 }, { width: 390, height: 844 }].entries()) {
  test(`Real thumbnail workload performance at ${viewport.width}px`, async ({ page }) => {
    test.setTimeout(360_000)
    const fixture = validatePerformanceFixture(input.performance)
    const user = input.users[index]
    const output = join(input.output, String(viewport.width)); mkdirSync(output)
    const phases: Phase[] = [], requests: Network[] = [], records = new Map<Request, Network>(), io: unknown[] = [], pageErrors: string[] = []
    let activePhase: Phase | undefined, passed = false, failure: string | undefined
    let activeSample: (() => Promise<Visible>) | undefined
    let nativeTimings: Native[] = []
    page.setDefaultTimeout(20000)
    await page.setViewportSize(viewport)
    await page.addInitScript(({ rangePath }) => {
      type Observed = { url: string; start: number; abortAt?: number; used: boolean; signal?: AbortSignal; abort?: () => void }
      const observed: Observed[] = [], native: Native[] = [], ranges: RangeNative[] = []
      ;(window as unknown as { __thumbnailPerformanceNative: Native[] }).__thumbnailPerformanceNative = native
      ;(window as unknown as { __thumbnailPerformanceRange: RangeNative[] }).__thumbnailPerformanceRange = ranges
      const original = window.fetch.bind(window)
      // Observe only. Keep the original request, Response and stream untouched.
      window.fetch = (resource, options) => {
        const url = new URL(resource instanceof Request ? resource.url : String(resource), location.href)
        const method = options?.method ?? (resource instanceof Request ? resource.method : 'GET')
        if (method.toUpperCase() === 'GET' && url.pathname.endsWith('/workspace/thumbnail')) {
          const value: Observed = { url: url.href, start: performance.timeOrigin + performance.now(), used: false }
          const signal = options?.signal ?? (resource instanceof Request ? resource.signal : undefined)
          if (signal) {
            value.signal = signal; value.abort = () => { value.abortAt = performance.timeOrigin + performance.now() }
            if (signal.aborted) value.abort(); else signal.addEventListener('abort', value.abort, { once: true })
          }
          observed.push(value)
        }
        return original(resource, options)
      }
      performance.setResourceTimingBufferSize(10000)
      new PerformanceObserver(list => {
        for (const entry of list.getEntries() as PerformanceResourceTiming[]) {
          const url = new URL(entry.name)
          if (url.pathname.endsWith('/workspace/content') && url.searchParams.get('path') === rangePath) {
            ranges.push({ url: url.href, start: performance.timeOrigin + entry.startTime, end: performance.timeOrigin + entry.responseEnd })
            continue
          }
          if (!url.pathname.endsWith('/workspace/thumbnail') || !url.searchParams.has('path')) continue
          const start = performance.timeOrigin + entry.startTime
          const value = observed.filter(item => !item.used && item.url === url.href && item.start <= start).sort((a, b) => b.start - a.start)[0]
          if (value) {
            value.used = true
            if (value.signal && value.abort) value.signal.removeEventListener('abort', value.abort)
          }
          native.push({ path: url.searchParams.get('path')!, size: Number(url.searchParams.get('size')), start, end: performance.timeOrigin + entry.responseEnd, abortAt: value?.abortAt, fetchObserved: !!value })
        }
      }).observe({ type: 'resource', buffered: true })
    }, { rangePath: fixture.range.path })
    page.on('pageerror', error => pageErrors.push(error.message))
    page.on('request', request => {
      const url = new URL(request.url())
      if (!url.pathname.endsWith('/workspace/thumbnail')) return
      const record: Network = { method: request.method(), phase: activePhase?.name ?? 'setup', path: url.searchParams.get('path') ?? '', size: Number(url.searchParams.get('size')), startedAt: Date.now() }
      requests.push(record); records.set(request, record)
    })
    page.on('response', response => { const record = records.get(response.request()); if (record) record.status = response.status() })
    page.on('requestfinished', request => { const record = records.get(request); if (record) record.endedAt = Date.now() })
    page.on('requestfailed', request => {
      const record = records.get(request)
      if (record) { record.endedAt = Date.now(); record.failed = request.failure()?.errorText; record.cancelled = record.failed === 'net::ERR_ABORTED' }
    })
    const collection = page.locator('[data-cy="workspace-file-collection"]')
    async function settleVisible(sample = activeSample) {
      if (!sample) throw new Error('Missing active phase sampler')
      await expect.poll(async () => {
        const observed = await sample()
        return observed.visible.length > 0 && observed.visible.every(path => observed.rendered.includes(path))
      }, { timeout: 60000, intervals: [50, 100, 200] }).toBe(true)
    }
    async function phase(name: string, expectedSize: 128 | 256, action: () => Promise<void>) {
      const value: Phase = { name, startedAt: Date.now(), endedAt: Date.now(), visiblePaths: [], renderedPaths: [] }
      phases.push(value); activePhase = value
      try {
        await runSampledPhase({ phase: value, directory: fixture.directory, expectedSize, read: () => visibleMedia(page),
          action: async sample => { activeSample = sample; await action() }, settle: settleVisible })
      } finally { activeSample = undefined; activePhase = undefined }
    }
    async function top() { await collection.evaluate(element => { element.scrollTop = 0 }); await collection.scrollIntoViewIfNeeded() }
    async function scrollToBottom() {
      for (let step = 0; step < 80; step++) {
        await settleVisible()
        const bottom = await collection.evaluate(element => {
          if (element.scrollTop + element.clientHeight >= element.scrollHeight - 2) return true
          element.scrollTop = Math.min(element.scrollHeight - element.clientHeight, element.scrollTop + Math.max(1, Math.floor(element.clientHeight * 0.7)))
          return false
        })
        if (bottom) return
      }
      throw new Error('Performance scroll did not reach directory end')
    }
    try {
      await page.goto('/admin/')
      await page.locator('[data-cy="login-username"]').fill('admin')
      await page.locator('[data-cy="login-password"]').fill(process.env.MEASIX_E2E_ADMIN_PASSWORD!)
      await page.locator('[data-cy="login-submit"]').click()
      await expect(page).toHaveURL(/\/admin\/(overview)?$/)
      await page.goto('/admin/remote-workspaces')
      await page.locator('.q-list > .q-item').filter({ hasText: user.displayName }).first().click()
      await page.getByRole('button', { name: '浏览文件', exact: true }).click()
      await expect(collection).toBeVisible()
      await phase('cold-list-first-viewport', 128, async () => {
        await page.getByRole('button', { name: '打开目录 ' + fixture.directory, exact: true }).click()
        await expect(collection.locator('[data-cy="workspace-file-entry"]')).toHaveCount(43)
        await top()
      })
      await phase('list-scroll-to-bottom', 128, scrollToBottom)
      await phase('list-return-top', 128, top)
      await page.screenshot({ path: join(output, 'performance-list.png'), fullPage: true })
      let rangeProof: { url: string; status: number; bytes: number; sha256: string; contentRange: string | null } | undefined
      await phase('cold-cards-with-files-range', 256, async () => {
        const firstRead = page.waitForRequest(request => request.method() === 'GET' && new URL(request.url()).pathname.endsWith('/workspace/thumbnail') && new URL(request.url()).searchParams.get('size') === '256')
        await page.locator('[data-cy="workspace-view-thumbnails"]').click()
        await firstRead
        const query = new URLSearchParams({ agentSpaceId: user.spaceId, path: fixture.range.path })
        rangeProof = await page.evaluate(async ({ url, range }) => {
          const response = await fetch(url, { credentials: 'same-origin', cache: 'no-store', headers: { Range: range } })
          const bytes = await response.arrayBuffer()
          const sha256 = [...new Uint8Array(await crypto.subtle.digest('SHA-256', bytes))].map(value => value.toString(16).padStart(2, '0')).join('')
          return { url, status: response.status, bytes: bytes.byteLength, sha256, contentRange: response.headers.get('Content-Range') }
        }, { url: `${input.baseURL}/api/admin/v1/users/${user.userId}/workspace/content?${query}`, range: `bytes=${fixture.range.start}-${fixture.range.start + fixture.range.length - 1}` })
        expect(rangeProof.status).toBe(206)
        expect(rangeProof.bytes).toBe(fixture.range.length)
        expect(rangeProof.sha256).toBe(fixture.range.sha256)
        expect(rangeProof.contentRange).toBe(`bytes ${fixture.range.start}-${fixture.range.start + fixture.range.length - 1}/${fixture.range.bytes}`)
        await top()
      })
      // Wait for the ResourceTiming observer after the images and Range body
      // finish. No Playwright callback interval establishes native IO overlap.
      await expect.poll(() => page.evaluate(url => (window as unknown as { __thumbnailPerformanceRange: RangeNative[] }).__thumbnailPerformanceRange.filter(value => value.url === url).length, rangeProof!.url)).toBe(1)
      const observed = await page.evaluate(() => ({ ranges: (window as unknown as { __thumbnailPerformanceRange: RangeNative[] }).__thumbnailPerformanceRange, thumbnails: (window as unknown as { __thumbnailPerformanceNative: Native[] }).__thumbnailPerformanceNative }))
      const rangeTiming = observed.ranges.find(value => value.url === rangeProof!.url)!
      const thumbnailReadIntervals = observed.thumbnails.filter(value => value.size === 256 && value.path.startsWith(fixture.directory + '/')).map(thumbnailReadInterval)
      const overlap = positiveTransferOverlap(rangeTiming, thumbnailReadIntervals)
      io.push({ kind: 'native Files Range alongside active thumbnail reads', ...rangeProof!, expectedSHA256: fixture.range.sha256, elapsedMs: rangeTiming.end - rangeTiming.start, rangeNativeInterval: rangeTiming, thumbnailReadIntervals, ...overlap })
      expect(overlap.overlappingThumbnailRequests).toBeGreaterThan(0)
      expect(overlap.overlapMs).toBeGreaterThan(0)
      await phase('cards-scroll-to-bottom', 256, scrollToBottom)
      await phase('cards-return-top', 256, top)
      await phase('return-list-128', 128, async () => { await page.locator('[data-cy="workspace-view-list"]').click(); await top() })
      await phase('return-cards-256', 256, async () => { await page.locator('[data-cy="workspace-view-thumbnails"]').click(); await top() })
      const seen = new Set(phases.flatMap(value => value.renderedPaths))
      expect(fixture.entries.every((entry: { path: string }) => seen.has(entry.path))).toBe(true)
      expect(requests.filter(value => value.method === 'OPTIONS')).toHaveLength(1)
      expect(pageErrors).toEqual([])
      await page.screenshot({ path: join(output, 'performance-cards.png'), fullPage: true })
      nativeTimings = await page.evaluate(() => (window as unknown as { __thumbnailPerformanceNative: Native[] }).__thumbnailPerformanceNative)
      expect(nativeTimings.length).toBeGreaterThan(0)
      expect(nativeTimings.every(value => value.fetchObserved)).toBe(true)
      expect(maximumConcurrentTransfers(nativeTimings.map(thumbnailReadInterval))).toBeLessThanOrEqual(2)
      passed = true
    } catch (error) {
      failure = error instanceof Error ? error.message : String(error)
      throw error
    } finally {
      if (!page.isClosed()) nativeTimings = await page.evaluate(() => (window as unknown as { __thumbnailPerformanceNative?: Native[] }).__thumbnailPerformanceNative ?? []).catch(() => [])
      const reads = nativeTimings.map(thumbnailReadInterval)
      const report = { schemaVersion: 1, passed, failure, viewport, servedAdminBuild: input.adminBuild, workload: fixture, summary: summarizePerformance({ phases, requests }), io, maximumActiveReads: maximumConcurrentTransfers(reads), maximumNativeNetwork: maximumConcurrentTransfers(nativeTimings), phases, requests, nativeTimings, pageErrors, limits: ['Observed UI/Core requests and rendered visibility only; no decoder task count or cache-hit inference', 'Browser-event duration uses Playwright request/finished clocks; IO overlap uses native Range ResourceTiming and abort-capped thumbnail intervals', 'Phase sampling starts before its action, requests 100 ms intervals plus explicit scroll checks, and retains sample timestamps; scheduling and DOM roundtrips may delay observations', 'First/all-visible timings include UI and network work and have no absolute latency gate', 'Generated gradients/geometry exercise dimensions and formats, not a photographic corpus'] }
      writeFileSync(join(output, 'performance.json'), JSON.stringify(report, null, 2) + '\n', { flag: 'wx' })
    }
  })
}
