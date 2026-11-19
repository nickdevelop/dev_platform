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

  it('renders CodeComment XML as structured card with snippet summary', async () => {
    renderedHeight(50)
    const xml = [
      '<CodeComment path="src/main.ts" line="10-15">',
      '<Description>',
      '检查挂载点逻辑',
      '</Description>',
      '<Snippet>',
      'line 1\nline 2\nline 3\nline 4\nline 5',
      '</Snippet>',
      '</CodeComment>',
      '',
      '用户提问正文',
    ].join('\n')

    const wrapper = mount(FoldableUserText, { props: { text: xml } })
    await wrapper.vm.$nextTick()

    const card = wrapper.find('[data-test="user-code-comment-card"]')
    expect(card.exists()).toBe(true)
    expect(wrapper.find('.comment-card-path').text()).toBe('src/main.ts')
    expect(wrapper.find('.comment-card-line').text()).toBe('L10-15')
    expect(wrapper.find('[data-test="comment-card-desc"]').text()).toBe('检查挂载点逻辑')
    expect(wrapper.find('[data-test="user-text-body"]').text()).toBe('用户提问正文')

    // Code snippet summary button should be present since it is 5 lines > 3 lines
    const snippetToggle = wrapper.find('[data-test="snippet-toggle-btn"]')
    expect(snippetToggle.exists()).toBe(true)
    expect(snippetToggle.text()).toContain('展开完整代码 (共 5 行)')

    // Clicking toggle expands full snippet
    await snippetToggle.trigger('click')
    expect(snippetToggle.text()).toBe('收起代码')

    // Clicking header emits open-file event
    await wrapper.find('[data-test="comment-card-header"]').trigger('click')
    expect(wrapper.emitted('open-file')?.[0]).toEqual(['src/main.ts', 10])
  })
})
