<script setup lang="ts">
// Middle column of the pure-chat area: group and conversation management,
// structured like the project tree (Group -> Conversations).
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import type { Chat, ChatGroup } from '../../../api/chats'
import { useChatsStore } from '../../../stores/chats'
import RunningIndicator from '../../../shared/components/RunningIndicator.vue'
import { UiDialog, UiButton } from '../../../shared/ui'
import ChatGroupDialog from './ChatGroupDialog.vue'
import MoveChatDialog from './MoveChatDialog.vue'

const chats = useChatsStore()
const router = useRouter()
const route = useRoute()

const search = ref('')
const dialog = ref<{
  type: 'create-group' | 'rename-group' | 'move-chat' | 'delete-group' | 'delete-chat'
  group?: ChatGroup | null
  chat?: Chat | null
} | null>(null)
const deleting = ref(false)
const deleteError = ref('')

const EXPANDED_GROUPS_STORAGE_KEY = 'dev-platform-chat-groups-expanded'
const expandedGroupIds = ref<number[]>([])
const expandedUngrouped = ref<boolean>(true)

try {
  const saved = localStorage.getItem(EXPANDED_GROUPS_STORAGE_KEY)
  if (saved) {
    expandedGroupIds.value = JSON.parse(saved)
  }
} catch {
  // Ignore localStorage parsing errors
}

function persistExpandedGroups() {
  try {
    localStorage.setItem(EXPANDED_GROUPS_STORAGE_KEY, JSON.stringify(expandedGroupIds.value))
  } catch {
    // Best-effort
  }
}

function toggleGroup(groupId: number) {
  if (expandedGroupIds.value.includes(groupId)) {
    expandedGroupIds.value = expandedGroupIds.value.filter((id) => id !== groupId)
  } else {
    expandedGroupIds.value.push(groupId)
  }
  persistExpandedGroups()
}

function isGroupExpanded(groupId: number): boolean {
  if (query.value !== '') return true
  return expandedGroupIds.value.includes(groupId)
}

const activeId = computed<number | null>(() => {
  const raw = route.params.chatId
  if (raw === undefined) return null
  const value = Number(raw)
  return Number.isFinite(value) && value > 0 ? value : null
})

const query = computed(() => search.value.trim().toLowerCase())

onMounted(async () => {
  await Promise.all([chats.loadGroups(), chats.loadChats()])
  // If no saved expanded states and groups exist, expand all groups by default
  if (expandedGroupIds.value.length === 0 && chats.groups.length > 0) {
    expandedGroupIds.value = chats.groups.map((g) => g.id)
    persistExpandedGroups()
  }
})

// The quick-assistant popover creates conversations outside this sidebar.
// Reload when that surface reports a list change.
watch(
  () => chats.listNonce,
  () => {
    void chats.loadChats()
  },
)

// Auto-expand group containing the active chat
watch(
  () => activeId.value,
  (id) => {
    if (!id) return
    const currentChat = chats.chats.find((c) => c.id === id)
    if (currentChat?.groupId && !expandedGroupIds.value.includes(currentChat.groupId)) {
      expandedGroupIds.value.push(currentChat.groupId)
      persistExpandedGroups()
    }
  },
  { immediate: true },
)

function chatsForGroup(groupId: number): Chat[] {
  return chats.chats.filter((c) => (c.groupId ?? 0) === groupId)
}

function getAssistantIcon(group: ChatGroup): string {
  if (group.icon && group.icon.trim()) return group.icon.trim()
  const trimmed = group.name.trim()
  return trimmed ? Array.from(trimmed)[0].toUpperCase() : '助'
}

function isSystemGroup(_group: ChatGroup): boolean {
  return false
}

function isSystemChat(_chat: Chat): boolean {
  return false
}

const ungroupedChats = computed(() => {
  return chats.chats.filter((c) => !c.groupId || c.groupId === 0)
})

interface VisibleGroup {
  group: ChatGroup
  chats: Chat[]
}

const visibleGroups = computed<VisibleGroup[]>(() => {
  const q = query.value
  return chats.groups
    .map((group) => {
      const allChats = chatsForGroup(group.id)
      const matchingChats = q === ''
        ? allChats
        : allChats.filter((c) => c.title.toLowerCase().includes(q))
      const groupMatches = q !== '' && group.name.toLowerCase().includes(q)
      return {
        group,
        chats: groupMatches ? allChats : matchingChats,
        matches: groupMatches || matchingChats.length > 0,
      }
    })
    .filter((item) => (q === '' ? true : item.matches))
    .map((item) => ({ group: item.group, chats: item.chats }))
})

