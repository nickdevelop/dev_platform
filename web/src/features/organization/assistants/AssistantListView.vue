<script setup lang="ts">
// AssistantListView: governance grid for the organization's AI assistants.
//
// Migration step: the hand-written `<table>` is now `el-table`, the row above it
// is the shared `ListToolbar` (create on the left, 常驻 `el-select` + keyword
// `el-input` + 搜索 / 重置 on the right), the row toggle is an `el-switch`,
// deletion goes through `ElMessageBox`, and the create/edit form opens in
// `AssistantEditDialog` instead of replacing this view in place. The
// `?action=new` / `?edit=<id>` deep links still resolve to the same editor.
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus, Search } from '@element-plus/icons-vue'
import { useChatsStore } from '../../../stores/chats'
import { useModelsStore } from '../../../stores/models'
import { usePromptsStore } from '../../../stores/prompts'
import type { ChatGroup, ChatGroupInput } from '../../../api/chats'
import { UiAlert } from '../../../shared/ui'
import ListToolbar from '../../../shared/components/ListToolbar.vue'
import AssistantEditDialog from './AssistantEditDialog.vue'

const route = useRoute()
const router = useRouter()
const chatsStore = useChatsStore()
const modelsStore = useModelsStore()
const promptsStore = usePromptsStore()

// The keyword in the field is a draft (ListToolbar applies it on 搜索 / Enter);
// the常驻 filter applies the moment it changes.
const keywordDraft = ref('')
const keyword = ref('')
const floatFilter = ref<'all' | 'floating' | 'normal'>('all')

const showDialog = ref(false)
const selectedAssistant = ref<ChatGroup | null>(null)
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

const floatFilterOptions = computed(() => [
  { label: `全部 (${chatsStore.groups.length})`, value: 'all' },
  { label: `快捷助手常驻 (${chatsStore.floatingAssistants.length})`, value: 'floating' },
  { label: '未常驻', value: 'normal' },
])

// The URL stays the source of truth for which editor is open, so a pasted link or
// a refresh lands on the same assistant.
function syncRouteState() {
  if (route.query.action === 'new') {
    selectedAssistant.value = null
    showDialog.value = true
  } else if (route.query.edit) {
    const target = chatsStore.groups.find((g) => g.id === Number(route.query.edit))
    if (target) {
      selectedAssistant.value = target
      showDialog.value = true
    }
  } else if (showDialog.value) {
    showDialog.value = false
    selectedAssistant.value = null
  }
}

function applySearch() {
  keyword.value = keywordDraft.value.trim()
}

