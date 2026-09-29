import { describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia } from 'pinia'
import { Quasar, QFile, QCheckbox, QBtn, QSeparator, QCardSection, QBanner, QLinearProgress, QList, QItem, QItemSection, QItemLabel, QIcon, QMenu, QDialog, QCard, QCardActions, QInput, ClosePopup } from 'quasar'
import WorkspaceFiles from './WorkspaceFiles.vue'
import * as client from '../api/client'

describe('workspace file confirmation', () => {
  it.each(['file', 'directory', 'version'])('requires a new overwrite confirmation after changing the %s', async change => {
    const fetch = vi.spyOn(client, 'apiFetch').mockResolvedValue({ entries: [{ path: 'a.txt', kind: 'FILE', etag: '"a"' }, { path: 'b.txt', kind: 'FILE', etag: '"b"' }, { path: 'folder', kind: 'DIRECTORY' }] })
    const wrapper = mount(WorkspaceFiles, { props: { baseUrl: '/workspace', spaceId: 'spc_test' }, global: { plugins: [createPinia(), [Quasar, { components: { QFile, QCheckbox, QBtn, QSeparator, QCardSection, QBanner, QLinearProgress, QList, QItem, QItemSection, QItemLabel, QIcon, QMenu, QDialog, QCard, QCardActions, QInput }, directives: { ClosePopup } }]] } })
    await flushPromises()
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
