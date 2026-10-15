// Unit tests for the generic question-form renderer: option fields render
// as radios, required fields gate the submit, and answers carry the field
// keys the server expects.
import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import QuestionForm from '../features/chat/interactions/QuestionForm.vue'

const fields = [
  {
    key: 'scope',
    type: 'string',
    title: '范围',
    required: true,
    options: [
      { value: 'small', label: '小', description: '够用为主' },
      { value: 'large', label: '大', description: '分布式' },
    ],
  },
  { key: 'note', type: 'string', title: '备注' },
]

function mountForm(overrides = {}) {
  return mount(QuestionForm, {
    props: { formId: 'frm_1', title: '选一个', fields, ...overrides },
  })
}

describe('QuestionForm', () => {
  it('renders the title and every option', () => {
    const wrapper = mountForm()
    expect(wrapper.text()).toContain('选一个')
    expect(wrapper.text()).toContain('范围')
    const radios = wrapper.findAll('input[type="radio"]')
    expect(radios).toHaveLength(2)
  })

  it('disables the submit until required fields are answered', async () => {
    const wrapper = mountForm()
    expect(wrapper.find('[data-test="submit-answer"]').attributes('disabled')).toBeDefined()

    await radios(wrapper)[0].setValue('small')
    expect(wrapper.find('[data-test="submit-answer"]').attributes('disabled')).toBeUndefined()
  })

  it('emits the answer keyed by field key', async () => {
    const wrapper = mountForm()
    await radios(wrapper)[1].setValue('large')
    await wrapper.find('[data-test="submit-answer"]').trigger('click')

    expect(wrapper.emitted('submit')?.[0]).toEqual([{ scope: 'large' }])
  })

  it('renders a note when there is nothing to fill', () => {
    const wrapper = mountForm({ fields: [] })
    expect(wrapper.text()).toContain('没有可填写的字段')
    expect(wrapper.find('[data-test="submit-answer"]').attributes('disabled')).toBeDefined()
  })

  it('renders booleans as yes/no buttons', async () => {
    const wrapper = mountForm({
      fields: [{ key: 'ok', type: 'boolean', title: '确认吗', required: true }],
    })
    const buttons = wrapper.findAll('.boolean-options button')
    expect(buttons).toHaveLength(2)
    await buttons[0].trigger('click')
    await wrapper.find('[data-test="submit-answer"]').trigger('click')
    expect(wrapper.emitted('submit')?.[0]).toEqual([{ ok: true }])
  })
})

function radios(wrapper: ReturnType<typeof mountForm>) {
  return wrapper.findAll('input[type="radio"]')
}
