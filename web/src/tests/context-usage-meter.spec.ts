import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import ContextUsageMeter from '../features/chat/composer/ContextUsageMeter.vue'
import type { ContextUsage } from '../shared/contextUsage'

describe('ContextUsageMeter', () => {
  it('hides when usage is empty or has no tokens/limit', () => {
    const wrapper = mount(ContextUsageMeter, {
      props: {
        usage: undefined,
      },
    })
    expect(wrapper.find('.context-meter').exists()).toBe(false)
  })

  it('renders 240-degree gauge and percentage for normal usage', () => {
    const usage: ContextUsage = {
      used: 25000,
      limit: 100000,
      percent: 25,
      estimated: false,
      parts: { input: 20000, output: 5000, cacheRead: 0, cacheWrite: 0, reasoning: 0 },
    }

    const wrapper = mount(ContextUsageMeter, {
      props: {
        usage,
        contextDetail: '↑20k ↓5k',
      },
    })

    const meter = wrapper.find('.context-meter')
    expect(meter.exists()).toBe(true)
    expect(meter.classes()).not.toContain('warning')
    expect(meter.classes()).not.toContain('danger')
    expect(meter.text()).toContain('25%')
    expect(meter.attributes('title')).toContain('25% · 25k / 100k')
    expect(meter.attributes('title')).toContain('↑20k ↓5k')

    // SVG elements
    const svg = meter.find('svg')
    expect(svg.exists()).toBe(true)
    const fillPath = meter.find('.meter-fill')
    expect(fillPath.exists()).toBe(true)
    expect(fillPath.attributes('stroke-dashoffset')).toBeTruthy()
  })

  it('applies warning state when usage reaches 80%', () => {
    const usage: ContextUsage = {
      used: 85000,
      limit: 100000,
      percent: 85,
      estimated: false,
      parts: { input: 80000, output: 5000, cacheRead: 0, cacheWrite: 0, reasoning: 0 },
    }

    const wrapper = mount(ContextUsageMeter, {
      props: { usage },
    })

    const meter = wrapper.find('.context-meter')
    expect(meter.classes()).toContain('warning')
    expect(meter.attributes('title')).toContain('接近上下文上限')
  })

  it('applies danger state when usage reaches 95%', () => {
    const usage: ContextUsage = {
      used: 96000,
      limit: 100000,
      percent: 96,
      estimated: false,
      parts: { input: 90000, output: 6000, cacheRead: 0, cacheWrite: 0, reasoning: 0 },
    }

    const wrapper = mount(ContextUsageMeter, {
      props: { usage },
    })

    const meter = wrapper.find('.context-meter')
    expect(meter.classes()).toContain('danger')
  })
})
