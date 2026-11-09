import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import DetailWorkspace from './DetailWorkspace.vue'

describe('DetailWorkspace', () => {
  it('keeps list and detail in one compact responsive workspace', () => {
    const wrapper = mount(DetailWorkspace, {
      props: { detailOpen: true },
      slots: {
        list: '<div data-test="list">list</div>',
        detail: '<div data-test="detail">detail</div>',
      },
    })

    expect(wrapper.get('[data-cy="detail-workspace"]').classes()).toContain('detail-workspace--open')
    expect(wrapper.get('[data-cy="detail-workspace-list"]').text()).toBe('list')
    expect(wrapper.get('[data-cy="detail-workspace-detail"]').text()).toBe('detail')
    expect(wrapper.find('.q-dialog').exists()).toBe(false)
  })

  it('does not render an empty detail pane before an item is selected', () => {
    const wrapper = mount(DetailWorkspace, {
      props: { detailOpen: false },
      slots: { list: '<div>list</div>' },
    })

    expect(wrapper.find('[data-cy="detail-workspace-detail"]').exists()).toBe(false)
  })
})
