<script setup lang="ts">
// EmployeeListView: the digital-employee roster of the organization center.
//
// `el-table` for the roster, the shared `ListToolbar` above it (create on the
// left, engine `el-select` + keyword `el-input` + 搜索 / 重置 on the right),
// `ElMessageBox` for deletion and `EmployeeEditDialog` for the form, so the route
// no longer swaps shape when an editor opens.
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus, User } from '@element-plus/icons-vue'
import { useRolesStore } from '../../../stores/roles'
import { usePromptsStore } from '../../../stores/prompts'
import { useModelsStore } from '../../../stores/models'
import { useAgentsStore } from '../../../stores/agents'
import BackendIcon from '../../../shared/components/BackendIcon.vue'
import type { AgentRole } from '../../../api/roles'
import ListToolbar from '../../../shared/components/ListToolbar.vue'
import ListEmptyState from '../../../shared/components/ListEmptyState.vue'
import EmployeeEditDialog from './EmployeeEditDialog.vue'

const route = useRoute()
const router = useRouter()
const rolesStore = useRolesStore()
const promptsStore = usePromptsStore()
const modelsStore = useModelsStore()
const agentsStore = useAgentsStore()

// The keyword in the field is a draft (ListToolbar applies it on 搜索 / Enter);
// the engine filter applies the moment it changes.
const keywordDraft = ref('')
const keyword = ref('')
const backendFilter = ref<string>('all')

const showDialog = ref(false)
const selectedEmployee = ref<AgentRole | null>(null)

onMounted(async () => {
  await Promise.all([
    rolesStore.loadRoles(),
    promptsStore.loadPrompts(),
    modelsStore.load(),
    agentsStore.loadBackends(),
  ])
  syncRouteState()
})

watch(
  () => [route.query, rolesStore.roles],
  () => {
    syncRouteState()
  },
  { deep: true },
)

// The URL is the source of truth for which editor is open.
function syncRouteState() {
  if (route.query.action === 'new') {
    selectedEmployee.value = null
    showDialog.value = true
  } else if (route.query.edit) {
    const target = rolesStore.roles.find((r) => r.id === Number(route.query.edit))
    if (target) {
      selectedEmployee.value = target
      showDialog.value = true
    }
  } else if (showDialog.value) {
    showDialog.value = false
    selectedEmployee.value = null
  }
}

const selectableBackends = computed(() => {
  const list = [{ id: 'llm', label: '通用大模型 (LLM)' }]
  for (const b of agentsStore.backends || []) {
    if (b.enabled !== false) list.push({ id: b.backend, label: b.label || b.backend })
  }
  return list
})

const backendFilterOptions = computed(() => [
  { label: `全部引擎 (${rolesStore.roles.length})`, value: 'all' },
  ...selectableBackends.value.map((b) => ({ label: b.label, value: b.id })),
])

function applySearch() {
  keyword.value = keywordDraft.value.trim()
}

function resetSearch() {
  keywordDraft.value = ''
  keyword.value = ''
  backendFilter.value = 'all'
}

const filteredEmployees = computed(() => {
  let list = rolesStore.roles || []
  const q = keyword.value.trim().toLowerCase()
  if (q) {
    list = list.filter(
      (r) =>
        r.name.toLowerCase().includes(q) ||
        (r.description && r.description.toLowerCase().includes(q)) ||
        r.model.toLowerCase().includes(q),
    )
  }
  if (backendFilter.value !== 'all') {
    list = list.filter((r) => (r.backend || 'llm') === backendFilter.value)
  }
  return list
})

// 分页固定坐在主体底部（v2 §四 三段式）：表格只铺当前页，正文自己滚。
const page = ref(1)
const pageSize = ref(10)
const pageSizes = [10, 20, 50]
const totalRows = computed(() => filteredEmployees.value.length)
const pagedEmployees = computed(() => {
  const start = (page.value - 1) * pageSize.value
  return filteredEmployees.value.slice(start, start + pageSize.value)
})

