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
      // Open modal directly or show in table
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
  <div v-if="task" class="task-detail-container" data-test="task-detail-view">
    <!-- Header Panel -->
    <header class="detail-header">
      <div class="header-main">
        <div class="badge-row">
          <span class="type-pill" :class="task.taskType">
            {{ getTaskTypeLabel(task.taskType) }}
          </span>
          <span class="run-status-badge" :class="getRunStatusBadgeClass(task.lastRunStatus)">
            {{ getRunStatusLabel(task.lastRunStatus) }}
          </span>
          <span v-if="task.scheduleEnabled" class="schedule-pill">
            ⏰ {{ getScheduleTypeLabel(task.scheduleType) }}
          </span>
        </div>

        <h1 class="task-title">{{ task.title }}</h1>
        <p v-if="task.description" class="task-desc">{{ task.description }}</p>

        <!-- Meta info -->
        <div class="meta-row">
          <div class="meta-item">
            <span class="meta-label">关联项目:</span>
            <span class="meta-val">{{ projectName }}</span>
          </div>
          <div class="meta-item">
            <span class="meta-label">Agent:</span>
            <BackendTag :backend="task.backend" />
          </div>
          <div class="meta-item">
            <span class="meta-label">模型:</span>
            <code class="meta-val">{{ task.model }}</code>
          </div>
          <div class="meta-item">
            <span class="meta-label">执行方式:</span>
            <span class="meta-val font-semibold">{{ getScheduleTypeLabel(task.scheduleType) }}</span>
          </div>
          <div v-if="task.nextRunAt" class="meta-item">
            <span class="meta-label">下次计划执行:</span>
            <span class="meta-val highlight">{{ formatDateTime(task.nextRunAt) }}</span>
          </div>
        </div>
      </div>

      <!-- Action buttons -->
      <div class="header-actions">
        <button
          type="button"
          class="btn-trigger"
          :disabled="triggering"
          data-test="btn-trigger-task"
          @click="handleManualTrigger"
        >
          {{ triggering ? '正在启动…' : '▶ 手动执行' }}
        </button>
        <button
          type="button"
          class="btn-edit"
          data-test="btn-edit-task"
          @click="showEditDialog = true"
        >
          编辑
        </button>
        <button
          type="button"
          class="btn-delete"
          data-test="btn-delete-task"
          @click="handleDelete"
        >
          删除
        </button>
      </div>
    </header>

    <p v-if="actionError" class="action-error-bar" role="alert">{{ actionError }}</p>

    <!-- Template & Execution Config Overview -->
    <section class="section-card">
      <div class="section-head">
        <h3>📋 会话执行模板</h3>
      </div>
      <div class="template-box">
        <div class="template-header">
          <span class="template-title">提示词模板内容</span>
        </div>
        <pre class="template-content">{{ task.promptTemplate }}</pre>
      </div>

      <!-- Scheduling Details if configured -->
      <div v-if="task.scheduleType !== 'manual'" class="schedule-summary-bar">
        <div v-if="task.scheduleType === 'once'" class="schedule-summary-item">
          <strong>定时执行时间：</strong>
          <span>{{ formatDateTime(task.scheduledAt) }}</span>
          <span class="status-tag" :class="{ enabled: task.scheduleEnabled }">
            {{ task.scheduleEnabled ? '等待执行' : '已完成/已停用' }}
          </span>
        </div>
        <div v-else-if="task.scheduleType === 'cron'" class="schedule-summary-item">
          <strong>周期表达式 (Cron)：</strong>
          <code>{{ task.cronExpr }}</code>
          <span class="status-tag" :class="{ enabled: task.scheduleEnabled }">
            {{ task.scheduleEnabled ? '持续调度中' : '已停用' }}
          </span>
          <span v-if="task.nextRunAt" class="next-run-hint">
            (预计下次执行: {{ formatDateTime(task.nextRunAt) }})
          </span>
        </div>
      </div>
    </section>

    <!-- Execution History Table -->
    <section class="section-card history-section">
      <div class="section-head">
        <h3>📜 任务执行历史</h3>
        <div class="head-right">
          <span class="count-tag">共 {{ taskRuns.length }} 次执行</span>
          <button type="button" class="btn-refresh" title="刷新历史" @click="refreshData">🔄</button>
        </div>
      </div>

      <div v-if="taskRuns.length === 0" class="empty-runs-box">
        <p class="empty-icon">⏳</p>
        <h4>暂无执行记录</h4>
        <p class="empty-desc">
          点击右上角的「手动执行」按钮，系统将根据当前模板立即为您启动一次独立的 Agent 会话。
        </p>
        <button
          type="button"
          class="btn-trigger-inline"
          :disabled="triggering"
          @click="handleManualTrigger"
        >
          立即开始首次执行
        </button>
      </div>

      <div v-else class="table-responsive">
        <table class="history-table" data-test="task-runs-table">
          <thead>
            <tr>
              <th style="width: 70px;">编号</th>
              <th style="width: 120px;">触发方式</th>
              <th style="width: 120px;">执行状态</th>
              <th>触发时间</th>
              <th style="width: 120px;">耗时</th>
              <th>执行会话 ID</th>
              <th style="width: 110px; text-align: right;">操作</th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="run in taskRuns"
              :key="run.id"
              class="run-row"
              data-test="task-run-row"
              @click="openRunSession(run)"
            >
              <td class="col-id">#{{ run.id }}</td>
              <td>
                <span class="trigger-pill" :class="run.triggerType">
                  {{
                    run.triggerType === 'cron'
                      ? '周期触发'
                      : run.triggerType === 'once'
                      ? '定时触发'
                      : '手动触发'
                  }}
                </span>
              </td>
              <td>
                <span class="run-status-badge" :class="getRunStatusBadgeClass(run.status)">
                  <span v-if="run.status === 'running'" class="spinner-dot"></span>
                  {{ getRunStatusLabel(run.status) }}
                </span>
              </td>
              <td class="col-time">{{ formatDateTime(run.triggeredAt) }}</td>
              <td class="col-duration">{{ calculateDuration(run) }}</td>
              <td class="col-session">
                <code class="session-id-text">{{ run.sessionId }}</code>
              </td>
              <td class="col-actions" style="text-align: right;">
                <button
                  type="button"
                  class="btn-view-session"
                  data-test="btn-view-session"
                  @click.stop="openRunSession(run)"
                >
                  💬 查看会话
                </button>
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

