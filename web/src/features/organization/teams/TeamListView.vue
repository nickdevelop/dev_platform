<script setup lang="ts">
// TeamListView: the multi-agent teams of the organization center.
//
// `el-table` for the roster, `el-input` for search, `ElMessageBox` for deletion
// and `TeamEditDialog` for the form (including the staffing picker), so opening
// an editor no longer replaces the whole view.
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus, Search } from '@element-plus/icons-vue'
import { useGroupsStore } from '../../../stores/groups'
import type { Group, GroupDetail } from '../../../api/groups'
import { UiAlert } from '../../../shared/ui'
import TeamEditDialog from './TeamEditDialog.vue'

const route = useRoute()
const router = useRouter()
const groupsStore = useGroupsStore()

const keyword = ref('')

const showDialog = ref(false)
const selectedTeamDetail = ref<GroupDetail | null>(null)
const loadingDetail = ref(false)
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
    selectedTeamDetail.value = null
    showDialog.value = true
    return
  }
  if (route.query.edit) {
    const targetId = Number(route.query.edit)
    if (!selectedTeamDetail.value || selectedTeamDetail.value.id !== targetId) {
      await loadTeamDetail(targetId)
    }
    return
  }
  if (showDialog.value) {
    showDialog.value = false
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
      showDialog.value = true
    } else {
      showDialog.value = false
    }
  } catch (err: any) {
    actionError.value = err?.message || '加载团队详情失败'
    showDialog.value = false
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
  showDialog.value = true
  void router.replace({ query: { action: 'new' } })
}

async function openEdit(team: Group) {
  await loadTeamDetail(team.id)
  void router.replace({ query: { edit: String(team.id) } })
}

async function closeEditor() {
  showDialog.value = false
  selectedTeamDetail.value = null
  await groupsStore.loadGroups()
  void router.replace({ query: {} })
}

async function removeTeam(team: Group) {
  try {
    await ElMessageBox.confirm(
      `删除后 AI 团队「${team.name}」绑定的协同编排与历史任务将被解绑。`,
      `删除 AI 团队「${team.name}」？`,
      { type: 'warning', confirmButtonText: '删除', cancelButtonText: '取消', confirmButtonType: 'danger' },
    )
  } catch {
    return
  }
  actionError.value = ''
  try {
    await groupsStore.deleteGroup(team.id)
    await groupsStore.loadGroups()
    ElMessage.success(`已删除团队「${team.name}」`)
  } catch (err: any) {
    actionError.value = err?.message || '删除团队失败'
    ElMessage.error(actionError.value)
  }
}
</script>

