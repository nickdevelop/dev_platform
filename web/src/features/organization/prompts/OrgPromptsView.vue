<script setup lang="ts">
// OrgPromptsView: AI prompt governance in the organization center.
// Role and workflow templates referenced by assistants, employees and teams.
//
// Element Plus carries the interaction surface: the list is an `el-table` (the
// same part the other three organization pages use, so a prompt row behaves like
// an assistant row), the row above it is the shared `ListToolbar` (create on the
// left, 类型 `el-select` + keyword `el-input` + 搜索 / 重置 on the right), and
// destructive work goes through `ElMessageBox` with the result reported by
// `ElMessage`.
//
// The list deliberately does not preview the prompt body: a body is a thousand
// characters of markdown, and one clipped column of it tells you nothing. The
// body belongs to the editor (编辑) and to 复制, both one click away. The search
// field still matches against it.
import { computed, onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { EditPen, Plus } from '@element-plus/icons-vue'
import { usePromptsStore } from '../../../stores/prompts'
import type { Prompt, PromptType } from '../../../api/prompts'
import { PromptManageDialog } from '../../prompts'
import ListToolbar from '../../../shared/components/ListToolbar.vue'
import ListEmptyState from '../../../shared/components/ListEmptyState.vue'

type PromptFilter = 'all' | 'role' | 'flow'

const promptsStore = usePromptsStore()

// The keyword in the field is a draft; the table only follows it on 搜索 / Enter
// (ListToolbar), while the type filter applies immediately.
const keywordDraft = ref('')
const keyword = ref('')
const filter = ref<PromptFilter>('all')
const showEditDialog = ref(false)
const editingPrompt = ref<Prompt | null>(null)

onMounted(async () => {
  if (promptsStore.prompts.length === 0) {
    await promptsStore.loadPrompts()
  }
})

const roleCount = computed(() => (promptsStore.prompts || []).filter((p) => p.type === 'role').length)
const flowCount = computed(() => (promptsStore.prompts || []).filter((p) => p.type === 'flow').length)

// The counts ride along in the segment labels, so the filter always states what
// clicking it will show.
const filterOptions = computed(() => [
  { label: `全部 (${promptsStore.prompts.length})`, value: 'all' as PromptFilter },
  { label: `角色类 (${roleCount.value})`, value: 'role' as PromptFilter },
  { label: `流程类 (${flowCount.value})`, value: 'flow' as PromptFilter },
])

function applySearch() {
  keyword.value = keywordDraft.value.trim()
}

function resetSearch() {
  keywordDraft.value = ''
  keyword.value = ''
  filter.value = 'all'
}

const filteredPrompts = computed(() => {
  let list = promptsStore.prompts || []
  if (filter.value !== 'all') {
    list = list.filter((p) => p.type === filter.value)
  }
  const kw = keyword.value.trim().toLowerCase()
  if (!kw) return list
  return list.filter(
    (p) =>
      p.name.toLowerCase().includes(kw) ||
      (p.description && p.description.toLowerCase().includes(kw)) ||
      p.content.toLowerCase().includes(kw),
  )
})

// A new prompt defaults to the type currently being browsed: creating from the
// flow tab should not preselect a role.
function newType(): PromptType {
  return filter.value === 'flow' ? 'flow' : 'role'
}

function openCreateDialog() {
  editingPrompt.value = null
  showEditDialog.value = true
}

function openEditDialog(prompt: Prompt) {
  editingPrompt.value = prompt
  showEditDialog.value = true
}

function onCloseDialog() {
  showEditDialog.value = false
}

async function removePrompt(prompt: Prompt) {
  try {
    await ElMessageBox.confirm(
      `删除后，引用「${prompt.name}」的角色、助手与协同团队将无法继续读取其内容。`,
      `删除提示词「${prompt.name}」？`,
      { type: 'warning', confirmButtonText: '删除', cancelButtonText: '取消', confirmButtonType: 'danger' },
    )
  } catch {
    // ElMessageBox rejects on cancel and on close; neither is an error.
    return
  }
  try {
    await promptsStore.deletePrompt(prompt.id)
    ElMessage.success(`已删除提示词「${prompt.name}」`)
  } catch (err: any) {
    ElMessage.error(promptsStore.error || err?.message || '删除提示词失败')
  }
}

async function copyPromptContent(prompt: Prompt) {
  try {
    await navigator.clipboard.writeText(prompt.content)
    ElMessage.success(`已复制「${prompt.name}」的正文`)
  } catch {
    ElMessage.error('当前浏览器不允许访问剪贴板')
  }
}

function formatDate(iso: string): string {
  if (!iso) return ''
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) return iso
  // Written out rather than left to `toLocaleString('zh-CN')`: that locale renders
  // the date with slashes and the time with a comma (2026/11/03 10:29), which in
  // a right-aligned mono column reads as two different formats glued together.
  const pad = (n: number) => String(n).padStart(2, '0')
  return (
    `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ` +
    `${pad(date.getHours())}:${pad(date.getMinutes())}`
  )
}

