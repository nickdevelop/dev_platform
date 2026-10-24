// Pure-chat API calls and the SSE event envelope. These DTOs mirror the
// server's internal/app/chat types and docs/api/openapi.yaml. Reads go through
// get(), writes through mutate(); the EventSource path is a same-origin GET and
// carries the session cookie.

import { get, mutate } from './client'
import type { ImageAttachment, MessageUsage } from './messageTypes'

export type ChatRole = 'user' | 'assistant'
export type ChatMessageStatus = 'pending' | 'complete' | 'failed' | 'cancelled'
export type ChatTurnOutcome = 'completed' | 'cancelled' | 'failed'

export interface ChatGroup {
  id: number
  name: string
  defaultModelId?: number
  systemPrompt?: string
  icon?: string
  showInFloat?: boolean
  systemKey?: string
  createdAt: string
  updatedAt: string
}

export interface ChatGroupList {
  groups: ChatGroup[]
}

export interface ChatGroupInput {
  name: string
  defaultModelId?: number
  systemPrompt?: string
  icon?: string
  showInFloat?: boolean
}

export interface Chat {
  id: number
  groupId?: number
  title: string
  modelId: number
  modelName: string
  running: boolean
  createdAt: string
  updatedAt: string
}

export interface ChatMessage {
  id: number
  chatId: number
  role: ChatRole
  text: string
  reasoning?: string
  images?: ImageAttachment[]
  status: ChatMessageStatus
  modelId?: number
  modelName?: string
  usage?: MessageUsage
  errorCode?: string
  error?: string
  createdAt: string
  updatedAt: string
}

export interface ChatList {
  chats: Chat[]
  total: number
}

export interface ChatSnapshot {
  conversation: Chat
  messages: ChatMessage[]
  running: boolean
  truncated: boolean
}

export interface ChatCreateInput {
  groupId?: number
  modelId: number
  text: string
  images?: ImageAttachment[]
}

export interface ChatSendInput {
  modelId?: number
  text: string
  images?: ImageAttachment[]
}

export interface ChatEditInput {
  text: string
  images?: ImageAttachment[]
}

export interface ChatUpdateInput {
  groupId?: number
  title?: string
  modelId?: number
}

export interface ChatDelta {
  messageId: number
  text?: string
  reasoning?: string
}

export interface ChatTurn {
  phase: 'started' | 'ended'
  outcome?: ChatTurnOutcome
}

export type ChatEvent =
  | { chatId: number; timestamp: string; type: 'snapshot'; snapshot: ChatSnapshot }
  | { chatId: number; timestamp: string; type: 'delta'; delta: ChatDelta }
  | { chatId: number; timestamp: string; type: 'message_update'; message: ChatMessage }
  | { chatId: number; timestamp: string; type: 'conversation'; conversation: Chat }
  | { chatId: number; timestamp: string; type: 'turn'; turn: ChatTurn }
  | { chatId: number; timestamp: string; type: 'closed'; reason: string }

export function listChats(limit?: number, offset?: number, groupId?: number): Promise<ChatList> {
  const params = new URLSearchParams()
  if (limit !== undefined && limit > 0) params.set('limit', String(limit))
  if (offset !== undefined && offset > 0) params.set('offset', String(offset))
  if (groupId !== undefined && groupId > 0) params.set('groupId', String(groupId))
  const query = params.toString()
  return get<ChatList>(`/chats${query ? `?${query}` : ''}`)
}

export function createChat(input: ChatCreateInput): Promise<Chat> {
  return mutate<Chat>('POST', '/chats', input)
}

export function getChat(id: number): Promise<Chat> {
  return get<Chat>(`/chats/${id}`)
}

export function updateChat(id: number, input: ChatUpdateInput): Promise<Chat> {
  return mutate<Chat>('PUT', `/chats/${id}`, input)
}

export function deleteChat(id: number): Promise<void> {
  return mutate<void>('DELETE', `/chats/${id}`)
}

export function sendChatMessage(id: number, input: ChatSendInput): Promise<Chat> {
  return mutate<Chat>('POST', `/chats/${id}/messages`, input)
}

export function stopChat(id: number): Promise<void> {
  return mutate<void>('POST', `/chats/${id}/stop`)
}

export function regenerateChat(id: number): Promise<Chat> {
  return mutate<Chat>('POST', `/chats/${id}/regenerate`)
}

export function editChatMessage(
  id: number,
  messageId: number,
  input: ChatEditInput,
): Promise<Chat> {
  return mutate<Chat>(
    'POST',
    `/chats/${id}/messages/${encodeURIComponent(String(messageId))}/edit`,
    input,
  )
}

// chatEventsPath is the authenticated SSE endpoint for one conversation.
// EventSource can only issue GETs and cannot set headers; the session cookie
// authenticates the stream, exactly like the agent session event stream.
export function chatEventsPath(id: number): string {
  return `/api/chats/${id}/events`
}

export function listChatGroups(): Promise<ChatGroupList> {
  return get<ChatGroupList>('/chat-groups')
}

export function createChatGroup(input: ChatGroupInput): Promise<ChatGroup> {
  return mutate<ChatGroup>('POST', '/chat-groups', input)
}

export function updateChatGroup(id: number, input: ChatGroupInput): Promise<ChatGroup> {
  return mutate<ChatGroup>('PUT', `/chat-groups/${id}`, input)
}

export function deleteChatGroup(id: number): Promise<void> {
  return mutate<void>('DELETE', `/chat-groups/${id}`)
}
