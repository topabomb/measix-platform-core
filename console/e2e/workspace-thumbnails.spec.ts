import { test, expect, type Page, type Request } from '@playwright/test'
import { readFileSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'
import { randomUUID } from 'node:crypto'
import { maximumConcurrentTransfers, thumbnailReadInterval, holdPreview } from '../../scripts/workspace-thumbnail-browser.mjs'

// Only the dedicated shared-harness entry supplies these isolated real peers.
// Successful thumbnail/file traffic is never replaced by page.route fixtures.
test.use({ trace: 'off', video: 'off' })
const inputPath = process.env.MEASIX_THUMBNAIL_BROWSER_INPUT
const input = inputPath ? JSON.parse(readFileSync(inputPath, 'utf8')) : undefined
test.skip(!input, 'Run through scripts/workspace-thumbnail-browser.mjs')

type User = { userId: string; displayName: string; spaceId: string }
type Network = { method: string; path: string; size: number; spaceId: string; started: number; status?: number; failed?: string; ended?: number }
type NativeTiming = {path:string;size:number;spaceId:string;start:number;end:number;networkEnd?:number;abortAt?:number;fetchObserved?:boolean}
type Audit = { created: { url: string; type: string }[]; revoked: string[]; live: string[] }
const pause = (ms: number) => new Promise(resolve => setTimeout(resolve, ms))

async function openSpace(page: Page, user: User) {
  await page.goto('/admin/remote-workspaces')
  await page.locator('.q-list > .q-item').filter({ hasText: user.displayName }).first().click()
  await page.getByRole('button', { name: '浏览文件', exact: true }).click()
  await expect(page.locator('[data-cy="workspace-files"]')).toBeVisible()
  await page.locator('.workspace-file-list').scrollIntoViewIfNeeded()
}
const image = (page: Page, path: string) => page.locator(`[data-thumbnail-path="${path}"] img`)
async function decoded(page: Page, path: string) {
  await expect(image(page, path)).toBeVisible()
  await expect.poll(() => image(page, path).evaluate((element: HTMLImageElement) => element.naturalWidth)).toBeGreaterThan(0)
}
async function audit(page: Page): Promise<Audit> {
  return page.evaluate(() => {
    const state = (window as unknown as { __thumbnailURLs: { created: { url: string; type: string }[]; revoked: string[]; live: Set<string> } }).__thumbnailURLs
    return { created: state.created, revoked: state.revoked, live: [...state.live] }
  })
}
async function menu(page: Page, path: string, action: string) {
  await page.getByRole('button', { name: path + ' 操作', exact: true }).click()
  await page.getByText(action, { exact: true }).last().click()
}

for (const [index, viewport] of [{ width: 1280, height: 900 }, { width: 390, height: 844 }].entries()) {
  test(`Real workspace thumbnails, cancellation and original file flows at ${viewport.width}px`, async ({ page }, testInfo) => {
    test.setTimeout(360_000)
    const user: User = input.users[index], other: User = input.users[1 - index]
    const network: Network[] = [], active = new Set<Request>(), records = new Map<Request, Network>(), errors: string[] = []
    const nativeTimings: NativeTiming[] = []
    const heldResponses:unknown[]=[]
    let failedNavigation:unknown
    const hold=(target:Page,path:string,size?:number)=>holdPreview(target,path,size,heldResponses)
    const checks: string[] = []
    const checked = (name: string) => { checks.push(name); console.log(`PASS ${viewport.width}px ${name}`) }
    let maximumActive = 0
    let directoryReads = 0, originalReads = 0, browserWrites = 0
    const path = '/api/admin/v1/users/' + user.userId + '/workspace'
    const output = join(input.output, String(viewport.width))
    const { mkdirSync } = await import('node:fs'); mkdirSync(output)
    await page.setViewportSize(viewport)
    page.setDefaultTimeout(15000)
    await page.exposeFunction('__recordThumbnailTiming', (value: NativeTiming) => { nativeTimings.push(thumbnailReadInterval(value)) })
    await page.addInitScript(() => {
      const state = { created: [] as { url: string; type: string }[], revoked: [] as string[], live: new Set<string>() }
      ;(window as unknown as { __thumbnailURLs: typeof state }).__thumbnailURLs = state
      const create = URL.createObjectURL.bind(URL), revoke = URL.revokeObjectURL.bind(URL)
      URL.createObjectURL = blob => { const url = create(blob); if (blob instanceof Blob && blob.type === 'image/webp') { state.created.push({ url, type: blob.type }); state.live.add(url) }; return url }
      URL.revokeObjectURL = url => { state.revoked.push(url); state.live.delete(url); revoke(url) }
      const fetches:{url:string;started:number;abortAt?:number;used:boolean;signal?:AbortSignal;listener?:()=>void}[]=[]
      const originalFetch=window.fetch.bind(window)
      // Observation only: preserve the original promise/response/stream. Abort
      // ends application reading even if native socket teardown finishes later.
      window.fetch=(resource,options)=>{
        const url=new URL(resource instanceof Request?resource.url:String(resource),location.href)
        const method=options?.method??(resource instanceof Request?resource.method:'GET')
        if(method.toUpperCase()==='GET'&&url.pathname.endsWith('/workspace/thumbnail')&&url.searchParams.has('path')){
          const value:{url:string;started:number;abortAt?:number;used:boolean;signal?:AbortSignal;listener?:()=>void}={url:url.href,started:performance.timeOrigin+performance.now(),used:false}
          const signal=options?.signal??(resource instanceof Request?resource.signal:undefined)
          if(signal){value.signal=signal;value.listener=()=>{value.abortAt=performance.timeOrigin+performance.now()};if(signal.aborted)value.listener();else signal.addEventListener('abort',value.listener,{once:true})}
          fetches.push(value)
        }
        return originalFetch(resource,options)
      }
      const uiEvents:unknown[]=[]
      ;(window as unknown as {__thumbnailUIEvents:unknown[]}).__thumbnailUIEvents=uiEvents
      for(const type of ['input','change','blur','click'])document.addEventListener(type,event=>{
        const target=event.target instanceof Element?event.target:undefined
        if(!target||!target.closest('.workspace-file-navigation,[data-cy="workspace-view-list"],[data-cy="workspace-view-thumbnails"]'))return
        uiEvents.push({type,at:performance.timeOrigin+performance.now(),tag:target.tagName,aria:target.getAttribute('aria-label'),text:target.textContent?.slice(0,40),viewButton:target.closest('[data-cy^="workspace-view-"]')?.getAttribute('data-cy'),filter:(document.querySelector('[aria-label="筛选当前目录"]') as HTMLInputElement|null)?.value,view:document.querySelector('[data-cy="workspace-file-collection"]')?.getAttribute('data-view')})
      },true)
      performance.setResourceTimingBufferSize(10000)
      new PerformanceObserver(list => {
        for (const value of list.getEntries() as PerformanceResourceTiming[]) {
          const url = new URL(value.name)
          if (!url.pathname.endsWith('/workspace/thumbnail') || !url.searchParams.has('path')) continue
          const start=performance.timeOrigin+value.startTime
          const observed=fetches.filter(item=>!item.used&&item.url===url.href&&item.started<=start).sort((a,b)=>b.started-a.started)[0]
          if(observed){observed.used=true;if(observed.signal&&observed.listener)observed.signal.removeEventListener('abort',observed.listener)}
          const callback = (window as unknown as { __recordThumbnailTiming:(value:{path:string;size:number;spaceId:string;start:number;end:number;abortAt?:number;fetchObserved:boolean})=>Promise<void> }).__recordThumbnailTiming
          void callback({path:url.searchParams.get('path')!,size:Number(url.searchParams.get('size')),spaceId:url.searchParams.get('agentSpaceId')!,start,end:performance.timeOrigin+value.responseEnd,abortAt:observed?.abortAt,fetchObserved:!!observed}).catch(()=>{})
        }
      }).observe({type:'resource',buffered:true})
    })
    page.on('pageerror', error => errors.push(error.message))
    page.on('request', request => {
      const url = new URL(request.url())
      if (request.method()==='GET' && url.pathname.endsWith('/workspace/files')) directoryReads++
      if (request.method()==='GET' && url.pathname.endsWith('/workspace/content')) originalReads++
      if (request.method()==='PUT' && url.pathname.endsWith('/workspace/content') || request.method()==='POST' && url.pathname.endsWith('/workspace/files')) browserWrites++
      if (!url.pathname.endsWith('/workspace/thumbnail')) return
      const record: Network = { method: request.method(), path: url.searchParams.get('path') ?? '', size:Number(url.searchParams.get('size')), spaceId: url.searchParams.get('agentSpaceId') ?? '', started: Date.now() }
      network.push(record); records.set(request, record)
      if (request.method() === 'GET') { active.add(request); maximumActive = Math.max(maximumActive, active.size) }
    })
    page.on('response', response => { const record = records.get(response.request()); if (record) record.status = response.status() })
    page.on('requestfinished', request => { active.delete(request); const record = records.get(request); if (record) record.ended = Date.now() })
    page.on('requestfailed', request => { active.delete(request); const record = records.get(request); if (record) { record.failed = request.failure()?.errorText; record.ended = Date.now() } })
    await page.goto('/admin/')
    await page.locator('[data-cy="login-username"]').fill('admin')
    await page.locator('[data-cy="login-password"]').fill(process.env.MEASIX_E2E_ADMIN_PASSWORD!)
    await page.locator('[data-cy="login-submit"]').click()
    await expect(page).toHaveURL(/\/admin\/(overview)?$/)
    const sessionResponse = await page.request.get(input.baseURL + '/api/admin/v1/session')
    const session = await sessionResponse.json()
    async function api(resource: string, method = 'GET', body?: unknown) {
      const response = await page.request.fetch(input.baseURL + resource, { method, headers: { Origin: input.baseURL, 'X-CSRF-Token': session.csrfToken, ...(body === undefined ? {} : { 'Content-Type': 'application/json' }), ...(method === 'POST' ? { 'Idempotency-Key': 'idem_' + randomUUID() } : {}) }, ...(body === undefined ? {} : { data: JSON.stringify(body) }) })
      expect(response.ok(), `${resource}: ${response.status()} ${await response.text()}`).toBe(true)
      const text = await response.text(); return text ? JSON.parse(text) : undefined
    }
    const file = (name: string) => path + '/content?' + new URLSearchParams({ agentSpaceId: user.spaceId, path: name })
    async function command(action: string) {
      const view = await api(path)
      const operation = await api(path, 'POST', { action, expectedRevision: view.bindingRevision, ...(action === 'DELETE' ? { confirmation: view.agentSpaceId } : {}) })
      await expect.poll(async () => (await api('/api/admin/v1/workspace-operations/' + operation.operationId)).state, { timeout: 120000 }).toBe('COMPLETED')
    }
    async function put(name: string, bytes: Buffer, etag?: string) {
      const response = await page.request.put(input.baseURL + file(name), { headers: { Origin: input.baseURL, 'X-CSRF-Token': session.csrfToken, ...(etag ? { 'If-Match': etag } : { 'If-None-Match': '*' }) }, data: bytes })
      expect(response.status(), await response.text()).toBe(200)
    }
    try {
      await openSpace(page, user)
      await decoded(page, '00-image.png'); await decoded(page, '01-video.mp4')
      const imageDimensions = await image(page, '00-image.png').evaluate((element: HTMLImageElement) => [element.naturalWidth, element.naturalHeight])
      expect(imageDimensions).toEqual([128, 32])
      expect(await image(page, '01-video.mp4').evaluate((element: HTMLImageElement) => [element.naturalWidth, element.naturalHeight])).toEqual([128, 72])
      expect(network.filter(record => record.method === 'GET').some(record => record.path === '29-image.png')).toBe(false)
      for(const name of ['00-image.png','01-video.mp4']) {
        await expect.poll(() => nativeTimings.some(value=>value.path===name&&value.spaceId===user.spaceId&&value.size===128&&value.end>value.start)).toBe(true)
      }
      expect(maximumConcurrentTransfers(nativeTimings)).toBeLessThanOrEqual(2)
      expect(network.filter(record => record.method === 'OPTIONS')).toHaveLength(1)
      expect(network.filter(record => record.method === 'GET').every(record => /\.(png|mp4|webm)$/.test(record.path))).toBe(true)
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
      await page.screenshot({ path: join(output, 'thumbnails.png'), fullPage: true })
      checked('real PNG/video guest-generated WebP decoded; bounded lazy visible demand and <=2 concurrent GETs')

      const collection = page.locator('[data-cy="workspace-file-collection"]')
      const listView = page.locator('[data-cy="workspace-view-list"]')
      const thumbnailView = page.locator('[data-cy="workspace-view-thumbnails"]')
      const search = page.getByLabel('筛选当前目录', { exact: true })
      await expect(collection).toHaveAttribute('data-view','list')
      await expect(listView).toHaveAttribute('aria-pressed','true')
      const beforeViewDirectoryReads=directoryReads, beforeViewOriginalReads=originalReads
      // Cancel a real 128px result held in flight before switching demand size.
      await search.fill('__no_matching_entry__')
      const oldSize = await hold(page,'00-image.png',128)
      await search.fill('00-image');await oldSize.ready
      await thumbnailView.click()
      await expect(collection).toHaveAttribute('data-view','thumbnails')
      await expect(thumbnailView).toHaveAttribute('aria-pressed','true')
      await expect(search).toHaveValue('00-image')
      await expect.poll(()=>network.some(record=>record.path==='00-image.png'&&record.size===128&&!!record.failed)).toBe(true)
      await decoded(page,'00-image.png')
      expect(await image(page,'00-image.png').evaluate((element:HTMLImageElement)=>[element.naturalWidth,element.naturalHeight])).toEqual([256,64])
      await oldSize.release()
      await page.waitForLoadState('networkidle')
      expect(await image(page,'00-image.png').evaluate((element:HTMLImageElement)=>element.naturalWidth)).toBe(256)
      await search.fill('')
      await page.locator('[data-thumbnail-path="01-video.mp4"]').scrollIntoViewIfNeeded()
      await decoded(page,'01-video.mp4')
      expect(await image(page,'01-video.mp4').evaluate((element:HTMLImageElement)=>[element.naturalWidth,element.naturalHeight])).toEqual([256,144])
      await expect(collection.locator('[data-file-path="01-video.mp4"] [data-cy="workspace-video-badge"]')).toBeVisible()
      await expect(collection.locator('[data-file-path="01-video.mp4"] [data-cy="workspace-video-badge"]')).toContainText('视频')
      const largeBox=await page.locator('[data-thumbnail-path="01-video.mp4"]').boundingBox()
      expect(largeBox?.width).toBeGreaterThanOrEqual(100)
      expect(largeBox?.height).toBeGreaterThanOrEqual(64)
      for(const name of ['00-image.png','01-video.mp4']) {
        await expect.poll(()=>nativeTimings.some(value=>value.path===name&&value.spaceId===user.spaceId&&value.size===256&&value.end>value.start)).toBe(true)
      }
      expect(directoryReads).toBe(beforeViewDirectoryReads)
      expect(originalReads).toBe(beforeViewOriginalReads)
      expect(network.filter(record=>record.method==='OPTIONS')).toHaveLength(1)
      expect(maximumConcurrentTransfers(nativeTimings)).toBeLessThanOrEqual(2)
      expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
      await page.locator('.workspace-file-toolbar').evaluate(element=>{window.scrollTo(0,Math.max(0,element.getBoundingClientRect().top+scrollY-64))})
      await collection.evaluate(element=>{element.scrollTop=0})
      await expect.poll(()=>collection.evaluate(container=>{
        const bounds=container.getBoundingClientRect(),left=Math.max(0,bounds.left),right=Math.min(innerWidth,bounds.right),top=Math.max(0,bounds.top),bottom=Math.min(innerHeight,bounds.bottom)
        const visible=[...container.querySelectorAll<HTMLElement>('[data-thumbnail-path]')].filter(slot=>{
          if(!/\.(png|mp4|webm)$/.test(slot.dataset.thumbnailPath??''))return false
          const rect=slot.getBoundingClientRect();return rect.right>left&&rect.left<right&&rect.bottom>top&&rect.top<bottom
        })
        return visible.length>0&&visible.every(slot=>{const image=slot.querySelector<HTMLImageElement>('img');return image?.complete&&image.naturalWidth>0})
      }),{timeout:30000}).toBe(true)
      await page.screenshot({path:join(output,'thumbnail-cards.png'),fullPage:true})
      await page.screenshot({path:join(output,'gallery.png')})
      const cardOpen=collection.getByRole('button',{name:'预览 00-image.png',exact:true})
      await cardOpen.click();await expect(page.locator('.workspace-preview .image-preview')).toBeVisible()
      await page.getByRole('button',{name:'关闭预览',exact:true}).click()
      await cardOpen.focus();await cardOpen.press('Enter')
      await expect(page.locator('.workspace-preview .image-preview')).toBeVisible()
      await page.getByRole('button',{name:'关闭预览',exact:true}).click()
      await menu(page,'notes.txt','复制')
      await expect(page.getByLabel('目标完整相对路径（从根目录起）',{exact:true})).toBeVisible()
      await page.getByRole('dialog').getByRole('button',{name:'取消',exact:true}).click()
      await expect(page.getByRole('dialog')).toHaveCount(0)
      await search.fill('01-video')
      await expect(collection.locator('[data-cy="workspace-file-entry"]')).toHaveCount(2)
      await expect(search).toHaveValue('01-video')
      await listView.click();await expect(search).toHaveValue('01-video')
      await thumbnailView.click();await expect(search).toHaveValue('01-video')
      await listView.click();await expect(collection).toHaveAttribute('data-view','list')
      expect(directoryReads).toBe(beforeViewDirectoryReads)
      expect(network.filter(record=>record.method==='OPTIONS')).toHaveLength(1)
      await search.fill('');await decoded(page,'00-image.png')
      checked('list128/cards256 switch preserves filter and directory without new OPTIONS/original reads; real dimensions, video badge, pointer/keyboard/menu and stale-size cancellation pass')

      // A real deleted destination fails after navigation already cancelled the
      // source directory's IO. Its retained list must rebuild visible demand.
      const beforeNavigationSource=(await page.request.head(input.baseURL+file('00-image.png'))).headers().etag
      await put('00-image.png',readFileSync(join(input.mediaDirectory,'image.png')),beforeNavigationSource)
      const currentNavigationSource=(await page.request.head(input.baseURL+file('00-image.png'))).headers().etag
      expect(currentNavigationSource).not.toBe(beforeNavigationSource)
      const cancelledNavigationStart=network.length
      const pendingNavigation=await hold(page,'00-image.png',128)
      await page.getByRole('button',{name:'刷新文件列表',exact:true}).click()
      await pendingNavigation.ready
      await page.locator('[data-thumbnail-path="01-video.mp4"]').scrollIntoViewIfNeeded()
      await decoded(page,'01-video.mp4')
      const videoBeforeFailedNavigation=await image(page,'01-video.mp4').getAttribute('src')
      const sourceListing=await api(path+'/files?'+new URLSearchParams({agentSpaceId:user.spaceId,path:''}))
      const vanished=sourceListing.entries.find((entry:{path:string;kind:string;etag?:string})=>entry.path==='vanishing'&&entry.kind==='DIRECTORY')
      expect(vanished).toBeTruthy()
      await api(path+'/files?'+new URLSearchParams({agentSpaceId:user.spaceId}),'POST',{action:'DELETE',path:'vanishing',sourceEtag:vanished.etag,recursiveConfirmed:true})
      const missingDestination=page.waitForResponse(response=>{
        const url=new URL(response.url())
        return response.request().method()==='GET'&&url.pathname.endsWith('/workspace/files')&&url.searchParams.get('agentSpaceId')===user.spaceId&&url.searchParams.get('path')==='vanishing'
      })
      await page.getByRole('button',{name:'打开目录 vanishing',exact:true}).click()
      const failedDestination=await missingDestination
      expect(failedDestination.status()).toBe(404)
      const destinationProblem=await failedDestination.json()
      expect(destinationProblem.code).toBe('file_not_found')
      await expect.poll(()=>network.slice(cancelledNavigationStart).some(record=>record.path==='00-image.png'&&record.size===128&&!!record.failed)).toBe(true)
      await pendingNavigation.release()
      await expect(page.locator('[data-cy="workspace-files"] [role="alert"]')).toContainText('文件已不存在')
      await expect(page.locator('.workspace-file-navigation .text-caption')).toHaveText('/')
      for(const name of ['nested','vanishing','00-image.png','01-video.mp4'])await expect(collection.locator(`[data-file-path="${name}"]`)).toHaveCount(1)
      await page.locator('[data-thumbnail-path="00-image.png"]').scrollIntoViewIfNeeded();await decoded(page,'00-image.png')
      await page.locator('[data-thumbnail-path="01-video.mp4"]').scrollIntoViewIfNeeded();await decoded(page,'01-video.mp4')
      expect((await audit(page)).revoked).toContain(videoBeforeFailedNavigation)
      expect(await image(page,'01-video.mp4').getAttribute('src')).not.toBe(videoBeforeFailedNavigation)
      expect(network.filter(record=>record.method==='OPTIONS')).toHaveLength(1)
      const afterFailedNavigationSource=(await page.request.head(input.baseURL+file('00-image.png'))).headers().etag
      expect(afterFailedNavigationSource).toBe(currentNavigationSource)
      failedNavigation={fromDirectory:'',requestedDirectory:'vanishing',currentDirectory:'',spaceId:user.spaceId,status:failedDestination.status(),code:destinationProblem.code,sourceETagBeforeRefresh:beforeNavigationSource,sourceETagAtNavigation:currentNavigationSource,sourceETagAfterFailure:afterFailedNavigationSource,oldPreviewCancelled:true,optionsRequests:network.filter(record=>record.method==='OPTIONS').length,visibleSourceThumbnailsRestored:true}
      await page.screenshot({path:join(output,'failed-navigation-restored.png'),fullPage:true})
      checked('real deleted-directory 404 retains source list/error, restores visible source thumbnails, cancels old IO and reuses capability')

      const firstURL = await image(page, '00-image.png').getAttribute('src')
      await page.getByRole('button', { name: '打开目录 nested', exact: true }).click()
      await decoded(page, 'nested/inside.png')
      expect((await audit(page)).revoked).toContain(firstURL)
      await page.getByRole('button', { name: '根目录', exact: true }).click()
      await decoded(page, '00-image.png')
      expect(network.filter(record => record.method === 'OPTIONS')).toHaveLength(1)
      checked('directory navigation reuses one capability observation and revokes old object URLs')

      const versionURL = await image(page, '00-image.png').getAttribute('src')
      const head = await page.request.head(input.baseURL + file('00-image.png'))
      const oldETag = head.headers().etag
      await put('00-image.png', readFileSync(join(input.mediaDirectory, 'changed.png')), oldETag)
      await page.getByRole('button', { name: '刷新文件列表', exact: true }).click()
      await expect.poll(() => image(page, '00-image.png').getAttribute('src')).not.toBe(versionURL)
      await decoded(page, '00-image.png')
      expect((await audit(page)).revoked).toContain(versionURL)
      const current = await page.request.head(input.baseURL + file('00-image.png'))
      expect(current.headers().etag).not.toBe(oldETag)
      checked('real source ETag replacement evicts the previous thumbnail')

      await page.getByRole('button', { name: '预览 00-image.png', exact: true }).click()
      await expect(page.locator('.workspace-preview .image-preview')).toBeVisible()
      expect(await page.locator('.workspace-preview .image-preview').evaluate((element: HTMLImageElement) => element.naturalWidth)).toBe(320)
      const downloadPromise = page.waitForEvent('download')
      await page.locator('.workspace-preview').getByRole('button', { name: '下载', exact: true }).click()
      const download = await downloadPromise
      expect(download.suggestedFilename()).toBe('00-image.png')
      await download.saveAs(join(output, 'original-image.png'))
      expect(readFileSync(join(output, 'original-image.png'))).toEqual(readFileSync(join(input.mediaDirectory, 'changed.png')))
      await page.getByRole('button', { name: '关闭预览', exact: true }).click()
      const h264 = await page.evaluate(() => document.createElement('video').canPlayType('video/mp4;codecs="avc1.42E01E"'))
      await page.getByRole('button', { name: '预览 01-video.mp4', exact: true }).click()
      if (!h264) await expect(page.locator('.workspace-preview')).toContainText('视频无法播放')
      else await expect.poll(() => page.locator('.workspace-preview video').evaluate((element: HTMLVideoElement) => element.readyState)).toBeGreaterThan(0)
      await page.getByRole('button', { name: '关闭预览', exact: true }).click()
      await page.getByRole('button', { name: '预览 01-video.webm', exact: true }).click()
      await expect(page.locator('.workspace-preview video')).toBeVisible()
      await expect.poll(() => page.locator('.workspace-preview video').evaluate((element: HTMLVideoElement) => element.readyState)).toBeGreaterThan(0)
      const videoURL = await page.locator('.workspace-preview video').getAttribute('src')
      expect(new URL(videoURL!, input.baseURL).searchParams.get('preview')).toBe('video')
      expect(new URL(videoURL!, input.baseURL).searchParams.get('expectedEtag')).toMatch(/^".+"$/)
      await page.getByRole('button', { name: '关闭预览', exact: true }).click()
      checked('original image preview/download bytes, fixed-version WebM playback, and capability-appropriate MP4 playback/fallback remain available (avc1=' + JSON.stringify(h264) + ')')

      await menu(page, 'notes.txt', '编辑文本')
      const editor = page.locator('.workspace-text-editor')
      await expect(editor.getByLabel('文本内容', { exact: true })).toHaveValue('original text\n')
      await editor.getByLabel('文本内容', { exact: true }).fill('saved by real browser\n')
      await editor.getByRole('button', { name: '保存并关闭', exact: true }).click()
      await expect(editor).toHaveCount(0)
      const saved = await page.request.get(input.baseURL + file('notes.txt'))
      expect(await saved.body()).toEqual(Buffer.from('\uFEFFsaved by real browser\r\n'))
      await page.getByRole('button', { name: '预览 sample.pdf', exact: true }).click()
      await expect(page.locator('.workspace-preview canvas')).toBeVisible()
      expect(await page.locator('.workspace-preview canvas').evaluate((element: HTMLCanvasElement) => element.width)).toBeGreaterThan(0)
      await page.getByRole('button', { name: '关闭预览', exact: true }).click()
      await page.getByRole('button',{name:'预览 readme.md',exact:true}).click()
      await expect(page.locator('.markdown-preview h1')).toHaveText('Real Markdown')
      await expect(page.locator('.markdown-preview img')).toHaveAttribute('src',/^blob:/)
      await expect(page.locator('.markdown-preview script')).toHaveCount(0)
      expect(await page.evaluate(()=>(globalThis as unknown as {__workspaceExecuted?:boolean}).__workspaceExecuted)).toBeUndefined()
      await page.getByRole('button',{name:'关闭预览',exact:true}).click()
      for (const name of ['unsafe.html', 'unsafe.svg']) {
        await page.getByRole('button', { name: '预览 ' + name, exact: true }).click()
        await expect(page.locator('.workspace-preview')).toContainText('此类型不在网页中执行或预览')
        await expect(page.locator('.workspace-preview iframe,.workspace-preview svg,.workspace-preview img')).toHaveCount(0)
        expect(await page.evaluate(() => (globalThis as unknown as { __workspaceExecuted?: boolean }).__workspaceExecuted)).toBeUndefined()
        await page.getByRole('button', { name: '关闭预览', exact: true }).click()
      }
      await page.locator('[data-cy="workspace-files"] input[type="file"]').setInputFiles({name:'browser-upload.txt',mimeType:'text/plain',buffer:Buffer.from('real browser upload\n')})
      await page.getByRole('button',{name:'上传',exact:true}).click()
      await expect(page.getByRole('button',{name:'预览 browser-upload.txt',exact:true})).toBeVisible()
      expect(await (await page.request.get(input.baseURL+file('browser-upload.txt'))).body()).toEqual(Buffer.from('real browser upload\n'))
      checked('text conditional save preserves BOM/CRLF; PDF and safe local-image Markdown render; HTML/SVG stay download-only; browser upload keeps original bytes')

      await search.fill('__no_matching_entry__')
      const pending = await hold(page, '29-image.png')
      await search.fill('29-image'); await pending.ready
      await search.fill('')
      await expect.poll(() => network.some(record => record.path === '29-image.png' && !!record.failed)).toBe(true)
      await pending.release()
      expect(await image(page, '29-image.png').count()).toBe(0)
      checked('offscreen demand cancels a deliberately delayed real response and cannot paint it later')

      await search.fill('__no_matching_entry__')
      const hidden = await hold(page, '28-image.png')
      await search.fill('28-image'); await hidden.ready
      await page.evaluate(() => { Object.defineProperty(document, 'hidden', { value: true, configurable: true }); document.dispatchEvent(new Event('visibilitychange')) })
      await expect.poll(async () => (await audit(page)).live.length).toBe(0)
      await expect.poll(() => network.some(record => record.path === '28-image.png' && !!record.failed)).toBe(true)
      await hidden.release()
      await page.evaluate(() => { Reflect.deleteProperty(document, 'hidden'); document.dispatchEvent(new Event('visibilitychange')) })
      await search.fill(''); await decoded(page, '00-image.png')
      checked('visibilitychange with explicit document.hidden emulation cancels delayed demand and revokes all thumbnail URLs; native OS/tab hiding not claimed')

      await search.fill('__no_matching_entry__')
      const switched = await hold(page, '27-image.png')
      await search.fill('27-image'); await switched.ready
      await page.getByRole('button', { name: '返回空间列表', exact: true }).click()
      await expect.poll(() => network.some(record => record.path === '27-image.png' && !!record.failed)).toBe(true)
      await switched.release()
      expect((await audit(page)).live).toHaveLength(0)
      await page.locator('.q-list > .q-item').filter({ hasText: other.displayName }).first().click()
      await page.getByRole('button', { name: '浏览文件', exact: true }).click()
      await decoded(page, '00-image.png')
      const otherView = await api('/api/admin/v1/users/' + other.userId + '/workspace')
      expect(network.filter(record => record.method === 'GET').at(-1)?.spaceId).toBe(otherView.agentSpaceId)
      await openSpace(page, user); await decoded(page, '00-image.png')
      checked('real user/scope switch aborts delayed response and never reuses another space thumbnail URL')

      await menu(page, 'notes.txt', '编辑文本')
      const draft = 'unsaved text remains copyable after access loss'
      const writesBeforeDraft=browserWrites
      await expect(editor.getByLabel('文本内容', { exact: true })).toHaveValue('saved by real browser\n')
      await editor.getByLabel('文本内容', { exact: true }).fill(draft)
      await command('REVOKE_DAV')
      await expect(editor).toContainText('工作区文件访问已失效', { timeout: 15000 })
      await expect(editor.getByLabel('文本内容', { exact: true })).toHaveValue(draft)
      await expect(editor.getByLabel('文本内容', { exact: true })).toBeEnabled()
      await editor.getByLabel('文本内容', { exact: true }).selectText()
      expect(await editor.getByLabel('文本内容', { exact: true }).evaluate((element:HTMLTextAreaElement)=>element.selectionEnd-element.selectionStart)).toBe(draft.length)
      await expect(editor.getByRole('button', { name: '保存并关闭', exact: true })).toBeDisabled()
      expect(browserWrites).toBe(writesBeforeDraft)
      await expect.poll(async () => (await audit(page)).live.length).toBe(0)
      await page.screenshot({ path: join(output, 'unsaved-access-loss.png'), fullPage: true })
      await command('SET_DAV')
      await expect(editor).toContainText('文件访问已恢复', { timeout: 15000 })
      await expect(editor.getByRole('button', { name: '保存并关闭', exact: true })).toBeDisabled()
      await editor.getByRole('button', { name: '关闭', exact: true }).click()
      await page.getByRole('button', { name: '放弃修改', exact: true }).click()
      checked('real DAV revoke/reissue cancels thumbnails while unsaved text stays copyable and cannot submit blindly')

      // Same-user replacement exercises the actual parent/component boundary,
      // rather than changing props on a standalone component fixture.
      await menu(page, 'notes.txt', '编辑文本')
      await editor.getByLabel('文本内容', { exact: true }).fill(draft)
      const previousSpace = user.spaceId
      await command('DELETE'); await command('CREATE')
      user.spaceId = (await api(path)).agentSpaceId
      expect(user.spaceId).not.toBe(previousSpace)
      await expect(editor.getByLabel('文本内容', { exact: true })).toHaveValue(draft, { timeout: 15000 })
      await expect(editor.getByLabel('文本内容', { exact: true })).toBeEnabled()
      await editor.getByLabel('文本内容', { exact: true }).selectText()
      expect(await editor.getByLabel('文本内容', { exact: true }).evaluate((element:HTMLTextAreaElement)=>element.selectionEnd-element.selectionStart)).toBe(draft.length)
      await expect(editor.getByRole('button', { name: '保存并关闭', exact: true })).toBeDisabled()
      expect(browserWrites).toBe(writesBeforeDraft)
      await page.screenshot({ path: join(output, 'unsaved-space-replacement.png'), fullPage: true })
      await editor.getByRole('button', { name: '关闭', exact: true }).click()
      await page.getByRole('button', { name: '放弃修改', exact: true }).click()
      await put('00-image.png', readFileSync(join(input.mediaDirectory, 'image.png')))
      await put('notes.txt', Buffer.from('\uFEFFreplacement target\r\n'))
      await page.getByRole('button', { name: '刷新文件列表', exact: true }).click()
      await expect(page.getByRole('button', { name: 'notes.txt 操作', exact: true })).toBeVisible()
      checked('same-user space replacement retains old unsaved text, disables its save, and cannot submit into the new target')

      await menu(page, 'notes.txt', '复制')
      await expect(page.getByLabel('目标完整相对路径（从根目录起）', { exact: true })).toBeVisible()
      await command('DELETE'); await command('CREATE')
      user.spaceId = (await api(path)).agentSpaceId
      await expect(page.getByLabel('目标完整相对路径（从根目录起）', { exact: true })).toHaveCount(0, { timeout: 15000 })
      expect(browserWrites).toBe(writesBeforeDraft)
      await put('00-image.png', readFileSync(join(input.mediaDirectory, 'image.png')))
      await put('notes.txt', Buffer.from('\uFEFFreplacement target\r\n'))
      checked('old file operation confirmation is cleared on real space replacement')
      expect(errors).toEqual([])
      expect(nativeTimings.every(value=>value.fetchObserved)).toBe(true)
      expect(maximumConcurrentTransfers(nativeTimings)).toBeLessThanOrEqual(2)
    } finally {
      writeFileSync(join(output, 'browser-network.json'), JSON.stringify({ width: viewport.width, servedAdminBuild: input.adminBuild, checks,heldResponses,failedNavigation, activeReadMetric:'native resource timing capped at actual AbortSignal event; raw network teardown retained separately',maximumActive:maximumConcurrentTransfers(nativeTimings),maximumNativeNetwork:maximumConcurrentTransfers(nativeTimings.map(value=>({...value,end:value.networkEnd!}))), eventCallbackMaximum:maximumActive, nativeTimings, network, pageErrors: errors, uiEvents:await page.evaluate(()=>(window as unknown as {__thumbnailUIEvents?:unknown[]}).__thumbnailUIEvents).catch(()=>undefined),urls: await audit(page).catch(() => undefined) }, null, 2) + '\n', { flag: 'wx' })
      await testInfo.attach('network-and-resource-audit', { path: join(output, 'browser-network.json'), contentType: 'application/json' })
    }
  })
}
