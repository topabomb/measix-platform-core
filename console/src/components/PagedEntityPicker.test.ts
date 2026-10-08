import { describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import {
  Quasar, QBtn, QCard, QCardActions, QCardSection, QDialog, QField, QIcon,
  QInput, QItem, QItemLabel, QItemSection, QList, QSeparator, QSpinner,
  QSpace,
  ClosePopup,
} from 'quasar'
import PagedEntityPicker, { type EntityPickerOption } from './PagedEntityPicker.vue'

function mountPicker(fetchPage: (query: string, cursor?: string) => Promise<{ items: EntityPickerOption[]; nextCursor?: string }>) {
  return mount(PagedEntityPicker, {
    props: { modelValue: undefined, label: 'User', emptyLabel: 'Any user', fetchPage },
    global: {
      plugins: [[Quasar, {
        components: {
          QBtn, QCard, QCardActions, QCardSection, QDialog, QField, QIcon,
          QInput, QItem, QItemLabel, QItemSection, QList, QSeparator, QSpinner,
          QSpace,
        },
        directives: { ClosePopup },
      }]],
    },
  })
}

describe('PagedEntityPicker', () => {
  it('opens from the rendered field control', async () => {
    const fetchPage = vi.fn().mockResolvedValue({ items: [] })
    const wrapper = mountPicker(fetchPage)

    await wrapper.get('[data-cy="entity-picker-trigger"]').trigger('click')
    await flushPromises()

    expect(fetchPage).toHaveBeenCalledWith('', undefined)
    expect(document.body.querySelector('[data-cy="entity-picker-dialog"]')).not.toBeNull()
    wrapper.unmount()
  })

  it('delegates search and cursor paging without loading an entire dataset', async () => {
    const fetchPage = vi.fn()
      .mockResolvedValueOnce({ items: [{ value: 'usr_1', label: 'Alice' }], nextCursor: 'next' })
      .mockResolvedValueOnce({ items: [{ value: 'usr_2', label: 'Alex' }] })
    const wrapper = mountPicker(fetchPage)
    const picker = wrapper.vm as unknown as {
      searchEntities: (term: string) => Promise<void>
      loadMore: () => Promise<void>
    }

    await picker.searchEntities('ali')
    await picker.loadMore()

    expect(fetchPage).toHaveBeenNthCalledWith(1, 'ali', undefined)
    expect(fetchPage).toHaveBeenNthCalledWith(2, 'ali', 'next')
    wrapper.unmount()
  })

  it('emits both the stable id and human-readable option', () => {
    const wrapper = mountPicker(vi.fn().mockResolvedValue({ items: [] }))
    const option = { value: 'usr_1', label: 'Alice', caption: 'alice' }
    const picker = wrapper.vm as unknown as { choose: (value: EntityPickerOption) => void }

    picker.choose(option)

    expect(wrapper.emitted('update:modelValue')).toEqual([['usr_1']])
    expect(wrapper.emitted('selected')).toEqual([[option]])
    wrapper.unmount()
  })
})
