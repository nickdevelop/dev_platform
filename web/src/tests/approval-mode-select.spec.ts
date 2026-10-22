// The approval-mode picker is the composer's second compact picker. These tests
// pin the contract SessionChatPanel relies on: a trigger showing the current
// mode, a listbox with the three modes, v-model emission and dismissal.
import { afterEach, describe, expect, it } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import ApprovalModeSelect from '../features/chat/composer/ApprovalModeSelect.vue'

afterEach(() => {
  document.body.innerHTML = ''
})

describe('ApprovalModeSelect', () => {
  it('shows the current mode and the three options', async () => {
    const wrapper = mount(ApprovalModeSelect, { props: { modelValue: 'default' } })

    const trigger = wrapper.find('[data-test="approval-mode-select"]')
    expect(trigger.text()).toContain('默认')
    expect(trigger.attributes('aria-expanded')).toBe('false')

    await trigger.trigger('click')
    await flushPromises()

    expect(trigger.attributes('aria-expanded')).toBe('true')
    const options = wrapper.findAll('[data-test^="approval-mode-option-"]')
    expect(options.map((option) => option.find('.approval-mode-option-name').text())).toEqual([
      '默认',
      '自动审批',
      '托管',
    ])
    expect(wrapper.find('[data-test="approval-mode-option-default"]').attributes('aria-selected')).toBe(
      'true',
    )
  })

  it('emits the chosen mode and closes the menu', async () => {
    const wrapper = mount(ApprovalModeSelect, { props: { modelValue: 'default' } })

    await wrapper.find('[data-test="approval-mode-select"]').trigger('click')
    await flushPromises()
    await wrapper.find('[data-test="approval-mode-option-autopilot"]').trigger('click')
    await flushPromises()

    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual(['autopilot'])
    expect(wrapper.find('[data-test="approval-mode-menu"]').exists()).toBe(false)
  })

  it('closes on Escape without changing the value', async () => {
    const wrapper = mount(ApprovalModeSelect, { props: { modelValue: 'auto-approve' } })

    await wrapper.find('[data-test="approval-mode-select"]').trigger('click')
    await flushPromises()
    await wrapper.find('[data-test="approval-mode-select"]').trigger('keydown', { key: 'Escape' })
    await flushPromises()

    expect(wrapper.find('[data-test="approval-mode-menu"]').exists()).toBe(false)
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
  })

  it('closes when the user clicks outside', async () => {
    const wrapper = mount(ApprovalModeSelect, { props: { modelValue: 'default' } })

    await wrapper.find('[data-test="approval-mode-select"]').trigger('click')
    await flushPromises()
    document.dispatchEvent(new MouseEvent('mousedown', { bubbles: true }))
    await flushPromises()

    expect(wrapper.find('[data-test="approval-mode-menu"]').exists()).toBe(false)
  })

  it('opens from the keyboard and focuses an option', async () => {
    const wrapper = mount(ApprovalModeSelect, {
      props: { modelValue: 'default' },
      attachTo: document.body,
    })

    await wrapper.find('[data-test="approval-mode-select"]').trigger('keydown', { key: 'ArrowDown' })
    await flushPromises()

    expect(wrapper.find('[data-test="approval-mode-menu"]').exists()).toBe(true)
    expect(document.activeElement).toBe(
      wrapper.find('[data-test="approval-mode-option-default"]').element,
    )
  })
})
