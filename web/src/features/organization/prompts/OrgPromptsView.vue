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
//
// Which editor is open lives in the URL (`?action=new` / `?edit=<id>`), exactly
// like the other three organization pages, so a pasted link opens the same thing
// a click does.
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { EditPen, Plus } from '@element-plus/icons-vue'
import { usePromptsStore } from '../../../stores/prompts'
import type { Prompt, PromptType } from '../../../api/prompts'
import { PromptManageDialog } from '../../prompts'
import ListToolbar from '../../../shared/components/ListToolbar.vue'
import ListEmptyState from '../../../shared/components/ListEmptyState.vue'

type PromptFilter = 'all' | 'role' | 'flow'

const route = useRoute()
const router = useRouter()
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
  syncRouteState()
})

watch(
  () => [route.query, promptsStore.prompts],
  () => {
    syncRouteState()
  },
  { deep: true },
)

// The URL is the source of truth for which editor is open.
function syncRouteState() {
  if (route.query.action === 'new') {
    editingPrompt.value = null
    showEditDialog.value = true
  } else if (route.query.edit) {
    const target = (promptsStore.prompts || []).find((x) => x.id === Number(route.query.edit))
    if (target) {
      editingPrompt.value = target
      showEditDialog.value = true
    }
  } else if (showEditDialog.value) {
    showEditDialog.value = false
    editingPrompt.value = null
  }
}

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

// 分页固定坐在主体底部：表格只铺当前页，正文自己滚。
// 条数预设走 10 / 20 / 50：10 在一屏 34px 行高里是看得完的一页，50 是“我就是想
// 一口气扫完”时的退路。
const page = ref(1)
const pageSize = ref(10)
const pageSizes = [10, 20, 50]
const totalRows = computed(() => filteredPrompts.value.length)
const pagedPrompts = computed(() => {
  const start = (page.value - 1) * pageSize.value
  return filteredPrompts.value.slice(start, start + pageSize.value)
})

// 条件一变就回到第一页，否则用户会停在一张空页上以为被筛选滤空了。
watch([keyword, filter], () => {
  page.value = 1
})

watch([totalRows, pageSize], () => {
  const lastPage = Math.max(1, Math.ceil(totalRows.value / pageSize.value))
  if (page.value > lastPage) page.value = lastPage
})

// A new prompt defaults to the type currently being browsed: creating from the
// flow tab should not preselect a role.
function newType(): PromptType {
  return filter.value === 'flow' ? 'flow' : 'role'
}

function openCreateDialog() {
  editingPrompt.value = null
  showEditDialog.value = true
  void router.replace({ query: { action: 'new' } })
}

function openEditDialog(prompt: Prompt) {
  editingPrompt.value = prompt
  showEditDialog.value = true
  void router.replace({ query: { edit: String(prompt.id) } })
}

function onCloseDialog() {
  showEditDialog.value = false
  editingPrompt.value = null
  void router.replace({ query: {} })
}

async function removePrompt(prompt: Prompt) {
  try {
    await ElMessageBox.confirm(
      `删除后，引用「${prompt.name}」的角色、助手与协同团队将无法继续读取其内容。`,
      `删除 AI 提示词「${prompt.name}」？`,
      { type: 'warning', confirmButtonText: '删除', cancelButtonText: '取消', confirmButtonType: 'danger' },
    )
  } catch {
    // ElMessageBox rejects on cancel and on close; neither is an error.
    return
  }
  try {
    await promptsStore.deletePrompt(prompt.id)
    ElMessage.success(`已删除 AI 提示词「${prompt.name}」`)
  } catch (err: any) {
    ElMessage.error(promptsStore.error || err?.message || '删除 AI 提示词失败')
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
    </header>

    <div class="list-page-panel" data-test="prompt-panel">
      <div class="list-panel-toolbar">
        <ListToolbar
          v-model:keyword="keywordDraft"
          placeholder="搜索提示词…"
          hint="搜索提示词名称、描述或正文…"
          search-test="btn-search-prompts"
          reset-test="btn-reset-prompts"
          @search="applySearch"
          @reset="resetSearch"
        >
          <template #actions>
            <el-button
              type="primary"
              :icon="Plus"
              data-test="btn-create-prompt"
              @click="openCreateDialog"
            >
              新建 AI 提示词
            </el-button>
          </template>

          <template #filters>
            <el-select
              v-model="filter"
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

      <div class="list-panel-table">
        <el-table
          :data="pagedPrompts"
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
              <!-- 分类 chip：档位由枚举值决定（映射见 shared/styles/entity-chip.css），
                 语义色在这里不可靠——cool / emerald / 青竹 里 success 与 primary 同色。 -->
            <el-tag
              :class="['entity-chip', 'entity-chip--' + row.type]"
              size="small"
              effect="light"
              disable-transitions
            >
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
              <span class="text-platform-text-muted">
                {{ formatDate(row.updatedAt || row.createdAt) }}
              </span>
            </template>
          </el-table-column>

          <el-table-column label="操作" width="168" align="right" fixed="right">
            <template #default="{ row }">
              <div class="inline-flex items-center">
                <el-button link type="primary" @click="openEditDialog(row as Prompt)">编辑</el-button>
                <el-button link type="danger" @click="removePrompt(row as Prompt)">删除</el-button>
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
                新建 AI 提示词
              </el-button>
            </ListEmptyState>
          </template>
        </el-table>
      </div>

      <div class="list-panel-footer" data-test="prompt-pagination">
        <el-pagination
          v-model:current-page="page"
          v-model:page-size="pageSize"
          :total="totalRows"
          :page-sizes="pageSizes"
          size="small"
          background
          layout="total, sizes, prev, pager, next"
        />
      </div>
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
