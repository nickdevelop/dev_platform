<script setup lang="ts">
// Task Detail View: displays session template info, allows manual execution,
// shows key execution metrics in a KPI strip, and presents run history as a clean table.
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import type { Task, TaskRun } from '../../api/tasks'
import BackendTag from '../../shared/components/BackendTag.vue'
import { useProjectsStore } from '../../stores/projects'
import { useTasksStore } from '../../stores/tasks'
import {
  formatDateTime,
  getRunStatusBadgeClass,
  getRunStatusLabel,
  getScheduleTypeLabel,
  getTaskTypeLabel,
} from './status'
import TaskChatDialog from './TaskChatDialog.vue'
import TaskManageDialog from './TaskManageDialog.vue'
import { UiButton, UiBadge, UiAlert } from '../../shared/ui'

const props = defineProps<{
  taskId: number
}>()

const emit = defineEmits<{
  deleted: []
}>()

const tasksStore = useTasksStore()
const projectsStore = useProjectsStore()

const triggering = ref(false)
const actionError = ref('')
const showEditDialog = ref(false)
const copiedTemplate = ref(false)

// Session Dialog State
const activeSessionId = ref<string | null>(null)
const viewingRun = ref<TaskRun | null>(null)
const showChatDialog = ref(false)

let pollTimer: ReturnType<typeof setInterval> | null = null

const task = computed<Task | null>(() => tasksStore.byId(props.taskId) ?? null)

const taskRuns = computed<TaskRun[]>(() => tasksStore.runsByTaskId(props.taskId))

const projectName = computed(() => {
  if (!task.value) return '-'
  return projectsStore.byId(task.value.projectId)?.name ?? `项目 #${task.value.projectId}`
})

// Metrics for the KPI strip
const totalRunsCount = computed(() => taskRuns.value.length)

const successRate = computed(() => {
  if (taskRuns.value.length === 0) return '-'
  const finished = taskRuns.value.filter((r) => r.status === 'success' || r.status === 'failed')
  if (finished.length === 0) return '-'
  const successes = finished.filter((r) => r.status === 'success').length
  const pct = Math.round((successes / finished.length) * 100)
  return `${pct}%`
})

const lastDurationText = computed(() => {
  if (taskRuns.value.length === 0) return '-'
  const latest = taskRuns.value[0]
  return calculateDuration(latest)
})

async function refreshData() {
  if (!props.taskId) return
  await Promise.all([
    tasksStore.loadTask(props.taskId),
    tasksStore.loadTaskRuns(props.taskId),
  ])
}

// Watch running states and start polling if needed
watch(
  taskRuns,
  (runs) => {
    const hasRunning = runs.some((r) => r.status === 'running')
    if (hasRunning && !pollTimer) {
      pollTimer = setInterval(() => {
        void refreshData()
      }, 3000)
    } else if (!hasRunning && pollTimer) {
      clearInterval(pollTimer)
      pollTimer = null
    }
  },
  { immediate: true },
)

watch(
  () => props.taskId,
  async (newId) => {
    if (newId) {
      actionError.value = ''
      await refreshData()
    }
  },
  { immediate: true },
)

onMounted(async () => {
  await projectsStore.load()
  await refreshData()
})

onBeforeUnmount(() => {
  if (pollTimer) {
    clearInterval(pollTimer)
    pollTimer = null
  }
})

// Trigger Manual Execution
async function handleManualTrigger() {
  if (!task.value || triggering.value) return
  triggering.value = true
  actionError.value = ''
  try {
    const res = await tasksStore.triggerExecution(task.value.id, 'manual')
    if (res.run) {
      openRunSession(res.run)
    }
  } catch (err: any) {
    actionError.value = err.message || '手动执行失败'
  } finally {
    triggering.value = false
  }
}

function openRunSession(run: TaskRun) {
  viewingRun.value = run
  activeSessionId.value = run.sessionId
  showChatDialog.value = true
}

async function handleDelete() {
  if (!task.value) return
  if (!confirm(`确定要删除任务「${task.value.title}」吗？`)) return
  try {
    await tasksStore.deleteTask(task.value.id)
    emit('deleted')
  } catch (err: any) {
    actionError.value = err.message || '删除任务失败'
  }
}

