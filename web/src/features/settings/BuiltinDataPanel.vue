<script setup lang="ts">
// Session data management for the backend that persists its transcript in the
// platform database (the embedded agent). The panel is capability-driven: the
// settings page mounts it only for a backend whose agent DTO reports
// sessionDataOwner, so it names no backend and stays neutral.
import { onMounted, ref } from 'vue'
import { describeApiError } from '../../api/client'
import {
  clearAgentDataSessions,
  deleteAgentDataSession,
  listAgentDataSessions,
  type AgentDataSession,
} from '../../api/settings'
import { UiAlert, UiButton } from '../../shared/ui'

const props = defineProps<{ backend: string }>()

const sessions = ref<AgentDataSession[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = 20
const loading = ref(false)
const error = ref('')
const // A pending delete/clear confirmation: the session id, or '*' for clear.
pendingConfirm = ref<string | null>(null)
const working = ref(false)
const success = ref('')

async function load() {
  loading.value = true
  error.value = ''
  try {
    const result = await listAgentDataSessions(page.value, pageSize)
    sessions.value = result.sessions
    total.value = result.total
  } catch (caught) {
    error.value = describeApiError(caught)
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

function totalPages(): number {
  return Math.max(1, Math.ceil(total.value / pageSize))
}

function goPage(target: number) {
  if (target < 1 || target > totalPages()) return
  page.value = target
  pendingConfirm.value = null
  void load()
}

function toggleConfirm(sessionId: string) {
  pendingConfirm.value = pendingConfirm.value === sessionId ? null : sessionId
}

async function confirmPending() {
  if (!pendingConfirm.value) return
  working.value = true
  error.value = ''
  success.value = ''
  try {
    if (pendingConfirm.value === '*') {
      const { deleted } = await clearAgentDataSessions()
      success.value = deleted > 0 ? `已清空 ${deleted} 条会话数据` : '没有可清空的会话数据'
    } else {
      await deleteAgentDataSession(pendingConfirm.value)
      success.value = '会话数据已删除'
    }
    pendingConfirm.value = null
    if (page.value > 1 && sessions.value.length <= 1) page.value = Math.max(1, page.value - 1)
    await load()
    setTimeout(() => {
      success.value = ''
    }, 2500)
  } catch (caught) {
    error.value = describeApiError(caught)
    pendingConfirm.value = null
  } finally {
    working.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="p-4 rounded-md bg-slate-50 dark:bg-slate-800/40 border border-slate-200 dark:border-slate-700/80 space-y-3" :data-test="`agent-data-panel-${props.backend}`">
    <div class="flex items-center justify-between pb-2 border-b border-slate-200/80 dark:border-slate-700/60">
      <div>
        <strong class="text-xs font-semibold text-slate-800 dark:text-slate-200 block" data-test="agent-data-title">{{ label(props.backend) }}</strong>
        <span class="text-[11px] text-slate-400">该 Agent 的会话数据保存在平台数据库中，可在此查看与删除</span>
      </div>
      <div class="flex items-center gap-2">
        <UiButton size="sm" variant="ghost" :loading="loading" data-test="agent-data-reload" @click="load">刷新</UiButton>
        <UiButton
          size="sm"
          variant="danger"
          :disabled="total === 0"
          :data-test="`agent-data-clear-${props.backend}`"
          @click="toggleConfirm('*')"
        >
          清空全部
        </UiButton>
      </div>
    </div>

    <UiAlert v-if="error" variant="error" data-test="agent-data-error">{{ error }}</UiAlert>
    <p v-if="success" class="text-[11px] text-emerald-600 dark:text-emerald-400 font-medium" data-test="agent-data-success">
      {{ success }}
    </p>

    <div
      v-if="pendingConfirm"
      class="flex items-center justify-between gap-2 p-2.5 rounded bg-rose-50 dark:bg-rose-950/40 border border-rose-200 dark:border-rose-900 text-xs text-rose-700 dark:text-rose-300"
      data-test="agent-data-confirm"
    >
      <span>{{ pendingConfirm === '*' ? '确认清空全部会话数据？删除后不可恢复。' : '确认删除这条会话数据？删除后不可恢复。' }}</span>
      <div class="flex items-center gap-1.5 shrink-0">
        <UiButton size="sm" variant="secondary" data-test="agent-data-cancel" @click="pendingConfirm = null">取消</UiButton>
        <UiButton size="sm" variant="danger" :loading="working" data-test="agent-data-confirm-delete" @click="confirmPending">确认删除</UiButton>
      </div>
    </div>

    <ul v-if="sessions.length" class="space-y-1.5 max-h-64 overflow-y-auto pr-1">
      <li
        v-for="session in sessions"
        :key="session.sessionId"
        :data-agent-data-session="session.sessionId"
        class="p-2.5 rounded bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 text-xs hover:border-slate-300 dark:hover:border-slate-700 transition-colors"
      >
        <div class="flex items-start justify-between gap-3">
          <div class="min-w-0 flex-1">
            <div class="flex items-center gap-2 min-w-0">
              <span class="font-medium text-slate-800 dark:text-slate-200 truncate" :title="session.title">
                {{ session.title || '(未命名会话)' }}
              </span>
              <code class="text-[10px] font-mono text-slate-400 shrink-0">{{ session.sessionId.slice(0, 8) }}</code>
            </div>
            <div class="flex items-center gap-2.5 mt-1 text-[11px] text-slate-400 min-w-0">
              <span class="truncate" :title="session.directory">{{ session.directory }}</span>
              <span v-if="session.model" class="shrink-0">{{ session.model }}</span>
              <span class="shrink-0 tabular-nums">{{ session.messageCount }} 条记录</span>
              <span class="shrink-0 tabular-nums" :title="formatTime(session.createdAt)">{{ formatTime(session.updatedAt) }}</span>
            </div>
          </div>
          <UiButton
            size="sm"
            variant="danger"
            class="shrink-0"
            :data-test="`agent-data-delete-${session.sessionId}`"
            :disabled="working && pendingConfirm !== null"
            @click="toggleConfirm(session.sessionId)"
          >
            删除
          </UiButton>
        </div>
      </li>
    </ul>
    <p v-else-if="!loading && !error" class="text-xs text-slate-400 py-3 text-center" data-test="agent-data-empty">暂无会话数据</p>

    <div v-if="total > pageSize" class="flex items-center justify-between pt-1 text-[11px] text-slate-400">
      <span class="tabular-nums">第 {{ page }} / {{ totalPages() }} 页，共 {{ total }} 条</span>
      <div class="flex items-center gap-1.5">
        <UiButton size="sm" variant="secondary" :disabled="page <= 1" @click="goPage(page - 1)">上一页</UiButton>
        <UiButton size="sm" variant="secondary" :disabled="page >= totalPages()" @click="goPage(page + 1)">下一页</UiButton>
      </div>
    </div>
  </div>
</template>
