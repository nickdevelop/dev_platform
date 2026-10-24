import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import {
  UiButton,
  UiInput,
  UiSelect,
  UiProgress,
  UiBadge,
  UiAlert,
  UiEmpty,
} from '../shared/ui/atomic'

describe('UiButton', () => {
  it('renders slot text and emits click', async () => {
    const wrapper = mount(UiButton, {
      slots: { default: '确认提交' },
      attrs: { 'data-test': 'my-btn' },
    })
    expect(wrapper.text()).toContain('确认提交')
    expect(wrapper.attributes('data-test')).toBe('my-btn')

    await wrapper.trigger('click')
    expect(wrapper.emitted('click')).toHaveLength(1)
  })

  it('prevents click when disabled or loading', async () => {
    const disabledWrapper = mount(UiButton, {
      props: { disabled: true },
      slots: { default: '禁用' },
    })
    await disabledWrapper.trigger('click')
    expect(disabledWrapper.emitted('click')).toBeUndefined()

    const loadingWrapper = mount(UiButton, {
      props: { loading: true },
      slots: { default: '加载中' },
    })
    await loadingWrapper.trigger('click')
    expect(loadingWrapper.emitted('click')).toBeUndefined()
    expect(loadingWrapper.find('svg[role="status"]').exists()).toBe(true)
  })
})

describe('UiInput', () => {
  it('updates modelValue on input and supports clearable', async () => {
    const wrapper = mount(UiInput, {
      props: {
        modelValue: 'hello',
        clearable: true,
      },
    })
    const input = wrapper.find('input')
    expect(input.element.value).toBe('hello')

    await input.setValue('world')
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual(['world'])

    const clearBtn = wrapper.find('button[title="清空"]')
    expect(clearBtn.exists()).toBe(true)
    await clearBtn.trigger('click')
    expect(wrapper.emitted('update:modelValue')?.[1]).toEqual([''])
    expect(wrapper.emitted('clear')).toHaveLength(1)
  })
})

describe('UiSelect', () => {
  it('renders options and emits update:modelValue', async () => {
    const wrapper = mount(UiSelect, {
      props: {
        modelValue: 'opt2',
        options: [
          { label: '选项一', value: 'opt1' },
          { label: '选项二', value: 'opt2' },
        ],
      },
    })
    const select = wrapper.find('select')
    expect(select.element.value).toBe('opt2')

    await select.setValue('opt1')
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual(['opt1'])
  })
})

describe('UiProgress', () => {
  it('calculates percentage correctly and maps auto variant', () => {
    const wrapper = mount(UiProgress, {
      props: {
        value: 80,
        max: 100,
        showText: true,
      },
    })
    const bar = wrapper.find('div[role="progressbar"]')
    expect(bar.attributes('style')).toContain('width: 80%')
    expect(wrapper.text()).toContain('80%')
    // 75% <= 80% < 90% -> warning (amber)
    expect(bar.classes()).toContain('bg-amber-500')
  })

  it('renders custom text and danger variant for >= 90%', () => {
    const wrapper = mount(UiProgress, {
      props: {
        value: 95,
        max: 100,
        text: '95/100 Tokens',
        showText: true,
      },
    })
    expect(wrapper.text()).toContain('95/100 Tokens')
    const bar = wrapper.find('div[role="progressbar"]')
    expect(bar.classes()).toContain('bg-red-500')
  })
})

describe('UiBadge', () => {
  it('renders dot and emits close', async () => {
    const wrapper = mount(UiBadge, {
      props: {
        variant: 'success',
        dot: true,
        closable: true,
      },
      slots: { default: '在线' },
    })
    expect(wrapper.text()).toContain('在线')
    expect(wrapper.find('[data-test="badge-dot"]').exists()).toBe(true)

    const closeBtn = wrapper.find('button[aria-label="移除标签"]')
    expect(closeBtn.exists()).toBe(true)
    await closeBtn.trigger('click')
    expect(wrapper.emitted('close')).toHaveLength(1)
  })
})

describe('UiAlert', () => {
  it('renders title, description and handles close', async () => {
    const wrapper = mount(UiAlert, {
      props: {
        variant: 'error',
        title: '错误通知',
        closable: true,
      },
      slots: { default: '操作失败，请重试' },
    })
    expect(wrapper.text()).toContain('错误通知')
    expect(wrapper.text()).toContain('操作失败，请重试')
    expect(wrapper.attributes('role')).toBe('alert')

    const closeBtn = wrapper.find('button[aria-label="关闭提示"]')
    await closeBtn.trigger('click')
    expect(wrapper.emitted('close')).toHaveLength(1)
  })
})

describe('UiEmpty', () => {
  it('renders icon, title, description and actions', () => {
    const wrapper = mount(UiEmpty, {
      props: {
        title: '暂无数据',
        description: '请点击下方按钮新建',
        icon: '📁',
      },
      slots: {
        actions: '<button class="action-btn">立即创建</button>',
      },
    })
    expect(wrapper.text()).toContain('📁')
    expect(wrapper.text()).toContain('暂无数据')
    expect(wrapper.text()).toContain('请点击下方按钮新建')
    expect(wrapper.find('.action-btn').exists()).toBe(true)
  })
})