<template>
  <div class="h-full flex flex-col min-h-0 bg-[var(--color-bg)] overflow-hidden" data-test="team-list">
    <header class="px-6 py-4 border-b border-border bg-surface shrink-0">
      <div class="flex items-center justify-between gap-4">
        <div>
          <div class="flex items-center gap-2.5">
            <h1 class="text-lg font-semibold text-foreground tracking-tight">AI 团队</h1>
            <span
              class="px-2 py-0.5 rounded-full text-2xs font-mono font-medium bg-surface-muted text-platform-text-muted border border-border"
            >
              {{ groupsStore.groups.length }} 个团队
            </span>
          </div>
          <p class="text-xs text-platform-text-muted mt-0.5">
            中控大脑调度与专职 AI 员工组成的多 Agent 协同作战单元。
          </p>
        </div>

        <el-button
          type="primary"
          class="shrink-0"
          :icon="Plus"
          data-test="btn-create-team"
          @click="openCreate"
        >
          新建 AI 团队
        </el-button>
      </div>

      <div class="flex items-center gap-3 mt-4 pt-3 border-t border-border/50">
        <el-input
          v-model="keyword"
          :prefix-icon="Search"
          placeholder="搜索团队名称、描述或大脑模型…"
          clearable
          class="search-input w-full sm:w-80"
        />
        <span v-if="loadingDetail" class="text-2xs text-platform-text-muted">正在加载团队详情…</span>
      </div>
    </header>

    <div v-if="actionError" class="px-6 pt-4">
      <UiAlert variant="error">{{ actionError }}</UiAlert>
    </div>

    <div class="flex-1 min-h-0 p-6 table-wrap">
      <el-table
        :data="filteredTeams"
        row-key="id"
        height="100%"
        data-test="team-table"
      >
        <el-table-column label="AI 团队" min-width="240" show-overflow-tooltip>
          <template #default="{ row }">
            <div class="flex items-center gap-2.5 min-w-0">
              <span class="team-mark" aria-hidden="true">
                <svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="1.8">
                  <circle cx="12" cy="5" r="3" />
                  <circle cx="5" cy="19" r="3" />
                  <circle cx="19" cy="19" r="3" />
                  <path d="M12 8v4M7.5 17.5 10 13.5M16.5 17.5 14 13.5" />
                </svg>
              </span>
              <span class="font-medium text-foreground truncate">{{ row.name }}</span>
              <span class="id-chip font-mono">#{{ row.id }}</span>
            </div>
          </template>
        </el-table-column>

        <el-table-column label="团队定位" min-width="220" show-overflow-tooltip>
          <template #default="{ row }">
            <span class="text-platform-text-muted">{{ row.description || '暂无团队描述' }}</span>
          </template>
        </el-table-column>

        <el-table-column label="中控调度大脑" width="200" show-overflow-tooltip>
          <template #default="{ row }">
            <span class="font-mono text-2xs">{{ row.orchestratorModel || '默认调度模型' }}</span>
          </template>
        </el-table-column>

        <el-table-column label="编制员工" width="110" align="right">
          <template #default="{ row }">
            <span class="font-mono text-2xs">{{ row.memberCount || 0 }} 名</span>
          </template>
        </el-table-column>

        <el-table-column label="协作任务" width="100" align="right">
          <template #default="{ row }">
            <span class="font-mono text-2xs text-platform-text-muted">{{ row.taskCount || 0 }} 项</span>
          </template>
        </el-table-column>

        <el-table-column label="操作" width="124" align="right" fixed="right">
          <template #default="{ row }">
            <div class="inline-flex items-center gap-1">
              <el-button
                link
                type="primary"
                size="small"
                :disabled="loadingDetail"
                :data-test="`btn-edit-team-${row.id}`"
                @click="openEdit(row as Group)"
              >
                编辑
              </el-button>
              <el-button
                link
                type="danger"
                size="small"
                :data-test="`btn-delete-team-${row.id}`"
                @click="removeTeam(row as Group)"
              >
                删除
              </el-button>
            </div>
          </template>
        </el-table-column>

        <template #empty>
          <div class="px-6 py-10 text-center">
            <p class="text-sm font-medium text-foreground">
              {{ keyword ? '未找到匹配的 AI 团队' : '当前暂无 AI 团队' }}
            </p>
            <p class="text-xs text-platform-text-muted mt-1 max-w-sm mx-auto">
              {{
                keyword
                  ? '没有符合搜索条件的团队，请尝试其他关键词。'
                  : '当前暂无 AI 团队，点击上方按钮组建由中控大脑与专职员工协同的作战单元。'
              }}
            </p>
            <el-button
              v-if="!keyword"
              type="primary"
              class="mt-4"
              :icon="Plus"
              data-test="btn-new-team"
              @click="openCreate"
            >
              新建 AI 团队
            </el-button>
          </div>
        </template>
      </el-table>
    </div>

    <TeamEditDialog
      v-if="showDialog"
      :team="selectedTeamDetail"
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

.team-mark {
  width: 24px;
  height: 24px;
  flex: none;
  display: flex;
  align-items: center;
  justify-content: center;
  border: 1px solid color-mix(in srgb, var(--color-primary) 24%, transparent);
  border-radius: var(--radius);
  background: color-mix(in srgb, var(--color-primary) 8%, var(--color-surface));
  color: var(--color-primary);
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
</style>