<style scoped>
.task-detail-container {
  display: flex;
  flex-direction: column;
  padding: 1.5rem 2rem;
  overflow-y: auto;
  height: 100%;
  gap: 1.5rem;
  background: var(--color-bg);
}

.detail-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  padding: 1.5rem;
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: 12px;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.04);
}

.header-main {
  flex: 1;
  min-width: 0;
}

.badge-row {
  display: flex;
  align-items: center;
  gap: 0.6rem;
  margin-bottom: 0.5rem;
}

.type-pill {
  font-size: 0.75rem;
  font-weight: 600;
  padding: 0.2rem 0.55rem;
  border-radius: 4px;
  background: rgba(59, 130, 246, 0.12);
  color: #3b82f6;
}

.type-pill.flow {
  background: rgba(168, 85, 247, 0.12);
  color: #a855f7;
}

.run-status-badge {
  display: inline-flex;
  align-items: center;
  gap: 0.35rem;
  font-size: 0.75rem;
  font-weight: 600;
  padding: 0.2rem 0.55rem;
  border-radius: 4px;
}

.badge-running {
  background: rgba(59, 130, 246, 0.15);
  color: #2563eb;
}

.badge-success {
  background: rgba(16, 185, 129, 0.15);
  color: #10b981;
}

.badge-failed {
  background: rgba(239, 68, 68, 0.15);
  color: #ef4444;
}

.badge-idle {
  background: rgba(156, 163, 175, 0.15);
  color: var(--color-text-muted);
}

.spinner-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: #2563eb;
  animation: pulse 1s infinite alternate;
}

@keyframes pulse {
  0% { transform: scale(0.8); opacity: 0.6; }
  100% { transform: scale(1.3); opacity: 1; }
}

.schedule-pill {
  font-size: 0.75rem;
  padding: 0.2rem 0.55rem;
  border-radius: 4px;
  background: var(--color-surface-muted);
  color: var(--color-text-muted);
  border: 1px solid var(--color-border);
}

.task-title {
  font-size: 1.45rem;
  font-weight: 700;
  color: var(--color-text);
  margin: 0.25rem 0 0.4rem 0;
}

.task-desc {
  font-size: 0.9rem;
  color: var(--color-text-muted);
  margin: 0 0 0.85rem 0;
  line-height: 1.4;
}

.meta-row {
  display: flex;
  flex-wrap: wrap;
  gap: 1.25rem;
  font-size: 0.85rem;
  color: var(--color-text-muted);
}

.meta-item {
  display: flex;
  align-items: center;
  gap: 0.35rem;
}

.meta-val {
  color: var(--color-text);
}

.font-semibold {
  font-weight: 600;
}

.highlight {
  color: var(--color-primary, #3b82f6);
  font-weight: 600;
}

.header-actions {
  display: flex;
  gap: 0.6rem;
  margin-left: 1.5rem;
}

.btn-trigger {
  padding: 0.55rem 1.2rem;
  background: var(--color-primary, #3b82f6);
  color: #fff;
  border: none;
  border-radius: 6px;
  font-weight: 600;
  font-size: 0.9rem;
  cursor: pointer;
  transition: opacity 0.15s;
}

.btn-trigger:hover:not(:disabled) {
  opacity: 0.9;
}

.btn-trigger:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

.btn-edit,
.btn-delete {
  padding: 0.55rem 0.9rem;
  border: 1px solid var(--color-border);
  border-radius: 6px;
  background: var(--color-surface);
  color: var(--color-text);
  cursor: pointer;
  font-size: 0.88rem;
  transition: all 0.15s;
}

.btn-edit:hover {
  background: var(--color-surface-muted);
}

.btn-delete:hover {
  border-color: #ef4444;
  color: #ef4444;
}

.action-error-bar {
  padding: 0.65rem 1rem;
  background: rgba(239, 68, 68, 0.1);
  color: #ef4444;
  border-radius: 6px;
  font-size: 0.88rem;
}

.section-card {
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: 12px;
  padding: 1.25rem 1.5rem;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.04);
}

.section-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 1rem;
}

