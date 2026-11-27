import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import ElementPlus from 'element-plus'

const routerMocks = vi.hoisted(() => ({
  push: vi.fn(),
  replace: vi.fn(),
}))

vi.mock('vue-router', () => ({
  useRouter: () => ({
    push: routerMocks.push,
    replace: routerMocks.replace,
  }),
  useRoute: () => ({
    query: {},
    params: {},
    path: '/org/channels',
  }),
}))

const mockState = vi.hoisted(() => ({
  descriptors: [
    {
      type: 'wechat',
      label: '个人微信',
      description: '通过 iLink Bot 协议接入个人微信',
      supportsQrLogin: true,
    },
  ],
  channels: [
    {
      id: 101,
      name: '微信研发助手渠道',
      channelType: 'wechat',
      enabled: true,
      status: 'connected',
      hasCredentials: true,
      accountId: 'wx-bot-1',
      targetMode: 'assistant',
      modelId: 0,
      promptId: 0,
      defaultProjectId: 0,
      worktreeId: 0,
      chatGroupId: 20,
      chatId: 300,
      typingIndicator: true,
      autoNotify: true,
      maxReplyLength: 2000,
      lastConnectedAt: '2026-11-26T08:00:00Z',
      createdAt: '2026-11-26T08:00:00Z',
      updatedAt: '2026-11-26T08:00:00Z',
    },
    {
      id: 102,
      name: '代码审查微信通道',
      channelType: 'wechat',
      enabled: true,
      status: 'connected',
      hasCredentials: true,
      accountId: 'wx-bot-2',
      targetMode: 'agent',
      modelId: 0,
      promptId: 5,
      defaultProjectId: 1,
      worktreeId: 0,
      defaultBackend: 'builtin',
      agentModel: 'gpt-4o',
      chatGroupId: 0,
      chatId: 0,
      activeSessionId: 'ses-bot-99',
      typingIndicator: true,
      autoNotify: true,
      maxReplyLength: 2000,
      lastConnectedAt: '2026-11-26T08:30:00Z',
      createdAt: '2026-11-26T08:30:00Z',
      updatedAt: '2026-11-26T08:30:00Z',
    },
  ],
}))

const apiMocks = vi.hoisted(() => ({
  listBotDescriptors: vi.fn(async () => ({ descriptors: mockState.descriptors })),
  listBotChannels: vi.fn(async () => ({ channels: [...mockState.channels] })),
  createBotChannel: vi.fn(async (input: any) => ({
    id: 103,
    name: input.name,
    channelType: input.channelType || 'wechat',
    enabled: false,
    status: 'unconfigured',
    hasCredentials: false,
    targetMode: input.targetMode || 'assistant',
    modelId: input.modelId ?? 0,
    promptId: input.promptId ?? 0,
    defaultProjectId: input.defaultProjectId ?? 0,
    worktreeId: input.worktreeId ?? 0,
    defaultBackend: input.targetMode === 'agent' ? 'builtin' : '',
    chatGroupId: input.chatGroupId ?? 0,
    chatId: 0,
    typingIndicator: input.typingIndicator ?? true,
    autoNotify: input.autoNotify ?? true,
    maxReplyLength: input.maxReplyLength ?? 2000,
    createdAt: '2026-11-26T09:00:00Z',
    updatedAt: '2026-11-26T09:00:00Z',
  })),
  updateBotChannel: vi.fn(async (id: number, input: any) => ({
    ...mockState.channels[0],
    id,
    ...input,
  })),
  deleteBotChannel: vi.fn(async () => {}),
  startBotChannel: vi.fn(async (id: number) => ({
    ...mockState.channels[0],
    id,
    enabled: true,
    status: 'connected',
  })),
  stopBotChannel: vi.fn(async (id: number) => ({
    ...mockState.channels[0],
    id,
    enabled: false,
    status: 'disconnected',
  })),
  resetBotChannelContext: vi.fn(async (id: number) => ({
    ...(id === 102 ? mockState.channels[1] : mockState.channels[0]),
    id,
    chatId: id === 102 ? 0 : 301,
    activeSessionId: id === 102 ? 'ses-bot-100' : '',
  })),
  startBotQRLogin: vi.fn(async (id: number) => ({
    channelId: id,
    status: 'pending',
    qrCode: 'https://ilink.example.com/qr',
    qrUrl: 'https://ilink.example.com/qr',
  })),
  getBotQRLoginStatus: vi.fn(async (id: number) => ({
    channelId: id,
    status: 'scanned',
  })),
  cancelBotQRLogin: vi.fn(async () => {}),
  listChats: vi.fn(async () => ({ chats: [], total: 0 })),
  listChatGroups: vi.fn(async () => ({
    groups: [
      {
        id: 20,
        name: '研发助手',
        defaultModelId: 1,
        enablePlatformTools: true,
        maxToolSteps: 10,
        createdAt: '',
        updatedAt: '',
      },
    ],
  })),
  subscribeBus: vi.fn(async () => null),
  unsubscribeBus: vi.fn(async () => {}),
  onStatusChange: vi.fn(() => () => {}),
}))