function resetSearch() {
  keywordDraft.value = ''
  keyword.value = ''
  floatFilter.value = 'all'
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

function personaSummary(group: ChatGroup): string {
  if (group.systemPrompt) return group.systemPrompt
  const promptName = getPromptDisplayName(group.promptId)
  return promptName ? `引用提示词「${promptName}」` : '暂无人设描述'
}

function openCreate() {
  selectedAssistant.value = null
  showDialog.value = true
  void router.replace({ query: { action: 'new' } })
}

function openEdit(group: ChatGroup) {
  selectedAssistant.value = group
  showDialog.value = true
  void router.replace({ query: { edit: String(group.id) } })
}

function closeEditor() {
  showDialog.value = false
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

async function removeAssistant(group: ChatGroup) {
  try {
    await ElMessageBox.confirm(
      `删除后 AI 助手「${group.name}」将不再可用，此操作不可逆。`,
      `删除 AI 助手「${group.name}」？`,
      { type: 'warning', confirmButtonText: '删除', cancelButtonText: '取消', confirmButtonType: 'danger' },
    )
  } catch {
    return
  }
  actionError.value = ''
  try {
    await chatsStore.deleteGroup(group.id)
    ElMessage.success(`已删除助手「${group.name}」`)
  } catch (err: any) {
    actionError.value = err?.message || '删除助手失败'
    ElMessage.error(actionError.value)
  }
}
</script>

<template>
  <div class="h-full flex flex-col min-h-0 bg-[var(--color-bg)] overflow-hidden" data-test="assistant-list">
    <header class="px-6 py-4 border-b border-border bg-surface shrink-0">
      <h1 class="text-lg font-semibold text-foreground tracking-tight">AI 助手</h1>
      <p class="text-xs text-platform-text-muted mt-0.5">
        日常大模型人设与平台自治工具治理。会话通过快捷助手随时呼出，无需在此直接发起。
      </p>

      <div class="mt-3">
        <ListToolbar
          v-model:keyword="keywordDraft"
          placeholder="搜索助手名称或人设关键词…"
          search-test="btn-search-assistant"
          reset-test="btn-reset-assistant"
          @search="applySearch"
          @reset="resetSearch"
        >
          <template #actions>
            <el-button
              type="primary"
              size="small"
              :icon="Plus"
              data-test="btn-create-assistant"
              @click="openCreate"
            >
              新建 AI 助手
            </el-button>
          </template>

          <template #filters>
            <el-select
              v-model="floatFilter"
              size="small"
              class="filter-select"
              data-test="select-float-filter"
              aria-label="按快捷助手常驻状态筛选"
            >
              <el-option
                v-for="option in floatFilterOptions"
                :key="option.value"
                :label="option.label"
                :value="option.value"
              />
            </el-select>
          </template>
        </ListToolbar>
      </div>
    </header>

    <div v-if="actionError" class="px-6 pt-4">
      <UiAlert variant="error">{{ actionError }}</UiAlert>
    </div>

    <div class="flex-1 min-h-0 p-6 table-wrap">
      <el-table :data="filteredAssistants" row-key="id" height="100%" data-test="assistant-table">
        <el-table-column label="AI 助手" min-width="200" show-overflow-tooltip>
          <template #default="{ row }">
            <div class="flex items-center gap-2.5 min-w-0">
              <span class="row-avatar select-none">{{ getAssistantIcon(row as ChatGroup) }}</span>
              <span class="font-medium text-foreground truncate">{{ row.name }}</span>
              <span class="id-chip font-mono">#{{ row.id }}</span>
            </div>
          </template>
        </el-table-column>

        <el-table-column label="人设来源" min-width="180" show-overflow-tooltip>
          <template #default="{ row }">
            <span class="text-platform-text-muted">{{ personaSummary(row as ChatGroup) }}</span>
          </template>
        </el-table-column>

        <el-table-column label="运行模型" width="170" show-overflow-tooltip>
          <template #default="{ row }">
            <span class="font-mono text-2xs">{{ getModelDisplayName(row.defaultModelId) }}</span>
          </template>
        </el-table-column>

        <el-table-column label="提示词规范" min-width="150" show-overflow-tooltip>
          <template #default="{ row }">
            <span v-if="getPromptDisplayName(row.promptId)" class="text-foreground">
              {{ getPromptDisplayName(row.promptId) }}
            </span>
            <span v-else class="text-platform-text-muted">
              {{ row.systemPrompt ? '内联人设' : '通用' }}
            </span>
          </template>
        </el-table-column>

        <el-table-column label="快捷助手浮窗" width="128" align="center">
          <template #default="{ row }">
            <el-switch
              size="small"
              :model-value="!!row.showInFloat"
              :loading="togglingFloatId === row.id"
              :aria-label="row.showInFloat ? '取消桌面快捷助手常驻' : '常驻到桌面快捷助手'"
              :data-test="`switch-float-${row.id}`"
              @change="toggleFloat(row as ChatGroup)"
            />
          </template>
        </el-table-column>

        <el-table-column label="平台自治能力" width="132">
          <template #default="{ row }">
            <el-tag v-if="row.enablePlatformTools" size="small" type="success" effect="light" disable-transitions>
              已自治 ({{ row.maxToolSteps || 10 }}步)
            </el-tag>
            <span v-else class="text-platform-text-muted">基础对话</span>
          </template>
        </el-table-column>

        <el-table-column label="操作" width="124" align="right" fixed="right">
          <template #default="{ row }">
            <div class="inline-flex items-center gap-1">
              <el-button
                link
                type="primary"
                size="small"
                :data-test="`btn-edit-assistant-${row.id}`"
                @click="openEdit(row as ChatGroup)"
              >
                编辑
              </el-button>
              <el-button
                link
                type="danger"
                size="small"
                :data-test="`btn-delete-assistant-${row.id}`"
                @click="removeAssistant(row as ChatGroup)"
              >
                删除
              </el-button>
            </div>
          </template>
        </el-table-column>

        <template #empty>
          <div class="px-6 py-10 text-center">
            <p class="text-sm font-medium text-foreground">
              {{ keyword ? '未找到匹配的 AI 助手' : '当前暂无 AI 助手' }}
            </p>
            <p class="text-xs text-platform-text-muted mt-1 max-w-sm mx-auto">
              {{
                keyword
                  ? '没有符合搜索条件的助手，请尝试其他关键词。'
                  : '当前暂无 AI 助手，点击上方按钮创建您的第一个助手。'
              }}
            </p>
            <el-button
              v-if="!keyword"
              type="primary"
              class="mt-4"
              :icon="Plus"
              data-test="btn-new-assistant"
              @click="openCreate"
            >
              新建 AI 助手
            </el-button>
          </div>
        </template>
      </el-table>
    </div>

    <AssistantEditDialog
      v-if="showDialog"
      :assistant="selectedAssistant"
      @saved="closeEditor"
      @cancel="closeEditor"
    />
  </div>
</template>

<style scoped>
/* Row chrome the library has no token for: the 24px avatar chip, the id chip and
   the persona summary line. All of it uses platform tokens. */
.filter-select {
  width: 176px;
}

.table-wrap {
  display: flex;
  flex-direction: column;
}

.row-avatar {
  width: 24px;
  height: 24px;
  flex: none;
  display: flex;
  align-items: center;
  justify-content: center;
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  background: var(--color-surface-muted);
  font-size: var(--text-size-12);
  color: var(--color-text);
}

.id-chip {
  flex: none;
  padding: 0 5px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  font-size: var(--text-size-10);
  line-height: 16px;
  color: var(--color-text-muted);
}

.persona-summary {
  margin: 0;
  font-size: var(--text-size-11);
  color: var(--color-text-muted);
}
</style>