const visibleUngroupedChats = computed(() => {
  const q = query.value
  if (q === '') return ungroupedChats.value
  return ungroupedChats.value.filter((c) => c.title.toLowerCase().includes(q))
})

function startNewChat(groupId?: number) {
  chats.startNewChat(groupId)
  if (route.name !== 'chat') {
    void router.push({ name: 'chat' })
  }
}

function openChat(id: number) {
  if (id === activeId.value) return
  void router.push({ name: 'chat-detail', params: { chatId: String(id) } })
}

function openCreateGroup() {
  dialog.value = { type: 'create-group' }
}

function openRenameGroup(group: ChatGroup) {
  dialog.value = { type: 'rename-group', group }
}

function openMoveChat(chat: Chat) {
  dialog.value = { type: 'move-chat', chat }
}

function onDeleteGroup(group: ChatGroup) {
  deleteError.value = ''
  dialog.value = { type: 'delete-group', group }
}

async function confirmDeleteGroup() {
  if (!dialog.value || dialog.value.type !== 'delete-group' || !dialog.value.group || deleting.value) return
  const group = dialog.value.group
  deleting.value = true
  deleteError.value = ''

  const success = await chats.deleteGroup(group.id)
  deleting.value = false
  if (!success) {
    deleteError.value = chats.groupsError || '删除助手失败，请重试。'
    return
  }

  dialog.value = null
  if (activeId.value) {
    // If active chat was in this group, navigate back to /chat
    if (!chats.chats.some((c) => c.id === activeId.value)) {
      void router.push({ name: 'chat' })
    }
  }
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

  const wasActive = activeId.value === chat.id
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
  <div class="chat-sidebar">
    <div class="sidebar-actions">
      <button type="button" class="action-btn new-group-btn" data-test="chat-new-group-btn" @click="openCreateGroup">
        + 新建助手
      </button>
    </div>

    <input
      v-model="search"
      type="search"
      class="search"
      placeholder="搜索助手或对话"
      aria-label="搜索助手或对话"
    />

    <p v-if="chats.listError || chats.groupsError" class="list-error" role="alert">
      {{ chats.listError || chats.groupsError }}
    </p>

    <div class="tree-container" data-test="chat-tree">
      <!-- Chat Assistants -->
      <ul class="group-tree" aria-label="助手列表">
        <li v-for="{ group, chats: groupChats } in visibleGroups" :key="group.id" class="group-item">
          <div class="group-row" :class="{ expanded: isGroupExpanded(group.id) }">
            <button
              type="button"
              class="group-toggle"
              :aria-expanded="isGroupExpanded(group.id)"
              @click.stop="toggleGroup(group.id)"
            >
              <svg
                class="caret-icon"
                :class="{ expanded: isGroupExpanded(group.id) }"
                viewBox="0 0 16 16"
                fill="none"
                stroke="currentColor"
                stroke-width="1.8"
                stroke-linecap="round"
                stroke-linejoin="round"
                aria-hidden="true"
              >
                <polyline points="6 4 10 8 6 12" />
              </svg>
              <span class="assistant-badge" :title="group.name">{{ getAssistantIcon(group) }}</span>
              <span class="group-name" :title="group.name">{{ group.name }}</span>
              <span class="count">{{ chatsForGroup(group.id).length }}</span>
            </button>

            <div class="group-controls">
              <button
                type="button"
                class="icon-btn"
                :aria-label="`在「${group.name}」中新建对话`"
                title="新建对话"
                @click.stop="startNewChat(group.id)"
              >
                <svg viewBox="0 0 16 16" width="12" height="12" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round">
                  <line x1="8" y1="3" x2="8" y2="13" />
                  <line x1="3" y1="8" x2="13" y2="8" />
                </svg>
              </button>
              <button
                type="button"
                class="icon-btn"
                :aria-label="`编辑助手「${group.name}」`"
                title="编辑助手"
                @click.stop="openRenameGroup(group)"
              >
                <svg viewBox="0 0 16 16" width="12" height="12" fill="none" stroke="currentColor" stroke-width="1.8">
                  <path d="M11 2.5l2.5 2.5L5 13.5H2.5V11L11 2.5z" />
                </svg>
              </button>
              <button
                type="button"
                class="icon-btn danger"
                :aria-label="`删除助手「${group.name}」`"
                title="删除助手"
                @click.stop="onDeleteGroup(group)"
              >
                <svg viewBox="0 0 16 16" width="12" height="12" fill="none" stroke="currentColor" stroke-width="1.8">
                  <path d="M3 4h10M6 4V2.5h4V4M4.5 4v9.5h7V4" />
                </svg>
              </button>
            </div>
          </div>

          <!-- Assistant Conversations -->
          <ul v-if="isGroupExpanded(group.id)" class="chat-sublist">
            <li v-for="chat in groupChats" :key="chat.id" class="chat-subitem">
              <div
                class="chat-row"
                :class="{ active: chat.id === activeId }"
                :aria-current="chat.id === activeId ? 'page' : undefined"
                :data-chat-id="chat.id"
                role="button"
                tabindex="0"
                @click="openChat(chat.id)"
                @keydown.enter.prevent="openChat(chat.id)"
                @keydown.space.prevent="openChat(chat.id)"
              >
                <!-- Row 1: Title + Running + Action Strip -->
                <div class="chat-row-top">
                  <RunningIndicator v-if="chat.running" class="chat-item-running shrink-0" />
                  <span class="chat-title truncate" :title="chat.title || '未命名对话'">
                    {{ chat.title || '未命名对话' }}
                  </span>
                  <div class="item-actions ml-auto shrink-0">
                    <button
                      type="button"
                      class="mini-btn"
                      title="移动对话"
                      aria-label="移动对话"
                      @click.stop="openMoveChat(chat)"
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
                      @click.stop="onDeleteChat(chat)"
                    >
                      <svg viewBox="0 0 16 16" width="11" height="11" fill="none" stroke="currentColor" stroke-width="1.6">
                        <path d="M3 4h10M6 4V2.5h4V4M4.5 4v9.5h7V4" />
                      </svg>
                    </button>
                  </div>
                </div>

                <!-- Row 2: Subtitle/Time -->
                <div class="chat-row-bottom">
                  <span class="chat-time">{{ formatChatTime(chat.updatedAt) }}</span>
                </div>
              </div>
            </li>
            <li v-if="groupChats.length === 0" class="sublist-empty">
              暂无对话
            </li>
          </ul>
        </li>

        <!-- Ungrouped Conversations -->
        <li v-if="visibleUngroupedChats.length > 0" class="group-item ungrouped">
          <div class="group-row" :class="{ expanded: query !== '' || expandedUngrouped }">
            <button
              type="button"
              class="group-toggle"
              :aria-expanded="query !== '' || expandedUngrouped"
              @click.stop="expandedUngrouped = !expandedUngrouped"
            >
              <svg
                class="caret-icon"
                :class="{ expanded: query !== '' || expandedUngrouped }"
                viewBox="0 0 16 16"
                fill="none"
                stroke="currentColor"
                stroke-width="1.8"
                stroke-linecap="round"
                stroke-linejoin="round"
                aria-hidden="true"
              >
                <polyline points="6 4 10 8 6 12" />
              </svg>
              <span class="group-name">未分组对话</span>
              <span class="count">{{ ungroupedChats.length }}</span>
            </button>
          </div>

          <ul v-if="query !== '' || expandedUngrouped" class="chat-sublist">
            <li v-for="chat in visibleUngroupedChats" :key="chat.id" class="chat-subitem">
              <div
                class="chat-row"
                :class="{ active: chat.id === activeId }"
                :aria-current="chat.id === activeId ? 'page' : undefined"
                :data-chat-id="chat.id"
                role="button"
                tabindex="0"
                @click="openChat(chat.id)"
                @keydown.enter.prevent="openChat(chat.id)"
                @keydown.space.prevent="openChat(chat.id)"
              >
                <div class="chat-row-top">
                  <RunningIndicator v-if="chat.running" class="chat-item-running shrink-0" />
                  <span class="chat-title truncate" :title="chat.title || '未命名对话'">
                    {{ chat.title || '未命名对话' }}
                  </span>
                  <div class="item-actions ml-auto shrink-0">
                    <button
                      type="button"
                      class="mini-btn"
                      title="移动对话"
                      aria-label="移动对话"
                      @click.stop="openMoveChat(chat)"
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
                      @click.stop="onDeleteChat(chat)"
                    >
                      <svg viewBox="0 0 16 16" width="11" height="11" fill="none" stroke="currentColor" stroke-width="1.6">
                        <path d="M3 4h10M6 4V2.5h4V4M4.5 4v9.5h7V4" />
                      </svg>
                    </button>
                  </div>
                </div>

                <div class="chat-row-bottom">
                  <span class="chat-time">{{ formatChatTime(chat.updatedAt) }}</span>
                </div>
              </div>
            </li>
          </ul>
        </li>
      </ul>

      <p
        v-if="!chats.loading && visibleGroups.length === 0 && visibleUngroupedChats.length === 0"
        class="empty"
      >
        {{ query ? '没有匹配的助手或对话。' : '还没有助手，点击「+ 新建助手」开始。' }}
      </p>
    </div>

    <!-- Modals -->
    <ChatGroupDialog
      v-if="dialog && (dialog.type === 'create-group' || dialog.type === 'rename-group')"
      :mode="dialog.type === 'create-group' ? 'create' : 'rename'"
      :group="dialog.group"
      @close="dialog = null"
      @submitted="() => { dialog = null; void chats.loadGroups() }"
    />

    <MoveChatDialog
      v-if="dialog && dialog.type === 'move-chat' && dialog.chat"
      :chat="dialog.chat"
      @close="dialog = null"
      @submitted="dialog = null"
    />

    <UiDialog
      v-if="dialog && dialog.type === 'delete-group' && dialog.group"
      title="删除助手"
      width-class="w-full max-w-md"
      data-test="delete-group-dialog"
      @close="dialog = null"
    >
      <div class="space-y-2">
        <p class="text-sm font-medium text-platform-text">
          确定要删除助手「{{ dialog.group.name }}」吗？
        </p>
        <p class="text-xs text-platform-text-muted leading-relaxed">
          {{ chatsForGroup(dialog.group.id).length > 0 ? `该助手下的 ${chatsForGroup(dialog.group.id).length} 个对话将一并删除，且无法恢复。` : '删除后，该助手将从平台中移除。' }}
        </p>
        <p v-if="deleteError" class="text-xs text-red-500" role="alert">{{ deleteError }}</p>
      </div>
      <template #footer>
        <UiButton type="button" variant="secondary" @click="dialog = null">取消</UiButton>
        <UiButton
          type="button"
          variant="danger"
          :disabled="deleting"
          data-test="confirm-delete-group"
          @click="confirmDeleteGroup"
        >
          {{ deleting ? '删除中…' : '删除' }}
        </UiButton>
      </template>
    </UiDialog>

    <UiDialog
      v-if="dialog && dialog.type === 'delete-chat' && dialog.chat"
      title="删除对话"
      width-class="w-full max-w-md"
      data-test="delete-chat-dialog"
      @close="dialog = null"
    >
      <div class="space-y-2">
        <p class="text-sm font-medium text-platform-text">
          确定要删除对话「{{ dialog.chat.title || '未命名对话' }}」吗？
        </p>
        <p class="text-xs text-platform-text-muted leading-relaxed">
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
  </div>
</template>

<style scoped>
.chat-sidebar {
  display: flex;
  flex-direction: column;
  min-height: 0;
  flex: 1;
  overflow: hidden;
}

.sidebar-actions {
  display: flex;
  gap: 6px;
  padding: 0 12px 8px 12px;
}

.action-btn {
  flex: 1;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  height: 26px;
  border: 1px solid var(--color-border);
  background: var(--color-surface);
  color: var(--color-text);
  border-radius: var(--radius, 4px);
  padding: 0 8px;
  font: inherit;
  font-size: 12px;
  font-weight: 500;
  cursor: pointer;
  transition: color 0.12s ease-out, border-color 0.12s ease-out, background-color 0.12s ease-out;
  white-space: nowrap;
}

.action-btn:hover {
  background: var(--color-surface-muted);
  border-color: var(--color-border-strong);
  color: var(--color-text);
}

.search {
  margin: 0 12px 8px 12px;
  padding: 4px 10px;
  height: 28px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius, 4px);
  background: var(--color-surface);
  color: var(--color-text);
  font: inherit;
  font-size: 12px;
  box-sizing: border-box;
  outline: none;
  transition: border-color 0.12s ease-out;
}

