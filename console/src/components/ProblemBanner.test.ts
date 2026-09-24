import { afterEach, describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { Quasar, QBanner } from 'quasar'
import { ApiProblem } from '../api/client'
import { i18n } from '../i18n'
import ProblemBanner from './ProblemBanner.vue'

function mountBanner(error: unknown) {
  return mount(ProblemBanner, {
    props: { error },
    global: { plugins: [[Quasar, { components: { QBanner } }]] },
  })
}

afterEach(() => {
  i18n.global.locale.value = 'en'
})

describe('ProblemBanner', () => {
  it('uses bilingual copy for a known Admin API problem and hides raw server detail', () => {
    const error = new ApiProblem(400, 'invalid_request', 'raw server detail')

    i18n.global.locale.value = 'en'
    expect(mountBanner(error).text()).toBe('Some submitted values are missing or invalid. Review the form and try again.')

    i18n.global.locale.value = 'zh'
    expect(mountBanner(error).text()).toBe('部分提交内容缺失或无效，请检查表单后重试。')
  })

  it('keeps diagnostic detail for an unknown problem code', () => {
    const wrapper = mountBanner(new ApiProblem(500, 'future_problem', 'diagnostic detail'))

    expect(wrapper.text()).toContain('future_problem')
    expect(wrapper.text()).toContain('diagnostic detail')
  })
})
