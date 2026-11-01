import { describe, expect, it } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import CronBuilder from '../shared/components/CronBuilder.vue'

describe('CronBuilder.vue', () => {
  it('renders default daily mode and initial expression', async () => {
    const wrapper = mount(CronBuilder, {
      props: {
        modelValue: '0 9 * * *',
      },
    })
    await flushPromises()

    const modeSelect = wrapper.find('[data-test="cron-mode-select"]')
    expect(modeSelect.exists()).toBe(true)
    expect((modeSelect.element as HTMLSelectElement).value).toBe('daily')

    const input = wrapper.find('[data-test="cron-input"]')
    expect(input.exists()).toBe(true)
    expect((input.element as HTMLInputElement).value).toBe('0 9 * * *')

    const explanation = wrapper.find('[data-test="cron-explanation"]')
    expect(explanation.text()).toContain('每天 09:00 执行一次')
  })

  it('generates minute interval expression and explanation', async () => {
    const wrapper = mount(CronBuilder, {
      props: {
        modelValue: '0 9 * * *',
      },
    })
    await flushPromises()

    const modeSelect = wrapper.find('[data-test="cron-mode-select"]')
    await modeSelect.setValue('minute')
    await flushPromises()

    const minuteSelect = wrapper.find('[data-test="cron-minute-select"]')
    expect(minuteSelect.exists()).toBe(true)
    await minuteSelect.setValue('15')
    await flushPromises()

    expect(wrapper.emitted('update:modelValue')?.at(-1)).toEqual(['*/15 * * * *'])
    expect(wrapper.find('[data-test="cron-explanation"]').text()).toBe('每隔 15 分钟执行一次')
  })

  it('generates hour interval expression and explanation', async () => {
    const wrapper = mount(CronBuilder, {
      props: {
        modelValue: '0 9 * * *',
      },
    })
    await flushPromises()

    const modeSelect = wrapper.find('[data-test="cron-mode-select"]')
    await modeSelect.setValue('hour')
    await flushPromises()

    const hourSelect = wrapper.find('[data-test="cron-hour-select"]')
    expect(hourSelect.exists()).toBe(true)
    await hourSelect.setValue('2')
    await flushPromises()

    expect(wrapper.emitted('update:modelValue')?.at(-1)).toEqual(['0 */2 * * *'])
    expect(wrapper.find('[data-test="cron-explanation"]').text()).toContain('每隔 2 小时的第 0 分钟执行一次')
  })

  it('generates workday expression and explanation', async () => {
    const wrapper = mount(CronBuilder, {
      props: {
        modelValue: '0 9 * * *',
      },
    })
    await flushPromises()

    const modeSelect = wrapper.find('[data-test="cron-mode-select"]')
    await modeSelect.setValue('workday')
    await flushPromises()

    const hourPicker = wrapper.find('[data-test="cron-hour-picker"]')
    await hourPicker.setValue('18')
    await flushPromises()

    expect(wrapper.emitted('update:modelValue')?.at(-1)).toEqual(['0 18 * * 1-5'])
    expect(wrapper.find('[data-test="cron-explanation"]').text()).toContain('每周一至周五 (工作日) 18:00 执行')
  })

  it('generates weekly expression with multiple selected days', async () => {
    const wrapper = mount(CronBuilder, {
      props: {
        modelValue: '0 9 * * *',
      },
    })
    await flushPromises()

    const modeSelect = wrapper.find('[data-test="cron-mode-select"]')
    await modeSelect.setValue('weekly')
    await flushPromises()

    // Default weekday is [1] (Mon). Click Friday (5).
    const buttons = wrapper.findAll('.cron-controls button')
    const friBtn = buttons.find((b) => b.text().includes('周五'))
    expect(friBtn?.exists()).toBe(true)
    await friBtn?.trigger('click')
    await flushPromises()

    expect(wrapper.emitted('update:modelValue')?.at(-1)).toEqual(['0 9 * * 1,5'])
    expect(wrapper.find('[data-test="cron-explanation"]').text()).toContain('每周 (周一、周五) 09:00 执行')
  })

  it('parses incoming expression and switches mode automatically', async () => {
    const wrapper = mount(CronBuilder, {
      props: {
        modelValue: '*/30 * * * *',
      },
    })
    await flushPromises()

    const modeSelect = wrapper.find('[data-test="cron-mode-select"]')
    expect((modeSelect.element as HTMLSelectElement).value).toBe('minute')
    expect(wrapper.find('[data-test="cron-explanation"]').text()).toBe('每隔 30 分钟执行一次')

    // Workday
    await wrapper.setProps({ modelValue: '30 8 * * 1-5' })
    await flushPromises()
    expect((modeSelect.element as HTMLSelectElement).value).toBe('workday')
    expect(wrapper.find('[data-test="cron-explanation"]').text()).toBe('每周一至周五 (工作日) 08:30 执行')

    // Non-standard expression falls back to custom mode
    await wrapper.setProps({ modelValue: '5 4 3 2 1' })
    await flushPromises()
    expect((modeSelect.element as HTMLSelectElement).value).toBe('custom')
    expect(wrapper.find('[data-test="cron-explanation"]').text()).toContain('按表达式执行: 5 4 3 2 1')
  })

  it('shows error notice for invalid 5-part cron expressions', async () => {
    const wrapper = mount(CronBuilder, {
      props: {
        modelValue: 'invalid-cron',
      },
    })
    await flushPromises()

    expect(wrapper.find('[data-test="cron-explanation"]').text()).toContain('格式错误')
  })
})
