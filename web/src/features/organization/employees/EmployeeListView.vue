<script setup lang="ts">
// EmployeeListView: Pure object governance for AI Employees (Agent Roles).
// Displays and manages digital employee profiles, execution engines, and personas.
// Follows design-taste-frontend standards:
//   - Zero dialogs/popups for creation & editing (uses dedicated EmployeeEditPanel workbench)
//   - Clean header with count badge, clear description, and primary action
//   - Compact search and engine filter segment
//   - Flat table with subtle hover states, zero resting shadows, and inline actions
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useRolesStore } from '../../../stores/roles'
import { usePromptsStore } from '../../../stores/prompts'
import { useModelsStore } from '../../../stores/models'
import { useAgentsStore } from '../../../stores/agents'
import BackendIcon from '../../../shared/components/BackendIcon.vue'
import type { AgentRole } from '../../../api/roles'
import { UiButton, UiAlert, UiConfirmDialog } from '../../../shared/ui'
import EmployeeEditPanel from './EmployeeEditPanel.vue'

const route = useRoute()
const router = useRouter()
const rolesStore = useRolesStore()
const promptsStore = usePromptsStore()
const modelsStore = useModelsStore()
const agentsStore = useAgentsStore()

const keyword = ref('')
const selectedBackendFilter = ref<string>('all')

// Mode: 'list' | 'create' | 'edit'
const mode = ref<'list' | 'create' | 'edit'>('list')
const selectedEmployee = ref<AgentRole | null>(null)

const pendingDelete = ref<AgentRole | null>(null)
const deleting = ref(false)
const deleteError = ref('')

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

function syncRouteState() {
  if (route.query.action === 'new') {
    mode.value = 'create'
    selectedEmployee.value = null
  } else if (route.query.edit) {
    const targetId = Number(route.query.edit)
    const found = rolesStore.roles.find((r) => r.id === targetId)
    if (found) {
      mode.value = 'edit'
      selectedEmployee.value = found
    }
  } else if (mode.value !== 'create' && mode.value !== 'edit') {
    mode.value = 'list'
    selectedEmployee.value = null
  }
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
  if (selectedBackendFilter.value !== 'all') {
    list = list.filter((r) => (r.backend || 'llm') === selectedBackendFilter.value)
  }
  return list
})

interface BackendChoice {
  id: string
  label: string
}

const selectableBackends = computed<BackendChoice[]>(() => {
  const list: BackendChoice[] = [
    { id: 'llm', label: '通用大模型 (LLM)' },
  ]
  for (const b of (agentsStore.backends || [])) {
    if (b.enabled !== false) {
      list.push({
        id: b.backend,
        label: b.label || b.backend,
      })
    }
  }
  return list
})

function getPromptName(promptId?: number): string {
  if (!promptId) return ''
  const p = promptsStore.byId(promptId)
  return p ? p.name : `提示词 #${promptId}`
}

function openCreate() {
  selectedEmployee.value = null
  mode.value = 'create'
  void router.replace({ query: { action: 'new' } })
}

function openEdit(role: AgentRole) {
  selectedEmployee.value = role
  mode.value = 'edit'
  void router.replace({ query: { edit: String(role.id) } })
}

function onSaved(_roleId: number) {
  mode.value = 'list'
  selectedEmployee.value = null
  void router.replace({ query: {} })
}

function onCancel() {
  mode.value = 'list'
  selectedEmployee.value = null
  void router.replace({ query: {} })
}

async function confirmDeleteRole() {
  if (!pendingDelete.value) return
  deleting.value = true
  deleteError.value = ''
  try {
    await rolesStore.deleteRole(pendingDelete.value.id)
    pendingDelete.value = null
  } catch (err: any) {
    deleteError.value = err?.message || '删除 AI 员工失败'
  } finally {
    deleting.value = false
  }
}
</script>

