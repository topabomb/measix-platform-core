import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { defineComponent, h, ref } from 'vue'
import { Quasar, QFile, QCheckbox, QBtn, QSeparator, QCardSection, QBanner, QLinearProgress, QList, QItem, QItemSection, QItemLabel, QIcon, QMenu, QDialog, QCard, QCardActions, QInput, QSpinner, ClosePopup } from 'quasar'
import WorkspaceFiles from '../components/WorkspaceFiles.vue'
import WorkspaceTextEditor from '../components/WorkspaceTextEditor.vue'
import * as client from '../api/client'
import { useSessionStore } from '../stores/session'
import { useWorkspaceThumbnails } from './workspaceThumbnails'
import type { components } from '../api/generated'
import supported from '../../../api/fixtures/workspace/thumbnail-capabilities-supported.json'
import unsupported from '../../../api/fixtures/workspace/thumbnail-capabilities-unsupported.json'

type Entry = components['schemas']['WorkspaceFileEntry']
const image = (path = 'photo.png', etag = '"source-1"'): Entry => ({ path, kind: 'FILE', etag })
const directory: Entry = { path: 'folder', kind: 'DIRECTORY' }
const space = 'spc_test'
const base = '/api/admin/v1/users/usr_target/workspace'
const wrappers: VueWrapper[] = []
const observers: Observer[] = []
const NativeURL = URL
class Observer {
  elements = new Set<Element>()
  constructor(readonly callback: IntersectionObserverCallback) { observers.push(this) }
  observe(element: Element) { this.elements.add(element) }
  unobserve(element: Element) { this.elements.delete(element) }
  disconnect() { this.elements.clear() }
  show(path: string, visible = true) {
    const element = [...this.elements].find(e => e.getAttribute('data-thumbnail-path') === path)
    expect(element, `observed row ${path}`).toBeDefined()
    this.callback([{ target: element!, isIntersecting: visible, intersectionRatio: visible ? 1 : 0 } as IntersectionObserverEntry], this as unknown as IntersectionObserver)
  }
}
function capability(value: unknown = supported, agentSpaceId = space) {
  return new Response(JSON.stringify(value), { headers: { 'Content-Type': 'application/json', 'Agent-Space-Id': agentSpaceId } })
}
function thumbnail(etag = '"source-1"', body: BodyInit = new Uint8Array([82, 73, 70, 70])) {
  return new Response(body, { headers: { 'Content-Type': 'image/webp', 'Preview-Source-ETag': etag, 'Agent-Space-Id': space } })
}
function deferred<T>() { let resolve!: (value: T) => void; const promise = new Promise<T>(r => { resolve = r }); return { promise, resolve } }
function render(entries: Entry[] = [image(), directory]) {
  const listing = vi.spyOn(client, 'apiFetch').mockResolvedValue({ entries })
  const pinia = createPinia(); setActivePinia(pinia)
  const session = useSessionStore(); session.session = { user: { userId: 'usr_admin', displayName: 'Admin', role: 'ADMIN' }, csrfToken: 'test-csrf', expiresAt: '2030-01-01T00:00:00Z' }
  const wrapper = mount(WorkspaceFiles, { props: { baseUrl: base, spaceId: space }, global: { stubs: { QDialog: { props: ['modelValue'], template: '<div v-if="modelValue"><slot/></div>' } }, plugins: [pinia, [Quasar, { components: { QFile, QCheckbox, QBtn, QSeparator, QCardSection, QBanner, QLinearProgress, QList, QItem, QItemSection, QItemLabel, QIcon, QMenu, QDialog, QCard, QCardActions, QInput, QSpinner }, directives: { ClosePopup } }]] } })
  wrappers.push(wrapper); return { wrapper, listing, session }
}
function show(path = 'photo.png', visible = true) { observers.at(-1)!.show(path, visible) }
function gets(fetch: ReturnType<typeof vi.fn>) { return fetch.mock.calls.filter(([, init]) => (init as RequestInit | undefined)?.method !== 'OPTIONS') }

beforeEach(() => {
  observers.length = 0
  vi.stubGlobal('IntersectionObserver', Observer)
  vi.spyOn(document, 'hidden', 'get').mockReturnValue(false)
  vi.stubGlobal('URL', class extends NativeURL {
    static createObjectURL = vi.fn().mockReturnValue('blob:thumbnail')
    static revokeObjectURL = vi.fn()
  })
})
afterEach(() => {
  wrappers.splice(0).forEach(w => w.unmount())
  client.setUnauthorizedHandler(undefined)
  vi.restoreAllMocks(); vi.unstubAllGlobals(); vi.useRealTimers()
})