function calculateDuration(run: TaskRun): string {
  if (!run.finishedAt) {
    return run.status === 'running' ? '运行中…' : '-'
  }
  const start = new Date(run.triggeredAt).getTime()
  const end = new Date(run.finishedAt).getTime()
  if (isNaN(start) || isNaN(end) || end < start) return '-'
  const sec = Math.round((end - start) / 1000)
  if (sec < 60) return `${sec}s`
  const min = Math.floor(sec / 60)
  const remSec = sec % 60
  return `${min}m ${remSec}s`
}

async function copyPromptTemplate() {
  if (!task.value?.promptTemplate) return
  try {
    await navigator.clipboard.writeText(task.value.promptTemplate)
    copiedTemplate.value = true
    setTimeout(() => {
      copiedTemplate.value = false
    }, 2000)
  } catch {
    // Ignore clipboard error
  }
}
</script>

<template>
  <div v-if="task" class="task-detail-container h-full overflow-y-auto p-6 md:p-8 space-y-6 bg-[var(--color-bg)]" data-test="task-detail-view">
    <!-- 1. Header Summary Panel -->
    <header class="detail-header p-6 rounded-[var(--radius,6px)] bg-[var(--color-surface)] border border-[var(--color-border)] flex items-start justify-between gap-6 flex-wrap">
      <div class="header-main flex-1 min-w-[280px] space-y-3">
        <!-- Badges row -->
        <div class="badge-row flex items-center gap-2 flex-wrap">
          <span class="type-pill" :class="task.taskType">
            {{ getTaskTypeLabel(task.taskType) }}
          </span>
          <span
            class="status-badge"
            :class="getRunStatusBadgeClass(task.lastRunStatus)"
          >
            {{ getRunStatusLabel(task.lastRunStatus) }}
          </span>
          <span v-if="task.scheduleEnabled" class="inline-flex items-center gap-1.5 text-xs px-2 py-0.5 rounded bg-amber-50 dark:bg-amber-950/40 text-amber-700 dark:text-amber-400 border border-amber-200 dark:border-amber-800">
            <svg viewBox="0 0 16 16" width="11" height="11" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true">
              <circle cx="8" cy="8" r="6" />
              <polyline points="8 4.5 8 8 10.5 9.5" />
            </svg>
            <span>{{ getScheduleTypeLabel(task.scheduleType) }}已激活</span>
          </span>
        </div>

        <!-- Task title & description -->
        <div>
          <h1 class="task-title text-xl md:text-2xl font-semibold text-[var(--color-text)] tracking-tight">
            {{ task.title }}
          </h1>
          <p v-if="task.description" class="task-desc text-xs md:text-sm text-[var(--color-text-muted)] mt-1 leading-relaxed">
            {{ task.description }}
          </p>
        </div>

        <!-- Meta info row -->
        <div class="meta-row flex items-center gap-5 text-xs text-[var(--color-text-muted)] pt-1 flex-wrap">
          <div class="meta-item flex items-center gap-1.5">
            <span class="text-slate-400">关联项目:</span>
            <span class="font-medium text-[var(--color-text)]">{{ projectName }}</span>
          </div>
          <div class="meta-item flex items-center gap-1.5">
            <span class="text-slate-400">Agent:</span>
            <BackendTag :backend="task.backend" />
          </div>
          <div class="meta-item flex items-center gap-1.5">
            <span class="text-slate-400">运行模型:</span>
            <code class="px-1.5 py-0.5 bg-[var(--color-surface-muted)] text-[var(--color-text)] rounded border border-[var(--color-border)] font-mono text-[11px]">{{ task.model }}</code>
          </div>
          <div class="meta-item flex items-center gap-1.5">
            <span class="text-slate-400">调度类型:</span>
            <span class="font-medium text-[var(--color-text)]">{{ getScheduleTypeLabel(task.scheduleType) }}</span>
          </div>
        </div>
      </div>

      <!-- Action buttons -->
      <div class="header-actions flex items-center gap-2.5 shrink-0">
        <UiButton
          variant="primary"
          class="btn-trigger px-3.5 py-1.5 text-xs font-medium"
          :disabled="triggering"
          :loading="triggering"
          data-test="btn-trigger-task"
          @click="handleManualTrigger"
        >
          <svg v-if="!triggering" class="mr-1.5 inline-block" viewBox="0 0 16 16" width="12" height="12" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
            <polygon points="5 3 13 8 5 13 5 3" />
          </svg>
          {{ triggering ? '启动执行中…' : '手动执行' }}
        </UiButton>
        <UiButton
          variant="secondary"
          class="btn-edit px-3 py-1.5 text-xs"
          data-test="btn-edit-task"
          @click="showEditDialog = true"
        >
          编辑配置
        </UiButton>
        <UiButton
          variant="danger"
          class="btn-delete px-3 py-1.5 text-xs"
          data-test="btn-delete-task"
          @click="handleDelete"
        >
          删除
        </UiButton>
      </div>
    </header>

    <!-- Error Alert banner if present -->
    <UiAlert v-if="actionError" variant="error" class="action-error-bar">
      {{ actionError }}
    </UiAlert>

    <!-- 2. KPI Metrics Strip (app-screens §5: 4-part divided strip, tabular-nums) -->
    <section class="task-kpi-strip" aria-label="执行概览指标">
      <div class="kpi-cell">
        <span class="kpi-label">累计运行次数</span>
        <span class="kpi-value">{{ totalRunsCount }}</span>
        <span class="kpi-sub">历史所有执行</span>
      </div>
      <div class="kpi-cell">
        <span class="kpi-label">执行成功率</span>
        <span class="kpi-value">{{ successRate }}</span>
        <span class="kpi-sub">已完成任务比例</span>
      </div>
      <div class="kpi-cell">
        <span class="kpi-label">最近执行耗时</span>
        <span class="kpi-value">{{ lastDurationText }}</span>
        <span class="kpi-sub">最新一次执行消耗</span>
      </div>
      <div class="kpi-cell">
        <span class="kpi-label">计划调度状态</span>
        <span class="kpi-value text-sm md:text-base font-medium truncate">
          {{ task.scheduleEnabled ? (task.scheduleType === 'cron' ? task.cronExpr : '定时等待中') : '未开启自动调度' }}
        </span>
        <span class="kpi-sub truncate">
          {{ task.nextRunAt ? `下次: ${formatDateTime(task.nextRunAt)}` : '仅支持手动触发' }}
        </span>
      </div>
    </section>

    <!-- 3. Session Template & Execution Configuration -->
    <section class="section-card p-6 rounded-[var(--radius,6px)] bg-[var(--color-surface)] border border-[var(--color-border)] space-y-4">
      <div class="section-head pb-3 border-b border-[var(--color-border)] flex items-center justify-between gap-4">
        <div class="flex items-center gap-2">
          <svg class="text-[var(--color-text-muted)]" viewBox="0 0 16 16" width="14" height="14" fill="none" stroke="currentColor" stroke-width="1.6" aria-hidden="true">
            <rect x="2.5" y="2.5" width="11" height="11" rx="1.5" />
            <line x1="5.5" y1="6" x2="10.5" y2="6" />
            <line x1="5.5" y1="9" x2="8.5" y2="9" />
          </svg>
          <h2 class="text-sm font-semibold text-[var(--color-text)]">会话执行模板</h2>
        </div>
        <button
          type="button"
          class="text-xs text-[var(--color-text-muted)] hover:text-[var(--color-text)] inline-flex items-center gap-1 transition-colors"
          title="复制模板内容"
          @click="copyPromptTemplate"
        >
          <svg viewBox="0 0 16 16" width="12" height="12" fill="none" stroke="currentColor" stroke-width="1.6" aria-hidden="true">
            <rect x="5" y="5" width="8" height="8" rx="1" />
            <path d="M3 11V3h8" />
          </svg>
          <span>{{ copiedTemplate ? '已复制' : '复制模板' }}</span>
        </button>
      </div>

      <div class="template-box space-y-1.5">
        <div class="template-header text-xs text-[var(--color-text-muted)] font-medium">
          提示词模板内容（启动会话时自动发送给 Agent）
        </div>
        <pre class="template-content p-4 rounded-[var(--radius,6px)] bg-[var(--color-surface-muted)] border border-[var(--color-border)] text-xs font-mono whitespace-pre-wrap leading-relaxed text-[var(--color-text)] overflow-x-auto select-text">{{ task.promptTemplate }}</pre>
      </div>

      <!-- Scheduling Details if configured -->
      <div v-if="task.scheduleType !== 'manual'" class="schedule-summary-bar pt-3 border-t border-[var(--color-border)] text-xs text-[var(--color-text-muted)] flex items-center justify-between gap-4 flex-wrap">
        <div v-if="task.scheduleType === 'once'" class="flex items-center gap-2">
          <span class="font-medium text-[var(--color-text)]">定时执行时间：</span>
          <span class="text-[var(--color-text)] tabular-nums">{{ formatDateTime(task.scheduledAt) }}</span>
          <span class="status-badge" :class="task.scheduleEnabled ? 'badge-running' : 'badge-idle'">
            {{ task.scheduleEnabled ? '等待触发' : '已完成 / 已停用' }}
          </span>
        </div>
        <div v-else-if="task.scheduleType === 'cron'" class="flex items-center gap-2 flex-wrap">
          <span class="font-medium text-[var(--color-text)]">Cron 周期表达式：</span>
          <code class="px-2 py-0.5 bg-[var(--color-surface-muted)] text-[var(--color-text)] rounded border border-[var(--color-border)] font-mono">{{ task.cronExpr }}</code>
          <span class="status-badge" :class="task.scheduleEnabled ? 'badge-success' : 'badge-idle'">
            {{ task.scheduleEnabled ? '持续调度中' : '已停用' }}
          </span>
          <span v-if="task.nextRunAt" class="text-slate-400 text-[11px] tabular-nums">
            (预计下次运行: {{ formatDateTime(task.nextRunAt) }})
          </span>
        </div>
      </div>
    </section>

    <!-- 4. Execution History Table (app-screens §7: sentence-case, right-aligned numbers, no zebra) -->
    <section class="section-card history-section p-6 rounded-[var(--radius,6px)] bg-[var(--color-surface)] border border-[var(--color-border)] space-y-4">
      <div class="section-head flex items-center justify-between pb-3 border-b border-[var(--color-border)]">
        <div class="flex items-center gap-2">
          <svg class="text-[var(--color-text-muted)]" viewBox="0 0 16 16" width="14" height="14" fill="none" stroke="currentColor" stroke-width="1.6" aria-hidden="true">
            <circle cx="8" cy="8" r="6" />
            <polyline points="8 4.5 8 8 10.5 9.5" />
          </svg>
          <h2 class="text-sm font-semibold text-[var(--color-text)]">任务执行历史</h2>
        </div>
        <div class="head-right flex items-center gap-3">
          <span class="count-tag text-xs text-[var(--color-text-muted)] tabular-nums">共 {{ taskRuns.length }} 次记录</span>
          <UiButton size="sm" variant="ghost" class="btn-refresh text-xs" title="刷新历史记录" @click="refreshData">
            <svg viewBox="0 0 16 16" width="12" height="12" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true">
              <path d="M14 8A6 6 0 1 1 9.5 2.5" />
              <polyline points="14 2.5 14 6 10.5 6" />
            </svg>
            <span class="ml-1">刷新</span>
          </UiButton>
        </div>
      </div>

      <!-- Empty state -->
      <div v-if="taskRuns.length === 0" class="empty-runs-box py-10 text-center flex flex-col items-center justify-center gap-3">
        <div class="w-10 h-10 rounded-full bg-[var(--color-surface-muted)] border border-[var(--color-border)] flex items-center justify-center text-[var(--color-text-muted)]">
          <svg viewBox="0 0 16 16" width="18" height="18" fill="none" stroke="currentColor" stroke-width="1.5">
            <circle cx="8" cy="8" r="6" />
            <polyline points="8 4.5 8 8 10.5 9.5" />
          </svg>
        </div>
        <div class="space-y-1">
          <p class="text-sm font-medium text-[var(--color-text)]">暂无执行记录</p>
          <p class="text-xs text-[var(--color-text-muted)] max-w-sm">
            点击上方或下方的「手动执行」，系统将立即使用该模板启动一次独立的 Agent 会话。
          </p>
        </div>
        <UiButton
          variant="primary"
          size="sm"
          class="btn-trigger-inline mt-1 text-xs"
          :disabled="triggering"
          @click="handleManualTrigger"
        >
          立即开启首次执行
        </UiButton>
      </div>

      <!-- Data table -->
      <div v-else class="table-responsive overflow-x-auto rounded-[var(--radius,6px)] border border-[var(--color-border)]">
        <table class="history-table w-full text-left text-xs border-collapse" data-test="task-runs-table">
          <thead class="bg-[var(--color-surface-muted)] text-[var(--color-text-muted)] font-medium border-b border-[var(--color-border)]">
            <tr>
              <th class="py-2.5 px-3.5 text-left font-medium" style="width: 80px;">序号</th>
              <th class="py-2.5 px-3.5 text-left font-medium" style="width: 110px;">触发方式</th>
              <th class="py-2.5 px-3.5 text-left font-medium" style="width: 110px;">运行状态</th>
              <th class="py-2.5 px-3.5 text-left font-medium">触发时间</th>
              <th class="py-2.5 px-3.5 text-right font-medium num" style="width: 100px;">耗时</th>
              <th class="py-2.5 px-3.5 text-left font-medium">会话 ID</th>
              <th class="py-2.5 px-3.5 text-right font-medium" style="width: 110px;">操作</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-[var(--color-border)] text-[var(--color-text)]">
            <tr
              v-for="run in taskRuns"
              :key="run.id"
              class="run-row hover:bg-[var(--color-surface-muted)] transition-colors cursor-pointer select-none"
              data-test="task-run-row"
              @click="openRunSession(run)"
            >
              <td class="col-id py-2.5 px-3.5 tabular-nums text-[var(--color-text-muted)]">#{{ run.id }}</td>
              <td class="py-2.5 px-3.5">
                <span class="inline-flex text-[11px] font-medium text-[var(--color-text-muted)]">
                  {{
                    run.triggerType === 'cron'
                      ? '周期触发'
                      : run.triggerType === 'once'
                      ? '定时触发'
                      : '手动触发'
                  }}
                </span>
              </td>
              <td class="py-2.5 px-3.5">
                <span
                  class="status-badge"
                  :class="getRunStatusBadgeClass(run.status)"
                >
                  <span v-if="run.status === 'running'" class="w-1.5 h-1.5 rounded-full bg-blue-500"></span>
                  {{ getRunStatusLabel(run.status) }}
                </span>
              </td>
              <td class="col-time py-2.5 px-3.5 text-[var(--color-text-muted)] tabular-nums">
                {{ formatDateTime(run.triggeredAt) }}
              </td>
              <td class="col-duration py-2.5 px-3.5 text-right text-[var(--color-text-muted)] tabular-nums num">
                {{ calculateDuration(run) }}
              </td>
              <td class="col-session py-2.5 px-3.5">
                <code class="session-id-text font-mono text-[11px] text-[var(--color-text-muted)] bg-[var(--color-surface-muted)] px-1.5 py-0.5 rounded border border-[var(--color-border)]">{{ run.sessionId }}</code>
              </td>
              <td class="col-actions py-2.5 px-3.5 text-right" style="text-align: right;">
                <UiButton
                  size="sm"
                  variant="ghost"
                  class="btn-view-session text-xs text-[var(--color-primary)] hover:bg-[var(--color-surface)]"
                  data-test="btn-view-session"
                  @click.stop="openRunSession(run)"
                >
                  <svg class="inline-block mr-1" viewBox="0 0 16 16" width="12" height="12" fill="none" stroke="currentColor" stroke-width="1.6" aria-hidden="true">
                    <path d="M2.5 4h11v7H5l-2.5 2.5V4z" />
                  </svg>
                  <span>查看会话</span>
                </UiButton>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>

    <!-- Modals -->
    <TaskManageDialog
      v-if="showEditDialog"
      :task="task"
      @close="showEditDialog = false"
      @saved="() => { showEditDialog = false; refreshData() }"
    />

    <TaskChatDialog
      v-if="showChatDialog && activeSessionId && task"
      :title="`任务会话 [${viewingRun?.triggerType || '执行'}] - ${task.title}`"
      :project-id="task.projectId"
      :session-id="activeSessionId"
      :backend="task.backend"
      :model="task.model"
      @close="showChatDialog = false"
    />
  </div>
</template>

<style scoped>
@import './tasks-view.css';
</style>
