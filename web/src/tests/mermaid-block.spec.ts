import { beforeEach, describe, expect, it } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import MermaidBlock from '../features/chat/rendering/MermaidBlock.vue'
import { clearMermaidCache, configureMermaidEngine } from '../mermaid/renderer'

const SAMPLE_SOURCE = 'graph TD\n  A --> B'

describe('MermaidBlock', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    clearMermaidCache()
    configureMermaidEngine(null)
  })

  it('renders svg by default when ready', async () => {
    configureMermaidEngine(async () => '<svg><text>test-diagram</text></svg>')
    const wrapper = mount(MermaidBlock, {
      props: { source: SAMPLE_SOURCE, ready: true, finished: true },
    })
    await flushPromises()

    expect(wrapper.find('[data-test="mermaid-canvas"]').html()).toContain('<svg')
    expect(wrapper.find('[data-test="mermaid-source"]').exists()).toBe(false)
    expect(wrapper.find('.mermaid-head').text()).toContain('图表 · mermaid')
  })

  it('switches between preview and source when clicking head row', async () => {
    configureMermaidEngine(async () => '<svg/>')
    const wrapper = mount(MermaidBlock, {
      props: { source: SAMPLE_SOURCE, ready: true, finished: true },
    })
    await flushPromises()

    await wrapper.find('.mermaid-head').trigger('click')
    expect(wrapper.find('[data-test="mermaid-source"]').text()).toContain('graph TD')
    expect(wrapper.find('[data-test="mermaid-canvas"]').exists()).toBe(false)

    await wrapper.find('.mermaid-head').trigger('click')
    expect(wrapper.find('[data-test="mermaid-canvas"]').exists()).toBe(true)
  })

  it('waits for fence to close during streaming and does not call engine', async () => {
    let callCount = 0
    configureMermaidEngine(async () => {
      callCount++
      return '<svg/>'
    })
    const wrapper = mount(MermaidBlock, {
      props: { source: 'graph TD\n  A', ready: false, finished: false },
    })
    await flushPromises()

    expect(callCount).toBe(0)
    expect(wrapper.find('[data-test="mermaid-waiting"]').exists()).toBe(true)

    await wrapper.setProps({ ready: true })
    await flushPromises()
    expect(callCount).toBe(1)
  })

  it('shows error and displays source when rendering fails', async () => {
    configureMermaidEngine(async () => {
      throw new Error('Mermaid parse error')
    })
    const wrapper = mount(MermaidBlock, {
      props: { source: 'invalid mermaid syntax', ready: true, finished: true },
    })
    await flushPromises()

    expect(wrapper.find('[data-test="mermaid-error"]').text()).toContain('无法渲染: Mermaid parse error')
    expect(wrapper.find('[data-test="mermaid-source"]').text()).toContain('invalid mermaid syntax')
  })

  it('preserves text labels inside svg elements after rendering', async () => {
    configureMermaidEngine(async () => '<svg><g class="node"><text class="nodeLabel"><tspan>核心流程节点</tspan></text></g></svg>')
    const wrapper = mount(MermaidBlock, {
      props: { source: 'graph TD\n  A[核心流程节点]', ready: true, finished: true },
    })
    await flushPromises()

    const canvas = wrapper.find('[data-test="mermaid-canvas"]')
    expect(canvas.html()).toContain('核心流程节点')
    expect(canvas.find('text').exists()).toBe(true)
  })
})
