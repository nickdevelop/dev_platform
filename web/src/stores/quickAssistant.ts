// Pinia store for the floating quick-assistant surface. It owns one isolated
// conversation stream so the popover can run while the main Chat page streams
// another conversation. Every conversation it creates lives in the reserved
// "quick-assistant" group; it never reuses the main chats store's active
// conversation state.

import { defineStore } from 'pinia'
import {
  createChat,
  getChat,
  listChatGroups,
  listChats,
  sendChatMessage,
  stopChat,
  updateChat,
  type Chat,
  type ChatDelta,
  type ChatGroup,
  type ChatMessage,
  type ChatSnapshot,
  type ChatTurn,
} from '../api/chats'
import { ApiError, describeApiError, UnauthorizedError } from '../api/client'
import type { ImageAttachment } from '../api/messageTypes'
import type { ChatEntry, StreamStatus } from '../features/chat/state/entry'
import {
  applyDelta as applyDeltaToEntries,
  entriesFromMessages,
  settleStreaming,
  upsertMessage,
} from '../features/chat/pure/chatEntries'
import { createChatStream, type ChatStream } from '../features/chat/pure/useChatStream'
import { useChatsStore } from './chats'

const QUICK_ASSISTANT_GROUP_KEY = 'quick-assistant'
const MODEL_STORAGE_KEY = 'dev-platform-quick-assistant-model'

function loadStoredModel(): number | null {
  try {
    const raw = localStorage.getItem(MODEL_STORAGE_KEY)
    if (!raw) return null
    const value = Number(raw)
    return Number.isFinite(value) && value > 0 ? value : null
  } catch {
    return null
  }
}

function persistModel(modelId: number) {
  try {
    localStorage.setItem(MODEL_STORAGE_KEY, String(modelId))
  } catch {
    // Best-effort preference only.
  }
}

// Each Pinia instance gets its own stream. A WeakMap keeps browser resources
// out of reactive state and prevents a test or HMR reload from sharing an
// EventSource across store instances.
const streams = new WeakMap<object, ChatStream>()

function streamFor(store: object): ChatStream {
  let stream = streams.get(store)
  if (!stream) {
    stream = createChatStream(store as Parameters<typeof createChatStream>[0])
    streams.set(store, stream)
  }
  return stream
}