.search:focus {
  border-color: var(--color-accent, #4a8ad6);
}

.search::placeholder {
  color: var(--color-text-muted);
}

.tree-container {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 0 8px 16px 8px;
}

.group-tree {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.group-item {
  display: flex;
  flex-direction: column;
}

.group-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 4px 6px;
  border-radius: var(--radius, 4px);
  transition: background-color 0.12s ease-out;
}

.group-row:hover {
  background: var(--color-surface-muted);
}

.group-toggle {
  display: flex;
  align-items: center;
  gap: 6px;
  border: none;
  background: transparent;
  color: var(--color-text);
  font: inherit;
  font-size: 13px;
  font-weight: 500;
  cursor: pointer;
  text-align: left;
  flex: 1;
  min-width: 0;
  padding: 2px 0;
}

.caret-icon {
  width: 12px;
  height: 12px;
  color: var(--color-text-muted);
  flex-shrink: 0;
  transition: transform 0.15s ease-out;
}

.caret-icon.expanded {
  transform: rotate(90deg);
}

.group-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.count {
  font-size: 11px;
  color: var(--color-text-muted);
  background: var(--color-surface-muted);
  border: 1px solid var(--color-border);
  border-radius: var(--radius, 4px);
  padding: 0 4px;
  font-weight: normal;
  font-variant-numeric: tabular-nums;
  flex-shrink: 0;
}

.assistant-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 18px;
  height: 18px;
  border-radius: 3px;
  background: var(--color-surface-muted);
  border: 1px solid var(--color-border);
  color: var(--color-text);
  font-size: 10px;
  font-weight: 600;
  flex-shrink: 0;
  user-select: none;
}

