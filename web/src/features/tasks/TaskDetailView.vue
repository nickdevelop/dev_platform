<script setup lang="ts">
// Task Detail View: displays session template info, allows manual execution,
// shows execution history as a table, and opens the agent session in a modal.
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
import { UiButton, UiBadge, UiAlert, UiEmpty } from '../../shared/ui'

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
  if (sec < 60) return `${sec} 秒`
  const min = Math.floor(sec / 60)
  const remSec = sec % 60
  return `${min} 分 ${remSec} 秒`
}
</script>

<template>
  <div v-if="task" class="task-detail-container h-full overflow-y-auto p-6 space-y-6 bg-slate-50 dark:bg-slate-950" data-test="task-detail-view">
    <!-- Header Panel -->
    <header class="detail-header p-5 rounded bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 shadow-sm flex items-start justify-between gap-6 flex-wrap">
      <div class="header-main flex-1 min-w-[280px] space-y-2">
        <div class="badge-row flex items-center gap-2 flex-wrap">
          <UiBadge variant="primary" size="sm">
            {{ getTaskTypeLabel(task.taskType) }}
          </UiBadge>
          <span class="run-status-badge text-xs px-2 py-0.5 rounded font-medium border" :class="getRunStatusBadgeClass(task.lastRunStatus)">
            {{ getRunStatusLabel(task.lastRunStatus) }}
          </span>
          <UiBadge v-if="task.scheduleEnabled" variant="warning" size="sm">
            <svg class="inline-block mr-1" viewBox="0 0 16 16" width="11" height="11" fill="none" stroke="currentColor" stroke-width="1.8">
              <circle cx="8" cy="8" r="6.5" />
              <polyline points="8 4 8 8 11 10" />
            </svg>
            <span>{{ getScheduleTypeLabel(task.scheduleType) }}</span>
          </UiBadge>
        </div>

        <h1 class="task-title text-xl font-bold text-slate-900 dark:text-slate-100">{{ task.title }}</h1>
        <p v-if="task.description" class="task-desc text-xs text-slate-500 dark:text-slate-400 leading-relaxed">{{ task.description }}</p>

        <!-- Meta info -->
        <div class="meta-row flex items-center gap-4 text-xs text-slate-600 dark:text-slate-400 pt-2 flex-wrap">
          <div class="meta-item flex items-center gap-1.5">
            <span class="meta-label text-slate-400">关联项目:</span>
            <span class="meta-val font-medium text-slate-700 dark:text-slate-200">{{ projectName }}</span>
          </div>
          <div class="meta-item flex items-center gap-1.5">
            <span class="meta-label text-slate-400">Agent:</span>
            <BackendTag :backend="task.backend" />
          </div>
          <div class="meta-item flex items-center gap-1.5">
            <span class="meta-label text-slate-400">模型:</span>
            <code class="meta-val px-1.5 py-0.5 bg-slate-100 dark:bg-slate-800 rounded font-mono">{{ task.model }}</code>
          </div>
          <div class="meta-item flex items-center gap-1.5">
            <span class="meta-label text-slate-400">执行方式:</span>
            <span class="meta-val font-semibold text-slate-800 dark:text-slate-200">{{ getScheduleTypeLabel(task.scheduleType) }}</span>
          </div>
          <div v-if="task.nextRunAt" class="meta-item flex items-center gap-1.5 text-blue-600 dark:text-blue-400">
            <span class="meta-label">下次计划执行:</span>
            <span class="meta-val font-medium">{{ formatDateTime(task.nextRunAt) }}</span>
          </div>
        </div>
      </div>

      <!-- Action buttons -->
      <div class="header-actions flex items-center gap-2 shrink-0">
        <UiButton
          variant="primary"
          class="btn-trigger"
          :disabled="triggering"
          :loading="triggering"
          data-test="btn-trigger-task"
          @click="handleManualTrigger"
        >
          {{ triggering ? '正在启动…' : '▶ 手动执行' }}
        </UiButton>
        <UiButton
          variant="secondary"
          class="btn-edit"
          data-test="btn-edit-task"
          @click="showEditDialog = true"
        >
          编辑
        </UiButton>
        <UiButton
          variant="danger"
          class="btn-delete"
          data-test="btn-delete-task"
          @click="handleDelete"
        >
          删除
        </UiButton>
      </div>
    </header>

    <UiAlert v-if="actionError" variant="error" class="action-error-bar">
      {{ actionError }}
    </UiAlert>

    <!-- Template & Execution Config Overview -->
    <section class="section-card p-5 rounded bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 shadow-sm space-y-3">
      <div class="section-head pb-2 border-b border-slate-100 dark:border-slate-800 flex items-center gap-2">
        <svg class="text-slate-400" viewBox="0 0 16 16" width="14" height="14" fill="none" stroke="currentColor" stroke-width="1.6">
          <rect x="3" y="3" width="10" height="11" rx="1" />
          <path d="M6 1.5h4v2H6v-2zM5.5 7h5M5.5 10h3" />
        </svg>
        <h3 class="text-sm font-semibold text-slate-800 dark:text-slate-200">会话执行模板</h3>
      </div>
      <div class="template-box space-y-1.5">
        <div class="template-header text-xs text-slate-400 font-medium">提示词模板内容</div>
        <pre class="template-content p-3 rounded bg-slate-50 dark:bg-slate-800/40 border border-slate-200 dark:border-slate-700 text-xs font-mono whitespace-pre-wrap leading-relaxed text-slate-800 dark:text-slate-200">{{ task.promptTemplate }}</pre>
      </div>

      <!-- Scheduling Details if configured -->
      <div v-if="task.scheduleType !== 'manual'" class="schedule-summary-bar pt-2 text-xs text-slate-600 dark:text-slate-300">
        <div v-if="task.scheduleType === 'once'" class="schedule-summary-item flex items-center gap-2">
          <strong>定时执行时间：</strong>
          <span class="font-mono">{{ formatDateTime(task.scheduledAt) }}</span>
          <UiBadge :variant="task.scheduleEnabled ? 'warning' : 'default'" size="sm">
            {{ task.scheduleEnabled ? '等待执行' : '已完成/已停用' }}
          </UiBadge>
        </div>
        <div v-else-if="task.scheduleType === 'cron'" class="schedule-summary-item flex items-center gap-2 flex-wrap">
          <strong>周期表达式 (Cron)：</strong>
          <code class="px-1.5 py-0.5 bg-slate-100 dark:bg-slate-800 rounded font-mono">{{ task.cronExpr }}</code>
          <UiBadge :variant="task.scheduleEnabled ? 'success' : 'default'" size="sm">
            {{ task.scheduleEnabled ? '持续调度中' : '已停用' }}
          </UiBadge>
          <span v-if="task.nextRunAt" class="text-slate-400 text-[11px]">
            (预计下次执行: {{ formatDateTime(task.nextRunAt) }})
          </span>
        </div>
      </div>
    </section>

    <!-- Execution History Table -->
    <section class="section-card history-section p-5 rounded bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 shadow-sm space-y-4">
      <div class="section-head flex items-center justify-between pb-2 border-b border-slate-100 dark:border-slate-800">
        <div class="flex items-center gap-2">
          <svg class="text-slate-400" viewBox="0 0 16 16" width="14" height="14" fill="none" stroke="currentColor" stroke-width="1.6">
            <path d="M13 2H3a1 1 0 0 0-1 1v10a1 1 0 0 0 1 1h10a1 1 0 0 0 1-1V3a1 1 0 0 0-1-1z" />
            <line x1="5" y1="5.5" x2="11" y2="5.5" />
            <line x1="5" y1="8" x2="11" y2="8" />
            <line x1="5" y1="10.5" x2="9" y2="10.5" />
          </svg>
          <h3 class="text-sm font-semibold text-slate-800 dark:text-slate-200">任务执行历史</h3>
        </div>
        <div class="head-right flex items-center gap-2">
          <span class="count-tag text-xs text-slate-400">共 {{ taskRuns.length }} 次执行</span>
          <UiButton size="sm" variant="ghost" class="btn-refresh" title="刷新历史" @click="refreshData">
            <svg viewBox="0 0 16 16" width="12" height="12" fill="none" stroke="currentColor" stroke-width="1.8">
              <path d="M14 8A6 6 0 1 1 9.5 2.5" />
              <polyline points="14 2.5 14 6 10.5 6" />
            </svg>
          </UiButton>
        </div>
      </div>

      <div v-if="taskRuns.length === 0" class="empty-runs-box py-6 text-center">
        <UiEmpty
          title="暂无执行记录"
          description="点击右上角的「手动执行」按钮，系统将根据当前模板立即为您启动一次独立的 Agent 会话。"
        >
          <template #icon>
            <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="1.8">
              <circle cx="12" cy="12" r="9" />
              <polyline points="12 6 12 12 16 14" />
            </svg>
          </template>
          <template #actions>
            <UiButton
              variant="primary"
              size="sm"
              class="btn-trigger-inline"
              :disabled="triggering"
              @click="handleManualTrigger"
            >
              立即开始首次执行
            </UiButton>
          </template>
        </UiEmpty>
      </div>

      <div v-else class="table-responsive overflow-x-auto rounded border border-slate-100 dark:border-slate-800">
        <table class="history-table w-full text-left text-xs border-collapse" data-test="task-runs-table">
          <thead class="bg-slate-50 dark:bg-slate-800/60 text-slate-500 dark:text-slate-400 font-medium border-b border-slate-100 dark:border-slate-800">
            <tr>
              <th class="py-2.5 px-3" style="width: 70px;">编号</th>
              <th class="py-2.5 px-3" style="width: 120px;">触发方式</th>
              <th class="py-2.5 px-3" style="width: 120px;">执行状态</th>
              <th class="py-2.5 px-3">触发时间</th>
              <th class="py-2.5 px-3" style="width: 120px;">耗时</th>
              <th class="py-2.5 px-3">执行会话 ID</th>
              <th class="py-2.5 px-3 text-right" style="width: 110px;">操作</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-slate-100 dark:divide-slate-800 text-slate-700 dark:text-slate-300">
            <tr
              v-for="run in taskRuns"
              :key="run.id"
              class="run-row hover:bg-slate-50/50 dark:hover:bg-slate-800/30 transition-colors cursor-pointer"
              data-test="task-run-row"
              @click="openRunSession(run)"
            >
              <td class="col-id py-2.5 px-3 font-mono font-medium text-slate-500">#{{ run.id }}</td>
              <td class="py-2.5 px-3">
                <UiBadge variant="default" size="sm">
                  {{
                    run.triggerType === 'cron'
                      ? '周期触发'
                      : run.triggerType === 'once'
                      ? '定时触发'
                      : '手动触发'
                  }}
                </UiBadge>
              </td>
              <td class="py-2.5 px-3">
                <span class="run-status-badge text-[11px] px-2 py-0.5 rounded font-medium border inline-flex items-center gap-1" :class="getRunStatusBadgeClass(run.status)">
                  <span v-if="run.status === 'running'" class="w-1.5 h-1.5 rounded-full bg-blue-500 inline-block"></span>
                  {{ getRunStatusLabel(run.status) }}
                </span>
              </td>
              <td class="col-time py-2.5 px-3 font-mono text-slate-500">{{ formatDateTime(run.triggeredAt) }}</td>
              <td class="col-duration py-2.5 px-3 text-slate-500">{{ calculateDuration(run) }}</td>
              <td class="col-session py-2.5 px-3">
                <code class="session-id-text font-mono text-slate-600 dark:text-slate-400">{{ run.sessionId }}</code>
              </td>
              <td class="col-actions py-2.5 px-3 text-right" style="text-align: right;">
                <UiButton
                  size="sm"
                  variant="ghost"
                  class="btn-view-session"
                  data-test="btn-view-session"
                  @click.stop="openRunSession(run)"
                >
                  <svg class="inline-block mr-1" viewBox="0 0 16 16" width="12" height="12" fill="none" stroke="currentColor" stroke-width="1.6">
                    <path d="M2 4h12v7H5l-3 3V4z" />
                  </svg>
                  <span>查看会话</span>
                </UiButton>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>

    <!-- Modal Dialogs -->
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
