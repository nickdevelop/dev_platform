<script setup lang="ts">
// AssistantListView: Pure object governance for AI Assistants.
// Displays and manages Assistant configurations (name, icon, model, prompt, floating launcher toggle, tools).
// Follows design-taste-frontend standards:
//   - Zero dialogs/popups for creation & editing (uses dedicated AssistantEditPanel workbench)
//   - Clean header with count badge, clear description, and primary action
//   - Compact search and segmented filter bar
//   - Flat table with subtle hover states, zero resting shadows, and direct toggles
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useChatsStore } from '../../../stores/chats'
import { useModelsStore } from '../../../stores/models'
import { usePromptsStore } from '../../../stores/prompts'
import type { ChatGroup, ChatGroupInput } from '../../../api/chats'
import { UiButton, UiAlert, UiConfirmDialog } from '../../../shared/ui'
import AssistantEditPanel from './AssistantEditPanel.vue'

const route = useRoute()
const router = useRouter()
const chatsStore = useChatsStore()
const modelsStore = useModelsStore()
const promptsStore = usePromptsStore()

const keyword = ref('')
const floatFilter = ref<'all' | 'floating' | 'normal'>('all')

// Mode: 'list' | 'create' | 'edit'
const mode = ref<'list' | 'create' | 'edit'>('list')
const selectedAssistant = ref<ChatGroup | null>(null)

const pendingDelete = ref<ChatGroup | null>(null)
const actionError = ref('')
const togglingFloatId = ref<number | null>(null)

onMounted(async () => {
  await Promise.all([
    chatsStore.loadGroups(),
    modelsStore.load(),
    promptsStore.loadPrompts(),
  ])
  syncRouteState()
})

watch(
  () => [route.query, chatsStore.groups],
  () => {
    syncRouteState()
  },
  { deep: true },
)

function syncRouteState() {
  if (route.query.action === 'new') {
    mode.value = 'create'
    selectedAssistant.value = null
  } else if (route.query.edit) {
    const targetId = Number(route.query.edit)
    const found = chatsStore.groups.find((g) => g.id === targetId)
    if (found) {
      mode.value = 'edit'
      selectedAssistant.value = found
    }
  } else if (mode.value !== 'create' && mode.value !== 'edit') {
    mode.value = 'list'
    selectedAssistant.value = null
  }
}

const filteredAssistants = computed(() => {
  let list = chatsStore.groups
  const q = keyword.value.trim().toLowerCase()
  if (q) {
    list = list.filter(
      (g) =>
        g.name.toLowerCase().includes(q) ||
        (g.systemPrompt && g.systemPrompt.toLowerCase().includes(q)),
    )
  }
  if (floatFilter.value === 'floating') {
    list = list.filter((g) => g.showInFloat === true)
  } else if (floatFilter.value === 'normal') {
    list = list.filter((g) => !g.showInFloat)
  }
  return list
})

function getAssistantIcon(group: ChatGroup): string {
  if (group.icon && group.icon.trim()) return group.icon.trim()
  const trimmed = group.name.trim()
  return trimmed ? Array.from(trimmed)[0].toUpperCase() : '助'
}

function getModelDisplayName(modelId?: number | null): string {
  if (!modelId) return '跟随默认'
  for (const svc of modelsStore.services || []) {
    for (const m of svc.models || []) {
      if (m.id === modelId) {
        return m.name || m.modelId
      }
    }
  }
  return `模型 #${modelId}`
}

function getPromptDisplayName(promptId?: number | null): string | null {
  if (!promptId) return null
  const p = promptsStore.byId(promptId)
  return p ? p.name : `提示词 #${promptId}`
}

function openCreate() {
  selectedAssistant.value = null
  mode.value = 'create'
  void router.replace({ query: { action: 'new' } })
}

function openEdit(group: ChatGroup) {
  selectedAssistant.value = group
  mode.value = 'edit'
  void router.replace({ query: { edit: String(group.id) } })
}

function onSaved(_saved: ChatGroup) {
  mode.value = 'list'
  selectedAssistant.value = null
  void router.replace({ query: {} })
}

function onCancel() {
  mode.value = 'list'
  selectedAssistant.value = null
  void router.replace({ query: {} })
}

