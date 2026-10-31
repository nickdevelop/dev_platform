<script setup lang="ts">
// Middle column of the pure-chat area: assistant (group) management.
// As specified, column 2 exclusively displays the assistant list.
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import type { ChatGroup } from '../../../api/chats'
import { useChatsStore } from '../../../stores/chats'
import { UiDialog, UiButton } from '../../../shared/ui'
import ChatGroupDialog from './ChatGroupDialog.vue'

const chats = useChatsStore()
const router = useRouter()
const route = useRoute()

const search = ref('')
const dialog = ref<{
  type: 'create-group' | 'rename-group' | 'delete-group'
  group?: ChatGroup | null
} | null>(null)
const deleting = ref(false)
const deleteError = ref('')

const query = computed(() => search.value.trim().toLowerCase())

onMounted(async () => {
  await Promise.all([chats.loadGroups(), chats.loadChats()])
  // Default select first assistant if none selected
  if (chats.selectedGroupId === null && chats.groups.length > 0) {
    chats.selectedGroupId = chats.groups[0].id
  }
})

// When groups load, ensure a valid selection
watch(
  () => chats.groups,
  (groups) => {
    if (chats.selectedGroupId === null && groups.length > 0) {
      chats.selectedGroupId = groups[0].id
    }
  },
)

// When current active conversation changes, sync selected assistant if conversation belongs to a group
watch(
  () => chats.active?.groupId,
  (groupId) => {
    if (groupId !== undefined && groupId !== null) {
      chats.selectedGroupId = groupId
    }
  },
)

function chatsForGroup(groupId: number): number {
  return chats.chats.filter((c) => (c.groupId ?? 0) === groupId).length
}

function getAssistantIcon(group: ChatGroup): string {
  if (group.icon && group.icon.trim()) return group.icon.trim()
  const trimmed = group.name.trim()
  return trimmed ? Array.from(trimmed)[0].toUpperCase() : '助'
}

const ungroupedCount = computed(() => {
  return chats.chats.filter((c) => !c.groupId || c.groupId === 0).length
})

const filteredGroups = computed(() => {
  const q = query.value
  if (!q) return chats.groups
  return chats.groups.filter((g) => g.name.toLowerCase().includes(q))
})

const showUngrouped = computed(() => {
  if (query.value) {
    return '未分组对话'.includes(query.value) && ungroupedCount.value > 0
  }
  return ungroupedCount.value > 0
})

function selectAssistant(groupId: number | null) {
  chats.selectedGroupId = groupId
}

function openCreateGroup() {
  dialog.value = { type: 'create-group' }
}

function openRenameGroup(group: ChatGroup) {
  dialog.value = { type: 'rename-group', group }
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

  if (chats.selectedGroupId === group.id) {
    chats.selectedGroupId = chats.groups.length > 0 ? chats.groups[0].id : null
  }

  dialog.value = null
  const activeChatId = route.params.chatId ? Number(route.params.chatId) : null
  if (activeChatId) {
    if (!chats.chats.some((c) => c.id === activeChatId)) {
      void router.push({ name: 'chat' })
    }
  }
}
</script>

<template>
  <div class="chat-sidebar" data-test="chat-sidebar">
    <div class="sidebar-actions">
      <button
        type="button"
        class="action-btn new-group-btn"
        data-test="chat-new-group-btn"
        @click="openCreateGroup"
      >
        + 新建助手
      </button>
    </div>

    <input
      v-model="search"
      type="search"
      class="search"
      placeholder="搜索助手"
      aria-label="搜索助手"
    />

    <p v-if="chats.listError || chats.groupsError" class="list-error" role="alert">
      {{ chats.listError || chats.groupsError }}
    </p>

    <div class="assistant-list-container" data-test="assistant-list">
      <ul class="assistant-list" aria-label="助手列表">
        <li
          v-for="group in filteredGroups"
          :key="group.id"
          class="assistant-item"
          :class="{ active: chats.selectedGroupId === group.id }"
          :data-group-id="group.id"
          role="button"
          tabindex="0"
          @click="selectAssistant(group.id)"
          @keydown.enter.prevent="selectAssistant(group.id)"
          @keydown.space.prevent="selectAssistant(group.id)"
        >
          <div class="assistant-avatar" :title="group.name">
            {{ getAssistantIcon(group) }}
          </div>

          <div class="assistant-info">
            <div class="assistant-name-row">
              <span class="assistant-name truncate" :title="group.name">{{ group.name }}</span>
              <span class="assistant-count">{{ chatsForGroup(group.id) }}</span>
            </div>
            <p v-if="group.systemPrompt" class="assistant-desc truncate">
              {{ group.systemPrompt }}
            </p>
          </div>

          <div class="assistant-actions" @click.stop>
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
        </li>

        <!-- Ungrouped Conversations entry if any -->
        <li
          v-if="showUngrouped"
          class="assistant-item ungrouped"
          :class="{ active: chats.selectedGroupId === 0 }"
          :data-group-id="0"
          role="button"
          tabindex="0"
          @click="selectAssistant(0)"
          @keydown.enter.prevent="selectAssistant(0)"
          @keydown.space.prevent="selectAssistant(0)"
        >
          <div class="assistant-avatar ungrouped-avatar" title="未分组对话">
            #
          </div>
          <div class="assistant-info">
            <div class="assistant-name-row">
              <span class="assistant-name truncate">未分组对话</span>
              <span class="assistant-count">{{ ungroupedCount }}</span>
            </div>
          </div>
        </li>
      </ul>

      <p
        v-if="!chats.groupsLoading && filteredGroups.length === 0 && !showUngrouped"
        class="empty"
      >
        {{ query ? '没有匹配的助手。' : '还没有助手，点击「+ 新建助手」开始。' }}
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

    <UiDialog
      v-if="dialog && dialog.type === 'delete-group' && dialog.group"
      title="删除助手"
      width-class="w-full max-w-md"
      data-test="delete-group-dialog"
      @close="dialog = null"
    >
      <div class="space-y-2">
        <p class="text-sm font-medium text-slate-800 dark:text-slate-100">
          确定要删除助手「{{ dialog.group.name }}」吗？
        </p>
        <p class="text-xs text-slate-500 leading-relaxed">
          {{ chatsForGroup(dialog.group.id) > 0 ? `该助手下的 ${chatsForGroup(dialog.group.id)} 个对话将一并删除，且无法恢复。` : '删除后，该助手将从平台中移除。' }}
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
  </div>
