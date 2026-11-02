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

function startAssistantChat(groupId: number) {
  chats.selectedGroupId = groupId
  chats.startNewChat(groupId === 0 ? undefined : groupId)
  if (route.name !== 'chat') {
    void router.push({ name: 'chat' })
  }
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
  <div class="chat-sidebar sidebar-panel" data-test="chat-sidebar">
    <!-- Zone A: Dedicated actions row -->
    <div class="sidebar-actions">
      <button
        type="button"
        class="action-btn new-group-btn"
        data-test="chat-new-group-btn"
        title="新建独立对话助手"
        @click="openCreateGroup"
      >
        <svg viewBox="0 0 16 16" width="12" height="12" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
          <line x1="8" y1="3" x2="8" y2="13" />
          <line x1="3" y1="8" x2="13" y2="8" />
        </svg>
        <span>新建助手</span>
      </button>
    </div>

    <!-- Zone B: Search input with inset magnifier -->
    <div class="sidebar-search-bar">
      <svg class="sidebar-search-icon" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true">
        <circle cx="7" cy="7" r="4.5" />
        <line x1="10.5" y1="10.5" x2="14" y2="14" stroke-linecap="round" />
      </svg>
      <input
        v-model="search"
        type="search"
        class="search sidebar-search-input"
        placeholder="搜索助手"
        aria-label="搜索助手"
      />
      <button
        v-if="search"
        type="button"
        class="sidebar-search-clear"
        title="清空搜索"
        aria-label="清空搜索"
        @click="search = ''"
      >
        <svg viewBox="0 0 16 16" width="10" height="10" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
          <line x1="4" y1="4" x2="12" y2="12" />
          <line x1="12" y1="4" x2="4" y2="12" />
        </svg>
      </button>
    </div>

    <p v-if="chats.listError || chats.groupsError" class="list-error" role="alert">
      {{ chats.listError || chats.groupsError }}
    </p>

    <!-- Zone C: Assistant List Items -->
    <div class="assistant-list-container sidebar-list-container" data-test="assistant-list">
      <ul class="assistant-list" aria-label="助手列表">
        <li
          v-for="group in filteredGroups"
          :key="group.id"
          class="assistant-item sidebar-card group"
          :class="{ active: chats.selectedGroupId === group.id }"
          :data-group-id="group.id"
          role="button"
          tabindex="0"
          @click="selectAssistant(group.id)"
          @keydown.enter.prevent="selectAssistant(group.id)"
          @keydown.space.prevent="selectAssistant(group.id)"
        >
          <!-- Row 1: Avatar + Name + Count -->
          <div class="sidebar-card-row-top">
            <div class="flex items-center gap-1.5 min-w-0 flex-1">
              <div class="assistant-avatar" :title="group.name">
                {{ getAssistantIcon(group) }}
              </div>
              <span class="assistant-name sidebar-card-title truncate" :title="group.name">{{ group.name }}</span>
              <button
                type="button"
                class="assistant-new-chat-btn"
                :aria-label="`在「${group.name}」下新建对话`"
                :title="`在「${group.name}」下新建对话`"
                data-test="assistant-new-chat-btn"
                @click.stop="startAssistantChat(group.id)"
              >
                <svg viewBox="0 0 16 16" width="10" height="10" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
                  <line x1="8" y1="3" x2="8" y2="13" />
                  <line x1="3" y1="8" x2="13" y2="8" />
                </svg>
              </button>
            </div>
            <span class="assistant-count sidebar-card-badge">{{ chatsForGroup(group.id) }}</span>
          </div>

          <!-- Row 2: Prompt description + Hover Actions -->
          <div class="sidebar-card-row-bottom">
            <span class="assistant-desc sidebar-card-meta truncate" :title="group.systemPrompt || '暂无系统提示词'">
              {{ group.systemPrompt || '无系统提示词' }}
            </span>
            <div class="assistant-actions sidebar-card-actions" @click.stop>
              <button
                type="button"
                class="icon-btn sidebar-icon-btn"
                :aria-label="`编辑助手「${group.name}」`"
                title="编辑助手"
                @click.stop="openRenameGroup(group)"
              >
                <svg viewBox="0 0 16 16" width="11" height="11" fill="none" stroke="currentColor" stroke-width="1.8">
                  <path d="M11 2.5l2.5 2.5L5 13.5H2.5V11L11 2.5z" />
                </svg>
              </button>
              <button
                type="button"
                class="icon-btn danger sidebar-icon-btn"
                :aria-label="`删除助手「${group.name}」`"
                title="删除助手"
                @click.stop="onDeleteGroup(group)"
              >
                <svg viewBox="0 0 16 16" width="11" height="11" fill="none" stroke="currentColor" stroke-width="1.8">
                  <path d="M3 4h10M6 4V2.5h4V4M4.5 4v9.5h7V4" />
                </svg>
              </button>
            </div>
          </div>
        </li>

        <!-- Ungrouped Conversations entry if any -->
        <li
          v-if="showUngrouped"
          class="assistant-item ungrouped sidebar-card"
          :class="{ active: chats.selectedGroupId === 0 }"
          :data-group-id="0"
          role="button"
          tabindex="0"
          @click="selectAssistant(0)"
          @keydown.enter.prevent="selectAssistant(0)"
          @keydown.space.prevent="selectAssistant(0)"
        >
          <div class="sidebar-card-row-top">
            <div class="flex items-center gap-1.5 min-w-0 flex-1">
              <div class="assistant-avatar ungrouped-avatar" title="未分组对话">
                #
              </div>
              <span class="assistant-name sidebar-card-title truncate">未分组对话</span>
              <button
                type="button"
                class="assistant-new-chat-btn"
                aria-label="在未分组下新建对话"
                title="在未分组下新建对话"
                data-test="assistant-new-chat-btn"
                @click.stop="startAssistantChat(0)"
              >
                <svg viewBox="0 0 16 16" width="10" height="10" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
                  <line x1="8" y1="3" x2="8" y2="13" />
                  <line x1="3" y1="8" x2="13" y2="8" />
                </svg>
              </button>
            </div>
            <span class="assistant-count sidebar-card-badge">{{ ungroupedCount }}</span>
          </div>
        </li>
      </ul>

      <p
        v-if="!chats.groupsLoading && filteredGroups.length === 0 && !showUngrouped"
        class="empty sidebar-empty"
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
}