async function toggleFloat(group: ChatGroup) {
  if (togglingFloatId.value !== null) return
  togglingFloatId.value = group.id
  actionError.value = ''
  try {
    const input: ChatGroupInput = {
      name: group.name,
      icon: group.icon,
      defaultModelId: group.defaultModelId,
      promptId: group.promptId,
      systemPrompt: group.systemPrompt,
      enablePlatformTools: group.enablePlatformTools,
      maxToolSteps: group.maxToolSteps,
      showInFloat: !group.showInFloat,
    }
    await chatsStore.updateGroup(group.id, input)
  } catch (err: any) {
    actionError.value = err?.message || '更新浮窗状态失败'
  } finally {
    togglingFloatId.value = null
  }
}

async function confirmDelete() {
  if (!pendingDelete.value) return
  actionError.value = ''
  try {
    await chatsStore.deleteGroup(pendingDelete.value.id)
    pendingDelete.value = null
  } catch (err: any) {
    actionError.value = err?.message || '删除助手失败'
  }
}
</script>

<template>
  <div class="h-full flex flex-col min-h-0 bg-[var(--color-bg)] overflow-hidden">
    <!-- View Switch: In-Place Edit/Create Panel (Zero Popups) -->
    <template v-if="mode === 'create' || mode === 'edit'">
      <AssistantEditPanel
        :assistant="selectedAssistant"
        @saved="onSaved"
        @cancel="onCancel"
      />
    </template>

    <!-- Standard List View -->
    <template v-else>
      <!-- Top Header: Title + Count + Zone A Action -->
      <header class="px-6 py-4 border-b border-border bg-surface shrink-0">
        <div class="flex items-center justify-between gap-4">
          <div>
            <div class="flex items-center gap-2.5">
              <h1 class="text-lg font-semibold text-foreground tracking-tight">AI 助手</h1>
              <span class="px-2 py-0.5 rounded-full text-2xs font-mono font-medium bg-surface-muted text-platform-text-muted border border-border">
                {{ chatsStore.groups.length }} 个助手
              </span>
            </div>
            <p class="text-xs text-platform-text-muted mt-0.5">
              日常大模型人设与平台自治工具治理。会话通过快捷助手随时呼出，无需在此直接发起。
            </p>
          </div>

          <UiButton
            variant="primary"
            class="shrink-0 text-xs"
            data-test="btn-create-assistant"
            @click="openCreate"
          >
            <svg viewBox="0 0 16 16" width="12" height="12" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" class="mr-1.5" aria-hidden="true">
              <line x1="8" y1="3" x2="8" y2="13" />
              <line x1="3" y1="8" x2="13" y2="8" />
            </svg>
            <span>新建 AI 助手</span>
          </UiButton>
        </div>

        <!-- Filter & Search Toolbar -->
        <div class="flex flex-col sm:flex-row items-stretch sm:items-center justify-between gap-3 mt-4 pt-3 border-t border-border/50">
          <!-- Search Input -->
          <div class="relative w-full sm:w-72">
            <svg class="absolute left-2.5 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-platform-text-muted pointer-events-none" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.8">
              <circle cx="7" cy="7" r="4.5" />
              <line x1="10.5" y1="10.5" x2="14" y2="14" stroke-linecap="round" />
            </svg>
            <input
              v-model="keyword"
              type="search"
              placeholder="搜索助手名称或人设关键词…"
              class="w-full pl-8 pr-7 py-1.5 text-xs rounded border border-border bg-surface-muted/60 text-foreground placeholder:text-platform-text-muted focus:outline-none focus:border-accent"
            />
            <button
              v-if="keyword"
              type="button"
              class="absolute right-2 top-1/2 -translate-y-1/2 text-platform-text-muted hover:text-foreground"
              title="清空搜索"
              @click="keyword = ''"
            >
              <svg viewBox="0 0 16 16" width="10" height="10" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
                <line x1="4" y1="4" x2="12" y2="12" />
                <line x1="12" y1="4" x2="4" y2="12" />
              </svg>
            </button>
          </div>

          <!-- Float Filter Segment -->
          <div class="flex items-center p-0.5 rounded bg-surface-muted border border-border text-2xs font-medium text-platform-text-muted self-start sm:self-auto">
            <button
              type="button"
              class="px-2.5 py-1 rounded transition-colors"
              :class="floatFilter === 'all' ? 'bg-surface text-foreground shadow-2xs font-semibold' : 'hover:text-foreground'"
              @click="floatFilter = 'all'"
            >
              全部 ({{ chatsStore.groups.length }})
            </button>
            <button
              type="button"
              class="px-2.5 py-1 rounded transition-colors"
              :class="floatFilter === 'floating' ? 'bg-surface text-foreground shadow-2xs font-semibold' : 'hover:text-foreground'"
              @click="floatFilter = 'floating'"
            >
              快捷助手常驻 ({{ chatsStore.floatingAssistants.length }})
            </button>
            <button
              type="button"
              class="px-2.5 py-1 rounded transition-colors"
              :class="floatFilter === 'normal' ? 'bg-surface text-foreground shadow-2xs font-semibold' : 'hover:text-foreground'"
              @click="floatFilter = 'normal'"
            >
              未常驻
            </button>
          </div>
        </div>
      </header>

      <!-- Error Banner -->
      <div v-if="actionError" class="px-6 pt-4">
        <UiAlert variant="error">
          {{ actionError }}
        </UiAlert>
      </div>

      <!-- Main Content Table View -->
      <div class="flex-1 min-h-0 overflow-auto p-6">
        <div v-if="filteredAssistants.length > 0" class="border border-border rounded-lg bg-surface overflow-hidden">
          <table class="w-full border-collapse text-left text-xs">
            <thead>
              <tr class="border-b border-border bg-surface-muted/60 text-2xs font-medium text-platform-text-muted uppercase tracking-wider">
                <th class="py-2.5 px-4">AI 助手</th>
                <th class="py-2.5 px-4">运行模型</th>
                <th class="py-2.5 px-4">提示词规范</th>
                <th class="py-2.5 px-4">快捷助手浮窗</th>
                <th class="py-2.5 px-4">平台自治能力</th>
                <th class="py-2.5 px-4 text-right">操作</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-border/60">
              <tr
                v-for="item in filteredAssistants"
                :key="item.id"
                class="hover:bg-surface-muted/30 transition-colors group"
              >
                <!-- Name & Icon -->
                <td class="py-3 px-4">
                  <div class="flex items-center gap-3">
                    <span class="w-8 h-8 rounded-md flex items-center justify-center text-sm bg-surface-muted border border-border shrink-0 font-medium">
                      {{ getAssistantIcon(item) }}
                    </span>
                    <div class="min-w-0 max-w-xs sm:max-w-sm">
                      <div class="flex items-center gap-1.5">
                        <span class="font-medium text-foreground truncate">{{ item.name }}</span>
                        <span class="text-3xs font-mono text-platform-text-muted">#{{ item.id }}</span>
                      </div>
                      <p class="text-2xs text-platform-text-muted truncate mt-0.5">
                        {{ item.systemPrompt || '暂无人设描述' }}
                      </p>
                    </div>
                  </div>
                </td>

                <!-- Model -->
                <td class="py-3 px-4">
                  <span class="inline-flex items-center px-2 py-0.5 rounded text-2xs font-mono bg-surface-muted text-foreground border border-border">
                    {{ getModelDisplayName(item.defaultModelId) }}
                  </span>
                </td>

                <!-- Prompt library -->
                <td class="py-3 px-4">
                  <span v-if="getPromptDisplayName(item.promptId)" class="inline-flex items-center gap-1 text-2xs text-foreground">
                    <svg viewBox="0 0 16 16" width="11" height="11" fill="none" stroke="currentColor" stroke-width="1.8" class="text-accent shrink-0">
                      <path d="M12 20h9" />
                      <path d="M16.5 3.5a2.121 2.121 0 0 1 3 3L7 19l-4 1 1-4L16.5 3.5z" />
                    </svg>
                    <span class="truncate max-w-[120px]">{{ getPromptDisplayName(item.promptId) }}</span>
                  </span>
                  <span v-else class="text-2xs text-platform-text-muted">
                    {{ item.systemPrompt ? '内联人设' : '通用' }}
                  </span>
                </td>

                <!-- Float Toggle -->
                <td class="py-3 px-4">
                  <button
                    type="button"
                    class="inline-flex items-center gap-1.5 px-2 py-1 rounded text-2xs font-medium transition-colors border"
                    :class="item.showInFloat
                      ? 'bg-accent/10 border-accent/30 text-accent hover:bg-accent/20'
                      : 'bg-surface-muted border-border text-platform-text-muted hover:text-foreground'"
                    :title="item.showInFloat ? '点击取消桌面快捷助手常驻' : '点击常驻到桌面快捷助手'"
                    :disabled="togglingFloatId === item.id"
                    @click="toggleFloat(item)"
                  >
                    <span
                      class="w-1.5 h-1.5 rounded-full"
                      :class="item.showInFloat ? 'bg-accent' : 'bg-platform-text-muted/60'"
                    ></span>
                    <span>{{ item.showInFloat ? '已常驻浮窗' : '未常驻' }}</span>
                  </button>
                </td>

                <!-- Platform tools -->
                <td class="py-3 px-4">
                  <div v-if="item.enablePlatformTools" class="inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-2xs font-mono bg-emerald-500/10 text-emerald-600 border border-emerald-500/20">
                    <svg viewBox="0 0 16 16" width="10" height="10" fill="none" stroke="currentColor" stroke-width="1.8">
                      <polyline points="3 8 6 11 13 4" />
                    </svg>
                    <span>已自治 ({{ item.maxToolSteps || 10 }}步)</span>
                  </div>
                  <span v-else class="text-2xs text-platform-text-muted">基础对话</span>
                </td>

                <!-- Actions -->
                <td class="py-3 px-4 text-right">
                  <div class="inline-flex items-center gap-1.5">
                    <button
                      type="button"
                      class="px-2 py-1 rounded text-2xs font-medium text-foreground hover:bg-surface-muted transition-colors border border-transparent hover:border-border"
                      title="编辑配置"
                      @click="openEdit(item)"
                    >
                      编辑
                    </button>
                    <button
                      type="button"
                      class="px-2 py-1 rounded text-2xs font-medium text-red-500 hover:bg-red-500/10 transition-colors border border-transparent hover:border-red-500/20"
                      title="删除助手"
                      @click="pendingDelete = item"
                    >
                      删除
                    </button>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>

        <!-- Empty State -->
        <div v-else class="flex flex-col items-center justify-center p-12 text-center rounded-lg border border-dashed border-border bg-surface/50">
          <div class="w-10 h-10 rounded-full bg-surface-muted border border-border flex items-center justify-center text-platform-text-muted mb-3">
            <svg viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="1.8">
              <path d="M12 2a2 2 0 0 1 2 2v2a2 2 0 0 1-2 2 2 2 0 0 1-2-2V4a2 2 0 0 1 2-2z" />
              <rect x="4" y="8" width="16" height="12" rx="2" />
              <circle cx="9" cy="13" r="1.25" fill="currentColor" />
              <circle cx="15" cy="13" r="1.25" fill="currentColor" />
              <path d="M9 17h6" />
            </svg>
          </div>
          <p class="text-sm font-medium text-foreground">未找到匹配的 AI 助手</p>
          <p class="text-xs text-platform-text-muted mt-1 max-w-sm">
            {{ keyword ? '没有符合搜索条件的助手，请尝试其他关键词。' : '当前暂无 AI 助手，点击上方按钮创建您的第一个助手。' }}
          </p>
          <UiButton
            v-if="!keyword"
            variant="primary"
            class="text-xs mt-4"
            @click="openCreate"
          >
            + 新建 AI 助手
          </UiButton>
        </div>
      </div>
    </template>

    <!-- Delete Confirmation -->
    <UiConfirmDialog
      v-if="pendingDelete"
      title="删除 AI 助手"
      :message="`确定要删除 AI 助手「${pendingDelete.name}」吗？此操作不可逆。`"
      confirm-text="删除"
      variant="danger"
      @confirm="confirmDelete"
      @cancel="pendingDelete = null"
    />
  </div>
</template>