// Three different silences, told apart: still loading, searched to nothing, and a
// library with nothing in it. An enum filter alone is not「搜不到」——它是「这一类
// 还没有」，退路是新建（弹窗会带着该类型打开），所以只有关键词才进 filtered。
// See ListEmptyState.
const emptyState = computed<'loading' | 'filtered' | 'empty'>(() => {
  if (promptsStore.loading) return 'loading'
  return keyword.value ? 'filtered' : 'empty'
})

const emptyTitle = computed(() =>
  emptyState.value === 'filtered'
    ? '没有匹配的提示词'
    : filter.value === 'all'
      ? '尚未创建任何提示词'
      : filter.value === 'role'
        ? '暂无角色类提示词'
        : '暂无流程类提示词',
)

const emptyHint = computed(() => {
  if (emptyState.value === 'filtered') {
    return filter.value === 'all'
      ? `关键词「${keyword.value}」没有命中任何提示词的名称、描述或正文。`
      : `在${filter.value === 'role' ? '角色类' : '流程类'}里，关键词「${keyword.value}」没有命中任何提示词。`
  }
  if (filter.value === 'flow') {
    return '流程类提示词偏向工作流程与协同任务规范，可在多 Agent 协同团队中统一引用。'
  }
  if (filter.value === 'role') {
    return '角色类提示词用于设定 Agent 人设、专业背景与思考准则，可在助手与数字员工中统一引用。'
  }
  return '角色类用于设定 Agent 人设与思考准则，流程类用于工作流程与协同任务规范，均可在助手、数字员工与团队中统一引用。'
})
</script>

<template>
  <div
    class="org-prompts-view h-full flex flex-col min-h-0 bg-[var(--color-bg)] overflow-hidden"
    data-test="org-prompts-view"
    aria-label="AI 提示词"
  >
    <header class="list-page-header">
      <h1 class="list-page-title">AI 提示词</h1>
      <p class="list-page-desc">
        管理角色类人设与流程类协同提示词，供助手、数字员工与协同团队统一引用。
      </p>

      <div class="list-page-toolbar">
        <ListToolbar
          v-model:keyword="keywordDraft"
          placeholder="搜索提示词名称、描述或正文…"
          search-test="btn-search-prompts"
          reset-test="btn-reset-prompts"
          @search="applySearch"
          @reset="resetSearch"
        >
          <template #actions>
            <el-button
              type="primary"
              size="small"
              :icon="Plus"
              data-test="btn-create-prompt"
              @click="openCreateDialog"
            >
              新建提示词
            </el-button>
          </template>

          <template #filters>
            <el-select
              v-model="filter"
              size="small"
              class="list-page-filter"
              data-test="select-prompt-type"
              aria-label="按提示词类型筛选"
            >
              <el-option
                v-for="option in filterOptions"
                :key="option.value"
                :label="option.label"
                :value="option.value"
              />
            </el-select>
          </template>
        </ListToolbar>
      </div>
    </header>

    <div class="list-page-body">
      <el-table
        :data="filteredPrompts"
        row-key="id"
        height="100%"
        data-test="prompt-table"
      >
        <el-table-column label="提示词名称" min-width="180">
          <template #default="{ row }">
            <span class="font-medium text-foreground" :data-test="`prompt-name-${row.id}`">
              {{ row.name }}
            </span>
          </template>
        </el-table-column>

        <el-table-column label="类型" width="92">
          <template #default="{ row }">
            <el-tag :type="row.type === 'role' ? 'primary' : 'success'" size="small" effect="light" disable-transitions>
              {{ row.type === 'role' ? '角色类' : '流程类' }}
            </el-tag>
          </template>
        </el-table-column>

        <el-table-column label="描述" min-width="320" show-overflow-tooltip>
          <template #default="{ row }">
            <span class="text-platform-text-muted">{{ row.description || '—' }}</span>
          </template>
        </el-table-column>

        <el-table-column label="更新时间" width="150" align="right">
          <template #default="{ row }">
            <span class="text-2xs font-mono text-platform-text-muted">
              {{ formatDate(row.updatedAt || row.createdAt) }}
            </span>
          </template>
        </el-table-column>

        <el-table-column label="操作" width="168" align="right" fixed="right">
          <template #default="{ row }">
            <div class="inline-flex items-center gap-1">
              <el-button link type="primary" size="small" @click="copyPromptContent(row as Prompt)">复制</el-button>
              <el-button link type="primary" size="small" @click="openEditDialog(row as Prompt)">编辑</el-button>
              <el-button link type="danger" size="small" @click="removePrompt(row as Prompt)">删除</el-button>
            </div>
          </template>
        </el-table-column>

        <template #empty>
          <ListEmptyState
            :state="emptyState"
            :icon="EditPen"
            :title="emptyTitle"
            :hint="emptyHint"
            @reset="resetSearch"
          >
            <el-button
              type="primary"
              :icon="Plus"
              data-test="btn-create-prompt-empty"
              @click="openCreateDialog"
            >
              新建提示词
            </el-button>
          </ListEmptyState>
        </template>
      </el-table>
    </div>

    <PromptManageDialog
      v-if="showEditDialog"
      :prompt="editingPrompt"
      :default-type="newType()"
      @close="onCloseDialog"
      @saved="onCloseDialog"
    />
  </div>
</template>
