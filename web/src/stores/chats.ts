// Pinia store for the pure-chat area. It owns the conversation list, the
// active transcript and the live SSE subscription state. Browser EventSource
// handles live in features/chat/pure/useChatStream, never in reactive state.

import { defineStore } from 'pinia'
import {
  createChat,
  deleteChat,
  editChatMessage,
  getChat,
  listChats,
  regenerateChat,
  sendChatMessage,
  stopChat,
  updateChat,
  type Chat,
  type ChatDelta,
  type ChatMessage,
  type ChatSnapshot,
  type ChatTurn,
} from '../api/chats'
import { ApiError, describeApiError, UnauthorizedError } from '../api/client'
import type { ImageAttachment } from '../api/messageTypes'
import type { ChatEntry, StreamStatus } from '../features/chat/state/entry'
import {
  applyDelta,
  entriesFromMessages,
  settleStreaming,
  upsertMessage,
} from '../features/chat/pure/chatEntries'
import { closeChatStream, openChatStream } from '../features/chat/pure/useChatStream'

export const useChatsStore = defineStore('chats', {
  state: () => ({
    chats: [] as Chat[],
    total: 0,
    loading: false,
    listError: '',
    listRequest: 0,
    activeId: null as number | null,
    active: null as Chat | null,
    messages: {} as Record<number, ChatEntry[]>,
    streamStatus: {} as Record<number, StreamStatus>,
    running: {} as Record<number, boolean>,
    truncated: {} as Record<number, boolean>,
    loadError: '',
    actionError: '',
    sendError: '',
    closedReason: '',
    // The selected model belongs to the /chat (no id) page only. Active
    // conversations keep their composer state in the mounted panel.
    draftModelId: null as number | null,
    // Bumped by "new chat" so an already mounted /chat panel clears its local
    // composer without needing a route change.
    draftNonce: 0,
  }),

  getters: {
    messagesFor: (state) => (chatId: number) => state.messages[chatId] ?? [],
    runningFor: (state) => (chatId: number) => state.running[chatId] === true,
    statusFor: (state) => (chatId: number) => state.streamStatus[chatId] ?? 'idle',
    truncatedFor: (state) => (chatId: number) => state.truncated[chatId] === true,
    isRunning(state): boolean {
      if (state.activeId === null) return false
      return state.running[state.activeId] === true
    },
    activeModelId(state): number | null {
      if (state.activeId === null) return null
      return state.active?.modelId ?? null
    },
    // The last settled assistant message is the only one regeneration may
    // replace. Its entry id carries the numeric message id.
    lastAssistantMessageId(state): number | null {
      if (state.activeId === null) return null
      const list = state.messages[state.activeId] ?? []
      for (let i = list.length - 1; i >= 0; i -= 1) {
        const entry = list[i]
        if (entry.kind !== 'message' || entry.role !== 'assistant') continue
        if (entry.streaming) return null
        const match = /^msg:(\d+)$/.exec(entry.id)
        return match ? Number(match[1]) : null
      }
      return null
    },
  },

  actions: {
    clearErrors() {
      this.actionError = ''
      this.sendError = ''
      this.loadError = ''
      this.closedReason = ''
    },

    async loadChats() {
      const request = ++this.listRequest
      this.loading = true
      try {
        const result = await listChats(200, 0)
        if (request !== this.listRequest) return
        this.chats = result.chats
        this.total = result.total
        this.listError = ''
        if (this.activeId !== null) {
          const found = result.chats.find((chat) => chat.id === this.activeId)
          if (found) this.active = found
        }
      } catch (error) {
        if (request !== this.listRequest) return
        if (error instanceof UnauthorizedError) return
        this.listError = describeApiError(error)
      } finally {
        if (request === this.listRequest) this.loading = false
      }
    },

    startNewChat() {
      closeChatStream()
      this.activeId = null
      this.active = null
      this.closedReason = ''
      this.loadError = ''
      this.sendError = ''
      this.actionError = ''
      this.draftNonce++
    },

    async openChat(chatId: number) {
      if (this.activeId === chatId && this.active) {
        openChatStream(this, chatId)
        return
      }
      closeChatStream()
      this.activeId = chatId
      this.active = null
      this.loadError = ''
      this.sendError = ''
      this.actionError = ''
      this.streamStatus = { ...this.streamStatus, [chatId]: 'connecting' }
      try {
        const chat = await getChat(chatId)
        if (this.activeId !== chatId) return
        this.active = chat
        this.running = { ...this.running, [chatId]: chat.running }
        openChatStream(this, chatId)
      } catch (error) {
        if (error instanceof UnauthorizedError) return
        this.loadError = describeApiError(error, '对话不存在或已删除。')
        this.activeId = null
      }
    },

    closeActive() {
      closeChatStream()
      this.activeId = null
      this.active = null
    },

    // createAndSend creates the conversation on its first send. The caller
    // navigates to the returned id; the SSE snapshot supplies the transcript.
    async createAndSend(text: string, images: ImageAttachment[], modelId: number): Promise<Chat | null> {
      this.sendError = ''
      this.actionError = ''
      try {
        const chat = await createChat({ modelId, text, images })
        this.chats = [chat, ...this.chats.filter((entry) => entry.id !== chat.id)]
        this.activeId = chat.id
        this.active = chat
        this.running = { ...this.running, [chat.id]: true }
        return chat
      } catch (error) {
        if (error instanceof UnauthorizedError) return null
        this.sendError = describeApiError(error)
        this.actionError = error instanceof ApiError ? error.code : ''
        return null
      }
    },

    async sendMessage(text: string, images: ImageAttachment[], modelId?: number): Promise<boolean> {
      const chatId = this.activeId
      if (chatId === null) return false
      this.sendError = ''
      this.actionError = ''
      try {
        const chat = await sendChatMessage(chatId, { text, images, modelId })
        this.active = chat
        this.running = { ...this.running, [chatId]: true }
        this.touch(chat)
        return true
      } catch (error) {
        if (error instanceof UnauthorizedError) return false
        this.sendError = describeApiError(error)
        this.actionError = error instanceof ApiError ? error.code : ''
        return false
      }
    },

    async stop(): Promise<boolean> {
      const chatId = this.activeId
      if (chatId === null) return false
      this.actionError = ''
      try {
        await stopChat(chatId)
        return true
      } catch (error) {
        if (error instanceof UnauthorizedError) return false
        this.sendError = describeApiError(error)
        return false
      }
    },

    async regenerate(): Promise<boolean> {
      const chatId = this.activeId
      if (chatId === null) return false
      this.sendError = ''
      this.actionError = ''
      try {
        const chat = await regenerateChat(chatId)
        this.active = chat
        this.running = { ...this.running, [chatId]: true }
        this.touch(chat)
        return true
      } catch (error) {
        if (error instanceof UnauthorizedError) return false
        this.sendError = describeApiError(error)
        this.actionError = error instanceof ApiError ? error.code : ''
        return false
      }
    },

    async editAndResend(
      messageId: number,
      text: string,
      images: ImageAttachment[],
    ): Promise<boolean> {
      const chatId = this.activeId
      if (chatId === null) return false
      this.sendError = ''
      this.actionError = ''
      try {
        const chat = await editChatMessage(chatId, messageId, { text, images })
        this.active = chat
        this.running = { ...this.running, [chatId]: true }
        this.touch(chat)
        return true
      } catch (error) {
        if (error instanceof UnauthorizedError) return false
        this.sendError = describeApiError(error)
        this.actionError = error instanceof ApiError ? error.code : ''
        return false
      }
    },

    async rename(title: string): Promise<boolean> {
      const chatId = this.activeId
      if (chatId === null) return false
      this.actionError = ''
      try {
        const chat = await updateChat(chatId, { title })
        this.active = chat
        this.touch(chat)
        return true
      } catch (error) {
        if (error instanceof UnauthorizedError) return false
        this.actionError = describeApiError(error)
        return false
      }
    },

    async setModel(modelId: number): Promise<boolean> {
      const chatId = this.activeId
      if (chatId === null) {
        this.draftModelId = modelId
        return true
      }
      this.actionError = ''
      try {
        const chat = await updateChat(chatId, { modelId })
        this.active = chat
        this.touch(chat)
        return true
      } catch (error) {
        if (error instanceof UnauthorizedError) return false
        this.actionError = describeApiError(error)
        return false
      }
    },

    async removeActive(): Promise<boolean> {
      const chatId = this.activeId
      if (chatId === null) return false
      this.actionError = ''
      try {
        await deleteChat(chatId)
      } catch (error) {
        if (error instanceof UnauthorizedError) return false
        this.actionError = describeApiError(error)
        return false
      }
      this.chats = this.chats.filter((chat) => chat.id !== chatId)
      this.messages = { ...this.messages, [chatId]: [] }
      this.startNewChat()
      return true
    },

    // --- Stream callbacks -------------------------------------------------

    applySnapshot(snapshot: ChatSnapshot) {
      const chat = snapshot.conversation
      this.activeId = chat.id
      this.active = chat
      this.messages = { ...this.messages, [chat.id]: entriesFromMessages(snapshot.messages) }
      this.running = { ...this.running, [chat.id]: snapshot.running }
      this.truncated = { ...this.truncated, [chat.id]: snapshot.truncated }
      this.streamStatus = { ...this.streamStatus, [chat.id]: 'open' }
      this.loadError = ''
      this.touch(chat)
    },

    applyDelta(delta: ChatDelta) {
      const chatId = this.activeId
      if (chatId === null) return
      this.messages = {
        ...this.messages,
        [chatId]: applyDelta(this.messages[chatId] ?? [], delta),
      }
    },

    applyMessageUpdate(message: ChatMessage) {
      const chatId = this.activeId
      if (chatId === null) return
      this.messages = {
        ...this.messages,
        [chatId]: upsertMessage(this.messages[chatId] ?? [], message),
      }
      if (message.status !== 'pending') {
        this.running = { ...this.running, [chatId]: false }
      }
    },

    applyConversation(chat: Chat) {
      if (this.activeId === chat.id) this.active = chat
      this.running = { ...this.running, [chat.id]: chat.running }
      this.touch(chat)
    },

    applyTurn(turn: ChatTurn) {
      const chatId = this.activeId
      if (chatId === null) return
      if (turn.phase === 'started') {
        this.running = { ...this.running, [chatId]: true }
        return
      }
      this.running = { ...this.running, [chatId]: false }
      this.messages = {
        ...this.messages,
        [chatId]: settleStreaming(this.messages[chatId] ?? []),
      }
    },

    handleClosed(reason: string) {
      this.closedReason = reason
      const chatId = this.activeId
      if (chatId === null) return
      this.running = { ...this.running, [chatId]: false }
      if (reason !== 'deleted') return
      this.chats = this.chats.filter((chat) => chat.id !== chatId)
      this.active = null
      this.activeId = null
    },

    touch(chat: Chat) {
      const next = [chat, ...this.chats.filter((entry) => entry.id !== chat.id)]
      next.sort((left, right) => {
        const delta = Date.parse(right.updatedAt) - Date.parse(left.updatedAt)
        return Number.isNaN(delta) || delta === 0 ? right.id - left.id : delta
      })
      this.chats = next
      if (this.activeId === chat.id) this.active = chat
    },
  },
})
