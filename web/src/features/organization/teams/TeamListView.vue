<script setup lang="ts">
// TeamListView: Pure object governance for AI Teams (Groups).
// Displays and manages team definitions, orchestrator brain models, and member staffing.
// Follows design-taste-frontend standards:
//   - Zero dialogs/popups for creation & editing (uses dedicated TeamEditPanel workbench)
//   - Clean header with count badge, clear description, and primary action
//   - Compact search filter bar
//   - Flat table with subtle hover states, zero resting shadows, and inline actions
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useGroupsStore } from '../../../stores/groups'
import type { Group, GroupDetail } from '../../../api/groups'
import { UiButton, UiAlert, UiConfirmDialog } from '../../../shared/ui'
import TeamEditPanel from './TeamEditPanel.vue'

const route = useRoute()
const router = useRouter()
const groupsStore = useGroupsStore()

const keyword = ref('')

// Mode: 'list' | 'create' | 'edit'
const mode = ref<'list' | 'create' | 'edit'>('list')
const selectedTeamDetail = ref<GroupDetail | null>(null)
const loadingDetail = ref(false)

const pendingDelete = ref<Group | null>(null)
const actionError = ref('')

onMounted(async () => {
  await groupsStore.loadGroups()
  syncRouteState()
})

watch(
  () => [route.query, groupsStore.groups],
  () => {
    syncRouteState()
  },
  { deep: true },
)

async function syncRouteState() {
  if (route.query.action === 'new') {
    mode.value = 'create'
    selectedTeamDetail.value = null
  } else if (route.query.edit) {
    const targetId = Number(route.query.edit)
    if (!selectedTeamDetail.value || selectedTeamDetail.value.id !== targetId) {
      await loadTeamDetail(targetId)
    }
  } else if (mode.value !== 'create' && mode.value !== 'edit') {
    mode.value = 'list'
    selectedTeamDetail.value = null
  }
}

async function loadTeamDetail(groupId: number) {
  loadingDetail.value = true
  actionError.value = ''
  try {
    const detail = await groupsStore.loadGroup(groupId)
    if (detail) {
      selectedTeamDetail.value = detail
      mode.value = 'edit'
    } else {
      mode.value = 'list'
    }
  } catch (err: any) {
    actionError.value = err?.message || '加载团队详情失败'
    mode.value = 'list'
  } finally {
    loadingDetail.value = false
  }
}

const filteredTeams = computed(() => {
  let list = groupsStore.groups || []
  const q = keyword.value.trim().toLowerCase()
  if (q) {
    list = list.filter(
      (g) =>
        g.name.toLowerCase().includes(q) ||
        (g.description && g.description.toLowerCase().includes(q)) ||
        (g.orchestratorModel && g.orchestratorModel.toLowerCase().includes(q)),
    )
  }
  return list
})

function openCreate() {
  selectedTeamDetail.value = null
  mode.value = 'create'
  void router.replace({ query: { action: 'new' } })
}

async function openEdit(team: Group) {
  await loadTeamDetail(team.id)
  void router.replace({ query: { edit: String(team.id) } })
}

async function onSaved(_groupId: number) {
  await groupsStore.loadGroups()
  mode.value = 'list'
  selectedTeamDetail.value = null
  void router.replace({ query: {} })
}

function onCancel() {
  mode.value = 'list'
  selectedTeamDetail.value = null
  void router.replace({ query: {} })
}

async function confirmDelete() {
  if (!pendingDelete.value) return
  actionError.value = ''
  try {
    await groupsStore.deleteGroup(pendingDelete.value.id)
    pendingDelete.value = null
    await groupsStore.loadGroups()
  } catch (err: any) {
    actionError.value = err?.message || '删除团队失败'
  }
}
</script>

