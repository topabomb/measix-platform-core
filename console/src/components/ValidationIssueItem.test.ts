import { afterEach, describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { Quasar, QIcon, QItem, QItemLabel, QItemSection } from 'quasar'
import { i18n } from '../i18n'
import ValidationIssueItem from './ValidationIssueItem.vue'

function mountIssue(code: string, message: string, field = 'modelId') {
  return mount(ValidationIssueItem, {
    props: {
      issue: {
        code,
        path: `assistants[1].${field}`,
        message,
        severity: 'ERROR',
        resourceKind: 'ASSISTANT',
        resourceId: 'asd_test',
        field,
      },
      target: 'Assistant · Workshop helper · Model',
    },
    global: {
      plugins: [[Quasar, { components: { QIcon, QItem, QItemLabel, QItemSection } }]],
    },
  })
}

afterEach(() => {
  i18n.global.locale.value = 'en'
})

describe('ValidationIssueItem', () => {
  it('renders a known authoritative issue as friendly English without exposing the server sentence', () => {
    i18n.global.locale.value = 'en'
    const wrapper = mountIssue('invalid_model_ref', 'assistant references an unknown or disabled model')

    expect(wrapper.text()).toContain('Select an enabled model. The current model is missing or disabled.')
    expect(wrapper.text()).toContain('invalid_model_ref · assistants[1].modelId')
    expect(wrapper.text()).not.toContain('assistant references an unknown or disabled model')
  })

  it('renders the same stable issue code in Chinese', () => {
    i18n.global.locale.value = 'zh'
    const wrapper = mountIssue('invalid_model_ref', 'assistant references an unknown or disabled model')

    expect(wrapper.text()).toContain('请选择已启用的模型；当前模型不存在或已停用。')
    expect(wrapper.text()).not.toContain('assistant references an unknown or disabled model')
  })

  it('uses a safe localized fallback for a future issue code while retaining technical identifiers', () => {
    i18n.global.locale.value = 'en'
    const wrapper = mountIssue('future_validation_code', 'untrusted backend detail')

    expect(wrapper.text()).toContain('This configuration is invalid. Use the technical code below when asking for support.')
    expect(wrapper.text()).toContain('future_validation_code · assistants[1].modelId')
    expect(wrapper.text()).not.toContain('untrusted backend detail')
  })
})
