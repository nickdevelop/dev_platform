// Unit tests for the project settings panel: project facts and the danger-zone
// actions that hand off to the manage dialog. Backend diagnostics and machine
// services belong to platform settings, so this panel must not touch them.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import ProjectSettingsPanel from '../features/projects/ProjectSettingsPanel.vue'
import { useProjectsStore } from '../stores/projects'

function mountPanel() {
  return mount(ProjectSettingsPanel, { props: { projectId: 1 } })
}

describe('ProjectSettingsPanel', () => {
  let fetchSpy: ReturnType<typeof vi.fn>

  beforeEach(() => {
    setActivePinia(createPinia())
    fetchSpy = vi.fn()
    vi.stubGlobal('fetch', fetchSpy)

    const projects = useProjectsStore()
    projects.projects = [{ id: 1, name: 'alpha', path: '/workspace/alpha' } as never]
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('shows the project name and path', async () => {
    const wrapper = mountPanel()
    await flushPromises()

    expect(wrapper.text()).toContain('alpha')
    expect(wrapper.text()).toContain('/workspace/alpha')
  })

  it('hands the danger-zone actions to the manage dialog with the project', async () => {
    const wrapper = mountPanel()
    await flushPromises()

    const pick = (label: string) => wrapper.findAll('button').find((item) => item.text() === label)!
    await pick('重命名').trigger('click')
    await pick('移动').trigger('click')
    await pick('解除登记').trigger('click')
    expect(wrapper.findAll('button').some((item) => item.text() === '删除目录')).toBe(false)

    expect(wrapper.emitted('manage')?.map((entry) => entry[0])).toEqual([
      'rename',
      'move',
      'unregister',
    ])
    expect((wrapper.emitted('manage')![0][1] as { id: number }).id).toBe(1)
  })

  // Diagnostics and services moved to Platform Settings → Agent. This panel
  // must stay quiet about them and make no agent API calls.
  it('shows no backend diagnostics or service notice and calls no agent API', async () => {
    const wrapper = mountPanel()
    await flushPromises()

    expect(wrapper.text()).not.toContain('后端诊断')
    expect(wrapper.find('[data-test="project-service-down"]').exists()).toBe(false)
    expect(wrapper.find('[data-backend-diagnostics]').exists()).toBe(false)
    expect(fetchSpy).not.toHaveBeenCalled()
  })
})
