<script setup lang="ts">
// Middle column of the pure-chat area: group and conversation management,
// structured like the project tree (Group -> Conversations).
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import type { Chat, ChatGroup } from '../../../api/chats'
import { useChatsStore } from '../../../stores/chats'
import RunningIndicator from '../../../shared/components/RunningIndicator.vue'
import ChatGroupDialog from './ChatGroupDialog.vue'
import MoveChatDialog from './MoveChatDialog.vue'

const chats = useChatsStore()
const router = useRouter()
const route = useRoute()

const search = ref('')
const dialog = ref<{
  type: 'create-group' | 'rename-group' | 'move-chat'
  group?: ChatGroup | null
  chat?: Chat | null
} | null>(null)

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

async function onDeleteGroup(group: ChatGroup) {
  const count = chatsForGroup(group.id).length
  const msg = count > 0
    ? `确定要删除助手「${group.name}」吗？\n该助手下的 ${count} 个对话将一并删除！`
    : `确定要删除助手「${group.name}」吗？`
  if (!window.confirm(msg)) return

  const success = await chats.deleteGroup(group.id)
  if (success && activeId.value) {
    // If active chat was in this group, navigate back to /chat
    if (!chats.chats.some((c) => c.id === activeId.value)) {
      void router.push({ name: 'chat' })
    }
  }
}

async function onDeleteChat(chat: Chat) {
  if (!window.confirm(`确定要删除对话「${chat.title || '未命名对话'}」吗？`)) return
  await chats.deleteChat(chat.id)
  if (activeId.value === chat.id) {
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
              <span class="caret">{{ isGroupExpanded(group.id) ? '▾' : '▸' }}</span>
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
                ＋
              </button>
              <button
                type="button"
                class="icon-btn"
                :aria-label="`编辑助手「${group.name}」`"
                title="编辑助手"
                @click.stop="openRenameGroup(group)"
              >
                ✎
              </button>
              <button
                type="button"
                class="icon-btn danger"
                :aria-label="`删除助手「${group.name}」`"
                title="删除助手"
                @click.stop="onDeleteGroup(group)"
              >
                🗑
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
                <div class="chat-main">
                  <span class="chat-title" :title="chat.title || '未命名对话'">
                    {{ chat.title || '未命名对话' }}
                  </span>
                  <span class="chat-time">{{ formatChatTime(chat.updatedAt) }}</span>
                </div>

                <div v-if="chat.running" class="chat-badges">
                  <RunningIndicator label="生成中" class="chat-item-running" />
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
              <span class="caret">{{ (query !== '' || expandedUngrouped) ? '▾' : '▸' }}</span>
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
                <div class="chat-main">
                  <span class="chat-title" :title="chat.title || '未命名对话'">
                    {{ chat.title || '未命名对话' }}
                  </span>
                  <span class="chat-time">{{ formatChatTime(chat.updatedAt) }}</span>
                </div>

                <div v-if="chat.running" class="chat-badges">
                  <RunningIndicator label="生成中" class="chat-item-running" />
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
  gap: calc(var(--space-unit) * 1.0);
  padding: 0 calc(var(--space-unit) * 2.0) calc(var(--space-unit) * 1.333);
}

.action-btn {
  flex: 1;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border: 1px solid var(--color-border-strong);
  background: var(--color-surface);
  color: var(--color-text);
  border-radius: var(--radius);
  padding: calc(var(--space-unit) * 0.667) calc(var(--space-unit) * 1.333);
  font: inherit;
  font-size: 0.857rem;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.15s ease;
  white-space: nowrap;
}

.action-btn:hover {
  background: var(--color-surface-muted);
  border-color: var(--color-primary);
  color: var(--color-primary);
}

.search {
  margin: 0 calc(var(--space-unit) * 2.0) calc(var(--space-unit) * 1.333);
  padding: calc(var(--space-unit) * 1.0) calc(var(--space-unit) * 1.667);
  border: 1px solid var(--color-border-strong);
  border-radius: var(--radius);
  background: var(--color-surface);
  color: var(--color-text);
  font: inherit;
  font-size: 1rem;
  box-sizing: border-box;
  outline: none;
  transition: border-color 0.15s ease, box-shadow 0.15s ease;
}

.search:focus {
  border-color: var(--color-primary);
  box-shadow: 0 0 0 2px var(--color-info-soft);
}

.search::placeholder {
  color: var(--color-text-muted);
}

.tree-container {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 0 calc(var(--space-unit) * 1.5) calc(var(--space-unit) * 2.0);
}

.group-tree {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: calc(var(--space-unit) * 0.5);
}

.group-item {
  display: flex;
  flex-direction: column;
}

.group-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: calc(var(--space-unit) * 0.5) calc(var(--space-unit) * 0.667);
  border-radius: var(--radius);
  transition: background-color 0.15s ease;
}

.group-row:hover {
  background: var(--color-surface-muted);
}

