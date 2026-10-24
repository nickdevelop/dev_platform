// Pure-chat sidebar: loads the global conversation list, filters titles
// locally, and makes both "new chat" and row selection navigate.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import ChatSidebar from '../features/chat/pure/ChatSidebar.vue'

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

async function mountSidebar(path = '/chat') {
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
  const wrapper = mount(ChatSidebar, { global: { plugins: [pinia, router] } })
  await flushPromises()
  return { wrapper, router }
}

beforeEach(() => {
  vi.stubGlobal(
    'fetch',
    vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url.includes('/api/chat-groups')) {
        return {
          status: 200,
          json: async () => ({
            groups: [
              {
                id: 10,
                name: '工作分组',
                createdAt: '2026-10-22T00:00:00Z',
                updatedAt: '2026-10-22T00:00:00Z',
              },
            ],
          }),
        } as Response
      }
      return {
        status: 200,
        json: async () => ({
          chats: [
            chat(1, '第一个对话', true, 10),
            chat(2, '购物清单', false, 0),
          ],
          total: 2,
        }),
      } as Response
    }),
  )
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('pure chat sidebar', () => {
  it('lists conversations and marks the running one', async () => {
    const { wrapper } = await mountSidebar('/chat/2')
    const items = wrapper.findAll('[data-chat-id]')
    expect(items).toHaveLength(2)
    expect(items[0].text()).toContain('第一个对话')
    expect(items[0].find('[data-test="running-indicator"]').exists()).toBe(true)
    expect(items[1].classes()).toContain('active')
  })

  it('filters by title and opens a conversation', async () => {
    const { wrapper, router } = await mountSidebar('/chat')
    await wrapper.find('input[type="search"]').setValue('购物')
    expect(wrapper.findAll('[data-chat-id]')).toHaveLength(1)

    await wrapper.find('[data-chat-id="2"]').trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.name).toBe('chat-detail')
    expect(router.currentRoute.value.params.chatId).toBe('2')
  })

  it('starts a new chat on assistant from any chat route', async () => {
    const { wrapper, router } = await mountSidebar('/chat/1')
    await wrapper.find('[aria-label="在「工作分组」中新建对话"]').trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.name).toBe('chat')
  })

  it('renders groups, ungrouped section, and opens create group dialog', async () => {
    const { wrapper } = await mountSidebar('/chat')
    expect(wrapper.text()).toContain('工作分组')
    expect(wrapper.text()).toContain('未分组对话')

    // Open create assistant dialog
    await wrapper.find('[data-test="chat-new-group-btn"]').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('新建助手')
  })
})

describe('assistant lifecycle and unlocked controls', () => {
  it('does not mark the assistant fixed and exposes edit, delete and create controls', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: RequestInfo | URL) => {
        const url = String(input)
        if (url.includes('/api/chat-groups')) {
          return {
            status: 200,
            json: async () => ({
              groups: [
                {
                  id: 7,
                  name: '快捷助手',
                  createdAt: '2026-10-23T00:00:00Z',
                  updatedAt: '2026-10-23T00:00:00Z',
                },
              ],
            }),
          } as Response
        }
        return {
          status: 200,
          json: async () => ({
            chats: [chat(3, '快捷对话', false, 7)],
            total: 1,
          }),
        } as Response
      }),
    )

    const { wrapper } = await mountSidebar('/chat')
    expect(wrapper.text()).toContain('快捷助手')
    expect(wrapper.text()).not.toContain('固定')
    expect(wrapper.find('[aria-label="编辑助手「快捷助手」"]').exists()).toBe(true)
    expect(wrapper.find('[aria-label="删除助手「快捷助手」"]').exists()).toBe(true)
    expect(wrapper.find('[aria-label="在「快捷助手」中新建对话"]').exists()).toBe(true)
  })
})
