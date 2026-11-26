import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import ElementPlus from 'element-plus'

const mockState = vi.hoisted(() => ({
  descriptors: [
    {
      type: 'wechat',
      label: '个人微信',
      description: '通过 iLink Bot 协议接入个人微信',
      supportsQrLogin: true,
    },
  ],
  tools: [
    {
      name: 'platform_list_projects',
      label: '列出工程',
      category: '平台查询',
      description: '获取平台已纳管工程列表',
      dangerous: false,
    },
    {
      name: 'platform_create_session',
      label: '创建编码会话',
      category: '会话协同',
      description: '在指定工作树中创建编码会话',
      dangerous: true,
    },
  ],
  channels: [
    {
      id: 101,
      name: '微信主号 Bot',
      channelType: 'wechat',
      enabled: true,
      status: 'connected',
      hasCredentials: true,
      accountId: 'wx-bot-1',
      modelId: 1,
      autoNotify: true,
      chatGroupId: 20,
      chatId: 300,
      lastConnectedAt: '2026-11-26T08:00:00Z',
      createdAt: '2026-11-26T08:00:00Z',
      updatedAt: '2026-11-26T08:00:00Z',
    },
  ],
}))

const apiMocks = vi.hoisted(() => ({
  listBotDescriptors: vi.fn(async () => ({ descriptors: mockState.descriptors })),
  listBotTools: vi.fn(async () => ({ tools: mockState.tools })),
  listBotChannels: vi.fn(async () => ({ channels: [...mockState.channels] })),
  createBotChannel: vi.fn(async (input: any) => ({
    id: 102,
    name: input.name,
    channelType: input.channelType || 'wechat',
    enabled: false,
    status: 'unconfigured',
    hasCredentials: false,
    modelId: input.modelId ?? 0,
    autoNotify: input.autoNotify ?? true,
    chatGroupId: 20,
    chatId: 301,
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
  subscribeBus: vi.fn(async () => null),
  unsubscribeBus: vi.fn(async () => {}),
  onStatusChange: vi.fn(() => () => {}),
  putEnginesSettings: vi.fn(async (s: any) => s),
  getEnginesSettings: vi.fn(async () => ({
    steward: {
      model: '',
      maxSteps: 10,
      enableThinking: false,
      systemPrompt: '',
      enablePlatformTools: true,
      enabledTools: [],
    },
    coder: {
      model: '',
      maxSteps: 20,
      permissionMode: 'auto',
      systemPrompt: '',
      injectPlatformDoc: true,
      injectUserDoc: true,
      injectProjectDoc: true,
      enableSkills: true,
      enabledTools: [],
    },
    orchestrator: {
      model: '',
      maxSteps: 20,
      timeoutSeconds: 600,
      systemPrompt: '',
      autoInjectRoles: true,
      enabledTools: [],
    },
    core: {
      fallbackModel: '',
      enableDebugLog: false,
      logRetentionDays: 7,
      searchEngine: 'auto',
    },
    bot: {
      maxSteps: 12,
      typingIndicator: true,
      autoNotifySession: true,
      maxReplyLength: 2000,
      enabledTools: ['platform_list_projects'],
    },
  })),
}))

vi.mock('../api/bot', () => ({
  listBotDescriptors: apiMocks.listBotDescriptors,
  listBotTools: apiMocks.listBotTools,
  listBotChannels: apiMocks.listBotChannels,
  createBotChannel: apiMocks.createBotChannel,
  updateBotChannel: apiMocks.updateBotChannel,
  deleteBotChannel: apiMocks.deleteBotChannel,
  startBotChannel: apiMocks.startBotChannel,
  stopBotChannel: apiMocks.stopBotChannel,
  startBotQRLogin: apiMocks.startBotQRLogin,
  getBotQRLoginStatus: apiMocks.getBotQRLoginStatus,
  cancelBotQRLogin: apiMocks.cancelBotQRLogin,
}))

vi.mock('../api/engines', () => ({
  getEnginesSettings: apiMocks.getEnginesSettings,
  putEnginesSettings: apiMocks.putEnginesSettings,
}))

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
    projects: [{ id: 1, name: 'dev_platform', path: '/data/projects/dev_platform' }],
  })),
  getProjectDisplayConfig: vi.fn(async () => ({ order: [], hidden: [] })),
}))

vi.mock('../api/agents', () => ({
  listAgentBackends: vi.fn(async () => ({
    backends: [{ backend: 'opencode', label: 'OpenCode', capabilities: {} }],
  })),
}))

vi.mock('../api/prompts', () => ({
  listPrompts: vi.fn(async () => []),
}))

vi.mock('../shared/events/UnifiedEventBusClient', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../shared/events/UnifiedEventBusClient')>()
  return {
    ...actual,
    unifiedEventBus: {
      subscribe: apiMocks.subscribeBus,
      unsubscribe: apiMocks.unsubscribeBus,
      onStatusChange: apiMocks.onStatusChange,
      onReconnect: vi.fn(() => () => {}),
    },
  }
})

import BotEngineView from '../features/engines/BotEngineView.vue'
import { useBotStore } from '../stores/bot'
import { useEnginesStore } from '../stores/engines'

describe('BotEngine store & view', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
  })

  it('useBotStore loads descriptors, tools, channels and performs channel CRUD', async () => {
    const store = useBotStore()
    await store.loadAll()

    expect(store.descriptors).toHaveLength(1)
    expect(store.tools).toHaveLength(2)
    expect(store.channels).toHaveLength(1)
    expect(apiMocks.subscribeBus).toHaveBeenCalledTimes(1)

    const created = await store.createChannel({
      name: '新微信渠道',
      channelType: 'wechat',
    })
    expect(created?.id).toBe(102)
    expect(store.channels).toHaveLength(2)

    const qr = await store.beginQRLogin(102)
    expect(qr?.status).toBe('pending')
    expect(store.qrStates[102]?.qrUrl).toBe('https://ilink.example.com/qr')

    const removed = await store.removeChannel(102)
    expect(removed).toBe(true)
    expect(store.channels).toHaveLength(1)
  })

  it('BotEngineView renders engine config, tool checklist, channel table and saves settings', async () => {
    const enginesStore = useEnginesStore()
    await enginesStore.loadSettings()

    const wrapper = mount(BotEngineView, {
      global: {
        plugins: [ElementPlus],
      },
    })

    await flushPromises()

    expect(wrapper.text()).toContain('Bot 引擎')
    expect(wrapper.text()).toContain('单轮最大工具调用步数')
    expect(wrapper.text()).toContain('Bot 引擎携带工具集')
    expect(wrapper.text()).toContain('列出工程')
    expect(wrapper.text()).toContain('微信主号 Bot')
    expect(wrapper.text()).toContain('Chat #300')
    expect(wrapper.text()).not.toContain('默认关联工程')
    expect(wrapper.text()).not.toContain('默认 Agent 后端')
    expect(wrapper.text()).not.toContain('全局默认系统提示词')
    expect(wrapper.text()).not.toContain('白名单')
    expect(wrapper.find('.entity-chip--wechat').exists()).toBe(true)

    await wrapper.get('[data-test="save-bot-engine"]').trigger('click')
    await flushPromises()

    expect(apiMocks.putEnginesSettings).toHaveBeenCalledTimes(1)
  })
})