export const useQuickAssistantStore = defineStore('quickAssistant', {
  state: () => ({
    groupId: null as number | null,
    currentAssistant: null as ChatGroup | null,
    chatId: null as number | null,
    chat: null as Chat | null,
    messages: [] as ChatEntry[],
    running: false,
    truncated: false,
    streamStatus: 'idle' as StreamStatus,
    loading: false,
    error: '',
    actionError: '',
    sendError: '',
    closedReason: '',
    modelId: loadStoredModel(),
    // Invalidates an in-flight open when the popover closes or a newer request
    // starts. It is deliberately not persisted.
    requestId: 0,
    // Recent 5 conversations for the current assistant dropdown
    recentChats: [] as Chat[],
    // Tracks whether the user explicitly initiated a new conversation draft
    isNewDraft: false,
    // Per-assistant conversation and draft state mapping
    assistantState: {} as Record<number, { chatId: number | null; isNewDraft: boolean }>,
  }),

  getters: {
    hasActiveChat: (state) => state.chatId !== null,
  },

  actions: {
    clearErrors() {
      this.error = ''
      this.actionError = ''
      this.sendError = ''
      this.closedReason = ''
    },

    async ensureGroup(targetGroupId?: number): Promise<number | null> {
      if (targetGroupId !== undefined) {
        this.groupId = targetGroupId
      }
      if (this.groupId !== null) return this.groupId
      try {
        const { groups } = await listChatGroups()
        if (groups.length === 0) {
          this.error = '尚未创建任何助手，请先在 Chat 页面新建助手。'
          return null
        }
        // Prefer floating assistants, then quick-assistant, then first available
        const group =
          groups.find((entry) => entry.showInFloat) ??
          groups.find((entry) => entry.systemKey === QUICK_ASSISTANT_GROUP_KEY) ??
          groups[0]
        if (!group) {
          this.error = '助手不可用，请刷新页面。'
          return null
        }
        this.groupId = group.id
        this.currentAssistant = group
        if (group.defaultModelId) {
          this.modelId = group.defaultModelId
        }
        return group.id
      } catch (error) {
        if (error instanceof UnauthorizedError) return null
        this.error = describeApiError(error)
        return null
      }
    },

    openAssistant(assistant: ChatGroup) {
      const changed = this.groupId !== assistant.id
      this.groupId = assistant.id
      this.currentAssistant = assistant
      if (assistant.defaultModelId) {
        this.modelId = assistant.defaultModelId
      }
      if (changed) {
        const saved = this.assistantState[assistant.id]
        if (saved) {
          this.isNewDraft = saved.isNewDraft
          this.chatId = saved.chatId
        } else {
          this.isNewDraft = false
          this.chatId = null
        }
      }
    },

    async fetchRecentChats(targetGroupId?: number) {
      const groupId = targetGroupId ?? this.groupId
      if (groupId === null) return
      try {
        const { chats } = await listChats(5, 0, groupId)
        this.recentChats = Array.isArray(chats) ? chats : []
      } catch (error) {
        if (error instanceof UnauthorizedError) return
      }
    },

    // restoreOrOpenLatest restores the assistant's previous conversation or
    // honors the draft mode if the user initiated a new chat. If neither
    // exists, it defaults to opening the latest conversation.
    async restoreOrOpenLatest(targetGroupId?: number) {
      const request = ++this.requestId
      this.loading = true
      this.clearErrors()
      try {
        const groupId = await this.ensureGroup(targetGroupId)
        if (request !== this.requestId || groupId === null) return

        const { chats } = await listChats(5, 0, groupId)
        if (request !== this.requestId) return
        this.recentChats = Array.isArray(chats) ? chats : []

        const saved = this.assistantState[groupId]
        const isNewDraft = saved !== undefined ? saved.isNewDraft : this.isNewDraft
        const currentChatId = saved !== undefined ? saved.chatId : this.chatId

        if (isNewDraft) {
          this.resetDraft()
          this.isNewDraft = true
          return
        }

        if (currentChatId !== null) {
          await this.openChat(currentChatId)
          return
        }

        if (this.recentChats.length === 0) {
          this.resetDraft()
          return
        }
        await this.openChat(this.recentChats[0].id)
      } catch (error) {
        if (request !== this.requestId) return
        if (error instanceof UnauthorizedError) return
        this.error = describeApiError(error)
      } finally {
        if (request === this.requestId) this.loading = false
      }
    },

    // openLatest delegates to restoreOrOpenLatest for backward compatibility.
    async openLatest(targetGroupId?: number) {
      return this.restoreOrOpenLatest(targetGroupId)
    },

    async openChat(chatId: number) {
      const request = this.requestId
      this.chatId = chatId
      this.isNewDraft = false
      if (this.groupId !== null) {
        this.assistantState[this.groupId] = { chatId, isNewDraft: false }
      }
      this.chat = null
      this.messages = []
      this.running = false
      this.truncated = false
      this.closedReason = ''
      this.streamStatus = 'connecting'
      this.error = ''
      try {
        const chat = await getChat(chatId)
        if (request !== this.requestId || this.chatId !== chatId) return
        this.chat = chat
        this.running = chat.running
        if (chat.modelId) {
          this.modelId = chat.modelId
          persistModel(chat.modelId)
        }
        streamFor(this).open(chatId)
      } catch (error) {
        if (request !== this.requestId) return
        if (error instanceof UnauthorizedError) return
        this.error = describeApiError(error, '对话不存在或已删除。')
        this.resetDraft()
      }
    },

    resetDraft() {
      streamFor(this).close()
      this.chatId = null
      this.chat = null
      this.messages = []
      this.running = false
      this.truncated = false
      this.streamStatus = 'idle'
      this.closedReason = ''
    },

    newChat() {
      if (this.running) return
      this.resetDraft()
      this.isNewDraft = true
      if (this.groupId !== null) {
        this.assistantState[this.groupId] = { chatId: null, isNewDraft: true }
      }
      this.clearErrors()
    },

    async send(text: string, images: ImageAttachment[], modelId: number): Promise<boolean> {
      const request = ++this.requestId
      this.sendError = ''
      this.actionError = ''
      try {
        if (this.chatId === null) {
          const groupId = await this.ensureGroup()
          if (request !== this.requestId) return false
          if (groupId === null) {
            this.sendError = this.error || '快捷助手分组不可用。'
            return false
          }
          const chat = await createChat({ groupId, modelId, text, images })
          if (request !== this.requestId) return false
          this.chatId = chat.id
          this.chat = chat
          this.isNewDraft = false
          if (this.groupId !== null) {
            this.assistantState[this.groupId] = { chatId: chat.id, isNewDraft: false }
            void this.fetchRecentChats(this.groupId)
          }
          this.messages = []
          this.running = true
          this.truncated = false
          this.streamStatus = 'connecting'
          this.modelId = modelId
          persistModel(modelId)
          streamFor(this).open(chat.id)
          useChatsStore().invalidateList()
          return true
        }

        const chat = await sendChatMessage(this.chatId, { text, images, modelId })
        if (request !== this.requestId) return false
        this.chat = chat
        this.running = true
        this.isNewDraft = false
        if (this.groupId !== null) {
          this.assistantState[this.groupId] = { chatId: this.chatId, isNewDraft: false }
          void this.fetchRecentChats(this.groupId)
        }
        this.modelId = modelId
        persistModel(modelId)
        streamFor(this).open(this.chatId)
        useChatsStore().invalidateList()
        return true
      } catch (error) {
        if (request !== this.requestId) return false
        if (error instanceof UnauthorizedError) return false
        this.sendError = describeApiError(error)
        this.actionError = error instanceof ApiError ? error.code : ''
        return false
      }
    },

    async stop(): Promise<boolean> {
      if (this.chatId === null) return false
      this.actionError = ''
      try {
        await stopChat(this.chatId)
        return true
      } catch (error) {
        if (error instanceof UnauthorizedError) return false
        this.sendError = describeApiError(error)
        return false
      }
    },

    async setModel(modelId: number): Promise<boolean> {
      const previous = this.modelId
      this.modelId = modelId
      persistModel(modelId)
      if (this.chatId === null) return true
      this.actionError = ''
      try {
        const chat = await updateChat(this.chatId, { modelId })
        if (this.chatId === chat.id) this.chat = chat
        return true
      } catch (error) {
        this.modelId = previous
        if (error instanceof UnauthorizedError) return false
        this.actionError = describeApiError(error)
        return false
      }
    },

    reconnect() {
      if (this.chatId === null) return
      streamFor(this).reconnect(this.chatId)
    },

    // close releases the browser EventSource but keeps the in-memory transcript
    // so reopening the popover does not flash an empty state. It also
    // invalidates an in-flight open so a late response cannot restart a stream
    // after the popover is gone.
    close() {
      this.requestId++
      this.loading = false
      streamFor(this).close()
      this.streamStatus = 'idle'
    },

    // --- Stream callbacks -------------------------------------------------

    applySnapshot(snapshot: ChatSnapshot) {
      const chat = snapshot.conversation
      this.chatId = chat.id
      this.chat = chat
      this.messages = entriesFromMessages(snapshot.messages)
      this.running = snapshot.running
      this.truncated = snapshot.truncated
      this.streamStatus = 'open'
      this.closedReason = ''
      if (chat.modelId) {
        this.modelId = chat.modelId
        persistModel(chat.modelId)
      }
      this.touchMainList()
    },

    applyDelta(chatId: number, delta: ChatDelta) {
      if (chatId !== this.chatId) return
      this.messages = applyDeltaToEntries(this.messages, delta)
    },

    applyMessageUpdate(chatId: number, message: ChatMessage) {
      if (chatId !== this.chatId) return
      this.messages = upsertMessage(this.messages, message)
      if (message.status !== 'pending') this.running = false
    },

    applyConversation(chat: Chat) {
      if (chat.id === this.chatId) this.chat = chat
      this.running = chat.running
      this.touchMainList()
    },

    applyTurn(chatId: number, turn: ChatTurn) {
      if (chatId !== this.chatId) return
      if (turn.phase === 'started') {
        this.running = true
        return
      }
      this.running = false
      this.messages = settleStreaming(this.messages)
    },

    handleClosed(chatId: number, reason: string) {
      if (chatId !== this.chatId) return
      this.running = false
      this.closedReason = reason
      if (reason === 'deleted') {
        this.resetDraft()
        useChatsStore().invalidateList()
      }
    },

    // touchMainList keeps the Chat sidebar in sync when the popover changes the
    // list or a title/model update arrives from the server.
    touchMainList() {
      useChatsStore().invalidateList()
    },
  },
})
