// Pure-chat log mapping: persisted messages and live deltas become the same
// ChatEntry vocabulary the shared renderer already understands. No agent
// events or capabilities are involved.

import type { ChatDelta, ChatMessage } from '../../../api/chats'
import type { ChatEntry } from '../state/entry'

function modelFor(message: ChatMessage): ChatEntry['model'] {
  if (!message.modelName && !message.modelId) return undefined
  return {
    id: message.modelName || String(message.modelId),
    provider: 'model',
  }
}

const errorLabels: Record<string, string> = {
  upstream_unreachable: '无法连接到模型服务，请检查 API 地址。',
  upstream_error: '模型服务返回错误，请检查模型配置与 Key。',
  generation_timeout: '生成超时，可重新生成。',
  generation_interrupted: '生成被中断，可重新生成。',
  generation_failed: '生成失败，可重新生成。',
}

export function chatErrorMessage(message: ChatMessage): string {
  if (message.errorCode && errorLabels[message.errorCode]) return errorLabels[message.errorCode]
  return message.error || '生成失败，可重新生成。'
}

function usageFor(message: ChatMessage): ChatEntry['usage'] {
  if (!message.usage) return undefined
  return {
    input: message.usage.input,
    output: message.usage.output,
  }
}

export function messageEntryID(message: ChatMessage): string {
  return `msg:${message.id}`
}

export function thinkingEntryID(message: ChatMessage): string {
  return `think:${message.id}`
}

export function errorEntryID(message: ChatMessage): string {
  return `err:${message.id}`
}

// entriesFromMessage returns the zero-to-three log lines one persisted message
// represents: assistant reasoning, the message bubble, and a failure note.
export function entriesFromMessage(message: ChatMessage): ChatEntry[] {
  const entries: ChatEntry[] = []
  const streaming = message.status === 'pending'
  if (message.reasoning) {
    entries.push({
      id: thinkingEntryID(message),
      kind: 'thinking',
      role: 'assistant',
      text: message.reasoning,
      streaming,
      timestamp: message.createdAt,
    })
  }
  entries.push({
    id: messageEntryID(message),
    kind: 'message',
    role: message.role,
    text: message.text,
    images: message.images,
    streaming,
    timestamp: message.createdAt,
    model: message.role === 'assistant' ? modelFor(message) : undefined,
    usage: message.role === 'assistant' ? usageFor(message) : undefined,
  })
  if (message.error || message.errorCode) {
    entries.push({
      id: errorEntryID(message),
      kind: 'error',
      role: 'system',
      text: chatErrorMessage(message),
      streaming: false,
      timestamp: message.updatedAt,
    })
  }
  return entries
}

export function entriesFromMessages(messages: ChatMessage[]): ChatEntry[] {
  const entries: ChatEntry[] = []
  for (const message of messages) {
    entries.push(...entriesFromMessage(message))
  }
  return entries
}

// upsertMessage replaces every entry belonging to one message, keeping the
// message's position in the log. A live message_update therefore settles the
// bubble in place instead of moving the answer to the bottom.
export function upsertMessage(list: ChatEntry[], message: ChatMessage): ChatEntry[] {
  const next = entriesFromMessage(message)
  const owned = new Set([messageEntryID(message), thinkingEntryID(message), errorEntryID(message)])
  const index = list.findIndex((entry) => entry.id === messageEntryID(message))
  const filtered = list.filter((entry) => !owned.has(entry.id))
  if (index === -1) {
    return [...filtered, ...next]
  }
  // Recompute the insertion point after filtering: the number of surviving
  // entries before the old message position is the message's stable slot.
  const before = list.slice(0, index).filter((entry) => !owned.has(entry.id)).length
  return [...filtered.slice(0, before), ...next, ...filtered.slice(before)]
}

// applyDelta appends one streaming increment. Reasoning is kept in its own
// collapsible entry immediately before the assistant bubble.
export function applyDelta(list: ChatEntry[], delta: ChatDelta): ChatEntry[] {
  const messageID = `msg:${delta.messageId}`
  const index = list.findIndex((entry) => entry.id === messageID)
  if (index === -1) return list
  const next = [...list]
  if (delta.reasoning) {
    const thinkingID = `think:${delta.messageId}`
    const thinkingIndex = next.findIndex((entry) => entry.id === thinkingID)
    if (thinkingIndex === -1) {
      next.splice(index, 0, {
        id: thinkingID,
        kind: 'thinking',
        role: 'assistant',
        text: delta.reasoning,
        streaming: true,
      })
    } else {
      next[thinkingIndex] = {
        ...next[thinkingIndex],
        text: next[thinkingIndex].text + delta.reasoning,
        streaming: true,
      }
    }
  }
  if (delta.text) {
    // The message may have shifted by one after inserting the thinking entry.
    const messageIndex = next.findIndex((entry) => entry.id === messageID)
    if (messageIndex === -1) return next
    next[messageIndex] = {
      ...next[messageIndex],
      text: next[messageIndex].text + delta.text,
      streaming: true,
    }
  }
  return next
}

// settleStreaming clears the cursor from every entry after a turn ends, so a
// missing final event cannot leave an answer looking permanently live.
export function settleStreaming(list: ChatEntry[]): ChatEntry[] {
  if (!list.some((entry) => entry.streaming)) return list
  return list.map((entry) => (entry.streaming ? { ...entry, streaming: false } : entry))
}
