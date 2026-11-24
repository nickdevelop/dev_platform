import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import EngineStepControl from '../features/engines/EngineStepControl.vue'

describe('EngineStepControl', () => {
  it('renders step limit label and default presets correctly', () => {
    const wrapper = mount(EngineStepControl, {
      props: {
        modelValue: 5,
        label: '单轮最大工具调用步数',
        hint: '防止陷入死循环。',
      },
    })

    expect(wrapper.text()).toContain('单轮最大工具调用步数')
    expect(wrapper.text()).toContain('当前设定: 5 步')
    expect(wrapper.text()).toContain('防止陷入死循环。')

    const input = wrapper.find('input.step-num-input')
    expect(input.exists()).toBe(true)
    expect((input.element as HTMLInputElement).value).toBe('5')

    const buttons = wrapper.findAll('.preset-btn')
    expect(buttons.length).toBe(5)
    expect(buttons[0].text()).toBe('5步')
    expect(buttons[4].text()).toBe('不限制')
    expect(buttons[0].classes()).toContain('active')
  })

  it('renders unlimited mode (-1) with proper badge style and active button', () => {
    const wrapper = mount(EngineStepControl, {
      props: {
        modelValue: -1,
        label: '单回合最大执行步数',
        presets: [
          { value: 10, label: '10步' },
          { value: 20, label: '20步' },
          { value: -1, label: '不限制' },
        ],
      },
    })

    expect(wrapper.text()).toContain('当前设定: 不限制 (-1)')

    const input = wrapper.find('input.step-num-input')
    expect((input.element as HTMLInputElement).value).toBe('-1')

    const unlimitedBtn = wrapper.findAll('.preset-btn').find((b) => b.text() === '不限制')
    expect(unlimitedBtn?.classes()).toContain('active')
  })

  it('emits update:modelValue when preset button is clicked', async () => {
    const wrapper = mount(EngineStepControl, {
      props: {
        modelValue: 5,
        label: '步数控制',
      },
    })

    const buttons = wrapper.findAll('.preset-btn')
    const unlimitedBtn = buttons.find((b) => b.text() === '不限制')
    expect(unlimitedBtn).toBeDefined()

    await unlimitedBtn!.trigger('click')
    expect(wrapper.emitted('update:modelValue')).toBeTruthy()
    expect(wrapper.emitted('update:modelValue')![0]).toEqual([-1])

    const tenBtn = buttons.find((b) => b.text() === '10步')
    await tenBtn!.trigger('click')
    expect(wrapper.emitted('update:modelValue')![1]).toEqual([10])
  })

  it('emits update:modelValue on typing positive or negative numbers in input', async () => {
    const wrapper = mount(EngineStepControl, {
      props: {
        modelValue: 5,
        label: '步数控制',
      },
    })

    const input = wrapper.find('input.step-num-input')
    await input.setValue('42')
    expect(wrapper.emitted('update:modelValue')![0]).toEqual([42])

    await input.setValue('-1')
    expect(wrapper.emitted('update:modelValue')![1]).toEqual([-1])

    // Non-positive (e.g. 0) falls back to -1 (unlimited)
    await input.setValue('0')
    expect(wrapper.emitted('update:modelValue')![2]).toEqual([-1])
  })
})