// 条件一变就回到第一页，否则用户会停在一张空页上以为被筛选滤空了。
watch([keyword, backendFilter], () => {
  page.value = 1
})

watch([totalRows, pageSize], () => {
  const lastPage = Math.max(1, Math.ceil(totalRows.value / pageSize.value))
  if (page.value > lastPage) page.value = lastPage
})

function getPromptName(promptId?: number): string {
  if (!promptId) return ''
  const p = promptsStore.byId(promptId)
  return p ? p.name : `提示词 #${promptId}`
}

function personaLabel(role: AgentRole): string {
  const name = getPromptName(role.promptId)
  if (name) return name
  return role.systemPrompt ? '专属人设' : '通用'
}

function openCreate() {
  selectedEmployee.value = null
  showDialog.value = true
  void router.replace({ query: { action: 'new' } })
}

function openEdit(role: AgentRole) {
  selectedEmployee.value = role
  showDialog.value = true
  void router.replace({ query: { edit: String(role.id) } })
}

function closeEditor() {
  showDialog.value = false
  selectedEmployee.value = null
  void router.replace({ query: {} })
}

async function removeEmployee(role: AgentRole) {
  try {
    await ElMessageBox.confirm(
      `删除后 AI 员工「${role.name}」将从花名册移除，被编制进 AI 团队的成员关系一并解除。`,
      `删除 AI 员工「${role.name}」？`,
      { type: 'warning', confirmButtonText: '删除', cancelButtonText: '取消', confirmButtonType: 'danger' },
    )
  } catch {
    return
  }
  try {
    await rolesStore.deleteRole(role.id)
    ElMessage.success(`已删除 AI 员工「${role.name}」`)
  } catch (err: any) {
    ElMessage.error(err?.message || '删除 AI 员工失败')
  }
}

// 三种沉默分别说：还在拉数据 / 搜不到 / 本来就空（见 ListEmptyState）。
// 引擎筛选单独不算「搜不到」——「这个引擎下还没有人」的退路是新建。
const emptyState = computed<'loading' | 'filtered' | 'empty'>(() => {
  if (rolesStore.loading) return 'loading'
  return keyword.value ? 'filtered' : 'empty'
})

const backendFilterLabel = computed(
  () => backendFilterOptions.value.find((o) => o.value === backendFilter.value)?.label ?? '',
)

const emptyTitle = computed(() => {
  if (emptyState.value === 'filtered') return '没有匹配的 AI 员工'
  return backendFilter.value === 'all' ? '当前暂无 AI 员工' : `暂无${backendFilterLabel.value}的 AI 员工`
})

const emptyHint = computed(() =>
  emptyState.value === 'filtered'
    ? `关键词「${keyword.value}」没有命中任何员工的姓名、岗位或模型。`
    : 'AI 员工是把人设、模型与执行引擎固定下来的专职专家，组建 AI 团队时直接从花名册勾选。',
)
</script>

