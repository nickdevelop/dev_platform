import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { flushPromises, mount } from '@vue/test-utils'
import PromptManageDialog from '../features/prompts/PromptManageDialog.vue'
import { usePromptsStore } from '../stores/prompts'
import { useAuthStore } from '../stores/auth'
import { installTestCSRF } from './csrf-fixture'

describe('PromptManageDialog Component', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    useAuthStore().$patch({ authenticated: true })
    installTestCSRF('csrf-token')
    vi.restoreAllMocks()
  })

  it('validates inputs and saves the prompt content', async () => {
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

  it('supports editing an existing prompt with initial values', async () => {
    const store = usePromptsStore()
    const updateSpy = vi.spyOn(store, 'updatePrompt').mockResolvedValue({
      id: 2,
      name: '修改后的提示词',
      type: 'flow',
      description: '修改后的描述',
      content: 'New body',
      createdAt: '',
      updatedAt: '',
    })

    const wrapper = mount(PromptManageDialog, {
      props: {
        prompt: {
          id: 2,
          name: '原提示词',
          type: 'role',
          description: '原描述',
          content: 'Old body',
          createdAt: '',
          updatedAt: '',
        },
      },
    })
    await flushPromises()

    expect(wrapper.text()).toContain('编辑 AI 提示词')
    await wrapper.find('[data-test="prompt-name-input"]').setValue('修改后的提示词')
    await wrapper.find('form.prompt-dialog-form').trigger('submit')
    await flushPromises()

    expect(updateSpy).toHaveBeenCalledWith(
      2,
      expect.objectContaining({
        name: '修改后的提示词',
      }),
    )
    expect(wrapper.emitted('saved')).toBeTruthy()
  })
})
