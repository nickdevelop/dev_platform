import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import EngineStepControl from '../features/engines/EngineStepControl.vue'

// The control is now `el-input-number` + `el-segmented` (resolver-driven, so
// no stubs): assertions follow the library's markup but keep asserting the
// same contract — current-value readout, preset selection, and the "-1 means
// unlimited" fallback for cleared / non-positive input.
function numberInput(wrapper: ReturnType<typeof mount>) {
  return wrapper.find('.step-num-input input')
}

async function commitNumber(wrapper: ReturnType<typeof mount>, value: string) {
  const input = numberInput(wrapper)
  await input.setValue(value)
  await input.trigger('change')
}

// The library wires each preset chip as a radio input with an onChange
// handler, so tests select a chip by firing `change` on that input.
function presetChip(wrapper: ReturnType<typeof mount>, text: string) {
  const chip = wrapper
    .findAll('.el-segmented__item')
    .find((item) => item.text() === text)
  return chip?.find('input')
}

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

    const input = numberInput(wrapper)
    expect(input.exists()).toBe(true)
    expect((input.element as HTMLInputElement).value).toBe('5')

    const presets = wrapper.findAll('.el-segmented__item')
    expect(presets.length).toBe(5)
    expect(presets[0].text()).toBe('5步')
    expect(presets[4].text()).toBe('不限制')
    expect(presets[0].classes()).toContain('is-selected')
  })

  it('renders unlimited mode (-1) with proper readout and active preset', () => {
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

    expect((numberInput(wrapper).element as HTMLInputElement).value).toBe('-1')

    const unlimitedBtn = wrapper
      .findAll('.el-segmented__item')
      .find((b) => b.text() === '不限制')
    expect(unlimitedBtn?.classes()).toContain('is-selected')
  })

  it('leaves the preset selection empty for a custom value', () => {
    const wrapper = mount(EngineStepControl, {
      props: { modelValue: 42, label: '步数控制' },
    })

    expect(wrapper.findAll('.el-segmented__item.is-selected')).toHaveLength(0)
  })

  it('emits update:modelValue when preset chip is clicked', async () => {
    const wrapper = mount(EngineStepControl, {
      props: {
        modelValue: 5,
        label: '步数控制',
      },
    })

    const unlimitedBtn = presetChip(wrapper, '不限制')
    expect(unlimitedBtn).toBeDefined()

    await unlimitedBtn!.trigger('change')
    expect(wrapper.emitted('update:modelValue')).toBeTruthy()
    expect(wrapper.emitted('update:modelValue')![0]).toEqual([-1])

    const tenBtn = presetChip(wrapper, '10步')
    await tenBtn!.trigger('change')
    expect(wrapper.emitted('update:modelValue')![1]).toEqual([10])
  })

  it('drives the bound value through v-model typing (unlimited fallback included)', async () => {
    // A real v-model parent rather than raw emitted-event indexing: the
    // control is fully controlled, and el-input-number only settles values
    // against the props it is given.
    const Parent = {
      components: { EngineStepControl },
      data: () => ({ value: 5 }),
      template: '<EngineStepControl label="步数控制" v-model="value" />',
    }
    const wrapper = mount(Parent)
    const vm = wrapper.vm as unknown as { value: number }

    await commitNumber(wrapper, '42')
    expect(vm.value).toBe(42)

    await commitNumber(wrapper, '-1')
    expect(vm.value).toBe(-1)

    // Non-positive (e.g. 0) falls back to -1 (unlimited)
    await commitNumber(wrapper, '0')
    expect(vm.value).toBe(-1)
  })
})
