import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import { Quasar, QBtn } from 'quasar'
import CursorPager from './CursorPager.vue'

describe('CursorPager', () => {
  it('keeps paging controls visible on the last page', async () => {
    const wrapper = mount(CursorPager, {
      props: { page: 1, count: 2, hasNext: false },
      global: {
        plugins: [[Quasar, { components: { QBtn } }]],
        mocks: { $t: (key: string, args?: Record<string, number>) => key === 'common.pageStatus' ? `page ${args?.page} count ${args?.count}` : key },
      },
    })

    expect(wrapper.get('[data-cy="cursor-pager"]').text()).toContain('page 1 count 2')
    expect(wrapper.findAll('button')).toHaveLength(2)
    expect(wrapper.findAll('button').every(button => button.attributes('disabled') !== undefined)).toBe(true)
  })
})
