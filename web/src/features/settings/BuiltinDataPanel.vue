<script setup lang="ts">
// Session data management for the backend that persists its transcript in the
// platform database (the embedded agent). The panel is capability-driven: the
// settings page mounts it only for a backend whose agent DTO reports
// sessionDataOwner, so it names no backend and stays neutral.
//
// Element Plus migration: rows live in a small `el-table` with `el-pagination`
// at the bottom, deletion and clearing go through `ElMessageBox` (danger), and
// every result — success or failure — is reported once through `ElMessage`.
// The inline confirm strip and the success/error banners are retired.
import { onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { describeApiError } from '../../api/client'
import {
  clearAgentDataSessions,
  deleteAgentDataSession,
  listAgentDataSessions,
  type AgentDataSession,
} from '../../api/settings'

const props = defineProps<{ backend: string }>()

const sessions = ref<AgentDataSession[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)
const loading = ref(false)
const working = ref(false)

async function load() {
  loading.value = true
  try {
    const result = await listAgentDataSessions(page.value, pageSize.value)
    sessions.value = result.sessions
    total.value = result.total
  } catch (caught) {
    ElMessage.error(describeApiError(caught))
  } finally {
    loading.value = false
  }
}

function label(backend: string): string {
  void backend
  return '会话数据管理'
}

function formatTime(value: string): string {
  const parsed = new Date(value)
  if (Number.isNaN(parsed.getTime())) return value
  return parsed.toLocaleString()
}

function onPageChange(target: number) {
  page.value = target
  void load()
}

// Destructive deletes go through the library's confirmation, never an inline
// strip: the question is asked once and answered once.
async function removeSession(session: AgentDataSession) {
  try {
    await ElMessageBox.confirm('确认删除这条会话数据？删除后不可恢复。', '删除会话数据', {
      type: 'warning',
      confirmButtonText: '确认删除',
      cancelButtonText: '取消',
      confirmButtonType: 'danger',
    })
  } catch {
    return
  }
  working.value = true
  try {
    await deleteAgentDataSession(session.sessionId)
    ElMessage.success('会话数据已删除')
    if (page.value > 1 && sessions.value.length <= 1) {
      page.value = Math.max(1, page.value - 1)
    }
    await load()
  } catch (caught) {
    ElMessage.error(describeApiError(caught))
  } finally {
    working.value = false
  }
}

async function clearAll() {
  try {
    await ElMessageBox.confirm('确认清空全部会话数据？删除后不可恢复。', '清空会话数据', {
      type: 'warning',
      confirmButtonText: '确认清空',
      cancelButtonText: '取消',
      confirmButtonType: 'danger',
    })
  } catch {
    return
  }
  working.value = true
  try {
    const { deleted } = await clearAgentDataSessions()
    ElMessage.success(deleted > 0 ? `已清空 ${deleted} 条会话数据` : '没有可清空的会话数据')
    page.value = 1
    await load()
  } catch (caught) {
    ElMessage.error(describeApiError(caught))
  } finally {
    working.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="p-4 rounded-md bg-platform-surface-muted/40 border border-platform-border/80 space-y-3" :data-test="`agent-data-panel-${props.backend}`">
    <div class="flex items-center justify-between pb-2 border-b border-platform-border/40">
      <div>
        <strong class="text-xs font-semibold text-platform-text block" data-test="agent-data-title">{{ label(props.backend) }}</strong>
        <span class="text-2xs text-platform-text-muted">该 Agent 的会话数据保存在平台数据库中，可在此查看与删除</span>
      </div>
      <div class="flex items-center gap-2">
        <el-button
          size="small"
          :loading="loading"
          data-test="agent-data-reload"
          @click="load"
        >
          刷新
        </el-button>
        <el-button
          size="small"
          type="danger"
          plain
          :disabled="total === 0"
          :loading="working"
          :data-test="`agent-data-clear-${props.backend}`"
          @click="clearAll"
        >
          清空全部
        </el-button>
      </div>
    </div>

    <div class="agent-data-table" data-test="agent-data-table">
      <el-table
        :data="sessions"
        size="small"
        row-key="sessionId"
        max-height="256"
        empty-text="暂无会话数据"
      >
        <el-table-column label="会话" min-width="200">
          <template #default="{ row }">
            <div class="flex items-center gap-2 min-w-0">
              <span class="text-xs font-medium text-platform-text truncate" :title="row.title">
                {{ row.title || '(未命名会话)' }}
              </span>
              <code class="text-2xs font-mono text-platform-text-muted shrink-0">{{ row.sessionId.slice(0, 8) }}</code>
            </div>
            <div class="flex items-center gap-2.5 mt-0.5 text-2xs text-platform-text-muted min-w-0">
              <span class="truncate" :title="row.directory">{{ row.directory }}</span>
              <span v-if="row.model" class="shrink-0">{{ row.model }}</span>
              <span class="shrink-0 tabular-nums">{{ row.messageCount }} 条记录</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="更新时间" width="150">
          <template #default="{ row }">
            <span class="text-2xs text-platform-text-muted tabular-nums" :title="formatTime(row.createdAt)">
              {{ formatTime(row.updatedAt) }}
            </span>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="72" align="right">
          <template #default="{ row }">
            <el-button
              size="small"
              type="danger"
              link
              :disabled="working"
              :data-test="`agent-data-delete-${row.sessionId}`"
              @click="removeSession(row as AgentDataSession)"
            >
              删除
            </el-button>
          </template>
        </el-table-column>
      </el-table>
    </div>

    <div v-if="total > pageSize" class="flex items-center justify-end pt-1" data-test="agent-data-pagination">
      <el-pagination
        small
        background
        layout="total, prev, pager, next"
        :total="total"
        :current-page="page"
        :page-size="pageSize"
        @current-change="onPageChange"
      />
    </div>
  </div>
</template>
