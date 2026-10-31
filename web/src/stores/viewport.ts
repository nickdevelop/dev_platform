import { defineStore } from 'pinia'

export interface ScrollViewportState {
  scrollTop: number
  pinned: boolean
}

export const useViewportStore = defineStore('viewport', {
  state: () => ({
    sessions: {} as Record<string, ScrollViewportState>,
    activities: {} as Record<string, ScrollViewportState>,
    chats: {} as Record<number, ScrollViewportState>,
  }),

  actions: {
    saveSessionScroll(sessionId: string, scrollTop: number, pinned: boolean) {
      if (!sessionId) return
      this.sessions[sessionId] = { scrollTop, pinned }
    },

    getSessionScroll(sessionId: string): ScrollViewportState | undefined {
      return this.sessions[sessionId]
    },

    saveActivityScroll(sessionId: string, scrollTop: number, pinned: boolean) {
      if (!sessionId) return
      this.activities[sessionId] = { scrollTop, pinned }
    },

    getActivityScroll(sessionId: string): ScrollViewportState | undefined {
      return this.activities[sessionId]
    },

    saveChatScroll(chatId: number, scrollTop: number, pinned: boolean) {
      if (!chatId || Number.isNaN(chatId)) return
      this.chats[chatId] = { scrollTop, pinned }
    },

    getChatScroll(chatId: number): ScrollViewportState | undefined {
      return this.chats[chatId]
    },

    clearSession(sessionId: string) {
      delete this.sessions[sessionId]
      delete this.activities[sessionId]
    },

    clearChat(chatId: number) {
      delete this.chats[chatId]
    },
  },
})