vi.mock('../api/bot', () => ({
  listBotDescriptors: apiMocks.listBotDescriptors,
  listBotChannels: apiMocks.listBotChannels,
  createBotChannel: apiMocks.createBotChannel,
  updateBotChannel: apiMocks.updateBotChannel,
  deleteBotChannel: apiMocks.deleteBotChannel,
  startBotChannel: apiMocks.startBotChannel,
  stopBotChannel: apiMocks.stopBotChannel,
  resetBotChannelContext: apiMocks.resetBotChannelContext,
  startBotQRLogin: apiMocks.startBotQRLogin,
  getBotQRLoginStatus: apiMocks.getBotQRLoginStatus,
  cancelBotQRLogin: apiMocks.cancelBotQRLogin,
}))

vi.mock('../api/chats', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/chats')>()
  return {
    ...actual,
    listChats: apiMocks.listChats,
    listChatGroups: apiMocks.listChatGroups,
  }
})

vi.mock('../api/models', () => ({
  listServices: vi.fn(async () => ({
    services: [
      {
        id: 1,
        name: 'DefaultProvider',
        baseUrl: 'https://api.example.com',
        hasKey: true,
        secretRef: 'ref',
        enabled: true,
        models: [
          {
            id: 1,
            serviceId: 1,
            modelId: 'gpt-4o',
            name: 'GPT-4o',
            supportsText: true,
            supportsImage: false,
            supportsTools: true,
            contextTokens: 128000,
            outputTokens: 8192,
            enabled: true,
            usable: true,
          },
        ],
      },
    ],
  })),
}))

vi.mock('../api/projects', () => ({
  listProjects: vi.fn(async () => ({
    projects: [{ id: 1, name: 'dev_platform', path: '/data/projects/dev_platform', isGit: false }],
  })),
  getProjectDisplayConfig: vi.fn(async () => ({ order: [], hidden: [] })),
}))

vi.mock('../api/agents', () => ({
  listAgentBackends: vi.fn(async () => ({
    backends: [
      {
        backend: 'builtin',
        label: '内置Agent',
        order: 900,
        serviceSupported: false,
        capabilities: { toolConfirmation: true },
      },
    ],
  })),
  listAgentBackendModels: vi.fn(async () => ({
    models: [{ id: 'gpt-4o', name: 'GPT-4o', provider: 'openai', enabled: true }],
    updatedAt: null,
  })),
}))

vi.mock('../api/prompts', () => ({
  listPrompts: vi.fn(async () => [
    {
      id: 5,
      name: '代码审阅专家',
      type: 'role',
      description: '专注审查代码缺陷',
      content: '你是代码审阅专家',
      createdAt: '',
      updatedAt: '',
    },
  ]),
}))

