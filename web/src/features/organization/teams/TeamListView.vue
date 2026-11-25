<script setup lang="ts">
// TeamListView: the multi-agent teams of the organization center.
//
// `el-table` for the roster, the shared `ListToolbar` above it (create on the
// left, keyword `el-input` + 搜索 / 重置 on the right), `ElMessageBox` for
// deletion and `TeamEditDialog` for the form (including the staffing picker), so
// opening an editor no longer replaces the whole view.
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Connection, Plus } from '@element-plus/icons-vue'
import { useGroupsStore } from '../../../stores/groups'
import type { Group, GroupDetail } from '../../../api/groups'
import ListToolbar from '../../../shared/components/ListToolbar.vue'
import ListEmptyState from '../../../shared/components/ListEmptyState.vue'
import TeamEditDialog from './TeamEditDialog.vue'

const route = useRoute()
const router = useRouter()
const groupsStore = useGroupsStore()

// The keyword in the field is a draft; ListToolbar applies it on 搜索 / Enter.
const keywordDraft = ref('')
const keyword = ref('')

const showDialog = ref(false)
const selectedTeamDetail = ref<GroupDetail | null>(null)
const loadingDetail = ref(false)

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
  try {
    const detail = await groupsStore.loadGroup(groupId)
    if (detail) {
      selectedTeamDetail.value = detail
      showDialog.value = true
    } else {
      showDialog.value = false
    }
  } catch (err: any) {
    ElMessage.error(err?.message || '加载团队详情失败')
    showDialog.value = false
  } finally {
    loadingDetail.value = false
  }
}

function applySearch() {
  keyword.value = keywordDraft.value.trim()
}

function resetSearch() {
  keywordDraft.value = ''
  keyword.value = ''
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

// 分页固定坐在主体底部（v2 §四 三段式）：表格只铺当前页，正文自己滚。
const page = ref(1)
const pageSize = ref(10)
const pageSizes = [10, 20, 50]
const totalRows = computed(() => filteredTeams.value.length)
const pagedTeams = computed(() => {
  const start = (page.value - 1) * pageSize.value
  return filteredTeams.value.slice(start, start + pageSize.value)
})

// 条件一变就回到第一页，否则用户会停在一张空页上以为被筛选滤空了。
watch([keyword], () => {
  page.value = 1
})

watch([totalRows, pageSize], () => {
  const lastPage = Math.max(1, Math.ceil(totalRows.value / pageSize.value))
  if (page.value > lastPage) page.value = lastPage
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
  try {
    await groupsStore.deleteGroup(team.id)
    await groupsStore.loadGroups()
    ElMessage.success(`已删除团队「${team.name}」`)
  } catch (err: any) {
    ElMessage.error(err?.message || '删除团队失败')
  }
}

// 三种沉默分别说：还在拉数据 / 搜不到 / 本来就空（见 ListEmptyState）。
// 团队列表没有枚举筛选，所以退路只有两条：重置关键词，或者组建一个。
const emptyState = computed<'loading' | 'filtered' | 'empty'>(() => {
  if (groupsStore.loading) return 'loading'
  return keyword.value ? 'filtered' : 'empty'
})

const emptyTitle = computed(() =>
  emptyState.value === 'filtered' ? '没有匹配的 AI 团队' : '当前暂无 AI 团队',
)

const emptyHint = computed(() =>
  emptyState.value === 'filtered'
    ? `关键词「${keyword.value}」没有命中任何团队的名称、描述或大脑模型。`
    : 'AI 团队由一个中控大脑负责拆解与调度，从 AI 员工花名册里勾选成员组成作战单元。',
)
</script>

<template>
  <div class="h-full flex flex-col min-h-0 bg-[var(--color-bg)] overflow-hidden" data-test="team-list">
    <header class="list-page-header">
      <h1 class="list-page-title">AI 团队</h1>
      <p class="list-page-desc">
        中控大脑调度与专职 AI 员工组成的多 Agent 协同作战单元。
      </p>

    </header>

    <div class="list-page-panel" data-test="team-panel">
      <div class="list-panel-toolbar">
        <ListToolbar
          v-model:keyword="keywordDraft"
          placeholder="搜索团队…"
          hint="搜索团队名称、描述或大脑模型…"
          search-test="btn-search-team"
          reset-test="btn-reset-team"
          @search="applySearch"
          @reset="resetSearch"
        >
          <template #actions>
            <el-button
              type="primary"
              :icon="Plus"
              data-test="btn-create-team"
              @click="openCreate"
            >
              新建 AI 团队
            </el-button>
            <span v-if="loadingDetail" class="loading-hint">正在加载团队详情…</span>
          </template>
        </ListToolbar>
      </div>

      <div class="list-panel-table">
        <el-table
          :data="pagedTeams"
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
              <span class="font-mono">{{ row.orchestratorModel || '默认调度模型' }}</span>
            </template>
          </el-table-column>

          <el-table-column label="编制员工" width="110" align="right">
            <template #default="{ row }">
              <span class="font-mono">{{ row.memberCount || 0 }} 名</span>
            </template>
          </el-table-column>

          <el-table-column label="协作任务" width="100" align="right">
            <template #default="{ row }">
              <span class="font-mono text-platform-text-muted">{{ row.taskCount || 0 }} 项</span>
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
            <ListEmptyState
              :state="emptyState"
              :icon="Connection"
              :title="emptyTitle"
              :hint="emptyHint"
              @reset="resetSearch"
            >
              <el-button
                type="primary"
                :icon="Plus"
                data-test="btn-new-team"
                @click="openCreate"
              >
                新建 AI 团队
              </el-button>
            </ListEmptyState>
          </template>
        </el-table>
      </div>

      <div class="list-panel-footer" data-test="team-pagination">
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

    <TeamEditDialog
      v-if="showDialog"
      :team="selectedTeamDetail"
      @saved="closeEditor"
      @cancel="closeEditor"
    />
  </div>
</template>

<style scoped>
/* Row chrome only: the page frame (header, table wrapper) comes from
   list-page.css. */
.loading-hint {
  font-size: var(--text-size-11);
  color: var(--color-text-muted);
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