</template>

<style scoped>
.chat-sidebar {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 0;
  height: 100%;
  padding: 0 10px 12px;
  background: var(--color-surface, #ffffff);
}

.sidebar-actions {
  display: flex;
  gap: 6px;
  margin-bottom: 8px;
}

.action-btn {
  flex: 1;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 4px;
  padding: 6px 12px;
  font-size: 0.8125rem;
  font-weight: 500;
  color: var(--color-text, #1e293b);
  background: var(--color-bg, #f8fafc);
  border: 1px solid var(--color-border, #e2e8f0);
  border-radius: 6px;
  cursor: pointer;
  transition: all 0.15s ease;
}

.action-btn:hover {
  background: var(--color-surface-muted, #f1f5f9);
  border-color: var(--color-border-strong, #cbd5e1);
}

.search {
  width: 100%;
  padding: 6px 10px;
  font-size: 0.8125rem;
  background: var(--color-bg, #f8fafc);
  border: 1px solid var(--color-border, #e2e8f0);
  border-radius: 6px;
  color: var(--color-text, #1e293b);
  outline: none;
  margin-bottom: 8px;
  transition: border-color 0.15s ease;
}

.search:focus {
  border-color: var(--color-primary, #3b82f6);
}

.assistant-list-container {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
}

.assistant-list {
  list-style: none;
  padding: 0;
  margin: 0;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.assistant-item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 10px;
  border-radius: 8px;
  cursor: pointer;
  border: 1px solid transparent;
  background: transparent;
  user-select: none;
  transition: background-color 0.15s ease, border-color 0.15s ease;
}

.assistant-item:hover {
  background: var(--color-surface-muted, #f1f5f9);
}

.assistant-item.active {
  background: var(--color-surface-muted, #f1f5f9);
  border-color: var(--color-border, #e2e8f0);
}

:root[data-theme='dark'] .assistant-item.active {
  background: rgba(255, 255, 255, 0.06);
  border-color: rgba(255, 255, 255, 0.12);
}

.assistant-avatar {
  width: 32px;
  height: 32px;
  border-radius: 6px;
  background: var(--color-bg, #f1f5f9);
  color: var(--color-text, #334155);
  font-size: 0.875rem;
  font-weight: 600;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  border: 1px solid var(--color-border, #e2e8f0);
}

.ungrouped-avatar {
  color: var(--color-text-muted, #64748b);
  background: transparent;
}

.assistant-info {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.assistant-name-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 6px;
}

.assistant-name {
  font-size: 0.8125rem;
  font-weight: 500;
  color: var(--color-text, #1e293b);
}

.assistant-count {
  font-size: 0.6875rem;
  color: var(--color-text-muted, #64748b);
  background: var(--color-bg, #f1f5f9);
  padding: 1px 5px;
  border-radius: 10px;
  flex-shrink: 0;
}

.assistant-desc {
  font-size: 0.6875rem;
  color: var(--color-text-muted, #64748b);
  margin: 0;
  line-height: 1.2;
}

.assistant-actions {
  display: none;
  align-items: center;
  gap: 2px;
  flex-shrink: 0;
}

.assistant-item:hover .assistant-actions,
.assistant-item:focus-within .assistant-actions {
  display: flex;
}

.icon-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 20px;
  height: 20px;
  padding: 0;
  border: none;
  background: transparent;
  color: var(--color-text-muted, #64748b);
  border-radius: 4px;
  cursor: pointer;
}

.icon-btn:hover {
  background: var(--color-border, #e2e8f0);
  color: var(--color-text, #1e293b);
}

.icon-btn.danger:hover {
  background: #fee2e2;
  color: #ef4444;
}

.empty {
  padding: 24px 8px;
  text-align: center;
  font-size: 0.8125rem;
  color: var(--color-text-muted, #64748b);
  margin: 0;
}

.list-error {
  padding: 6px 8px;
  font-size: 0.75rem;
  color: #ef4444;
  margin: 0 0 8px;
}
</style>