.group-toggle {
  display: flex;
  align-items: center;
  gap: calc(var(--space-unit) * 0.6);
  border: none;
  background: transparent;
  color: var(--color-text);
  font: inherit;
  font-size: 1rem;
  font-weight: 600;
  cursor: pointer;
  text-align: left;
  flex: 1;
  min-width: 0;
  padding: calc(var(--space-unit) * 0.667) 0;
}

.caret {
  font-size: 0.857rem;
  color: var(--color-text-muted);
  width: 1em;
  text-align: center;
}

.group-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.count {
  font-size: 0.857rem;
  color: var(--color-text-muted);
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: 9999px;
  padding: 0 0.5em;
  font-weight: normal;
  flex-shrink: 0;
}

.assistant-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 22px;
  height: 22px;
  border-radius: 6px;
  background: var(--color-primary-soft, rgba(59, 130, 246, 0.12));
  color: var(--color-primary);
  font-size: 0.8rem;
  font-weight: 600;
  flex-shrink: 0;
  user-select: none;
}

.system-badge {
  font-size: 0.68rem;
  color: var(--color-primary);
  border: 1px solid var(--color-primary);
  border-radius: 9999px;
  padding: 0 0.45em;
  flex-shrink: 0;
}

.group-controls {
  display: flex;
  align-items: center;
  gap: calc(var(--space-unit) * 0.2);
  opacity: 0;
  transition: opacity 0.15s ease;
}

.group-row:hover .group-controls,
.group-row:focus-within .group-controls {
  opacity: 1;
}

.icon-btn {
  border: none;
  background: transparent;
  color: var(--color-text-muted);
  font-size: 0.929rem;
  cursor: pointer;
  padding: calc(var(--space-unit) * 0.333) calc(var(--space-unit) * 0.5);
  border-radius: var(--radius);
  line-height: 1;
  transition: all 0.15s ease;
}

.icon-btn:hover {
  color: var(--color-text);
  background: var(--color-surface);
}

.icon-btn.danger:hover {
  color: var(--color-danger);
}

.chat-sublist {
  list-style: none;
  margin: calc(var(--space-unit) * 0.2) 0 calc(var(--space-unit) * 0.5) calc(var(--space-unit) * 1.5);
  padding: 0 0 0 calc(var(--space-unit) * 0.6);
  border-left: 1px solid var(--color-border);
  display: flex;
  flex-direction: column;
  gap: calc(var(--space-unit) * 0.3);
}

.chat-subitem {
  width: 100%;
}

.chat-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: calc(var(--space-unit) * 0.5);
  padding: calc(var(--space-unit) * 0.75) calc(var(--space-unit) * 1.0);
  border: 1px solid transparent;
  border-radius: var(--radius);
  background: transparent;
  color: var(--color-text);
  cursor: pointer;
  outline: none;
  transition: all 0.15s ease;
  user-select: none;
}

.chat-row:hover {
  border-color: var(--color-border);
  background: var(--color-surface-muted);
}

.chat-row.active {
  border-color: var(--color-primary);
  background: var(--color-surface-muted);
  box-shadow: 0 0 6px var(--color-primary-soft, rgba(59, 130, 246, 0.2));
  font-weight: 500;
}

.chat-main {
  display: flex;
  flex-direction: column;
  gap: calc(var(--space-unit) * 0.2);
  min-width: 0;
  flex: 1;
}

.chat-title {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 0.929rem;
}

.chat-time {
  font-size: 0.75rem;
  color: var(--color-text-muted);
}

.chat-badges {
  display: flex;
  align-items: center;
  gap: calc(var(--space-unit) * 0.4);
  flex-shrink: 0;
}

.item-actions {
  display: flex;
  align-items: center;
  gap: calc(var(--space-unit) * 0.2);
  opacity: 0;
  transition: opacity 0.15s ease;
}

.chat-row:hover .item-actions,
.chat-row:focus-within .item-actions {
  opacity: 1;
}

.mini-btn {
  border: none;
  background: transparent;
  color: var(--color-text-muted);
  font-size: 0.857rem;
  cursor: pointer;
  padding: calc(var(--space-unit) * 0.333) calc(var(--space-unit) * 0.5);
  border-radius: var(--radius);
  line-height: 1;
  transition: all 0.15s ease;
}

.mini-btn:hover {
  color: var(--color-primary);
  background: var(--color-surface);
}

.mini-btn.danger:hover {
  color: var(--color-danger);
}

.sublist-empty {
  padding: calc(var(--space-unit) * 0.5) calc(var(--space-unit) * 0.8);
  font-size: 0.78rem;
  color: var(--color-text-muted);
}

.empty,
.list-error {
  margin: calc(var(--space-unit) * 1.5) calc(var(--space-unit) * 1.0);
  font-size: 0.857rem;
  color: var(--color-text-muted);
}

.list-error {
  color: var(--color-danger);
}
</style>