.group-controls {
  display: flex;
  align-items: center;
  gap: 2px;
  opacity: 0;
  transition: opacity 0.12s ease-out;
}

.group-row:hover .group-controls,
.group-row:focus-within .group-controls {
  opacity: 1;
}

.icon-btn {
  width: 20px;
  height: 20px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border: none;
  background: transparent;
  color: var(--color-text-muted);
  font-size: 12px;
  cursor: pointer;
  padding: 0;
  border-radius: var(--radius, 4px);
  transition: color 0.12s ease-out, background-color 0.12s ease-out;
}

.icon-btn:hover {
  color: var(--color-text);
  background: var(--color-surface-muted);
}

.icon-btn.danger:hover {
  color: var(--color-danger, #e5566f);
  background: color-mix(in srgb, var(--color-danger, #e5566f) 10%, transparent);
}

.chat-sublist {
  list-style: none;
  margin: 2px 0 4px 8px;
  padding: 0 0 0 6px;
  border-left: 1px solid var(--color-border);
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.chat-subitem {
  width: 100%;
}

.chat-row {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 5px 8px;
  border-radius: var(--radius, 4px);
  border-left: 2px solid transparent;
  background: transparent;
  color: var(--color-text);
  cursor: pointer;
  outline: none;
  transition: background-color 0.12s ease-out, border-color 0.12s ease-out;
  user-select: none;
}

.chat-row:hover {
  background: var(--color-surface-muted);
}

.chat-row.active {
  border-left-color: var(--color-accent, #4a8ad6);
  background: var(--color-surface-muted);
  font-weight: 500;
}

.chat-row-top {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
}

.chat-title {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 12px;
  flex: 1;
}

.chat-row-bottom {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  min-width: 0;
}

.chat-time {
  font-size: 11px;
  color: var(--color-text-muted);
  font-variant-numeric: tabular-nums;
}

.item-actions {
  display: flex;
  align-items: center;
  gap: 2px;
  opacity: 0;
  transition: opacity 0.12s ease-out;
}

.chat-row:hover .item-actions,
.chat-row:focus-within .item-actions {
  opacity: 1;
}

.mini-btn {
  width: 18px;
  height: 18px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border: none;
  background: transparent;
  color: var(--color-text-muted);
  cursor: pointer;
  padding: 0;
  border-radius: var(--radius, 4px);
  transition: color 0.12s ease-out;
}

.mini-btn:hover {
  color: var(--color-text);
}

.mini-btn.danger:hover {
  color: var(--color-danger, #e5566f);
}

.sublist-empty {
  padding: 4px 8px;
  font-size: 11px;
  color: var(--color-text-muted);
}

.empty,
.list-error {
  margin: 12px 8px;
  font-size: 12px;
  color: var(--color-text-muted);
}

.list-error {
  color: var(--color-danger, #e5566f);
}
</style>