describe('workspace optional thumbnail lifecycle', () => {
  it('loads the directory without awaiting OPTIONS and reuses capability across directory navigation', async () => {
    const pending = deferred<Response>()
    const fetch = vi.fn().mockReturnValue(pending.promise); vi.stubGlobal('fetch', fetch)
    const { wrapper, listing } = render(); await flushPromises()
    expect(wrapper.find('[aria-label="预览 photo.png"]').exists()).toBe(true)
    expect(fetch).toHaveBeenCalledTimes(1)
    expect(fetch.mock.calls[0]![1]).toMatchObject({ method: 'OPTIONS', credentials: 'same-origin' })
    expect(new URL(String(fetch.mock.calls[0]![0]), 'http://localhost').searchParams.toString()).toBe('agentSpaceId=spc_test')
    listing.mockResolvedValueOnce({ entries: [image('folder/inside.mov')] })
    await wrapper.get('[aria-label="打开目录 folder"]').trigger('keydown', { key: 'Enter' }); await flushPromises()
    pending.resolve(capability()); await flushPromises()
    expect(fetch).toHaveBeenCalledTimes(1)
    expect(wrapper.find('[aria-label="预览 inside.mov"]').exists()).toBe(true)
  })

  it('loads only visible strong-version media, with two concurrent reads and a separate candidate policy', async () => {
    const pending = [deferred<Response>(), deferred<Response>(), deferred<Response>()]
    let index = 0
    const fetch = vi.fn((_url: string, init: RequestInit) => init.method === 'OPTIONS' ? Promise.resolve(capability()) : pending[index++]!.promise)
    vi.stubGlobal('fetch', fetch)
    const { wrapper } = render([image(), image('clip.mov'), image('animation.apng'), image('bitmap.bmp'), image('weak.png', 'W/"weak"'), { path: 'missing.png', kind: 'FILE' }, image('page.svg'), image('page.html'), image('paper.pdf'), directory])
    await flushPromises(); expect(gets(fetch)).toHaveLength(0)
    show(); show('clip.mov'); show('animation.apng'); await flushPromises()
    expect(gets(fetch)).toHaveLength(2)
    expect(String(gets(fetch)[1]![0])).toContain('clip.mov')
    pending[0]!.resolve(thumbnail()); await flushPromises()
    expect(gets(fetch)).toHaveLength(3)
    expect(wrapper.find('img.workspace-thumbnail').exists()).toBe(true)
    expect([...observers.at(-1)!.elements].map(e => e.getAttribute('data-thumbnail-path'))).toEqual(expect.arrayContaining(['bitmap.bmp', 'animation.apng', 'clip.mov']))
    expect([...observers.at(-1)!.elements].map(e => e.getAttribute('data-thumbnail-path'))).not.toEqual(expect.arrayContaining(['weak.png']))
    const url = new URL(String(gets(fetch)[0]![0]), 'http://localhost')
    expect(url.pathname).toBe(base + '/thumbnail')
    expect(Object.fromEntries(url.searchParams)).toEqual({ agentSpaceId: space, path: 'photo.png', expectedEtag: '"source-1"', size: '128' })
  })

  it.each(['baseUrl', 'spaceId', 'identity'])('discards a late old listing after changing %s', async change => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(capability(unsupported)))
    const pending = deferred<unknown>()
    const { wrapper, listing, session } = render(); await flushPromises()
    listing.mockReset().mockReturnValueOnce(pending.promise as never).mockResolvedValue({ entries: [image('new.png')] })
    await wrapper.get('[aria-label="刷新文件列表"]').trigger('click'); await flushPromises()
    const signal = listing.mock.calls[0]![1]!.signal
    if (change === 'identity') session.session = { ...session.session!, user: { ...session.user!, userId: 'usr_next' } }
    else await wrapper.setProps({ [change]: change === 'baseUrl' ? '/api/admin/v1/users/usr_next/workspace' : 'spc_next' })
    await flushPromises(); expect(signal?.aborted).toBe(true)
    pending.resolve({ entries: [image('stale.png')] }); await flushPromises()
    expect(wrapper.find('[aria-label="预览 stale.png"]').exists()).toBe(false)
    expect(wrapper.find('[aria-label="预览 new.png"]').exists()).toBe(true)
  })

  it.each(['unsupported', 'unavailable', 'unknown-schema'])('retains the original list and icons for %s capability without downloading originals', async mode => {
    const fetch = vi.fn().mockImplementation(() => Promise.resolve(mode === 'unsupported' ? capability(unsupported) : mode === 'unavailable' ? new Response('{"code":"preview_unavailable"}', { status: 503 }) : capability({ ...supported, schemaVersion: 2 })))
    vi.stubGlobal('fetch', fetch)
    const { wrapper } = render(); await flushPromises(); show(); await flushPromises()
    expect(gets(fetch)).toHaveLength(0)
    expect(wrapper.find('img').exists()).toBe(false)
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    expect(wrapper.find('[aria-label="预览 photo.png"]').exists()).toBe(true)
  })

  it('uses the original Cookie and unauthorized handler for optional requests', async () => {
    const unauthorized = vi.fn(); client.setUnauthorizedHandler(unauthorized)
    const fetch = vi.fn().mockResolvedValue(new Response('{"code":"invalid_session"}', { status: 401 }))
    vi.stubGlobal('fetch', fetch); render(); await flushPromises()
    expect(unauthorized).toHaveBeenCalledTimes(1)
    expect(fetch.mock.calls[0]![1].credentials).toBe('same-origin')
  })

  it('cancels an offscreen response, keeps its slot until completion, and discards its late body', async () => {
    const reads = [deferred<Response>(), deferred<Response>(), deferred<Response>()]
    let index = 0
    const fetch = vi.fn((_url: string, init: RequestInit) => init.method === 'OPTIONS' ? Promise.resolve(capability()) : reads[index++]!.promise)
    vi.stubGlobal('fetch', fetch); render([image(), image('second.png'), image('third.png')]); await flushPromises()
    show(); show('second.png'); show('third.png'); await flushPromises()
    const firstSignal = gets(fetch)[0]![1].signal as AbortSignal
    show('photo.png', false); expect(firstSignal.aborted).toBe(true)
    expect(gets(fetch)).toHaveLength(2)
    reads[0]!.resolve(thumbnail()); await flushPromises()
    expect(gets(fetch)).toHaveLength(3)
    expect(URL.createObjectURL).not.toHaveBeenCalled()
  })

  it('releases loaded URLs when offscreen and when the source version changes', async () => {
    const fetch = vi.fn((_url: string, init: RequestInit) => Promise.resolve(init.method === 'OPTIONS' ? capability() : thumbnail(new URL(_url, 'http://localhost').searchParams.get('expectedEtag')!)))
    vi.stubGlobal('fetch', fetch)
    const { wrapper, listing } = render(); await flushPromises(); show(); await flushPromises()
    expect(wrapper.find('img.workspace-thumbnail').exists()).toBe(true)
    show('photo.png', false); await flushPromises()
    expect(URL.revokeObjectURL).toHaveBeenCalledTimes(1)
    expect(wrapper.find('img.workspace-thumbnail').exists()).toBe(false)
    show(); await flushPromises()
    listing.mockResolvedValueOnce({ entries: [image('photo.png', '"source-2"')] })
    await wrapper.get('[aria-label="刷新文件列表"]').trigger('click'); await flushPromises()
    expect(URL.revokeObjectURL).toHaveBeenCalledTimes(2)
    expect(wrapper.find('img.workspace-thumbnail').exists()).toBe(false)
    show(); await flushPromises()
    expect(new URL(String(gets(fetch).at(-1)![0]), 'http://localhost').searchParams.get('expectedEtag')).toBe('"source-2"')
    expect(fetch.mock.calls.filter(([, init]) => init.method === 'OPTIONS')).toHaveLength(1)
  })

  it.each(['hidden', 'available', 'space', 'base', 'identity', 'directory', 'unmount'])('aborts a pending image and releases URLs on %s', async change => {
    const pending = deferred<Response>(); let index = 0
    const fetch = vi.fn((_url: string, init: RequestInit) => init.method === 'OPTIONS' ? Promise.resolve(capability()) : index++ === 0 ? Promise.resolve(thumbnail()) : pending.promise)
    vi.stubGlobal('fetch', fetch)
    const { wrapper, session } = render([image(), image('second.png'), directory]); await flushPromises()
    show(); show('second.png'); await flushPromises()
    const signal = gets(fetch)[1]![1].signal as AbortSignal
    if (change === 'hidden') { vi.spyOn(document, 'hidden', 'get').mockReturnValue(true); document.dispatchEvent(new Event('visibilitychange')) }
    if (change === 'available') await wrapper.setProps({ available: false })
    if (change === 'space') await wrapper.setProps({ spaceId: 'spc_next' })
    if (change === 'base') await wrapper.setProps({ baseUrl: '/another/workspace' })
    if (change === 'identity') session.session = { ...session.session!, user: { ...session.user!, userId: 'usr_next' } }
    if (change === 'directory') await wrapper.get('[aria-label="打开目录 folder"]').trigger('click')
    if (change === 'unmount') wrapper.unmount()
    await flushPromises(); expect(signal.aborted).toBe(true)
    expect(URL.revokeObjectURL).toHaveBeenCalledWith('blob:thumbnail')
    const created = vi.mocked(URL.createObjectURL).mock.calls.length
    pending.resolve(thumbnail()); await flushPromises()
    expect(URL.createObjectURL).toHaveBeenCalledTimes(created)
  })

  it.each(['declared-limit', 'stream-limit', 'mime', 'source-version', 'error'])('isolates a %s thumbnail failure without automatic retry or original-file fallback', async mode => {
    let response: Response
    if (mode === 'declared-limit') { response = thumbnail(); response.headers.set('Content-Length', String(1024 * 1024 + 1)) }
    else if (mode === 'stream-limit') response = thumbnail('"source-1"', new Uint8Array(1024 * 1024 + 1))
    else if (mode === 'mime') response = new Response('<html>error</html>', { headers: { 'Content-Type': 'text/html' } })
    else if (mode === 'source-version') response = thumbnail('"different"')
    else response = new Response('{"code":"unsupported_preview_media"}', { status: 415 })
    const fetch = vi.fn((_url: string, init: RequestInit) => Promise.resolve(init.method === 'OPTIONS' ? capability() : response))
    vi.stubGlobal('fetch', fetch); const { wrapper } = render(); await flushPromises(); show(); await flushPromises()
    show(); await flushPromises()
    expect(gets(fetch)).toHaveLength(1)
    expect(URL.createObjectURL).not.toHaveBeenCalled()
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    expect(wrapper.find('[aria-label="预览 photo.png"]').exists()).toBe(true)
  })

  it('cancels a streaming response body immediately when the row leaves view', async () => {
    const cancelled = vi.fn()
    const stream = new ReadableStream<Uint8Array>({ start(c) { c.enqueue(new Uint8Array([1])) }, cancel: cancelled })
    const fetch = vi.fn((_url: string, init: RequestInit) => Promise.resolve(init.method === 'OPTIONS' ? capability() : thumbnail('"source-1"', stream)))
    vi.stubGlobal('fetch', fetch); render(); await flushPromises(); show(); await flushPromises()
    show('photo.png', false); await flushPromises()
    expect(cancelled).toHaveBeenCalledTimes(1)
  })

  it('falls back to the icon and releases a URL if the browser cannot decode the thumbnail', async () => {
    const fetch = vi.fn((_url: string, init: RequestInit) => Promise.resolve(init.method === 'OPTIONS' ? capability() : thumbnail()))
    vi.stubGlobal('fetch', fetch); const { wrapper } = render(); await flushPromises(); show(); await flushPromises()
    await wrapper.get('img.workspace-thumbnail').trigger('error'); await flushPromises()
    expect(wrapper.find('img.workspace-thumbnail').exists()).toBe(false)
    expect(URL.revokeObjectURL).toHaveBeenCalledWith('blob:thumbnail')
    expect(gets(fetch)).toHaveLength(1)
  })

  it('does not repeat requests after an empty image response', async () => {
    let index = 0
    const fetch = vi.fn((_url: string, init: RequestInit) => Promise.resolve(init.method === 'OPTIONS' ? capability() : index++ === 0 ? thumbnail('"source-1"', new Uint8Array()) : new Response('', { status: 503 })))
    vi.stubGlobal('fetch', fetch); render(); await flushPromises(); show(); await flushPromises()
    expect(gets(fetch)).toHaveLength(1)
  })

  it('aborts discovery and does not start another optional request after the Admin session is cleared', async () => {
    const pending = deferred<Response>()
    const fetch = vi.fn().mockReturnValue(pending.promise); vi.stubGlobal('fetch', fetch)
    const { session } = render(); await flushPromises()
    const signal = fetch.mock.calls[0]![1].signal as AbortSignal
    session.clear(); await flushPromises()
    expect(signal.aborted).toBe(true)
    expect(fetch).toHaveBeenCalledTimes(1)
    pending.resolve(capability()); await flushPromises()
    expect(URL.createObjectURL).not.toHaveBeenCalled()
  })

  it('ignores late capability discovery from the previous space', async () => {
    const pending = deferred<Response>()
    const fetch = vi.fn().mockReturnValueOnce(pending.promise).mockImplementation(() => Promise.resolve(capability(unsupported, 'spc_next')))
    vi.stubGlobal('fetch', fetch); const { wrapper } = render(); await flushPromises()
    const signal = fetch.mock.calls[0]![1].signal as AbortSignal
    await wrapper.setProps({ spaceId: 'spc_next' }); await flushPromises()
    expect(signal.aborted).toBe(true)
    pending.resolve(capability()); await flushPromises(); show(); await flushPromises()
    expect(fetch).toHaveBeenCalledTimes(2)
    expect(gets(fetch)).toHaveLength(0)
  })

  it('cancels pending discovery while hidden and resumes only the visible current scope', async () => {
    const pending = deferred<Response>()
    const fetch = vi.fn().mockReturnValueOnce(pending.promise).mockImplementation((_url: string, init: RequestInit) => Promise.resolve(init.method === 'OPTIONS' ? capability() : thumbnail()))
    vi.stubGlobal('fetch', fetch); render(); await flushPromises()
    const signal = fetch.mock.calls[0]![1].signal as AbortSignal
    const hidden = vi.spyOn(document, 'hidden', 'get').mockReturnValue(true)
    document.dispatchEvent(new Event('visibilitychange')); expect(signal.aborted).toBe(true)
    pending.resolve(capability()); await flushPromises(); expect(fetch).toHaveBeenCalledTimes(1)
    hidden.mockReturnValue(false); document.dispatchEvent(new Event('visibilitychange')); await flushPromises()
    expect(fetch).toHaveBeenCalledTimes(2); show(); await flushPromises(); expect(gets(fetch)).toHaveLength(1)
  })

  it('honors the discovered media kind and never observes weak malformed or non-media versions', async () => {
    const fetch = vi.fn((_url: string, init: RequestInit) => Promise.resolve(init.method === 'OPTIONS' ? capability({ ...supported, mediaKinds: ['IMAGE'] }) : thumbnail()))
    vi.stubGlobal('fetch', fetch)
    render([image('ok.bmp'), image('clip.mov'), image('weak.png', 'W/"v"'), image('unquoted.png', 'v'), image('list.png', '"v", "w"'), image('unsafe.svg'), image('page.html'), image('paper.pdf')])
    await flushPromises(); show('ok.bmp'); show('clip.mov'); await flushPromises()
    expect(gets(fetch)).toHaveLength(1)
    expect(new URL(String(gets(fetch)[0]![0]), 'http://localhost').searchParams.get('path')).toBe('ok.bmp')
    expect([...observers.at(-1)!.elements].map(e => e.getAttribute('data-thumbnail-path')).sort()).toEqual(['clip.mov', 'ok.bmp'])
  })

  it('preserves an unsaved text draft on a target change and never binds it to the new workspace', async () => {
    vi.stubGlobal('fetch', vi.fn().mockImplementation(() => Promise.resolve(capability(unsupported))))
    const { wrapper, listing } = render(); await flushPromises()
    await wrapper.findAllComponents(QBtn).find(button => button.props('label') === '新建文本')!.trigger('click'); await flushPromises()
    const editor = wrapper.getComponent(WorkspaceTextEditor)
    await editor.findAllComponents(QInput).find(input => input.props('label') === '文本内容')!.setValue('draft to retain')
    await wrapper.setProps({ baseUrl: '/another/workspace', spaceId: 'spc_next' }); await flushPromises()
    const retained = wrapper.findComponent(WorkspaceTextEditor)
    expect(retained.exists()).toBe(true)
    expect(retained.props()).toMatchObject({ baseUrl: base, spaceId: space, available: false })
    expect(retained.findAllComponents(QInput).find(input => input.props('label') === '文本内容')!.props('modelValue')).toBe('draft to retain')
    expect(retained.findAllComponents(QBtn).find(button => button.props('label') === '保存并关闭')!.props('disable')).toBe(true)
    const previous = listing.mock.calls.length
    retained.vm.$emit('saved'); await flushPromises()
    expect(listing).toHaveBeenCalledTimes(previous)
  })

  it('dismisses an old file mutation confirmation before a different workspace can receive it', async () => {
    vi.stubGlobal('fetch', vi.fn().mockImplementation(() => Promise.resolve(capability(unsupported))))
    const { wrapper } = render(); await flushPromises()
    await wrapper.get('[aria-label="photo.png 操作"]').trigger('click'); await flushPromises()
    const deletion = [...document.body.querySelectorAll<HTMLElement>('.q-menu .q-item')].find(item => item.textContent === '删除')
    expect(deletion).toBeDefined(); deletion!.click(); await flushPromises()
    expect(wrapper.find('.workspace-file-form').exists()).toBe(true)
    await wrapper.get('[data-cy="workspace-view-thumbnails"]').trigger('click'); await flushPromises()
    expect(wrapper.get('.workspace-file-form').text()).toContain('photo.png')
    await wrapper.setProps({ baseUrl: '/another/workspace' }); await flushPromises()
    expect(wrapper.find('.workspace-file-form').exists()).toBe(false)
  })

  it('never submits a copy after its overwrite lookup returns to a different scope', async () => {
    vi.stubGlobal('fetch', vi.fn().mockImplementation(() => Promise.resolve(capability(unsupported))))
    const { wrapper, listing } = render(); await flushPromises()
    await wrapper.get('[aria-label="photo.png 操作"]').trigger('click'); await flushPromises()
    const copy = [...document.body.querySelectorAll<HTMLElement>('.q-menu .q-item')].find(item => item.textContent === '复制')
    expect(copy).toBeDefined(); copy!.click(); await flushPromises()
    await wrapper.findAllComponents(QInput).find(input => input.props('label') === '目标完整相对路径（从根目录起）')!.setValue('copy.png')
    await wrapper.findAllComponents(QCheckbox).find(input => input.props('label') === '确认覆盖目标普通文件的当前版本')!.setValue(true)
    const pending = deferred<unknown>()
    listing.mockReset().mockReturnValueOnce(pending.promise as never).mockResolvedValue({ entries: [] })
    await wrapper.findAllComponents(QBtn).find(button => button.props('label') === '确认')!.trigger('click'); await flushPromises()
    const signal = listing.mock.calls[0]![1]!.signal as AbortSignal
    await wrapper.setProps({ spaceId: 'spc_next' }); await flushPromises()
    expect(signal.aborted).toBe(true)
    pending.resolve({ entries: [image('copy.png', '"target"')] }); await flushPromises()
    expect(listing.mock.calls.filter(([, init]) => init?.method === 'POST')).toHaveLength(0)
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
  })

  it('clears an upload selection when its workspace scope changes', async () => {
    vi.stubGlobal('fetch', vi.fn().mockImplementation(() => Promise.resolve(capability(unsupported))))
    const { wrapper } = render(); await flushPromises()
    await wrapper.getComponent(QFile).setValue(new File(['local'], 'upload.txt'))
    await wrapper.setProps({ spaceId: 'spc_next' }); await flushPromises()
    expect(wrapper.getComponent(QFile).props('modelValue')).toBeUndefined()
  })

  it('keeps the original mutation authorization context across an overwrite lookup', async () => {
    vi.stubGlobal('fetch', vi.fn().mockImplementation(() => Promise.resolve(capability(unsupported))))
    const { wrapper, listing, session } = render(); await flushPromises()
    await wrapper.get('[aria-label="photo.png 操作"]').trigger('click'); await flushPromises()
    const copy = [...document.body.querySelectorAll<HTMLElement>('.q-menu .q-item')].find(item => item.textContent === '复制')
    copy!.click(); await flushPromises()
    await wrapper.findAllComponents(QInput).find(input => input.props('label') === '目标完整相对路径（从根目录起）')!.setValue('copy.png')
    await wrapper.findAllComponents(QCheckbox).find(input => input.props('label') === '确认覆盖目标普通文件的当前版本')!.setValue(true)
    const pending = deferred<unknown>()
    listing.mockReset().mockReturnValueOnce(pending.promise as never).mockResolvedValue({ entries: [] })
    await wrapper.findAllComponents(QBtn).find(button => button.props('label') === '确认')!.trigger('click'); await flushPromises()
    session.session!.csrfToken = 'renewed-csrf'
    pending.resolve({ entries: [image('copy.png', '"target"')] }); await flushPromises()
    const write = listing.mock.calls.find(([, init]) => init?.method === 'POST')!
    expect(write[2]).toBe('test-csrf')
    expect(new URL(write[0], 'http://localhost').searchParams.get('agentSpaceId')).toBe(space)
    expect(JSON.parse(write[1]!.body as string)).toMatchObject({ action: 'COPY', path: 'photo.png', destination: 'copy.png', sourceEtag: '"source-1"', targetEtag: '"target"' })
  })

  it('recovers transfer controls after access is lost during an upload and retains the unknown outcome', async () => {
    class PendingXHR {
      upload = {}
      onabort?: () => void
      open() {}
      setRequestHeader() {}
      send() {}
      abort() { this.onabort?.() }
    }
    vi.stubGlobal('XMLHttpRequest', PendingXHR)
    vi.stubGlobal('fetch', vi.fn().mockImplementation(() => Promise.resolve(capability(unsupported))))
    const { wrapper } = render(); await flushPromises()
    await wrapper.getComponent(QFile).setValue(new File(['local'], 'upload.txt'))
    await wrapper.findAllComponents(QBtn).find(button => button.props('label') === '上传')!.trigger('click'); await flushPromises()
    expect(wrapper.getComponent(QFile).props('disable')).toBe(true)
    await wrapper.setProps({ available: false }); await flushPromises()
    await wrapper.setProps({ available: true }); await flushPromises()
    expect(wrapper.getComponent(QFile).props('disable')).toBe(false)
    expect(wrapper.getComponent(QFile).props('modelValue')).toBeUndefined()
    expect(wrapper.text()).toContain('结果未知')
    await wrapper.getComponent(QFile).setValue(new File(['next'], 'different.txt'))
    expect(wrapper.findAllComponents(QBtn).find(button => button.props('label') === '上传')!.props('disable')).toBe(false)
  })

  it('honors a shared 429 cooldown, retries the waiting key, and serializes later visible work without rediscovery', async () => {
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout', 'Date'] })
    const first = deferred<Response>(), retry = deferred<Response>()
    let secondAttempts = 0
    const fetch = vi.fn((url: string, init: RequestInit) => {
      if (init.method === 'OPTIONS') return Promise.resolve(capability())
      const path = new URL(url, 'http://localhost').searchParams.get('path')
      if (path === 'photo.png') return first.promise
      if (path === 'second.png' && secondAttempts++ === 0) return Promise.resolve(new Response('{"code":"preview_busy"}', { status: 429, headers: { 'Retry-After': '1' } }))
      if (path === 'second.png') return retry.promise
      return Promise.resolve(thumbnail())
    })
    vi.stubGlobal('fetch', fetch); render([image(), image('second.png'), image('third.png')]); await flushPromises()
    show(); show('second.png'); show('third.png'); await flushPromises()
    expect(gets(fetch)).toHaveLength(2)
    first.resolve(thumbnail()); await flushPromises()
    await vi.advanceTimersByTimeAsync(999); expect(gets(fetch)).toHaveLength(2)
    await vi.advanceTimersByTimeAsync(1); await flushPromises()
    expect(gets(fetch)).toHaveLength(3)
    expect(new URL(String(gets(fetch)[2]![0]), 'http://localhost').searchParams.get('path')).toBe('second.png')
    retry.resolve(thumbnail()); await flushPromises(); expect(gets(fetch)).toHaveLength(4)
    expect(fetch.mock.calls.filter(([, init]) => init.method === 'OPTIONS')).toHaveLength(1)
  })

  it('bounds repeated 429 retries', async () => {
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout', 'Date'] })
    const fetch = vi.fn((_url: string, init: RequestInit) => Promise.resolve(init.method === 'OPTIONS' ? capability() : new Response('{"code":"preview_busy"}', { status: 429, headers: { 'Retry-After': '1' } })))
    vi.stubGlobal('fetch', fetch); const { wrapper } = render(); await flushPromises(); show(); await flushPromises()
    await vi.advanceTimersByTimeAsync(1000); await flushPromises(); expect(gets(fetch)).toHaveLength(2)
    await vi.advanceTimersByTimeAsync(1000); await flushPromises(); expect(gets(fetch)).toHaveLength(3)
    await vi.advanceTimersByTimeAsync(10000); await flushPromises(); expect(gets(fetch)).toHaveLength(3)
    wrapper.unmount()
  })

  it.each(['hidden', 'space', 'available', 'unmount'])('cancels its single cooldown timer on %s', async change => {
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout', 'Date'] })
    const fetch = vi.fn((_url: string, init: RequestInit) => Promise.resolve(init.method === 'OPTIONS' ? capability() : new Response('{}', { status: 429, headers: { 'Retry-After': '1' } })))
    vi.stubGlobal('fetch', fetch)
    const Harness = defineComponent({
      props: { spaceId: { type: String, default: space }, available: { type: Boolean, default: true } },
      setup(props) {
        const entries = ref<Entry[]>([])
        const thumbnails = useWorkspaceThumbnails({ identity: () => 'usr_admin', baseUrl: () => base, spaceId: () => props.spaceId, available: () => props.available, entries: () => entries.value })
        return { entries, thumbnails }
      },
      render() { return h('div', this.entries.map(entry => h('span', { 'data-thumbnail-path': entry.path, ref: element => this.thumbnails.bind(entry, element) }))) },
    })
    const wrapper = mount(Harness); wrappers.push(wrapper)
    wrapper.vm.entries = [image()]; await flushPromises(); show(); await flushPromises()
    expect(vi.getTimerCount()).toBe(1)
    if (change === 'hidden') { vi.spyOn(document, 'hidden', 'get').mockReturnValue(true); document.dispatchEvent(new Event('visibilitychange')) }
    if (change === 'space') await wrapper.setProps({ spaceId: 'spc_next' })
    if (change === 'available') await wrapper.setProps({ available: false })
    if (change === 'unmount') wrapper.unmount()
    await flushPromises(); expect(vi.getTimerCount()).toBe(0)
    if (change === 'hidden') {
      await vi.advanceTimersByTimeAsync(500)
      vi.spyOn(document, 'hidden', 'get').mockReturnValue(false); document.dispatchEvent(new Event('visibilitychange')); show(); await flushPromises()
      expect(vi.getTimerCount()).toBe(1)
      await vi.advanceTimersByTimeAsync(499); expect(gets(fetch)).toHaveLength(1)
      await vi.advanceTimersByTimeAsync(1); await flushPromises(); expect(gets(fetch)).toHaveLength(2)
    }
  })

  it('defaults to the list and shares directory navigation, filtering, menus and an open draft with thumbnail cards', async () => {
    const fetch = vi.fn().mockImplementation(() => Promise.resolve(capability(unsupported))); vi.stubGlobal('fetch', fetch)
    const { wrapper, listing } = render(); await flushPromises()
    expect(wrapper.get('[data-cy="workspace-file-collection"]').attributes('data-view')).toBe('list')
    await wrapper.get('[data-cy="workspace-view-thumbnails"]').trigger('click')
    listing.mockResolvedValueOnce({ entries: [image('folder/photo.png'), image('folder/clip.mov'), { path: 'folder/notes.txt', kind: 'FILE' }] })
    await wrapper.get('[aria-label="打开目录 folder"]').trigger('keydown', { key: 'Enter' }); await flushPromises()
    expect(wrapper.findAll('[data-cy="workspace-file-entry"]')).toHaveLength(3)
    expect(wrapper.get('[data-cy="workspace-video-badge"]').text()).toContain('视频')
    expect(wrapper.find('[aria-label="notes.txt 操作"]').exists()).toBe(true)
    const search = wrapper.findAllComponents(QInput).find(input => input.props('label') === '筛选当前目录')!
    await search.setValue('photo')
    await wrapper.findAllComponents(QBtn).find(button => button.props('label') === '新建文本')!.trigger('click'); await flushPromises()
    const editor = wrapper.getComponent(WorkspaceTextEditor)
    await editor.findAllComponents(QInput).find(input => input.props('label') === '文本内容')!.setValue('same open draft')
    const previousListings = listing.mock.calls.length, previousOptions = fetch.mock.calls.length
    await wrapper.get('[data-cy="workspace-view-list"]').trigger('click')
    await wrapper.get('[data-cy="workspace-view-thumbnails"]').trigger('click'); await flushPromises()
    expect(wrapper.get('[data-cy="workspace-file-collection"]').attributes('data-view')).toBe('thumbnails')
    expect(search.props('modelValue')).toBe('photo')
    expect(wrapper.find('[aria-label="预览 photo.png"]').exists()).toBe(true)
    expect(editor.props('path')).toBe('folder/新文件.txt')
    expect(editor.findAllComponents(QInput).find(input => input.props('label') === '文本内容')!.props('modelValue')).toBe('same open draft')
    expect(listing).toHaveBeenCalledTimes(previousListings)
    expect(fetch).toHaveBeenCalledTimes(previousOptions)
  })

  it('cancels and releases old-size demands, discards their late responses and lazily requests 256px in the same scope', async () => {
    const old = deferred<Response>(), larger = deferred<Response>()
    const fetch = vi.fn((url: string, init: RequestInit) => {
      if (init.method === 'OPTIONS') return Promise.resolve(capability())
      const query = new URL(url, 'http://localhost').searchParams
      if (query.get('size') === '256') return larger.promise
      return query.get('path') === 'photo.png' ? Promise.resolve(thumbnail()) : old.promise
    })
    vi.stubGlobal('fetch', fetch); const { wrapper, listing } = render([image(), image('clip.mov')]); await flushPromises()
    show(); show('clip.mov'); await flushPromises()
    const oldSignal = gets(fetch)[1]![1].signal as AbortSignal
    expect(URL.createObjectURL).toHaveBeenCalledTimes(1)
    await wrapper.get('[data-cy="workspace-view-thumbnails"]').trigger('click'); await flushPromises()
    expect(oldSignal.aborted).toBe(true); expect(URL.revokeObjectURL).toHaveBeenCalledTimes(1)
    expect(gets(fetch)).toHaveLength(2)
    show(); show('clip.mov'); await flushPromises()
    expect(new URL(String(gets(fetch).at(-1)![0]), 'http://localhost').searchParams.get('size')).toBe('256')
    old.resolve(thumbnail()); await flushPromises()
    expect(URL.createObjectURL).toHaveBeenCalledTimes(1)
    expect(gets(fetch)).toHaveLength(4)
    larger.resolve(thumbnail()); await flushPromises()
    expect(wrapper.find('img.workspace-thumbnail').exists()).toBe(true)
    expect(listing).toHaveBeenCalledTimes(1)
    expect(fetch.mock.calls.filter(([, init]) => init.method === 'OPTIONS')).toHaveLength(1)
  })

  it('keeps the shared 429 deadline and capability when changing presentation size', async () => {
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout', 'Date'] })
    const fetch = vi.fn((url: string, init: RequestInit) => Promise.resolve(init.method === 'OPTIONS' ? capability() : new URL(url, 'http://localhost').searchParams.get('size') === '128' ? new Response('{}', { status: 429, headers: { 'Retry-After': '1' } }) : thumbnail()))
    vi.stubGlobal('fetch', fetch); const { wrapper } = render(); await flushPromises(); show(); await flushPromises()
    await wrapper.get('[data-cy="workspace-view-thumbnails"]').trigger('click'); await flushPromises(); show(); await flushPromises()
    await vi.advanceTimersByTimeAsync(999); expect(gets(fetch)).toHaveLength(1)
    await vi.advanceTimersByTimeAsync(1); await flushPromises(); expect(gets(fetch)).toHaveLength(2)
    expect(new URL(String(gets(fetch)[1]![0]), 'http://localhost').searchParams.get('size')).toBe('256')
    expect(fetch.mock.calls.filter(([, init]) => init.method === 'OPTIONS')).toHaveLength(1)
  })

  it.each(['list', 'thumbnails'])('restores visible current-directory thumbnails after a failed navigation in %s mode', async mode => {
    const fetch = vi.fn((_url: string, init: RequestInit) => Promise.resolve(init.method === 'OPTIONS' ? capability() : thumbnail()))
    vi.stubGlobal('fetch', fetch)
    const { wrapper, listing } = render(); await flushPromises()
    if (mode === 'thumbnails') await wrapper.get('[data-cy="workspace-view-thumbnails"]').trigger('click')
    show(); await flushPromises(); expect(wrapper.find('img.workspace-thumbnail').exists()).toBe(true)
    listing.mockRejectedValueOnce(new Error('target directory no longer exists'))
    await wrapper.get('[aria-label="打开目录 folder"]').trigger('click'); await flushPromises()
    expect(wrapper.find('[aria-label="预览 photo.png"]').exists()).toBe(true)
    expect(wrapper.get('[role="alert"]').text()).toContain('target directory no longer exists')
    expect([...observers.at(-1)!.elements].some(element => element.getAttribute('data-thumbnail-path') === 'photo.png')).toBe(true)
    show(); await flushPromises()
    expect(wrapper.find('img.workspace-thumbnail').exists()).toBe(true)
    expect(gets(fetch)).toHaveLength(2)
    expect(fetch.mock.calls.filter(([, init]) => init.method === 'OPTIONS')).toHaveLength(1)
  })
})