.assistant-list {
  list-style: none;
  padding: 0;
  margin: 0;
  display: flex;
  flex-direction: column;
  gap: 3px;
}

.assistant-avatar {
  width: 22px;
  height: 22px;
  border-radius: var(--radius, 4px);
  background: var(--color-surface-muted, #f1f5f9);
  color: var(--color-text, #334155);
  font-size: 11px;
  font-weight: 600;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  border: 1px solid var(--color-border);
}

.assistant-new-chat-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 18px;
  height: 18px;
  padding: 0;
  border-radius: var(--radius, 4px);
  border: 1px solid transparent;
  background: transparent;
  color: var(--color-text-muted);
  cursor: pointer;
  opacity: 0;
  flex-shrink: 0;
  transition: opacity 0.15s ease, background-color 0.15s ease, color 0.15s ease, border-color 0.15s ease;
}

.assistant-item:hover .assistant-new-chat-btn,
.assistant-item:focus-within .assistant-new-chat-btn,
.assistant-new-chat-btn:focus {
  opacity: 0.85;
}

.assistant-new-chat-btn:hover {
  opacity: 1;
  color: var(--color-primary);
  background: var(--color-surface);
  border-color: var(--color-border);
}

.ungrouped-avatar {
  color: var(--color-text-muted);
  background: transparent;
}

.assistant-count {
  font-size: 11px;
  font-variant-numeric: tabular-nums;
  color: var(--color-text-muted);
  background: var(--color-surface-muted);
  padding: 1px 5px;
  border-radius: 3px;
  flex-shrink: 0;
}

.assistant-desc {
  font-size: 11px;
  color: var(--color-text-muted);
  margin: 0;
  line-height: 1.2;
}

.list-error {
  padding: 6px 12px;
  font-size: 12px;
  color: var(--color-danger);
  margin: 0 0 6px;
}
</style>
