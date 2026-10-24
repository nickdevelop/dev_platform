import { create } from 'zustand'
import {
  createChat,
  createChatGroup,
  deleteChat,
  deleteChatGroup,
  getChat,
  listChatGroups,
  listChats,
  regenerateChat,
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
import type { ChatEntry, StreamStatus } from '../features/chat/pure/../state/entry'
import { applyDelta, entriesFromMessages, settleStreaming, upsertMessage } from '../features/chat/pure/chatEntries'
import type { ChatStreamStore } from '../features/chat/pure/useChatStream'
import { closeChatStream, openChatStream } from '../features/chat/pure/useChatStream'

interface ChatsState {
  groups: ChatGroup[]
  chats: Chat[]
  total: number
  loading: boolean
  listError: string
  groupsError: string
  activeId: number | null
  active: Chat | null
  messages: Record<number, ChatEntry[]>
  streamStatus: Record<number, StreamStatus>
  running: Record<number, boolean>
  truncated: Record<number, boolean>
  loadError: string
  actionError: string
  sendError: string
  draftModelId: number | null
  loadGroups: () => Promise<void>
  loadChats: () => Promise<void>
  openChat: (chatId: number) => Promise<void>
  startNewChat: (groupId?: number) => void
  createAndSend: (text: string, modelId: number, groupId?: number, images?: ImageAttachment[]) => Promise<Chat | null>
  sendMessage: (text: string, modelId?: number, images?: ImageAttachment[]) => Promise<boolean>
  stop: () => Promise<boolean>
  regenerate: () => Promise<boolean>
  removeActive: () => Promise<boolean>
  rename: (title: string) => Promise<boolean>
  setModel: (modelId: number) => Promise<boolean>
  createGroup: (name: string) => Promise<ChatGroup | null>
  deleteGroup: (id: number) => Promise<boolean>
  moveToGroup: (chatId: number, groupId: number) => Promise<boolean>
  setStreamStatus: (chatId: number, status: StreamStatus) => void
  applySnapshot: (snapshot: ChatSnapshot) => void
  applyDelta: (chatId: number, delta: ChatDelta) => void
  applyMessageUpdate: (chatId: number, message: ChatMessage) => void
  applyConversation: (chat: Chat) => void
  applyTurn: (chatId: number, turn: ChatTurn) => void
  handleClosed: (chatId: number, reason: string) => void
}

function touchList(list: Chat[], chat: Chat): Chat[] {
  return [chat, ...list.filter((entry) => entry.id !== chat.id)].sort((left, right) => {
    const delta = Date.parse(right.updatedAt) - Date.parse(left.updatedAt)
    return Number.isNaN(delta) || delta === 0 ? right.id - left.id : delta
  })
}

export const useChatsStore = create<ChatsState>((set, getState) => ({
  groups: [],
  chats: [],
  total: 0,
  loading: false,
  listError: '',
  groupsError: '',
  activeId: null,
  active: null,
  messages: {},
  streamStatus: {},
  running: {},
  truncated: {},
  loadError: '',
  actionError: '',
  sendError: '',
  draftModelId: null,

  loadGroups: async () => {
    try {
      const result = await listChatGroups()
      set({ groups: result.groups, groupsError: '' })
    } catch (error) {
      if (error instanceof UnauthorizedError) return
      set({ groupsError: describeApiError(error) })
    }
  },

  loadChats: async () => {
    set({ loading: true })
    try {
      const result = await listChats(200, 0)
      set((state) => ({
        chats: result.chats,
        total: result.total,
        loading: false,
        listError: '',
        active:
          state.activeId === null
            ? state.active
            : result.chats.find((chat) => chat.id === state.activeId) ?? state.active,
      }))
    } catch (error) {
      if (error instanceof UnauthorizedError) {
        set({ loading: false })
        return
      }
      set({ loading: false, listError: describeApiError(error) })
    }
  },

  openChat: async (chatId) => {
    closeChatStream(chatStreamStore)
    set({
      activeId: chatId,
      active: null,
      loadError: '',
      sendError: '',
      actionError: '',
      streamStatus: { ...getState().streamStatus, [chatId]: 'connecting' },
    })
    try {
      const chat = await getChat(chatId)
      if (getState().activeId !== chatId) return
      set((state) => ({
        active: chat,
        running: { ...state.running, [chat.id]: chat.running },
      }))
      openChatStream(chatStreamStore, chatId)
    } catch (error) {
      if (error instanceof UnauthorizedError) return
      set({ loadError: describeApiError(error, '对话不存在或已删除。'), activeId: null })
    }
  },

  startNewChat: (groupId) => {
    closeChatStream(chatStreamStore)
    set({
      activeId: null,
      active: null,
      loadError: '',
      sendError: '',
      actionError: '',
      selectedGroupId: groupId,
    } as Partial<ChatsState>)
  },

  createAndSend: async (text, modelId, groupId, images) => {
    set({ sendError: '', actionError: '' })
    try {
      const chat = await createChat({ modelId, text, groupId, images })
      set((state) => ({
        chats: [chat, ...state.chats.filter((entry) => entry.id !== chat.id)],
        activeId: chat.id,
        active: chat,
        running: { ...state.running, [chat.id]: true },
      }))
      return chat
    } catch (error) {
      if (error instanceof UnauthorizedError) return null
      set({ sendError: describeApiError(error), actionError: error instanceof ApiError ? error.code : '' })
      return null
    }
  },

  sendMessage: async (text, modelId, images) => {
    const chatId = getState().activeId
    if (chatId === null) return false
    set({ sendError: '', actionError: '' })
    try {
      const chat = await sendChatMessage(chatId, { text, images, modelId })
      set((state) => ({
        active: chat,
        chats: touchList(state.chats, chat),
        running: { ...state.running, [chatId]: true },
      }))
      return true
    } catch (error) {
      if (error instanceof UnauthorizedError) return false
      set({ sendError: describeApiError(error), actionError: error instanceof ApiError ? error.code : '' })
      return false
    }
  },

  regenerate: async () => {
    const chatId = getState().activeId
    if (chatId === null) return false
    set({ sendError: '', actionError: '' })
    try {
      const chat = await regenerateChat(chatId)
      set((state) => ({
        active: chat,
        chats: touchList(state.chats, chat),
        running: { ...state.running, [chatId]: true },
      }))
      return true
    } catch (error) {
      if (error instanceof UnauthorizedError) return false
      set({ sendError: describeApiError(error), actionError: error instanceof ApiError ? error.code : '' })
      return false
    }
  },

  stop: async () => {
    const chatId = getState().activeId
    if (chatId === null) return false
    try {
      await stopChat(chatId)
      return true
    } catch (error) {
      if (error instanceof UnauthorizedError) return false
      set({ sendError: describeApiError(error) })
      return false
    }
  },

  removeActive: async () => {
    const chatId = getState().activeId
    if (chatId === null) return false
    try {
      await deleteChat(chatId)
      set((state) => ({
        chats: state.chats.filter((chat) => chat.id !== chatId),
        messages: { ...state.messages, [chatId]: [] },
        activeId: null,
        active: null,
      }))
      closeChatStream(chatStreamStore)
      return true
    } catch (error) {
      if (error instanceof UnauthorizedError) return false
      set({ actionError: describeApiError(error) })
      return false
    }
  },

  rename: async (title) => {
    const chatId = getState().activeId
    if (chatId === null) return false
    try {
      const chat = await updateChat(chatId, { title })
      set((state) => ({ active: chat, chats: touchList(state.chats, chat) }))
      return true
    } catch (error) {
      if (error instanceof UnauthorizedError) return false
      set({ actionError: describeApiError(error) })
      return false
    }
  },

  setModel: async (modelId) => {
    const chatId = getState().activeId
    if (chatId === null) {
      set({ draftModelId: modelId })
      return true
    }
    try {
      const chat = await updateChat(chatId, { modelId })
      set((state) => ({ active: chat, chats: touchList(state.chats, chat) }))
      return true
    } catch (error) {
      if (error instanceof UnauthorizedError) return false
      set({ actionError: describeApiError(error) })
      return false
    }
  },

  createGroup: async (name) => {
    try {
      const created = await createChatGroup({ name })
      set((state) => ({ groups: [...state.groups, created] }))
      return created
    } catch (error) {
      if (error instanceof UnauthorizedError) return null
      set({ actionError: describeApiError(error) })
      return null
    }
  },

  deleteGroup: async (id) => {
    try {
      await deleteChatGroup(id)
      set((state) => ({
        groups: state.groups.filter((group) => group.id !== id),
        chats: state.chats.filter((chat) => chat.groupId !== id),
      }))
      return true
    } catch (error) {
      if (error instanceof UnauthorizedError) return false
      set({ actionError: describeApiError(error) })
      return false
    }
  },

  moveToGroup: async (chatId, groupId) => {
    try {
      const chat = await updateChat(chatId, { groupId })
      set((state) => ({ chats: state.chats.map((entry) => (entry.id === chatId ? chat : entry)) }))
      return true
    } catch (error) {
      if (error instanceof UnauthorizedError) return false
      set({ actionError: describeApiError(error) })
      return false
    }
  },

  setStreamStatus: (chatId, status) => {
    set((state) => ({ streamStatus: { ...state.streamStatus, [chatId]: status } }))
  },

  applySnapshot: (snapshot) => {
    const chat = snapshot.conversation
    set((state) => ({
      activeId: chat.id,
      active: chat,
      messages: { ...state.messages, [chat.id]: entriesFromMessages(snapshot.messages) },
      running: { ...state.running, [chat.id]: snapshot.running },
      truncated: { ...state.truncated, [chat.id]: snapshot.truncated },
      streamStatus: { ...state.streamStatus, [chat.id]: 'open' },
      chats: touchList(state.chats, chat),
    }))
  },

  applyDelta: (chatId, delta) => {
    set((state) => ({ messages: { ...state.messages, [chatId]: applyDelta(state.messages[chatId] ?? [], delta) } }))
  },

  applyMessageUpdate: (chatId, message) => {
    set((state) => ({
      messages: { ...state.messages, [chatId]: upsertMessage(state.messages[chatId] ?? [], message) },
      running: message.status === 'pending' ? state.running : { ...state.running, [chatId]: false },
    }))
  },

  applyConversation: (chat) => {
    set((state) => ({
      active: state.activeId === chat.id ? chat : state.active,
      running: { ...state.running, [chat.id]: chat.running },
      chats: touchList(state.chats, chat),
    }))
  },

  applyTurn: (chatId, turn) => {
    if (turn.phase === 'started') {
      set((state) => ({ running: { ...state.running, [chatId]: true } }))
      return
    }
    set((state) => ({
      running: { ...state.running, [chatId]: false },
      messages: { ...state.messages, [chatId]: settleStreaming(state.messages[chatId] ?? []) },
    }))
  },

  handleClosed: (chatId, reason) => {
    set((state) => ({
      running: { ...state.running, [chatId]: false },
      chats: reason === 'deleted' ? state.chats.filter((chat) => chat.id !== chatId) : state.chats,
      activeId: reason === 'deleted' && state.activeId === chatId ? null : state.activeId,
      active: reason === 'deleted' && state.activeId === chatId ? null : state.active,
    }))
  },
}))

const chatStreamStore: ChatStreamStore = {
  get streamStatus() { return useChatsStore.getState().streamStatus },
  setStreamStatus: (chatId, status) => useChatsStore.getState().setStreamStatus(chatId, status),
  applySnapshot: (snapshot) => useChatsStore.getState().applySnapshot(snapshot),
  applyDelta: (chatId, delta) => useChatsStore.getState().applyDelta(chatId, delta),
  applyMessageUpdate: (chatId, message) => useChatsStore.getState().applyMessageUpdate(chatId, message),
  applyConversation: (chat) => useChatsStore.getState().applyConversation(chat),
  applyTurn: (chatId, turn) => useChatsStore.getState().applyTurn(chatId, turn),
  handleClosed: (chatId, reason) => useChatsStore.getState().handleClosed(chatId, reason),
}
