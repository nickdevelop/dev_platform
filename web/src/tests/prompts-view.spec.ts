import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { flushPromises, mount } from '@vue/test-utils'
import PromptsSidebar from '../features/prompts/PromptsSidebar.vue'
import PromptsView from '../features/prompts/PromptsView.vue'
import PromptManageDialog from '../features/prompts/PromptManageDialog.vue'
import { usePromptsStore } from '../stores/prompts'
import { useAuthStore } from '../stores/auth'
import { installTestCSRF } from './csrf-fixture'

const routeParams: Record<string, string> = {}
const routerPush = vi.fn()

vi.mock('vue-router', () => ({
  useRoute: () => ({ params: routeParams }),
  useRouter: () => ({ push: routerPush }),
}))

describe('Prompts UI Components', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    useAuthStore().$patch({ authenticated: true })
  installTestCSRF('csrf-token')
    Object.keys(routeParams).forEach((k) => delete routeParams[k])
    routerPush.mockClear()
    vi.restoreAllMocks()
  })

  it('PromptManageDialog validates inputs and saves the prompt content', async () => {
    const store = usePromptsStore()
    const createSpy = vi.spyOn(store, 'createPrompt').mockResolvedValue({
      id: 1,
      name: '测试提示词',
      type: 'role',
      description: '描述',
      content: 'Hello',
      createdAt: '',
      updatedAt: '',
    })

    const wrapper = mount(PromptManageDialog)
    await flushPromises()

    // Submit empty should not trigger createPrompt
    await wrapper.find('form.prompt-dialog-form').trigger('submit')
    await flushPromises()
    expect(createSpy).not.toHaveBeenCalled()

    // The placeholder-insertion control is gone.
    expect(wrapper.find('[data-test="btn-insert-placeholder"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('提示词内容')
    expect(wrapper.text()).not.toContain('模版')

    // Fill name and content directly.
    await wrapper.find('[data-test="prompt-name-input"]').setValue('测试提示词')
    await wrapper.find('[data-test="prompt-content-input"]').setValue('Hello')

    // Submit valid
    await wrapper.find('form.prompt-dialog-form').trigger('submit')
    await flushPromises()
    expect(createSpy).toHaveBeenCalledWith(
      expect.objectContaining({
        name: '测试提示词',
        content: 'Hello',
      }),
    )
    expect(wrapper.emitted('saved')).toBeTruthy()
  })

  it('PromptsSidebar displays list, supports filtering, and opens create dialog', async () => {
    const store = usePromptsStore()
    store.prompts = [
      { id: 1, name: 'Code Review', type: 'role', description: 'Review code', content: 'Review the code', createdAt: '', updatedAt: '' },
      { id: 2, name: 'Commit Helper', type: 'role', description: 'Write git commit', content: 'commit', createdAt: '', updatedAt: '' },
    ]
    vi.spyOn(store, 'loadPrompts').mockResolvedValue(undefined as never)
    routeParams.promptId = '1'

    const wrapper = mount(PromptsSidebar)
    await flushPromises()

    // 2 prompts displayed
    expect(wrapper.findAll('.prompt-card')).toHaveLength(2)
    // Card 1 is active
    expect(wrapper.find('[data-test="prompt-card-1"]').classes()).toContain('active')

    // Filter by keyword
    await wrapper.find('[data-test="prompt-search-input"]').setValue('commit')
    await flushPromises()
    expect(wrapper.findAll('.prompt-card')).toHaveLength(1)
    expect(wrapper.find('[data-test="prompt-card-2"]').exists()).toBe(true)

    // Open create dialog via the dedicated actions row below the shared title
    expect(wrapper.find('.sidebar-head').exists()).toBe(false)
    expect(wrapper.find('h2').exists()).toBe(false)

    const actions = wrapper.find('.sidebar-actions')
    expect(actions.exists()).toBe(true)
    expect(actions.findAll('button')).toHaveLength(1)
    const newBtn = actions.find('[data-test="new-prompt-btn"]')
    expect(newBtn.exists()).toBe(true)
    await newBtn.trigger('click')
    await flushPromises()
    expect(wrapper.findComponent(PromptManageDialog).exists()).toBe(true)
  })

  it('PromptsView shows prompt content without placeholder badges', async () => {
    const store = usePromptsStore()
    store.prompts = [
      {
        id: 1,
        name: '代码审查',
        type: 'role',
        description: '审查代码',
        content: '请审查以下代码并给出建议。',
        createdAt: '2026-10-18T10:00:00Z',
        updatedAt: '2026-10-18T12:00:00Z',
      },
    ]
    routeParams.promptId = '1'

    const wrapper = mount(PromptsView)
    await flushPromises()

    expect(wrapper.text()).toContain('代码审查')
    expect(wrapper.text()).toContain('请审查以下代码并给出建议。')
    expect(wrapper.text()).toContain('提示词正文')
    expect(wrapper.text()).not.toContain('{user_prompt}')
    expect(wrapper.text()).not.toContain('模版')

    // Verify real-time synthesis input and output are removed
    expect(wrapper.find('[data-test="prompt-test-input"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="prompt-preview-output"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="copy-template-btn"]').exists()).toBe(true)
  })
})
