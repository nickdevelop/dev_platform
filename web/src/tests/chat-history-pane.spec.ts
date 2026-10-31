// ChatHistoryPane tests: conversation list, running indicator,
// assistant selector dropdown, new-chat action, search, and delete/move dialogs.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import ChatHistoryPane from '../features/chat/pure/ChatHistoryPane.vue'
import { useChatsStore } from '../stores/chats'

const chat = (id: number, title: string, running = false, groupId = 0) => ({
  id,
  groupId,
  title,
  modelId: 1,
  modelName: 'Test Model',
  running,
  createdAt: '2026-10-22T00:00:00Z',
  updatedAt: '2026-10-22T00:00:00Z',
})

async function mountPane(path = '/chat', props = {}) {
  const pinia = createPinia()
  setActivePinia(pinia)
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/chat', name: 'chat', component: { template: '<div />' } },
      { path: '/chat/:chatId', name: 'chat-detail', component: { template: '<div />' } },
    ],
  })
  await router.push(path)
  await router.isReady()

  const chatsStore = useChatsStore()
  chatsStore.groups = [
    { id: 10, name: '工作助手', createdAt: '2026-10-22T00:00:00Z', updatedAt: '2026-10-22T00:00:00Z' },
    { id: 20, name: '学习助手', createdAt: '2026-10-22T00:00:00Z', updatedAt: '2026-10-22T00:00:00Z' },
  ]
  chatsStore.chats = [
    chat(1, '工作周报讨论', true, 10),
    chat(2, '架构设计评审', false, 10),
    chat(3, '算法学习心得', false, 20),
    chat(4, '未分类杂记', false, 0),
  ]
  chatsStore.selectedGroupId = 10

  const wrapper = mount(ChatHistoryPane, {
    props,
    global: { plugins: [pinia, router] },
  })
  await flushPromises()
  return { wrapper, router, chatsStore }
}

describe('ChatHistoryPane', () => {
  it('renders assistant selector with current assistant selected and lists matching conversations', async () => {
    const { wrapper } = await mountPane('/chat')

    const select = wrapper.find<HTMLSelectElement>('[data-test="assistant-select"]')
    expect(select.exists()).toBe(true)
    expect(select.element.value).toBe('10')

    const items = wrapper.findAll('[data-chat-id]')
    expect(items).toHaveLength(2)
    expect(items[0].text()).toContain('工作周报讨论')
    expect(items[0].find('[data-test="running-indicator"]').exists()).toBe(true)
    expect(items[1].text()).toContain('架构设计评审')
  })

  it('switches assistant from top selector and updates conversation list', async () => {
    const { wrapper, chatsStore } = await mountPane('/chat')

    const select = wrapper.find<HTMLSelectElement>('[data-test="assistant-select"]')
    await select.setValue('20')
    await flushPromises()

    expect(chatsStore.selectedGroupId).toBe(20)
    const items = wrapper.findAll('[data-chat-id]')
    expect(items).toHaveLength(1)
    expect(items[0].text()).toContain('算法学习心得')
  })

  it('filters conversations by search input', async () => {
    const { wrapper } = await mountPane('/chat')

    await wrapper.find('input[type="search"]').setValue('周报')
    const items = wrapper.findAll('[data-chat-id]')
    expect(items).toHaveLength(1)
    expect(items[0].text()).toContain('工作周报讨论')
  })

  it('navigates to conversation on click', async () => {
    const { wrapper, router } = await mountPane('/chat')

    await wrapper.find('[data-chat-id="2"]').trigger('click')
    await flushPromises()

    expect(router.currentRoute.value.name).toBe('chat-detail')
    expect(router.currentRoute.value.params.chatId).toBe('2')
  })

  it('triggers new chat on button click', async () => {
    const { wrapper, router } = await mountPane('/chat/2', { chatId: 2 })

    await wrapper.find('[data-test="history-new-chat-btn"]').trigger('click')
    await flushPromises()

    expect(router.currentRoute.value.name).toBe('chat')
  })

  it('opens delete conversation confirmation dialog', async () => {
    const { wrapper } = await mountPane('/chat')

    await wrapper.find('[data-chat-id="1"] button[title="删除对话"]').trigger('click')
    const dialog = wrapper.find('[data-test="delete-chat-dialog"]')
    expect(dialog.exists()).toBe(true)
    expect(dialog.text()).toContain('确定要删除对话「工作周报讨论」吗？')
  })
})