<template>
  <div class="h-full flex flex-col min-h-0 bg-[var(--color-bg)] overflow-hidden">
    <!-- View Switch: In-Place Edit/Create Panel (Zero Popups) -->
    <template v-if="mode === 'create' || mode === 'edit'">
      <TeamEditPanel
        :team="selectedTeamDetail"
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
              <h1 class="text-lg font-semibold text-foreground tracking-tight">AI 团队</h1>
              <span class="px-2 py-0.5 rounded-full text-2xs font-mono font-medium bg-surface-muted text-platform-text-muted border border-border">
                {{ groupsStore.groups.length }} 个团队
              </span>
            </div>
            <p class="text-xs text-platform-text-muted mt-0.5">
              中控大脑调度与专职 AI 员工组成的多 Agent 协同作战单元。
            </p>
          </div>

          <UiButton
            variant="primary"
            class="shrink-0 text-xs"
            data-test="btn-create-team"
            @click="openCreate"
          >
            <svg viewBox="0 0 16 16" width="12" height="12" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" class="mr-1.5" aria-hidden="true">
              <line x1="8" y1="3" x2="8" y2="13" />
              <line x1="3" y1="8" x2="13" y2="8" />
            </svg>
            <span>新建 AI 团队</span>
          </UiButton>
        </div>

        <!-- Filter & Search Toolbar -->
        <div class="flex flex-col sm:flex-row items-stretch sm:items-center justify-between gap-3 mt-4 pt-3 border-t border-border/50">
          <div class="relative w-full sm:w-72">
            <svg class="absolute left-2.5 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-platform-text-muted pointer-events-none" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.8">
              <circle cx="7" cy="7" r="4.5" />
              <line x1="10.5" y1="10.5" x2="14" y2="14" stroke-linecap="round" />
            </svg>
            <input
              v-model="keyword"
              type="search"
              placeholder="搜索团队名称、描述或大脑模型…"
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
        <div v-if="filteredTeams.length > 0" class="border border-border rounded-lg bg-surface overflow-hidden">
          <table class="w-full border-collapse text-left text-xs">
            <thead>
              <tr class="border-b border-border bg-surface-muted/60 text-2xs font-medium text-platform-text-muted uppercase tracking-wider">
                <th class="py-2.5 px-4">AI 团队</th>
                <th class="py-2.5 px-4">中控调度大脑 (Orchestrator)</th>
                <th class="py-2.5 px-4">编制员工</th>
                <th class="py-2.5 px-4">协作任务</th>
                <th class="py-2.5 px-4 text-right">操作</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-border/60">
              <tr
                v-for="item in filteredTeams"
                :key="item.id"
                class="hover:bg-surface-muted/30 transition-colors group"
              >
                <!-- Name & Description -->
                <td class="py-3 px-4">
                  <div class="flex items-center gap-3">
                    <span class="w-8 h-8 rounded-md flex items-center justify-center text-accent bg-accent/10 border border-accent/20 shrink-0">
                      <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="1.8">
                        <circle cx="12" cy="5" r="3" />
                        <circle cx="5" cy="19" r="3" />
                        <circle cx="19" cy="19" r="3" />
                        <path d="M12 8v4M7.5 17.5 10 13.5M16.5 17.5 14 13.5" />
                      </svg>
                    </span>
                    <div class="min-w-0 max-w-xs sm:max-w-sm">
                      <div class="flex items-center gap-1.5">
                        <span class="font-medium text-foreground truncate">{{ item.name }}</span>
                        <span class="text-3xs font-mono text-platform-text-muted">#{{ item.id }}</span>
                      </div>
                      <p class="text-2xs text-platform-text-muted truncate mt-0.5">
                        {{ item.description || '暂无团队描述' }}
                      </p>
                    </div>
                  </div>
                </td>

                <!-- Orchestrator Model -->
                <td class="py-3 px-4">
                  <span class="inline-flex items-center gap-1 px-2 py-0.5 rounded text-2xs font-mono bg-surface-muted text-foreground border border-border">
                    <svg viewBox="0 0 16 16" width="11" height="11" fill="none" stroke="currentColor" stroke-width="1.8" class="text-accent shrink-0">
                      <circle cx="8" cy="8" r="3" />
                      <path d="M8 1v2M8 13v2M1 8h2M13 8h2" />
                    </svg>
                    <span>{{ item.orchestratorModel || '默认调度模型' }}</span>
                  </span>
                </td>

                <!-- Member Count -->
                <td class="py-3 px-4">
                  <span class="inline-flex items-center gap-1 px-2 py-0.5 rounded text-2xs font-mono bg-surface-muted text-foreground border border-border">
                    <svg viewBox="0 0 16 16" width="11" height="11" fill="none" stroke="currentColor" stroke-width="1.8">
                      <path d="M11 14v-1.5a3 3 0 0 0-3-3H4a3 3 0 0 0-3 3V14" />
                      <circle cx="6" cy="5" r="2.5" />
                      <path d="M15 14v-1a2.5 2.5 0 0 0-2-2.45" />
                      <path d="M10.5 2.6a2.5 2.5 0 0 1 0 4.8" />
                    </svg>
                    <span>{{ item.memberCount || 0 }} 名编制员工</span>
                  </span>
                </td>

                <!-- Task Count -->
                <td class="py-3 px-4">
                  <span class="text-2xs font-mono text-platform-text-muted">
                    {{ item.taskCount || 0 }} 项任务
                  </span>
                </td>

                <!-- Actions -->
                <td class="py-3 px-4 text-right">
                  <div class="inline-flex items-center gap-1.5">
                    <button
                      type="button"
                      class="px-2 py-1 rounded text-2xs font-medium text-foreground hover:bg-surface-muted transition-colors border border-transparent hover:border-border"
                      title="编辑设置与编制员工"
                      :disabled="loadingDetail"
                      @click="openEdit(item)"
                    >
                      编辑
                    </button>
                    <button
                      type="button"
                      class="px-2 py-1 rounded text-2xs font-medium text-red-500 hover:bg-red-500/10 transition-colors border border-transparent hover:border-red-500/20"
                      title="删除团队"
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
              <circle cx="12" cy="5" r="3" />
              <circle cx="5" cy="19" r="3" />
              <circle cx="19" cy="19" r="3" />
              <path d="M12 8v4M7.5 17.5 10 13.5M16.5 17.5 14 13.5" />
            </svg>
          </div>
          <p class="text-sm font-medium text-foreground">未找到匹配的 AI 团队</p>
          <p class="text-xs text-platform-text-muted mt-1 max-w-sm">
            {{ keyword ? '没有符合搜索条件的团队，请尝试其他关键词。' : '当前暂无 AI 团队，点击上方按钮组建由中控大脑与专职员工协同的作战单元。' }}
          </p>
          <UiButton
            v-if="!keyword"
            variant="primary"
            class="text-xs mt-4"
            @click="openCreate"
          >
            + 新建 AI 团队
          </UiButton>
        </div>
      </div>
    </template>

    <!-- Delete Confirmation -->
    <UiConfirmDialog
      v-if="pendingDelete"
      title="删除 AI 团队"
      :message="`确定要删除 AI 团队「${pendingDelete.name}」吗？绑定的协同编排与历史任务将被解绑。`"
      confirm-text="删除"
      variant="danger"
      @confirm="confirmDelete"
      @cancel="pendingDelete = null"
    />
  </div>
</template>
