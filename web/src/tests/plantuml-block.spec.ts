// The diagram block is the part of the chat a person actually looks at, so its
// tests pin what they get: a drawing by default, the source one click away, a
// reason plus the source when drawing fails, and no engine work at all while a
// reply is still arriving.
import { beforeEach, describe, expect, it } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import PlantumlBlock from '../components/PlantumlBlock.vue'
import { clearDiagramCache, configureEngineLoader, type Engine } from '../plantuml/renderer'
import { useSettingsStore } from '../stores/settings'

function fakeEngine(
  render: (lines: string[], opts: { dark: boolean }) => string | Promise<string>,
): Engine {
  return (lines, opts, onSuccess, onError) => {
    void Promise.resolve(render(lines, opts)).then(onSuccess, onError)
  }
}

const SOURCE = '@startuml\nA->B\n@enduml'

describe('PlantumlBlock', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    clearDiagramCache()
    configureEngineLoader(null)
  })

  it('shows the diagram by default once ready', async () => {
    configureEngineLoader(fakeEngine(() => '<svg><text>seq</text></svg>'))
    const wrapper = mount(PlantumlBlock, { props: { source: SOURCE, ready: true, finished: true } })
    await flushPromises()

    expect(wrapper.find('[data-test="plantuml-canvas"]').html()).toContain('<svg')
    expect(wrapper.find('[data-test="plantuml-source"]').exists()).toBe(false)
    expect(wrapper.find('.plantuml-head').text()).toContain('图表 · plantuml')
  })

  it('switches to the source and back with the head row', async () => {
    configureEngineLoader(fakeEngine(() => '<svg/>'))
    const wrapper = mount(PlantumlBlock, { props: { source: SOURCE, ready: true, finished: true } })
    await flushPromises()

    await wrapper.find('.plantuml-head').trigger('click')
    expect(wrapper.find('[data-test="plantuml-source"]').text()).toContain('@startuml')
    expect(wrapper.find('[data-test="plantuml-canvas"]').exists()).toBe(false)
    expect(wrapper.find('.plantuml-head').text()).toContain('返回预览')

    await wrapper.find('.plantuml-head').trigger('click')
    expect(wrapper.find('[data-test="plantuml-canvas"]').exists()).toBe(true)
    expect(wrapper.find('.plantuml-head').text()).toContain('查看源码')
  })

  it('carries no fold marker: the head row switches the view, it never collapses', async () => {
    // The row used to lead with a ▾/▸ chevron, the same glyph the thinking and
    // tool blocks use for "click to open this part". Clicking it only swaps the
    // drawing for its source, which is what the label on the right already says
    // — so the chevron promised a fold that never happens.
    configureEngineLoader(fakeEngine(() => '<svg/>'))
    const wrapper = mount(PlantumlBlock, { props: { source: SOURCE, ready: true, finished: true } })
    await flushPromises()

    const head = wrapper.find('.plantuml-head')
    expect(head.text()).not.toMatch(/[▾▸]/)
    expect(head.attributes('aria-expanded')).toBeUndefined()

    await head.trigger('click')
    expect(head.text()).not.toMatch(/[▾▸]/)
    expect(head.attributes('aria-expanded')).toBeUndefined()
  })

  it('waits for a closed fence and never asks the engine while streaming', async () => {
    let calls = 0
    configureEngineLoader(
      fakeEngine(() => {
        calls++
        return '<svg/>'
      }),
    )
    const wrapper = mount(PlantumlBlock, { props: { source: '@startuml\nA->B', ready: false, finished: false } })
    await flushPromises()

    expect(calls).toBe(0)
    expect(wrapper.find('[data-test="plantuml-waiting"]').exists()).toBe(true)

    await wrapper.setProps({ ready: true })
    await flushPromises()
    expect(calls).toBe(1)
  })

  it('reveals the source with the reason when rendering fails', async () => {
    configureEngineLoader(
      fakeEngine(() => {
        throw new Error('boom')
      }),
    )
    const wrapper = mount(PlantumlBlock, {
      props: { source: '@startuml\n!include <C4/C4_Container>\n@enduml', ready: true, finished: true },
    })
    await flushPromises()

    expect(wrapper.find('[data-test="plantuml-error"]').text()).toContain('无法渲染')
    expect(wrapper.find('[data-test="plantuml-source"]').text()).toContain('!include')
    expect(wrapper.find('[data-test="plantuml-canvas"]').exists()).toBe(false)
  })

  it('shows the source when the reply ended before the fence closed', async () => {
    // A stopped or truncated reply is final: an empty block that waits forever
    // would hide the text the person came to read.
    let calls = 0
    configureEngineLoader(
      fakeEngine(() => {
        calls++
        return '<svg/>'
      }),
    )
    const wrapper = mount(PlantumlBlock, {
      props: { source: '@startuml\nA->B', ready: false, finished: false },
    })
    await flushPromises()
    expect(wrapper.find('[data-test="plantuml-waiting"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="plantuml-source"]').exists()).toBe(false)

    await wrapper.setProps({ finished: true })
    await flushPromises()
    expect(calls).toBe(0)
    expect(wrapper.find('[data-test="plantuml-source"]').text()).toContain('A->B')
    expect(wrapper.find('[data-test="plantuml-waiting"]').text()).toContain('没画完')
    expect(wrapper.find('.plantuml-head').attributes('disabled')).toBeDefined()
  })

  it('leaves the source open when the palette changes under it', async () => {
    configureEngineLoader(fakeEngine(() => '<svg/>'))
    const wrapper = mount(PlantumlBlock, { props: { source: SOURCE, ready: true, finished: true } })
    await flushPromises()
    await wrapper.find('.plantuml-head').trigger('click')
    expect(wrapper.find('[data-test="plantuml-source"]').exists()).toBe(true)

    const settings = useSettingsStore()
    settings.appearance = { ...settings.appearance, theme: 'dark' }
    await flushPromises()
    expect(wrapper.find('[data-test="plantuml-source"]').exists()).toBe(true)
  })

  it('sits the drawing on a dark canvas only when the palette is dark', async () => {
    configureEngineLoader(fakeEngine(() => '<svg><text>x</text></svg>'))
    const settings = useSettingsStore()
    settings.appearance = { ...settings.appearance, theme: 'dark' }
    const dark = mount(PlantumlBlock, { props: { source: SOURCE, ready: true, finished: true } })
    await flushPromises()
    expect(dark.find('[data-test="plantuml-canvas"]').classes()).toContain('is-dark')
    dark.unmount()

    settings.appearance = { ...settings.appearance, theme: 'light' }
    const light = mount(PlantumlBlock, { props: { source: SOURCE, ready: true, finished: true } })
    await flushPromises()
    expect(light.find('[data-test="plantuml-canvas"]').classes()).not.toContain('is-dark')
  })

  it('renders in dark mode for the dark palette only', async () => {
    const seen: boolean[] = []
    configureEngineLoader(
      fakeEngine((_lines, opts) => {
        seen.push(opts.dark)
        return '<svg/>'
      }),
    )
    const settings = useSettingsStore()
    settings.appearance = { ...settings.appearance, theme: 'cool' }
    const light = mount(PlantumlBlock, { props: { source: SOURCE, ready: true, finished: true } })
    await flushPromises()

    // The light block is unmounted so it cannot redraw itself when the palette
    // changes; this case is about which flag a freshly mounted block sends.
    light.unmount()
    settings.appearance = { ...settings.appearance, theme: 'dark' }
    mount(PlantumlBlock, { props: { source: SOURCE, ready: true, finished: true } })
    await flushPromises()

    expect(seen).toEqual([false, true])
  })

  it('opens the drawing in a popup viewer and closes it again', async () => {
    configureEngineLoader(fakeEngine(() => '<svg><text>seq</text></svg>'))
    const wrapper = mount(PlantumlBlock, {
      props: { source: SOURCE, ready: true, finished: true },
      attachTo: document.body,
    })
    await flushPromises()

    expect(wrapper.find('[data-test="plantuml-expand"]').exists()).toBe(true)
    expect(document.body.querySelector('[data-test="plantuml-viewer"]')).toBeNull()

    await wrapper.find('[data-test="plantuml-expand"]').trigger('click')
    await flushPromises()
    const viewer = document.body.querySelector('[data-test="plantuml-viewer"]')
    expect(viewer).not.toBeNull()
    expect(viewer?.innerHTML).toContain('<svg')
    expect(viewer?.querySelector('[data-test="plantuml-zoom-reset"]')?.textContent).toContain('100%')

    const close = document.body.querySelector(
      '[data-test="plantuml-viewer-close"]',
    ) as HTMLElement
    close.click()
    await flushPromises()
    expect(document.body.querySelector('[data-test="plantuml-viewer"]')).toBeNull()
    wrapper.unmount()
  })

  it('zooms the viewer and drags it to pan', async () => {
    configureEngineLoader(fakeEngine(() => '<svg/>'))
    const wrapper = mount(PlantumlBlock, {
      props: { source: SOURCE, ready: true, finished: true },
      attachTo: document.body,
    })
    await flushPromises()
    await wrapper.find('[data-test="plantuml-expand"]').trigger('click')
    await flushPromises()

    const zoomIn = document.body.querySelector('[data-test="plantuml-zoom-in"]') as HTMLElement
    zoomIn.click()
    await flushPromises()
    expect(
      document.body.querySelector('[data-test="plantuml-zoom-reset"]')?.textContent,
    ).toContain('125%')

    const canvas = document.body.querySelector(
      '[data-test="plantuml-viewer-canvas"]',
    ) as HTMLElement
    const content = document.body.querySelector(
      '[data-test="plantuml-viewer-content"]',
    ) as HTMLElement
    const before = content.style.transform
    canvas.dispatchEvent(new MouseEvent('mousedown', { button: 0, clientX: 100, clientY: 100, bubbles: true }))
    canvas.dispatchEvent(new MouseEvent('mousemove', { clientX: 130, clientY: 140, bubbles: true }))
    canvas.dispatchEvent(new MouseEvent('mouseup', { bubbles: true }))
    await flushPromises()
    expect(content.style.transform).not.toBe(before)
    expect(content.style.transform).toContain('translate(30px, 40px')

    const zoomOut = document.body.querySelector('[data-test="plantuml-zoom-out"]') as HTMLElement
    zoomOut.click()
    await flushPromises()
    expect(
      document.body.querySelector('[data-test="plantuml-zoom-reset"]')?.textContent,
    ).toContain('100%')
    wrapper.unmount()
  })

  it('hides the popup button until there is a drawing to enlarge', async () => {
    configureEngineLoader(fakeEngine(() => '<svg/>'))
    const waiting = mount(PlantumlBlock, {
      props: { source: '@startuml\nA->B', ready: false, finished: false },
    })
    await flushPromises()
    expect(waiting.find('[data-test="plantuml-expand"]').exists()).toBe(false)
    waiting.unmount()

    configureEngineLoader(
      fakeEngine(() => {
        throw new Error('boom')
      }),
    )
    const failed = mount(PlantumlBlock, {
      props: { source: SOURCE, ready: true, finished: true },
    })
    await flushPromises()
    expect(failed.find('[data-test="plantuml-expand"]').exists()).toBe(false)
    failed.unmount()
  })
})