<template>
  <div class="h-full flex flex-col min-h-0 bg-[var(--color-bg)] overflow-hidden">
    <!-- View Switch: In-Place Edit/Create Panel (Zero Popups) -->
    <template v-if="mode === 'create' || mode === 'edit'">
      <EmployeeEditPanel
        :employee="selectedEmployee"
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
              <h1 class="text-lg font-semibold text-foreground tracking-tight">AI 员工</h1>
              <span class="px-2 py-0.5 rounded-full text-2xs font-mono font-medium bg-surface-muted text-platform-text-muted border border-border">
                {{ rolesStore.roles.length }} 名员工
              </span>
            </div>
            <p class="text-xs text-platform-text-muted mt-0.5">
              专职数字员工与执行引擎分工，作为 AI 团队的专业协同专家编制。
            </p>
          </div>

          <UiButton
            variant="primary"
            class="shrink-0 text-xs"
            data-test="btn-create-employee"
            @click="openCreate"
          >
            <svg viewBox="0 0 16 16" width="12" height="12" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" class="mr-1.5" aria-hidden="true">
              <line x1="8" y1="3" x2="8" y2="13" />
              <line x1="3" y1="8" x2="13" y2="8" />
            </svg>
            <span>新建 AI 员工</span>
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
              placeholder="搜索员工姓名、岗位或模型…"
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

          <!-- Backend Filter Segment -->
          <div class="flex items-center gap-1 overflow-x-auto pb-1 sm:pb-0 text-2xs font-medium text-platform-text-muted">
            <button
              type="button"
              class="px-2.5 py-1 rounded border transition-colors whitespace-nowrap"
              :class="selectedBackendFilter === 'all'
                ? 'bg-surface text-foreground border-accent/40 shadow-2xs font-semibold'
                : 'border-border bg-surface-muted/60 hover:text-foreground'"
              @click="selectedBackendFilter = 'all'"
            >
              全部引擎
            </button>
            <button
              v-for="b in selectableBackends"
              :key="b.id"
              type="button"
              class="px-2.5 py-1 rounded border transition-colors whitespace-nowrap"
              :class="selectedBackendFilter === b.id
                ? 'bg-surface text-foreground border-accent/40 shadow-2xs font-semibold'
                : 'border-border bg-surface-muted/60 hover:text-foreground'"
              @click="selectedBackendFilter = b.id"
            >
              {{ b.label }}
            </button>
          </div>
        </div>
      </header>

      <!-- Error Banner -->
      <div v-if="deleteError" class="px-6 pt-4">
        <UiAlert variant="error">
          {{ deleteError }}
        </UiAlert>
      </div>

      <!-- Main Content Table View -->
      <div class="flex-1 min-h-0 overflow-auto p-6">
        <div v-if="filteredEmployees.length > 0" class="border border-border rounded-lg bg-surface overflow-hidden">
          <table class="w-full border-collapse text-left text-xs">
            <thead>
              <tr class="border-b border-border bg-surface-muted/60 text-2xs font-medium text-platform-text-muted uppercase tracking-wider">
                <th class="py-2.5 px-4">AI 员工 / 岗位</th>
                <th class="py-2.5 px-4">执行引擎 (Backend)</th>
                <th class="py-2.5 px-4">运行模型 (Model)</th>
                <th class="py-2.5 px-4">人设与规范</th>
                <th class="py-2.5 px-4 text-right">操作</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-border/60">
              <tr
                v-for="item in filteredEmployees"
                :key="item.id"
                class="hover:bg-surface-muted/30 transition-colors group"
              >
                <!-- Name & Color -->
                <td class="py-3 px-4">
                  <div class="flex items-center gap-3">
                    <span
                      class="w-8 h-8 rounded-md flex items-center justify-center text-white text-xs font-semibold shadow-2xs shrink-0"
                      :style="{ backgroundColor: item.color || '#3b82f6' }"
                    >
                      {{ item.name.slice(0, 1) }}
                    </span>
                    <div class="min-w-0 max-w-xs sm:max-w-sm">
                      <div class="flex items-center gap-1.5">
                        <span class="font-medium text-foreground truncate">{{ item.name }}</span>
                        <span class="text-3xs font-mono text-platform-text-muted">#{{ item.id }}</span>
                      </div>
                      <p class="text-2xs text-platform-text-muted truncate mt-0.5">
                        {{ item.description || '专职数字员工 / 协同专家角色' }}
                      </p>
                    </div>
                  </div>
                </td>

                <!-- Backend Engine -->
                <td class="py-3 px-4">
                  <div class="inline-flex items-center gap-1.5 px-2 py-0.5 rounded text-2xs font-medium bg-surface-muted border border-border text-foreground">
                    <BackendIcon :backend="item.backend || 'llm'" class="w-3.5 h-3.5" />
                    <span>{{ item.backend || 'LLM' }}</span>
                  </div>
                </td>

                <!-- Model -->
                <td class="py-3 px-4">
                  <span class="inline-flex items-center px-2 py-0.5 rounded text-2xs font-mono bg-surface-muted text-foreground border border-border">
                    {{ item.model }}
                  </span>
                </td>

                <!-- Prompt library -->
                <td class="py-3 px-4">
                  <span v-if="item.promptId" class="inline-flex items-center gap-1 text-2xs text-foreground">
                    <svg viewBox="0 0 16 16" width="11" height="11" fill="none" stroke="currentColor" stroke-width="1.8" class="text-accent shrink-0">
                      <path d="M12 20h9" />
                      <path d="M16.5 3.5a2.121 2.121 0 0 1 3 3L7 19l-4 1 1-4L16.5 3.5z" />
                    </svg>
                    <span class="truncate max-w-[130px]">{{ getPromptName(item.promptId) }}</span>
                  </span>
                  <span v-else class="text-2xs text-platform-text-muted">
                    {{ item.systemPrompt ? '专属人设' : '通用' }}
                  </span>
                </td>

                <!-- Actions -->
                <td class="py-3 px-4 text-right">
                  <div class="inline-flex items-center gap-1.5">
                    <button
                      type="button"
                      class="px-2 py-1 rounded text-2xs font-medium text-foreground hover:bg-surface-muted transition-colors border border-transparent hover:border-border"
                      title="编辑档案"
                      @click="openEdit(item)"
                    >
                      编辑
                    </button>
                    <button
                      type="button"
                      class="px-2 py-1 rounded text-2xs font-medium text-red-500 hover:bg-red-500/10 transition-colors border border-transparent hover:border-red-500/20"
                      title="删除员工"
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
              <path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2" />
              <circle cx="9" cy="7" r="4" />
              <path d="M23 21v-2a4 4 0 0 0-3-3.87" />
              <path d="M16 3.13a4 4 0 0 1 0 7.75" />
            </svg>
          </div>
          <p class="text-sm font-medium text-foreground">未找到匹配的 AI 员工</p>
          <p class="text-xs text-platform-text-muted mt-1 max-w-sm">
            {{ keyword ? '没有符合搜索条件的数字员工，请尝试其他关键词。' : '当前暂无 AI 员工，点击上方按钮创建专职数字专家。' }}
          </p>
          <UiButton
            v-if="!keyword"
            variant="primary"
            class="text-xs mt-4"
            @click="openCreate"
          >
            + 新建 AI 员工
          </UiButton>
        </div>
      </div>
    </template>

    <!-- Delete Confirmation -->
    <UiConfirmDialog
      v-if="pendingDelete"
      title="删除 AI 员工"
      :message="`确定要删除 AI 员工「${pendingDelete.name}」吗？被编制进 AI 团队的成员关系将一并解除。`"
      confirm-text="删除"
      variant="danger"
      @confirm="confirmDeleteRole"
      @cancel="pendingDelete = null"
    />
  </div>
</template>
