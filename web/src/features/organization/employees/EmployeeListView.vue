<script setup lang="ts">
// EmployeeListView: the digital-employee roster of the organization center.
//
// `el-table` for the roster, `el-input` for search, `el-segmented` for the
// execution-engine filter, `ElMessageBox` for deletion and `EmployeeEditDialog`
// for the form, so the route no longer swaps shape when an editor opens.
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus, Search } from '@element-plus/icons-vue'
import { useRolesStore } from '../../../stores/roles'
import { usePromptsStore } from '../../../stores/prompts'
import { useModelsStore } from '../../../stores/models'
import { useAgentsStore } from '../../../stores/agents'
import BackendIcon from '../../../shared/components/BackendIcon.vue'
import type { AgentRole } from '../../../api/roles'
import EmployeeEditDialog from './EmployeeEditDialog.vue'

const route = useRoute()
const router = useRouter()
const rolesStore = useRolesStore()
const promptsStore = usePromptsStore()
const modelsStore = useModelsStore()
const agentsStore = useAgentsStore()

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
</script>

<template>
  <div class="h-full flex flex-col min-h-0 bg-[var(--color-bg)] overflow-hidden" data-test="employee-list">
    <header class="px-6 py-4 border-b border-border bg-surface shrink-0">
      <div class="flex items-center justify-between gap-4">
        <div>
          <div class="flex items-center gap-2.5">
            <h1 class="text-lg font-semibold text-foreground tracking-tight">AI 员工</h1>
            <span
              class="px-2 py-0.5 rounded-full text-2xs font-mono font-medium bg-surface-muted text-platform-text-muted border border-border"
            >
              {{ rolesStore.roles.length }} 名员工
            </span>
          </div>
          <p class="text-xs text-platform-text-muted mt-0.5">
            专职数字员工与执行引擎分工，作为 AI 团队的专业协同专家编制。
          </p>
        </div>

        <el-button
          type="primary"
          class="shrink-0"
          :icon="Plus"
          data-test="btn-create-employee"
          @click="openCreate"
        >
          新建 AI 员工
        </el-button>
      </div>

      <div class="flex flex-col sm:flex-row items-stretch sm:items-center justify-between gap-3 mt-4 pt-3 border-t border-border/50">
        <el-input
          v-model="keyword"
          :prefix-icon="Search"
          placeholder="搜索员工姓名、岗位或模型…"
          clearable
          class="search-input w-full sm:w-80"
        />

        <el-segmented
          v-model="backendFilter"
          :options="backendFilterOptions"
          class="self-start sm:self-auto max-w-full overflow-x-auto"
        />
      </div>
    </header>

    <div class="flex-1 min-h-0 p-6 table-wrap">
      <el-table :data="filteredEmployees" row-key="id" height="100%" data-test="employee-table">
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
            <span class="font-mono text-2xs">{{ row.model }}</span>
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
          <div class="px-6 py-10 text-center">
            <p class="text-sm font-medium text-foreground">
              {{ keyword || backendFilter !== 'all' ? '未找到匹配的 AI 员工' : '当前暂无 AI 员工' }}
            </p>
            <p class="text-xs text-platform-text-muted mt-1 max-w-sm mx-auto">
              {{
                keyword
                  ? '没有符合搜索条件的数字员工，请尝试其他关键词。'
                  : '当前暂无 AI 员工，点击上方按钮创建专职数字专家。'
              }}
            </p>
            <el-button
              v-if="!keyword"
              type="primary"
              class="mt-4"
              :icon="Plus"
              data-test="btn-new-employee"
              @click="openCreate"
            >
              新建 AI 员工
            </el-button>
          </div>
        </template>
      </el-table>
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
.search-input {
  max-width: 320px;
}

.table-wrap {
  display: flex;
  flex-direction: column;
}

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
