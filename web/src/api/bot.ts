import { get, mutate } from './client'

export interface ChannelDescriptor {
  type: string
  label: string
  description: string
  supportsQrLogin: boolean
}

export interface BotToolMeta {
  name: string
  label: string
  category: string
  description: string
  dangerous?: boolean
}

export type BotChannelStatus =
  | 'unconfigured'
  | 'waiting_qr'
  | 'scanned'
  | 'connected'
  | 'expired'
  | 'disconnected'
  | 'error'

export interface BotChannel {
  id: number
  name: string
  channelType: string
  enabled: boolean
  status: BotChannelStatus | string
  lastError?: string
  hasCredentials: boolean
  accountId?: string
  modelId: number
  autoNotify: boolean
  chatGroupId: number
  chatId: number
  lastConnectedAt?: string
  createdAt: string
  updatedAt: string
}

export interface CreateBotChannelInput {
  name: string
  channelType: string
  modelId?: number
  autoNotify?: boolean
}

export interface UpdateBotChannelInput {
  name?: string
  enabled?: boolean
  modelId?: number
  autoNotify?: boolean
}

export interface QRLoginState {
  channelId: number
  status: string
  qrCode?: string
  qrUrl?: string
  error?: string
}

export function listBotDescriptors(): Promise<{ descriptors: ChannelDescriptor[] }> {
  return get<{ descriptors: ChannelDescriptor[] }>('/bot/descriptors')
}

export function listBotTools(): Promise<{ tools: BotToolMeta[] }> {
  return get<{ tools: BotToolMeta[] }>('/bot/tools')
}

export function listBotChannels(): Promise<{ channels: BotChannel[] }> {
  return get<{ channels: BotChannel[] }>('/bot/channels')
}

export function createBotChannel(input: CreateBotChannelInput): Promise<BotChannel> {
  return mutate<BotChannel>('POST', '/bot/channels', input)
}

export function updateBotChannel(id: number, input: UpdateBotChannelInput): Promise<BotChannel> {
  return mutate<BotChannel>('PUT', `/bot/channels/${id}`, input)
}

export function deleteBotChannel(id: number): Promise<void> {
  return mutate<void>('DELETE', `/bot/channels/${id}`)
}

export function startBotChannel(id: number): Promise<BotChannel> {
  return mutate<BotChannel>('POST', `/bot/channels/${id}/start`, {})
}

export function stopBotChannel(id: number): Promise<BotChannel> {
  return mutate<BotChannel>('POST', `/bot/channels/${id}/stop`, {})
}

export function startBotQRLogin(id: number): Promise<QRLoginState> {
  return mutate<QRLoginState>('POST', `/bot/channels/${id}/login/qr`, {})
}

export function cancelBotQRLogin(id: number): Promise<void> {
  return mutate<void>('POST', `/bot/channels/${id}/login/cancel`, {})
}

export function getBotQRLoginStatus(id: number): Promise<QRLoginState> {
  return get<QRLoginState>(`/bot/channels/${id}/login/status`)
}
