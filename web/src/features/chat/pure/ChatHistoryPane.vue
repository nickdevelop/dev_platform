<script setup lang="ts">
// Right column of the chat pane: assistant conversation list (history pane).
// Displays the conversation list for the currently selected assistant,
// with an assistant selector at the top, a quick "new chat" action, search,
// and per-conversation management (move, delete).
import { computed, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import type { Chat } from '../../../api/chats'
import { useChatsStore } from '../../../stores/chats'
import RunningIndicator from '../../../shared/components/RunningIndicator.vue'
import { UiDialog, UiButton } from '../../../shared/ui'
import MoveChatDialog from './MoveChatDialog.vue'

const props = defineProps<{
  chatId?: number
}>()

const chats = useChatsStore()
const router = useRouter()
const route = useRoute()

const search = ref('')
const dialog = ref<{
  type: 'move-chat' | 'delete-chat'
  chat: Chat
} | null>(null)
const deleting = ref(false)
const deleteError = ref('')

const activeChatId = computed<number | null>(() => {
  if (props.chatId !== undefined && props.chatId !== null) return props.chatId
  const raw = route.params.chatId
  if (raw === undefined) return null
  const value = Number(raw)
  return Number.isFinite(value) && value > 0 ? value : null
})

// Assistant options for the selector at the top
const assistantOptions = computed(() => {
  const options = chats.groups.map((g) => ({
    id: g.id,
    name: g.name,
    icon: g.icon,
  }))
  options.push({
    id: 0,
    name: '未分组对话',
    icon: '#',
  })
  return options
})

const selectedAssistantId = computed<number>({
  get() {
    if (chats.selectedGroupId !== null && chats.selectedGroupId !== undefined) {
      return chats.selectedGroupId
    }
    return chats.groups[0]?.id ?? 0
  },
  set(val: number) {
    chats.selectedGroupId = val
  },
})

const query = computed(() => search.value.trim().toLowerCase())

const filteredChats = computed<Chat[]>(() => {
  const groupId = selectedAssistantId.value
  let list: Chat[]
  if (groupId === 0) {
    list = chats.chats.filter((c) => !c.groupId || c.groupId === 0)
  } else {
    list = chats.chats.filter((c) => c.groupId === groupId)
  }

  const q = query.value
  if (q) {
    list = list.filter((c) => c.title.toLowerCase().includes(q))
  }

  // Sort by updatedAt descending
  return [...list].sort((a, b) => {
    const tA = a.updatedAt ? new Date(a.updatedAt).getTime() : 0
    const tB = b.updatedAt ? new Date(b.updatedAt).getTime() : 0
    return tB - tA
  })
})

function onAssistantChange(event: Event) {
  const target = event.target as HTMLSelectElement
  const val = Number(target.value)
  chats.selectedGroupId = val
}

function startNewChat() {
  const groupId = chats.selectedGroupId !== 0 ? chats.selectedGroupId : undefined
  chats.startNewChat(groupId ?? undefined)
  if (route.name !== 'chat') {
    void router.push({ name: 'chat' })
  }
}

function openChat(id: number) {
  if (id === activeChatId.value) return
  void router.push({ name: 'chat-detail', params: { chatId: String(id) } })
}

function openMoveChat(chat: Chat) {
  dialog.value = { type: 'move-chat', chat }
}

function onDeleteChat(chat: Chat) {
  deleteError.value = ''
  dialog.value = { type: 'delete-chat', chat }
}

async function confirmDeleteChat() {
  if (!dialog.value || dialog.value.type !== 'delete-chat' || !dialog.value.chat || deleting.value) return
  const chat = dialog.value.chat
  deleting.value = true
  deleteError.value = ''

  const success = await chats.deleteChat(chat.id)
  deleting.value = false
  if (!success) {
    deleteError.value = chats.listError || '删除对话失败，请重试。'
    return
  }

  const wasActive = activeChatId.value === chat.id
  dialog.value = null
  if (wasActive) {
    void router.push({ name: 'chat' })
  }
}

function formatChatTime(isoString?: string): string {
  if (!isoString) return ''
  const date = new Date(isoString)
  if (isNaN(date.getTime())) return ''

  const now = new Date()
  const isToday =
    date.getFullYear() === now.getFullYear() &&
    date.getMonth() === now.getMonth() &&
    date.getDate() === now.getDate()

  if (isToday) {
    const hours = String(date.getHours()).padStart(2, '0')
    const minutes = String(date.getMinutes()).padStart(2, '0')
    return `${hours}:${minutes}`
  } else {
    const month = String(date.getMonth() + 1).padStart(2, '0')
    const day = String(date.getDate()).padStart(2, '0')
    return `${month}-${day}`
  }
}
</script>

<template>
  <aside class="chat-history-pane" aria-label="助手对话清单" data-test="chat-history-pane">
    <!-- Top Bar: Assistant selector & New Chat -->
    <div class="history-head">
      <div class="assistant-selector-wrap">
        <label for="assistant-select" class="sr-only">选择助手</label>
        <select
          id="assistant-select"
          class="assistant-select"
          data-test="assistant-select"
          :value="selectedAssistantId"
          @change="onAssistantChange"
        >
          <option v-for="opt in assistantOptions" :key="opt.id" :value="opt.id">
            {{ opt.name }}
          </option>
        </select>
      </div>

      <button
        type="button"
        class="new-chat-btn"
        data-test="history-new-chat-btn"
        title="新建对话"
        aria-label="新建对话"
        @click="startNewChat"
      >
        <svg viewBox="0 0 16 16" width="13" height="13" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
          <line x1="8" y1="3" x2="8" y2="13" />
          <line x1="3" y1="8" x2="13" y2="8" />
        </svg>
        <span>新建</span>
      </button>
    </div>

    <!-- Search box -->
    <div class="search-wrap">
      <input
        v-model="search"
        type="search"
        class="history-search"
        placeholder="搜索当前助手对话"
        aria-label="搜索当前助手对话"
      />
    </div>

    <!-- Conversation list -->
    <div class="conversations-scroll" data-test="conversations-scroll">
      <ul v-if="filteredChats.length > 0" class="conversation-list">
        <li
          v-for="item in filteredChats"
          :key="item.id"
          class="conversation-item"
          :class="{ active: item.id === activeChatId }"
          :data-chat-id="item.id"
          role="button"
          tabindex="0"
          @click="openChat(item.id)"
          @keydown.enter.prevent="openChat(item.id)"
          @keydown.space.prevent="openChat(item.id)"
        >
          <div class="item-top">
            <RunningIndicator v-if="item.running" class="item-running shrink-0" />
            <span class="item-title truncate" :title="item.title || '未命名对话'">
              {{ item.title || '未命名对话' }}
            </span>
            <div class="item-actions" @click.stop>
              <button
                type="button"
                class="mini-btn"
                title="移动对话"
                aria-label="移动对话"
                @click.stop="openMoveChat(item)"
              >
                <svg viewBox="0 0 16 16" width="11" height="11" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round">
                  <polyline points="5 9 8 6 11 9" />
                  <line x1="8" y1="6" x2="8" y2="13" />
                </svg>
              </button>
              <button
                type="button"
                class="mini-btn danger"
                title="删除对话"
                aria-label="删除对话"
                @click.stop="onDeleteChat(item)"
              >
                <svg viewBox="0 0 16 16" width="11" height="11" fill="none" stroke="currentColor" stroke-width="1.6">
                  <path d="M3 4h10M6 4V2.5h4V4M4.5 4v9.5h7V4" />
                </svg>
              </button>
            </div>
          </div>
          <div class="item-bottom">
            <span class="item-time">{{ formatChatTime(item.updatedAt) }}</span>
          </div>
        </li>
      </ul>

      <div v-else class="empty-state">
        <p class="empty-text">
          {{ query ? '未找到匹配的对话' : '暂无对话记录' }}
        </p>
        <UiButton
          v-if="!query"
          size="sm"
          variant="secondary"
          class="mt-2 text-xs"
          @click="startNewChat"
        >
          开始新对话
        </UiButton>
      </div>
    </div>

    <!-- Modals -->
    <MoveChatDialog
      v-if="dialog && dialog.type === 'move-chat' && dialog.chat"
      :chat="dialog.chat"
      @close="dialog = null"
      @submitted="dialog = null"
    />

    <UiDialog
      v-if="dialog && dialog.type === 'delete-chat' && dialog.chat"
      title="删除对话"
      width-class="w-full max-w-md"
      data-test="delete-chat-dialog"
      @close="dialog = null"
    >
      <div class="space-y-2">
        <p class="text-sm font-medium text-slate-800 dark:text-slate-100">
          确定要删除对话「{{ dialog.chat.title || '未命名对话' }}」吗？
        </p>
        <p class="text-xs text-slate-500 leading-relaxed">
          删除后，该对话及其全部消息将从平台中移除，且无法恢复。
        </p>
        <p v-if="deleteError" class="text-xs text-red-500" role="alert">{{ deleteError }}</p>
      </div>
      <template #footer>
        <UiButton type="button" variant="secondary" @click="dialog = null">取消</UiButton>
        <UiButton
          type="button"
          variant="danger"
          :disabled="deleting"
          data-test="confirm-delete-chat"
          @click="confirmDeleteChat"
        >
          {{ deleting ? '删除中…' : '删除' }}
        </UiButton>
      </template>
    </UiDialog>
  </aside>
</template>

<style scoped>
.chat-history-pane {
  display: flex;
  flex-direction: column;
  width: 280px;
  height: 100%;
  min-height: 0;
  flex-shrink: 0;
  border-left: 1px solid var(--color-border, #e2e8f0);
  background: var(--color-surface, #ffffff);
  box-sizing: border-box;
}

.history-head {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 12px 6px;
  border-bottom: 1px solid transparent;
}

.assistant-selector-wrap {
  flex: 1;
  min-width: 0;
}

.assistant-select {
  width: 100%;
  padding: 5px 8px;
  font-size: 0.8125rem;
  font-weight: 500;
  color: var(--color-text, #1e293b);
  background: var(--color-bg, #f8fafc);
  border: 1px solid var(--color-border, #e2e8f0);
  border-radius: 6px;
  outline: none;
  cursor: pointer;
  text-overflow: ellipsis;
  white-space: nowrap;
  overflow: hidden;
  transition: border-color 0.15s ease;
}

.assistant-select:focus {
  border-color: var(--color-primary, #3b82f6);
}

.new-chat-btn {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 5px 9px;
  font-size: 0.75rem;
  font-weight: 500;
  color: var(--color-primary, #3b82f6);
  background: var(--color-bg, #f8fafc);
  border: 1px solid var(--color-border, #e2e8f0);
  border-radius: 6px;
  cursor: pointer;
  flex-shrink: 0;
  transition: all 0.15s ease;
}

.new-chat-btn:hover {
  background: var(--color-surface-muted, #f1f5f9);
  border-color: var(--color-border-strong, #cbd5e1);
}

.search-wrap {
  padding: 0 12px 6px;
}

.history-search {
  width: 100%;
  padding: 5px 8px;
  font-size: 0.75rem;
  color: var(--color-text, #1e293b);
  background: var(--color-bg, #f8fafc);
  border: 1px solid var(--color-border, #e2e8f0);
  border-radius: 6px;
  outline: none;
  box-sizing: border-box;
  transition: border-color 0.15s ease;
}

.history-search:focus {
  border-color: var(--color-primary, #3b82f6);
}

.conversations-scroll {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 2px 8px 12px;
}

.conversation-list {
  list-style: none;
  padding: 0;
  margin: 0;
  display: flex;
  flex-direction: column;
  gap: 3px;
}

.conversation-item {
  padding: 7px 8px;
  border-radius: 6px;
  border: 1px solid transparent;
  cursor: pointer;
  background: transparent;
  user-select: none;
  transition: background-color 0.15s ease, border-color 0.15s ease;
}

.conversation-item:hover {
  background: var(--color-surface-muted, #f1f5f9);
}

.conversation-item.active {
  background: var(--color-surface-muted, #f1f5f9);
  border-color: var(--color-border, #e2e8f0);
}

:root[data-theme='dark'] .conversation-item.active {
  background: rgba(255, 255, 255, 0.06);
  border-color: rgba(255, 255, 255, 0.12);
}

.item-top {
  display: flex;
  align-items: center;
  gap: 6px;
}

.item-running {
  flex-shrink: 0;
}

.item-title {
  flex: 1;
  min-width: 0;
  font-size: 0.8125rem;
  font-weight: 500;
  color: var(--color-text, #1e293b);
}

.item-actions {
  display: none;
  align-items: center;
  gap: 2px;
  margin-left: auto;
  flex-shrink: 0;
}

.conversation-item:hover .item-actions,
.conversation-item:focus-within .item-actions {
  display: flex;
}

.mini-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 18px;
  height: 18px;
  padding: 0;
  border: none;
  background: transparent;
  color: var(--color-text-muted, #64748b);
  border-radius: 3px;
  cursor: pointer;
}

.mini-btn:hover {
  background: var(--color-border, #e2e8f0);
  color: var(--color-text, #1e293b);
}

.mini-btn.danger:hover {
  background: #fee2e2;
  color: #ef4444;
}

.item-bottom {
  display: flex;
  align-items: center;
  margin-top: 2px;
  padding-left: 1px;
}

.item-time {
  font-size: 0.6875rem;
  color: var(--color-text-muted, #94a3b8);
}

.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 36px 12px;
  text-align: center;
}

.empty-text {
  font-size: 0.75rem;
  color: var(--color-text-muted, #94a3b8);
  margin: 0;
}

.sr-only {
  position: absolute;
  width: 1px;
  height: 1px;
  padding: 0;
  margin: -1px;
  overflow: hidden;
  clip: rect(0, 0, 0, 0);
  white-space: nowrap;
  border-width: 0;
}
</style>
