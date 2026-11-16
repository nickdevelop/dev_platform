import { mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import FoldableUserText from '../features/chat/rendering/FoldableUserText.vue'

// Line height is 20px, so a 10-line fold is 200px. scrollHeight stands in for
// the rendered height of the text.
function renderedHeight(px: number) {
  vi.spyOn(Element.prototype, 'scrollHeight', 'get').mockReturnValue(px)
  vi.spyOn(window, 'getComputedStyle').mockReturnValue({
    lineHeight: '20px',
    fontSize: '14px',
  } as CSSStyleDeclaration)
}

describe('FoldableUserText', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('shows short text in full without a toggle', async () => {
    renderedHeight(180)
    const wrapper = mount(FoldableUserText, { props: { text: 'short' } })
    await wrapper.vm.$nextTick()

    expect(wrapper.find('[data-test="user-text-toggle"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="user-text-body"]').classes()).not.toContain('is-folded')
  })

  it('treats exactly ten lines as not overflowing', async () => {
    renderedHeight(200)
    const wrapper = mount(FoldableUserText, { props: { text: 'ten' } })
    await wrapper.vm.$nextTick()

    expect(wrapper.find('[data-test="user-text-toggle"]').exists()).toBe(false)
  })

  it('folds text beyond ten visual lines and toggles it open and closed', async () => {
    renderedHeight(420)
    const wrapper = mount(FoldableUserText, { props: { text: 'long' } })
    await wrapper.vm.$nextTick()

    const body = wrapper.find('[data-test="user-text-body"]')
    const toggle = wrapper.find('[data-test="user-text-toggle"]')
    expect(body.classes()).toContain('is-folded')
    expect(body.attributes('style')).toContain('max-height: 200px')
    expect(toggle.text()).toBe('展开')
    expect(toggle.attributes('aria-expanded')).toBe('false')

    await toggle.trigger('click')
    expect(body.classes()).not.toContain('is-folded')
    expect(body.attributes('style') ?? '').not.toContain('max-height')
    expect(toggle.text()).toBe('收起')
    expect(toggle.attributes('aria-expanded')).toBe('true')

    await toggle.trigger('click')
    expect(body.classes()).toContain('is-folded')
    expect(toggle.text()).toBe('展开')
  })
})
