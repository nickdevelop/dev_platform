import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { flushPromises, mount } from '@vue/test-utils'
import PromptsSidebar from '../features/prompts/PromptsSidebar.vue'
import PromptsView from '../features/prompts/PromptsView.vue'
import PromptManageDialog from '../features/prompts/PromptManageDialog.vue'
import { usePromptsStore } from '../stores/prompts'
import { useAuthStore } from '../stores/auth'

const routeParams: Record<string, string> = {}
const routerPush = vi.fn()

vi.mock('vue-router', () => ({
  useRoute: () => ({ params: routeParams }),
  useRouter: () => ({ push: routerPush }),
}))

describe('Prompts UI Components', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    useAuthStore().$patch({ authenticated: true, csrfToken: 'csrf-token' })
    Object.keys(routeParams).forEach((k) => delete routeParams[k])
    routerPush.mockClear()
    vi.restoreAllMocks()
  })

  it('PromptManageDialog validates inputs and inserts {user_prompt} placeholder', async () => {
    const store = usePromptsStore()
    const createSpy = vi.spyOn(store, 'createPrompt').mockResolvedValue({
      id: 1,
      name: '测试模板',
      description: '描述',
      content: 'Hello {user_prompt}',
      createdAt: '',
      updatedAt: '',
    })

    const wrapper = mount(PromptManageDialog)
    await flushPromises()

    // Submit empty should not trigger createPrompt
    await wrapper.find('form.prompt-dialog-form').trigger('submit')
    await flushPromises()
    expect(createSpy).not.toHaveBeenCalled()

    // Fill name
    await wrapper.find('[data-test="prompt-name-input"]').setValue('测试模板')
    // Click insert placeholder
    await wrapper.find('[data-test="btn-insert-placeholder"]').trigger('click')
    await flushPromises()

    const textarea = wrapper.find<HTMLTextAreaElement>('[data-test="prompt-content-input"]')
    expect(textarea.element.value).toContain('{user_prompt}')

    // Submit valid
    await wrapper.find('form.prompt-dialog-form').trigger('submit')
    await flushPromises()
    expect(createSpy).toHaveBeenCalledWith(
      expect.objectContaining({
        name: '测试模板',
        content: '{user_prompt}',
      }),
    )
    expect(wrapper.emitted('saved')).toBeTruthy()
  })

  it('PromptsSidebar displays list, supports filtering, and opens create dialog', async () => {
    const store = usePromptsStore()
    store.prompts = [
      { id: 1, name: 'Code Review', description: 'Review code', content: '{user_prompt}', createdAt: '', updatedAt: '' },
      { id: 2, name: 'Commit Helper', description: 'Write git commit', content: 'commit', createdAt: '', updatedAt: '' },
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

    // Open create dialog
    const head = wrapper.find('.sidebar-head')
    expect(head.find('h2').text()).toBe('提示词')
    expect(head.findAll('button')).toHaveLength(0)

    const actions = wrapper.find('.sidebar-actions')
    expect(actions.exists()).toBe(true)
    const newBtn = actions.find('[data-test="new-prompt-btn"]')
    expect(newBtn.exists()).toBe(true)
    await newBtn.trigger('click')
    await flushPromises()
    expect(wrapper.findComponent(PromptManageDialog).exists()).toBe(true)
  })

  it('PromptsView shows template content and provides real-time placeholder replacement preview', async () => {
    const store = usePromptsStore()
    store.prompts = [
      {
        id: 1,
        name: '代码审查',
        description: '审查代码',
        content: '请审查：\n\n{user_prompt}\n\n谢谢。',
        createdAt: '2026-10-18T10:00:00Z',
        updatedAt: '2026-10-18T12:00:00Z',
      },
    ]
    routeParams.promptId = '1'

    const wrapper = mount(PromptsView)
    await flushPromises()

    expect(wrapper.text()).toContain('代码审查')
    expect(wrapper.text()).toContain('请审查：')
    expect(wrapper.text()).toContain('{user_prompt}')

    // Enter test text in preview
    await wrapper.find('[data-test="prompt-test-input"]').setValue('const x = 1;')
    await flushPromises()

    const previewOutput = wrapper.find('[data-test="prompt-preview-output"]')
    expect(previewOutput.text()).toBe('请审查：\n\nconst x = 1;\n\n谢谢。')
  })
})