<template>
  <div class="h-full flex flex-col min-h-0 bg-[var(--color-bg)] overflow-hidden" data-test="employee-list">
    <header class="list-page-header">
      <h1 class="list-page-title">AI 员工</h1>
      <p class="list-page-desc">
        专职数字员工与执行引擎分工，作为 AI 团队的专业协同专家编制。
      </p>

    </header>

    <div class="list-page-panel" data-test="employee-panel">
      <div class="list-panel-toolbar">
        <ListToolbar
          v-model:keyword="keywordDraft"
          placeholder="搜索员工…"
          hint="搜索员工姓名、岗位或模型…"
          search-test="btn-search-employee"
          reset-test="btn-reset-employee"
          @search="applySearch"
          @reset="resetSearch"
        >
          <template #actions>
            <el-button
              type="primary"
              :icon="Plus"
              data-test="btn-create-employee"
              @click="openCreate"
            >
              新建 AI 员工
            </el-button>
          </template>

          <template #filters>
            <el-select
              v-model="backendFilter"
              class="list-page-filter"
              data-test="select-backend-filter"
              aria-label="按执行引擎筛选"
            >
              <el-option
                v-for="option in backendFilterOptions"
                :key="option.value"
                :label="option.label"
                :value="option.value"
              />
            </el-select>
          </template>
        </ListToolbar>
      </div>

      <div class="list-panel-table">
        <el-table :data="pagedEmployees" row-key="id" height="100%" data-test="employee-table">
          <el-table-column label="AI 员工 / 岗位" min-width="190" show-overflow-tooltip>
            <template #default="{ row }">
              <div class="flex items-center gap-2.5 min-w-0">
                <span class="row-monogram" :style="{ background: row.color || 'var(--color-primary)' }">
                  {{ row.name.slice(0, 1) }}
                </span>
                <span class="font-medium text-foreground truncate">{{ row.name }}</span>
                <span class="id-chip font-mono">#{{ row.id }}</span>
              </div>
            </template>
          </el-table-column>

          <el-table-column label="岗位职责" min-width="200" show-overflow-tooltip>
            <template #default="{ row }">
              <span class="text-platform-text-muted">{{ row.description || '专职数字员工 / 协同专家角色' }}</span>
            </template>
          </el-table-column>

          <el-table-column label="执行引擎" width="164">
            <template #default="{ row }">
              <span class="engine-chip">
                <BackendIcon :backend="row.backend || 'llm'" class="engine-icon" />
                <span class="truncate">{{ row.backend || 'LLM' }}</span>
              </span>
            </template>
          </el-table-column>

          <el-table-column label="运行模型" width="180" show-overflow-tooltip>
            <template #default="{ row }">
              <span class="font-mono">{{ row.model }}</span>
            </template>
          </el-table-column>

          <el-table-column label="人设与规范" min-width="150" show-overflow-tooltip>
            <template #default="{ row }">
              <span :class="row.promptId ? 'text-foreground' : 'text-platform-text-muted'">
                {{ personaLabel(row as AgentRole) }}
              </span>
            </template>
          </el-table-column>

          <el-table-column label="操作" width="124" align="right" fixed="right">
            <template #default="{ row }">
              <div class="inline-flex items-center gap-1">
                <el-button
                  link
                  type="primary"
                  size="small"
                  :data-test="`btn-edit-employee-${row.id}`"
                  @click="openEdit(row as AgentRole)"
                >
                  编辑
                </el-button>
                <el-button
                  link
                  type="danger"
                  size="small"
                  :data-test="`btn-delete-employee-${row.id}`"
                  @click="removeEmployee(row as AgentRole)"
                >
                  删除
                </el-button>
              </div>
            </template>
          </el-table-column>

          <template #empty>
            <ListEmptyState
              :state="emptyState"
              :icon="User"
              :title="emptyTitle"
              :hint="emptyHint"
              @reset="resetSearch"
            >
              <el-button
                type="primary"
                :icon="Plus"
                data-test="btn-new-employee"
                @click="openCreate"
              >
                新建 AI 员工
              </el-button>
            </ListEmptyState>
          </template>
        </el-table>
      </div>

      <div class="list-panel-footer" data-test="employee-pagination">
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

    <EmployeeEditDialog
      v-if="showDialog"
      :employee="selectedEmployee"
      @saved="closeEditor"
      @cancel="closeEditor"
    />
  </div>
</template>

<style scoped>
/* Row chrome only: the page frame (header, filter width, table wrapper) comes
   from list-page.css. */

/* The employee's identity colour is data, so it rides on the monogram chip
   instead of appearing as a second badge. */
.row-monogram {
  width: 24px;
  height: 24px;
  flex: none;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: var(--radius);
  font-size: var(--text-size-12);
  font-weight: 600;
  color: #fff;
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

.engine-chip {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  max-width: 100%;
  padding: 0 6px;
  height: 20px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  background: var(--color-surface-muted);
  font-size: var(--text-size-11);
  color: var(--color-text);
}

.engine-icon {
  width: 14px;
  height: 14px;
  flex: none;
}
</style>
