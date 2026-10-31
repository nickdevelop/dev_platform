import { describe, it, expect, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import ProjectSidebar from '../features/projects/ProjectSidebar.vue'
import ChatSidebar from '../features/chat/pure/ChatSidebar.vue'
import TasksSidebar from '../features/tasks/TasksSidebar.vue'
import PromptsSidebar from '../features/prompts/PromptsSidebar.vue'
import TerminalSidebar from '../features/terminal/TerminalSidebar.vue'
import SettingsNav from '../features/settings/SettingsNav.vue'

describe('Column 2 (Sidebar) Design Consistency across menus', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('ProjectSidebar conforms to Data Terminal 3-zone standard', () => {
    const wrapper = mount(ProjectSidebar, {
      props: {
        activeProjectId: null,
        activeSessionId: null,
      },
    })

    // Zone A: Actions
    expect(wrapper.find('.sidebar-actions').exists()).toBe(true)
    expect(wrapper.find('.action-btn.register').exists()).toBe(true)
    expect(wrapper.find('.action-btn.create').exists()).toBe(true)

    // Zone B: Search with inset icon and filter
    expect(wrapper.find('.sidebar-search-bar').exists()).toBe(true)
    expect(wrapper.find('.sidebar-search-icon').exists()).toBe(true)
    expect(wrapper.find('.sidebar-filter-bar').exists()).toBe(true)
    expect(wrapper.find('[data-test="show-task-sessions"]').exists()).toBe(true)

    // Zone C: Tree container
    expect(wrapper.find('.tree').exists()).toBe(true)
  })

  it('ChatSidebar conforms to Data Terminal 3-zone standard', () => {
    const wrapper = mount(ChatSidebar)

    // Zone A: Action
    expect(wrapper.find('.sidebar-actions').exists()).toBe(true)
    expect(wrapper.find('[data-test="chat-new-group-btn"]').exists()).toBe(true)

    // Zone B: Search with inset icon
    expect(wrapper.find('.sidebar-search-bar').exists()).toBe(true)
    expect(wrapper.find('.sidebar-search-icon').exists()).toBe(true)

    // Zone C: Assistant list
    expect(wrapper.find('[data-test="assistant-list"]').exists()).toBe(true)
  })

  it('TasksSidebar conforms to Data Terminal 3-zone standard', () => {
    const wrapper = mount(TasksSidebar)

    // Zone A: Action
    expect(wrapper.find('.sidebar-actions').exists()).toBe(true)
    expect(wrapper.find('[data-test="btn-new-task"]').exists()).toBe(true)

    // Zone B: Search with inset icon and dropdown filters
    expect(wrapper.find('.sidebar-search-bar').exists()).toBe(true)
    expect(wrapper.find('.sidebar-search-icon').exists()).toBe(true)
    expect(wrapper.find('.sidebar-filters').exists()).toBe(true)

    // Zone C: Task list
    expect(wrapper.find('.task-list').exists()).toBe(true)
  })

  it('PromptsSidebar conforms to Data Terminal 3-zone standard', () => {
    const wrapper = mount(PromptsSidebar)

    // Zone A: Action
    expect(wrapper.find('.sidebar-actions').exists()).toBe(true)
    expect(wrapper.find('[data-test="new-prompt-btn"]').exists()).toBe(true)

    // Zone B: Search with inset icon
    expect(wrapper.find('.sidebar-search-bar').exists()).toBe(true)
    expect(wrapper.find('.sidebar-search-icon').exists()).toBe(true)

    // Zone C: Prompts list
    expect(wrapper.find('.prompts-list').exists()).toBe(true)
  })

  it('TerminalSidebar conforms to Data Terminal 3-zone standard', () => {
    const wrapper = mount(TerminalSidebar)

    // Zone A: Action
    expect(wrapper.find('.sidebar-actions').exists()).toBe(true)
    expect(wrapper.find('[data-test="create-server-btn"]').exists()).toBe(true)

    // Zone B: Search with inset icon
    expect(wrapper.find('.sidebar-search-bar').exists()).toBe(true)
    expect(wrapper.find('.sidebar-search-icon').exists()).toBe(true)

    // Zone C: Server list
    expect(wrapper.find('.server-list-container').exists()).toBe(true)
  })

  it('SettingsNav conforms to Data Terminal single-header standard', () => {
    const wrapper = mount(SettingsNav, {
      global: {
        stubs: {
          RouterLink: {
            template: '<a :class="$attrs.class"><slot /></a>',
          },
        },
      },
    })

    // Double header must be eliminated
    expect(wrapper.find('.settings-nav-header').exists()).toBe(false)

    // Zone C: Navigation list with standard cards
    expect(wrapper.find('.settings-links').exists()).toBe(true)
    const links = wrapper.findAll('.settings-link')
    expect(links.length).toBe(5)
    expect(links[0].classes()).toContain('sidebar-card')
  })
})
