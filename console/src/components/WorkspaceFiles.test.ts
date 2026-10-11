import { describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia } from 'pinia'
import { Quasar, QFile, QCheckbox, QBtn, QSeparator, QCardSection, QBanner, QLinearProgress, QList, QItem, QItemSection, QItemLabel, QIcon, QMenu, QDialog, QCard, QCardActions, QInput, ClosePopup } from 'quasar'
import WorkspaceFiles from './WorkspaceFiles.vue'
import * as client from '../api/client'

describe('workspace file confirmation', () => {
  it('ignores an older directory failure after a newer directory succeeds', async () => {
    let rejectOld!: (error: Error) => void
    const oldRequest = new Promise((_, reject) => { rejectOld = reject })
    const fetch = vi.spyOn(client, 'apiFetch').mockResolvedValueOnce({ entries: [{ path: 'older', kind: 'DIRECTORY' }, { path: 'newer', kind: 'DIRECTORY' }] }).mockReturnValueOnce(oldRequest).mockResolvedValueOnce({ entries: [{ path: 'newer/current.txt', kind: 'FILE' }] })
    const wrapper = mount(WorkspaceFiles, { props: { baseUrl: '/workspace', spaceId: 'spc_test' }, global: { plugins: [createPinia(), [Quasar, { components: { QFile, QCheckbox, QBtn, QSeparator, QCardSection, QBanner, QLinearProgress, QList, QItem, QItemSection, QItemLabel, QIcon, QMenu, QDialog, QCard, QCardActions, QInput }, directives: { ClosePopup } }]] } })
    try {
      await flushPromises()
      await wrapper.get('[aria-label="打开目录 older"]').trigger('click')
      await wrapper.get('[aria-label="打开目录 newer"]').trigger('click'); await flushPromises()
      expect(fetch).toHaveBeenCalledTimes(3)
      expect(wrapper.find('[aria-label="预览 current.txt"]').exists()).toBe(true)
      rejectOld(new Error('old directory failed')); await flushPromises()
      expect(wrapper.text()).not.toContain('old directory failed')
      expect(wrapper.find('[aria-label="预览 current.txt"]').exists()).toBe(true)
    } finally { wrapper.unmount(); vi.restoreAllMocks() }
  })
  it.each(['failure', 'success'])('discards an aborted listing %s while access is unavailable', async outcome => {
    let resolveOld!: (value: unknown) => void, rejectOld!: (error: Error) => void
    const oldRequest = new Promise((resolve, reject) => { resolveOld = resolve; rejectOld = reject })
    const fetch = vi.spyOn(client, 'apiFetch').mockReturnValue(oldRequest)
    const wrapper = mount(WorkspaceFiles, { props: { baseUrl: '/workspace', spaceId: 'spc_test' }, global: { plugins: [createPinia(), [Quasar, { components: { QFile, QCheckbox, QBtn, QSeparator, QCardSection, QBanner, QLinearProgress, QList, QItem, QItemSection, QItemLabel, QIcon, QMenu, QDialog, QCard, QCardActions, QInput }, directives: { ClosePopup } }]] } })
    try {
      await wrapper.setProps({ available: false })
      expect(fetch.mock.calls[0]![1]!.signal?.aborted).toBe(true)
      if (outcome === 'failure') rejectOld(new Error('aborted listing failed'))
      else resolveOld({ entries: [{ path: 'stale.txt', kind: 'FILE' }] })
      await flushPromises()
      expect(wrapper.text()).not.toContain('aborted listing failed')
      expect(wrapper.find('[aria-label="预览 stale.txt"]').exists()).toBe(false)
      expect(wrapper.findComponent(QLinearProgress).exists()).toBe(false)
    } finally { wrapper.unmount(); vi.restoreAllMocks() }
  })
  it('filters the current directory without changing the destination and resets the filter on navigation', async () => {
    const fetch = vi.spyOn(client, 'apiFetch').mockResolvedValue({ entries: [{ path: 'notes.txt', kind: 'FILE' }, { path: 'video.mp4', kind: 'FILE' }, { path: 'folder', kind: 'DIRECTORY' }] })
    const wrapper = mount(WorkspaceFiles, { props: { baseUrl: '/workspace', spaceId: 'spc_test' }, global: { plugins: [createPinia(), [Quasar, { components: { QFile, QCheckbox, QBtn, QSeparator, QCardSection, QBanner, QLinearProgress, QList, QItem, QItemSection, QItemLabel, QIcon, QMenu, QDialog, QCard, QCardActions, QInput }, directives: { ClosePopup } }]] } })
    try {
      await flushPromises()
      const search = wrapper.findAllComponents(QInput).find(input => input.props('label') === '筛选当前目录')
      expect(search).toBeDefined()
      await search!.setValue('VIDEO')
      expect(wrapper.find('[aria-label="预览 video.mp4"]').exists()).toBe(true)
      expect(wrapper.find('[aria-label="预览 notes.txt"]').exists()).toBe(false)
      expect(fetch).toHaveBeenCalledTimes(1)
      await search!.setValue('folder')
      fetch.mockResolvedValueOnce({ entries: [{ path: 'folder/inside.txt', kind: 'FILE' }] })
      await wrapper.find('[aria-label="打开目录 folder"]').trigger('click'); await flushPromises()
      expect(search!.props('modelValue')).toBe('')
      expect(wrapper.find('[aria-label="预览 inside.txt"]').exists()).toBe(true)
    } finally { wrapper.unmount(); vi.restoreAllMocks() }
  })
  it.each(['file', 'directory', 'version'])('requires a new overwrite confirmation after changing the %s', async change => {
    const fetch = vi.spyOn(client, 'apiFetch').mockResolvedValue({ usedBytes: 24576, entries: [{ path: 'a.txt', kind: 'FILE', etag: '"a"' }, { path: 'b.txt', kind: 'FILE', etag: '"b"' }, { path: 'folder', kind: 'DIRECTORY' }] })
    const wrapper = mount(WorkspaceFiles, { props: { baseUrl: '/workspace', spaceId: 'spc_test' }, global: { plugins: [createPinia(), [Quasar, { components: { QFile, QCheckbox, QBtn, QSeparator, QCardSection, QBanner, QLinearProgress, QList, QItem, QItemSection, QItemLabel, QIcon, QMenu, QDialog, QCard, QCardActions, QInput }, directives: { ClosePopup } }]] } })
    await flushPromises()
    expect(wrapper.text()).toContain('已用 24 KiB')
    await wrapper.findComponent(QFile).setValue(new File(['a'], 'a.txt'))
    await wrapper.findComponent(QCheckbox).setValue(true)
    expect(wrapper.findComponent(QCheckbox).props('modelValue')).toBe(true)
    if (change === 'file') await wrapper.findComponent(QFile).setValue(new File(['b'], 'b.txt'))
    else {
      fetch.mockResolvedValue({ entries: [{ path: change === 'directory' ? 'folder/a.txt' : 'a.txt', kind: 'FILE', etag: '"new-version"' }] })
      if (change === 'directory') await wrapper.find('[aria-label="打开目录 folder"]').trigger('click')
      else await wrapper.findAllComponents(QBtn).find(button => button.props('label') === '刷新')!.trigger('click')
      await flushPromises()
    }
    expect(wrapper.findComponent(QCheckbox).props('modelValue')).toBe(false)
    wrapper.unmount(); vi.restoreAllMocks()
  })
})
