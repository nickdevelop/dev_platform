import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import ChatSideRail from '../features/chat/pane/ChatSideRail.vue'

describe('ChatSideRail', () => {
  it('renders a vertical toolbar with the requested side and accessible name', () => {
    const wrapper = mount(ChatSideRail, {
      props: { side: 'left', label: '文件', ariaLabel: '会话文件导航' },
      slots: { default: '<button class="rail-btn is-active">tree</button>' },
    })

    expect(wrapper.classes()).toContain('chat-side-rail')
    expect(wrapper.classes()).toContain('left')
    expect(wrapper.attributes('role')).toBe('toolbar')
    expect(wrapper.attributes('aria-orientation')).toBe('vertical')
    expect(wrapper.attributes('aria-label')).toBe('会话文件导航')
    expect(wrapper.attributes('data-test')).toBe('chat-side-rail-left')
    expect(wrapper.find('button.rail-btn').exists()).toBe(true)
  })

  it('renders the short vertical label without exposing it to assistive tech twice', () => {
    const wrapper = mount(ChatSideRail, {
      props: { side: 'right', label: '预览', ariaLabel: '会话预览导航' },
    })

    expect(wrapper.classes()).toContain('right')
    expect(wrapper.attributes('data-test')).toBe('chat-side-rail-right')
    const label = wrapper.find('.rail-label')
    expect(label.text()).toBe('预览')
    expect(label.attributes('aria-hidden')).toBe('true')
  })
})