vi.mock('../shared/events/UnifiedEventBusClient', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../shared/events/UnifiedEventBusClient')>()
  return {
    ...actual,
    unifiedEventBus: {
      ensureConnection: vi.fn(() => null),
      getStatus: vi.fn(() => 'open'),
      subscribe: apiMocks.subscribeBus,
      unsubscribe: apiMocks.unsubscribeBus,
      onStatusChange: apiMocks.onStatusChange,
      onReconnect: vi.fn(() => () => {}),
    },
  }
})

import ChannelListView from '../features/organization/channels/ChannelListView.vue'
import ChannelEditDialog from '../features/organization/channels/ChannelEditDialog.vue'
import ChannelChatDrawer from '../features/organization/channels/ChannelChatDrawer.vue'
import { useBotStore } from '../stores/bot'

describe('Message Channels under Organization Center', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
  })

  it('useBotStore loads descriptors and channels, supports dual-mode CRUD and resetContext', async () => {
    const store = useBotStore()
    await store.loadAll()

    expect(store.descriptors).toHaveLength(1)
    expect(store.channels).toHaveLength(2)
    expect(apiMocks.subscribeBus).toHaveBeenCalledTimes(1)

    const created = await store.createChannel({
      name: '新建项目助手通道',
      channelType: 'wechat',
      targetMode: 'agent',
      defaultProjectId: 1,
      promptId: 5,
    })
    expect(created?.id).toBe(103)
    expect(created?.targetMode).toBe('agent')
    expect(store.channels).toHaveLength(3)

    const reset = await store.resetContext(101)
    expect(reset?.chatId).toBe(301)

    const qr = await store.beginQRLogin(103)
    expect(qr?.status).toBe('pending')

    const removed = await store.removeChannel(103)
    expect(removed).toBe(true)
    expect(store.channels).toHaveLength(2)
  })

  it('ChannelListView renders 3-pane list layout, mode filters, dual binding rows and opens dialog/drawer', async () => {
    const wrapper = mount(ChannelListView, {
      global: {
        plugins: [ElementPlus],
      },
    })

    await flushPromises()

    expect(wrapper.find('h1.list-page-title').text()).toBe('消息渠道')
    expect(wrapper.find('[data-test="list-toolbar"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="channel-table"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="channel-pagination"]').exists()).toBe(true)

    expect(wrapper.text()).toContain('微信研发助手渠道')
    expect(wrapper.text()).toContain('研发助手')
    expect(wrapper.text()).toContain('BOT 对话 #300')
    expect(wrapper.text()).toContain('代码审查微信通道')
    expect(wrapper.text()).toContain('dev_platform')
    expect(wrapper.text()).toContain('代码审阅专家')
    expect(wrapper.text()).toContain('会话 ses-bot-99')
    expect(wrapper.find('.entity-chip--assistant').exists()).toBe(true)
    expect(wrapper.find('.entity-chip--agent').exists()).toBe(true)

    // Jump to active session for agent-mode channel
    await wrapper.get('[data-test="btn-context-channel-102"]').trigger('click')
    expect(routerMocks.push).toHaveBeenCalledWith({
      name: 'session',
      params: { projectId: '1', sessionId: 'ses-bot-99' },
    })

    // Open assistant-mode bot chat drawer (isolated source=bot query)
    await wrapper.get('[data-test="btn-context-channel-101"]').trigger('click')
    await flushPromises()
    expect(wrapper.findComponent(ChannelChatDrawer).exists()).toBe(true)
    expect(apiMocks.listChats).toHaveBeenCalledWith(30, 0, undefined, {
      source: 'bot',
      channelId: 101,
    })

    // Open create channel dialog
    await wrapper.get('[data-test="btn-create-channel"]').trigger('click')
    await flushPromises()
    expect(wrapper.findComponent(ChannelEditDialog).exists()).toBe(true)
    expect(wrapper.text()).toContain('新建消息渠道')
    expect(wrapper.text()).toContain('AI 助手绑定配置')

    wrapper.unmount()
  })
})
