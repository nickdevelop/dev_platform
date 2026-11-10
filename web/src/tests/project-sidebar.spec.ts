import { beforeEach, describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import ProjectSidebar from '../features/projects/ProjectSidebar.vue'
import ProjectTree from '../features/projects/ProjectTree.vue'

describe('ProjectSidebar', () => {
  beforeEach(() => {
    localStorage.clear()
    setActivePinia(createPinia())
  })

  it('forwards the project settings request from the tree to the shell', () => {
    const wrapper = mount(ProjectSidebar, {
      props: { activeProjectId: null, activeSessionId: null },
      global: { stubs: { ProjectTree: true } },
    })

    wrapper.findComponent(ProjectTree).vm.$emit('open-settings', 7)

    expect(wrapper.emitted('open-settings')).toEqual([[7]])
  })
})
