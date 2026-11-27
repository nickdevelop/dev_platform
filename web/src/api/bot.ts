import { get, mutate } from './client'

export interface ChannelDescriptor {
  type: string
  label: string
  description: string
  supportsQrLogin: boolean
}

export type BotChannelStatus =
  | 'unconfigured'
  | 'waiting_qr'
  | 'scanned'
  | 'connected'
  | 'expired'
  | 'disconnected'
  | 'error'

export type BotTargetMode = 'assistant' | 'agent'

export interface BotChannel {
  id: number
  name: string
  channelType: string
  enabled: boolean
  status: BotChannelStatus | string
  lastError?: string
  hasCredentials: boolean
  accountId?: string
  targetMode: BotTargetMode | string
  modelId: number
  promptId: number
  systemPrompt?: string
  defaultProjectId: number
  worktreeId: number
  defaultBackend?: string
  agentModel?: string
  chatGroupId: number
  chatId: number
  activeSessionId?: string
  typingIndicator: boolean
  autoNotify: boolean
  maxReplyLength: number
  lastConnectedAt?: string
  createdAt: string
  updatedAt: string
}

export interface CreateBotChannelInput {
  name: string
  channelType: string
  targetMode: BotTargetMode
  chatGroupId?: number
  modelId?: number
  defaultProjectId?: number
  worktreeId?: number
  promptId?: number
  systemPrompt?: string
  agentModel?: string
  typingIndicator?: boolean
  autoNotify?: boolean
  maxReplyLength?: number
}

export interface UpdateBotChannelInput {
  name?: string
  enabled?: boolean
  targetMode?: BotTargetMode
  chatGroupId?: number
  modelId?: number
  defaultProjectId?: number
  worktreeId?: number
  promptId?: number
  systemPrompt?: string
  agentModel?: string
  typingIndicator?: boolean
  autoNotify?: boolean
  maxReplyLength?: number
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

export function resetBotChannelContext(id: number): Promise<BotChannel> {
  return mutate<BotChannel>('POST', `/bot/channels/${id}/new`, {})
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
