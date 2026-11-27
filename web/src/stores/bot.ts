import { defineStore } from 'pinia'
import { ref } from 'vue'
import {
  cancelBotQRLogin,
  createBotChannel,
  deleteBotChannel,
  getBotQRLoginStatus,
  listBotChannels,
  listBotDescriptors,
  resetBotChannelContext,
  startBotChannel,
  startBotQRLogin,
  stopBotChannel,
  updateBotChannel,
  type BotChannel,
  type ChannelDescriptor,
  type CreateBotChannelInput,
  type QRLoginState,
  type UpdateBotChannelInput,
} from '../api/bot'
import { describeApiError } from '../api/client'
import type { EventEnvelope } from '../api/events'
import { unifiedEventBus } from '../shared/events/UnifiedEventBusClient'

export const useBotStore = defineStore('bot', () => {
  const descriptors = ref<ChannelDescriptor[]>([])
  const channels = ref<BotChannel[]>([])
  const qrStates = ref<Record<number, QRLoginState>>({})
  const loading = ref(false)
  const error = ref<string | null>(null)
  let busUnsubscribed = true

  function applyChannelUpdate(updated: BotChannel) {
    const idx = channels.value.findIndex((c) => c.id === updated.id)
    if (idx === -1) {
      channels.value = [...channels.value, updated]
    } else {
      const next = [...channels.value]
      next[idx] = updated
      channels.value = next
    }
  }

  function handleEnvelope(env: EventEnvelope<any>) {
    if (env.channel !== 'bot' || !env.payload) return
    const p = env.payload as {
      kind?: string
      channel?: BotChannel
      login?: QRLoginState
      channelId?: number
      deleted?: boolean
    }
    if (p.deleted && p.channelId) {
      channels.value = channels.value.filter((c) => c.id !== p.channelId)
      return
    }
    if (p.kind === 'channel' && p.channel) {
      applyChannelUpdate(p.channel)
    } else if (p.kind === 'qr_login' && p.login) {
      qrStates.value = {
        ...qrStates.value,
        [p.login.channelId]: p.login,
      }
    }
  }

  function subscribeBus() {
    if (!busUnsubscribed) return
    busUnsubscribed = false
    void unifiedEventBus.subscribe('bot', 'all', handleEnvelope)
  }

  function unsubscribeBus() {
    if (busUnsubscribed) return
    busUnsubscribed = true
    void unifiedEventBus.unsubscribe('bot', 'all', handleEnvelope)
  }

  async function loadAll() {
    loading.value = true
    error.value = null
    try {
      const [descRes, chRes] = await Promise.all([
        listBotDescriptors(),
        listBotChannels(),
      ])
      descriptors.value = descRes.descriptors || []
      channels.value = chRes.channels || []
      subscribeBus()
    } catch (err: unknown) {
      error.value = describeApiError(err, '加载消息渠道信息失败')
    } finally {
      loading.value = false
    }
  }

  async function createChannel(input: CreateBotChannelInput): Promise<BotChannel | null> {
    error.value = null
    try {
      const created = await createBotChannel(input)
      applyChannelUpdate(created)
      return created
    } catch (err: unknown) {
      error.value = describeApiError(err, '创建渠道失败')
      return null
    }
  }

  async function updateChannel(id: number, input: UpdateBotChannelInput): Promise<BotChannel | null> {
    error.value = null
    try {
      const updated = await updateBotChannel(id, input)
      applyChannelUpdate(updated)
      return updated
    } catch (err: unknown) {
      error.value = describeApiError(err, '更新渠道失败')
      return null
    }
  }

  async function removeChannel(id: number): Promise<boolean> {
    error.value = null
    try {
      await deleteBotChannel(id)
      channels.value = channels.value.filter((c) => c.id !== id)
      return true
    } catch (err: unknown) {
      error.value = describeApiError(err, '删除渠道失败')
      return false
    }
  }

  async function startChannel(id: number): Promise<boolean> {
    error.value = null
    try {
      const updated = await startBotChannel(id)
      applyChannelUpdate(updated)
      return true
    } catch (err: unknown) {
      error.value = describeApiError(err, '启动渠道失败')
      return false
    }
  }

  async function stopChannel(id: number): Promise<boolean> {
    error.value = null
    try {
      const updated = await stopBotChannel(id)
      applyChannelUpdate(updated)
      return true
    } catch (err: unknown) {
      error.value = describeApiError(err, '停止渠道失败')
      return false
    }
  }

  async function resetContext(id: number): Promise<BotChannel | null> {
    error.value = null
    try {
      const updated = await resetBotChannelContext(id)
      applyChannelUpdate(updated)
      return updated
    } catch (err: unknown) {
      error.value = describeApiError(err, '新建渠道会话失败')
      return null
    }
  }

  async function beginQRLogin(id: number): Promise<QRLoginState | null> {
    error.value = null
    try {
      const state = await startBotQRLogin(id)
      qrStates.value = { ...qrStates.value, [id]: state }
      return state
    } catch (err: unknown) {
      error.value = describeApiError(err, '获取登录二维码失败')
      return null
    }
  }

  async function refreshQRStatus(id: number): Promise<QRLoginState | null> {
    try {
      const state = await getBotQRLoginStatus(id)
      qrStates.value = { ...qrStates.value, [id]: state }
      return state
    } catch {
      return null
    }
  }

  async function cancelQRLogin(id: number): Promise<void> {
    try {
      await cancelBotQRLogin(id)
    } catch {
      // Ignore cancel errors.
    }
  }

  return {
    descriptors,
    channels,
    qrStates,
    loading,
    error,
    loadAll,
    subscribeBus,
    unsubscribeBus,
    createChannel,
    updateChannel,
    removeChannel,
    startChannel,
    stopChannel,
    resetContext,
    beginQRLogin,
    refreshQRStatus,
    cancelQRLogin,
  }
})