.section-head h3 {
  font-size: 1.05rem;
  font-weight: 700;
  color: var(--color-text);
  margin: 0;
}

.head-right {
  display: flex;
  align-items: center;
  gap: 0.6rem;
}

.count-tag {
  font-size: 0.8rem;
  color: var(--color-text-muted);
}

.btn-refresh {
  background: none;
  border: 1px solid var(--color-border);
  border-radius: 4px;
  cursor: pointer;
  padding: 0.2rem 0.4rem;
  font-size: 0.85rem;
}

.template-box {
  background: var(--color-surface-muted);
  border: 1px solid var(--color-border);
  border-radius: 8px;
  overflow: hidden;
}

.template-header {
  padding: 0.4rem 0.75rem;
  background: rgba(0, 0, 0, 0.03);
  border-bottom: 1px solid var(--color-border);
  font-size: 0.78rem;
  font-weight: 600;
  color: var(--color-text-muted);
}

.template-content {
  padding: 0.75rem 1rem;
  margin: 0;
  font-family: inherit;
  font-size: 0.88rem;
  color: var(--color-text);
  white-space: pre-wrap;
  word-break: break-word;
  line-height: 1.45;
  max-height: 12rem;
  overflow-y: auto;
}

.schedule-summary-bar {
  margin-top: 0.85rem;
  padding: 0.65rem 0.9rem;
  background: var(--color-surface-muted);
  border-radius: 6px;
  font-size: 0.85rem;
}

.schedule-summary-item {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  flex-wrap: wrap;
}

.status-tag {
  font-size: 0.75rem;
  padding: 0.15rem 0.4rem;
  border-radius: 4px;
  background: rgba(156, 163, 175, 0.2);
  color: var(--color-text-muted);
}

.status-tag.enabled {
  background: rgba(16, 185, 129, 0.15);
  color: #10b981;
}

.next-run-hint {
  color: var(--color-text-muted);
  font-size: 0.8rem;
}

/* History Table */
.empty-runs-box {
  text-align: center;
  padding: 2.5rem 1rem;
}

.empty-icon {
  font-size: 2rem;
  margin-bottom: 0.5rem;
}

.empty-runs-box h4 {
  font-size: 1.05rem;
  color: var(--color-text);
  margin: 0 0 0.35rem 0;
}

.empty-desc {
  font-size: 0.85rem;
  color: var(--color-text-muted);
  max-width: 28rem;
  margin: 0 auto 1.25rem auto;
}

.btn-trigger-inline {
  padding: 0.5rem 1.2rem;
  background: var(--color-primary, #3b82f6);
  color: #fff;
  border: none;
  border-radius: 6px;
  font-weight: 600;
  font-size: 0.88rem;
  cursor: pointer;
}

.table-responsive {
  overflow-x: auto;
}

.history-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 0.88rem;
}

.history-table th {
  text-align: left;
  padding: 0.65rem 0.75rem;
  border-bottom: 1.5px solid var(--color-border);
  color: var(--color-text-muted);
  font-weight: 600;
  font-size: 0.8rem;
  text-transform: uppercase;
  letter-spacing: 0.03em;
}

.history-table td {
  padding: 0.75rem;
  border-bottom: 1px solid var(--color-border);
  color: var(--color-text);
  vertical-align: middle;
}

.run-row {
  cursor: pointer;
  transition: background 0.12s;
}

.run-row:hover {
  background: var(--color-surface-muted);
}

.col-id {
  font-weight: 600;
  color: var(--color-text-muted);
}

.trigger-pill {
  font-size: 0.75rem;
  padding: 0.15rem 0.45rem;
  border-radius: 4px;
  background: var(--color-surface-muted);
  border: 1px solid var(--color-border);
}

.trigger-pill.cron {
  color: #8b5cf6;
  border-color: rgba(139, 92, 246, 0.3);
}

.trigger-pill.once {
  color: #f59e0b;
  border-color: rgba(245, 158, 11, 0.3);
}

.col-time,
.col-duration {
  color: var(--color-text-muted);
  font-size: 0.82rem;
}

.session-id-text {
  font-family: monospace;
  font-size: 0.82rem;
  background: var(--color-surface-muted);
  padding: 0.15rem 0.35rem;
  border-radius: 4px;
}

.btn-view-session {
  padding: 0.35rem 0.65rem;
  font-size: 0.8rem;
  border: 1px solid var(--color-border);
  border-radius: 4px;
  background: var(--color-surface);
  color: var(--color-primary, #3b82f6);
  cursor: pointer;
  font-weight: 500;
  transition: all 0.15s;
}

.btn-view-session:hover {
  background: var(--color-primary, #3b82f6);
  color: #fff;
  border-color: var(--color-primary, #3b82f6);
}
</style>
