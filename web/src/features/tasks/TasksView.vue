<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import type { FlowType, Task, TaskStatus } from '../../api/tasks'
import BackendTag from '../../shared/components/BackendTag.vue'
import { useAgentsStore } from '../../stores/agents'
import { useProjectsStore } from '../../stores/projects'
import { useTasksStore } from '../../stores/tasks'
import SessionChatPanel from '../chat/SessionChatPanel.vue'
import FlowStepper from './FlowStepper.vue'
import ScheduleConfigDialog from './ScheduleConfigDialog.vue'
import TaskManageDialog from './TaskManageDialog.vue'

const route = useRoute()
const router = useRouter()
const tasksStore = useTasksStore()
const projectsStore = useProjectsStore()
const agentsStore = useAgentsStore()

const searchKeyword = ref('')
const selectedProjectFilter = ref<number | undefined>(undefined)
const selectedFlowFilter = ref<FlowType | undefined>(undefined)
const selectedStatusFilter = ref<TaskStatus | undefined>(undefined)

const activeTaskId = ref<number | null>(null)
const showManageDialog = ref(false)
const editingTask = ref<Task | null>(null)
const showScheduleDialog = ref(false)
const schedulingTask = ref<Task | null>(null)

const creatingSession = ref(false)
const triggering = ref(false)
const actionError = ref('')

// The status filter offers every status a task can hold, in flow order. The
// labels match the badges in the list so a filter reads like the status it keeps.
const statusOptions: TaskStatus[] = [
  'draft',
  'in_progress',
  'ready_for_execution',
  'executing',
  'completed',
  'failed',
]

onMounted(async () => {
  await Promise.all([
    tasksStore.loadFlowDefs(),
    projectsStore.load(),
    agentsStore.loadBackends(),
    tasksStore.loadTasks(),
  ])

  if (route.params.taskId) {
    activeTaskId.value = Number(route.params.taskId)
  } else if (tasksStore.tasks.length > 0) {
    activeTaskId.value = tasksStore.tasks[0].id
  }
})

watch(
  () => route.params.taskId,
  (newId) => {
    if (newId) {
      activeTaskId.value = Number(newId)
    }
  },
)

const filteredTasks = computed(() => {
  return tasksStore.tasks.filter((t) => {
    if (searchKeyword.value.trim()) {
      const q = searchKeyword.value.trim().toLowerCase()
      const matchTitle = t.title.toLowerCase().includes(q)
      const matchDesc = t.description?.toLowerCase().includes(q)
      if (!matchTitle && !matchDesc) return false
    }
    if (selectedProjectFilter.value && t.projectId !== selectedProjectFilter.value) {
      return false
    }
    if (selectedFlowFilter.value && t.flowType !== selectedFlowFilter.value) {
      return false
    }
    if (selectedStatusFilter.value && t.status !== selectedStatusFilter.value) {
      return false
    }
    return true
  })
})

const activeTask = computed(() => {
  if (!activeTaskId.value) return null
  return tasksStore.byId(activeTaskId.value) ?? null
})

const activeFlowDef = computed(() => {
  if (!activeTask.value) return undefined
  return tasksStore.flowDefByType(activeTask.value.flowType)
})

const isFinalStep = computed(() => {
  if (!activeTask.value) return false
  const maxStep = activeTask.value.flowType === 'transactional' ? 2 : 4
  return activeTask.value.currentStep === maxStep
})

function selectTask(id: number) {
  activeTaskId.value = id
  void router.push({ name: 'task-detail', params: { taskId: id } })
}

function openCreateDialog() {
  editingTask.value = null
  showManageDialog.value = true
}

function openEditDialog(t: Task) {
  editingTask.value = t
  showManageDialog.value = true
}

function openScheduleDialog(t: Task) {
  schedulingTask.value = t
  showScheduleDialog.value = true
}

async function deleteTask(t: Task) {
  if (!confirm(`确定要删除任务「${t.title}」吗？`)) return
  await tasksStore.deleteTask(t.id)
  if (activeTaskId.value === t.id) {
    activeTaskId.value = tasksStore.tasks.length > 0 ? tasksStore.tasks[0].id : null
    if (activeTaskId.value) {
      void router.push({ name: 'task-detail', params: { taskId: activeTaskId.value } })
    } else {
      void router.push({ name: 'tasks' })
    }
  }
}

async function handleAdvanceStep(confirmed: boolean, notes?: string) {
  if (!activeTask.value) return
  actionError.value = ''
  try {
    await tasksStore.advanceStep(activeTask.value.id, { confirmed, notes })
  } catch (err: any) {
    actionError.value = err.message || '步骤推进失败'
  }
}

async function createSessionForTask() {
  if (!activeTask.value) return
  creatingSession.value = true
  actionError.value = ''
  try {
    await tasksStore.createSession(activeTask.value.id)
  } catch (err: any) {
    actionError.value = err.message || '创建会话失败'
  } finally {
    creatingSession.value = false
  }
}

async function triggerExecutionNow() {
  if (!activeTask.value) return
  triggering.value = true
  actionError.value = ''
  try {
    await tasksStore.triggerExecution(activeTask.value.id)
  } catch (err: any) {
    actionError.value = err.message || '触发执行失败'
  } finally {
    triggering.value = false
  }
}

function getProjectName(pid: number) {
  return projectsStore.byId(pid)?.name ?? `项目 #${pid}`
}

function getStatusBadgeClass(status: TaskStatus) {
  switch (status) {
    case 'ready_for_execution':
      return 'badge-ready'
    case 'executing':
      return 'badge-executing'
    case 'completed':
      return 'badge-completed'
    case 'failed':
      return 'badge-failed'
    case 'in_progress':
      return 'badge-progress'
    default:
      return 'badge-draft'
  }
}

function getStatusLabel(status: TaskStatus) {
  switch (status) {
    case 'draft':
      return '草稿'
    case 'in_progress':
      return '进行中'
    case 'ready_for_execution':
      return '就绪待执行'
    case 'executing':
      return '执行中'
    case 'completed':
      return '已完成'
    case 'failed':
      return '执行失败'
    default:
      return status
  }
}
</script>

<template>
  <div class="tasks-workbench">
    <!-- Middle Column: Task List & Filters. Mirrors the project sidebar: same
         width, head, search and row metrics, so switching between the project
         and task centers is one continuous surface. -->
    <aside class="tasks-sidebar" aria-label="任务列表">
      <div class="sidebar-head">
        <h2>任务</h2>
        <div class="head-actions">
          <button type="button" class="new-task" @click="openCreateDialog">新建</button>
        </div>
      </div>

      <input
        v-model="searchKeyword"
        type="search"
        class="search"
        placeholder="搜索任务名称或描述"
        aria-label="搜索任务"
      />

      <div class="filters">
        <select v-model="selectedProjectFilter" class="filter-select" aria-label="按项目筛选">
          <option :value="undefined">全部项目</option>
          <option v-for="p in projectsStore.projects" :key="p.id" :value="p.id">
            {{ p.name }}
          </option>
        </select>

        <select v-model="selectedFlowFilter" class="filter-select" aria-label="按流程筛选">
          <option :value="undefined">全部流程</option>
          <option value="transactional">事务型</option>
          <option value="development">开发类</option>
        </select>

        <select
          v-model="selectedStatusFilter"
          class="filter-select status-filter"
          aria-label="按状态筛选"
        >
          <option :value="undefined">全部状态</option>
          <option v-for="status in statusOptions" :key="status" :value="status">
            {{ getStatusLabel(status) }}
          </option>
        </select>
      </div>

      <!-- Task items list -->
      <div class="task-list">
        <p v-if="filteredTasks.length === 0" class="empty-list">暂无匹配任务</p>
        <div
          v-for="t in filteredTasks"
          :key="t.id"
          class="task-card"
          :class="{ active: activeTaskId === t.id }"
          @click="selectTask(t.id)"
        >
          <div class="card-top">
            <span class="flow-pill" :class="t.flowType">
              {{ t.flowType === 'transactional' ? '事务型' : '开发类' }}
            </span>
            <span class="status-badge" :class="getStatusBadgeClass(t.status)">
              {{ getStatusLabel(t.status) }}
            </span>
          </div>

          <h4 class="card-title">{{ t.title }}</h4>

          <div class="card-meta">
            <span class="meta-project">{{ getProjectName(t.projectId) }}</span>
            <span class="meta-step">
              步骤 {{ t.currentStep }}/{{ t.flowType === 'transactional' ? 2 : 4 }}
            </span>
          </div>

          <div v-if="t.scheduleEnabled" class="schedule-tag">
            <span class="tag-icon">⏰</span>
            <span v-if="t.scheduleType === 'cron'">周期调度: {{ t.cronExpr }}</span>
            <span v-else>定时调度</span>
          </div>
        </div>
      </div>
    </aside>

    <!-- Right Column: Task Detail & Flow Execution Workspace -->
    <main class="tasks-content">
      <div v-if="!activeTask" class="empty-workbench">
        <div class="empty-card">
          <div class="empty-icon">📋</div>
          <h3>选择任务或新建任务</h3>
          <p>从左侧列表中选择一个任务以查看其编排流程、会话与调度配置，或点击上方新建任务。</p>
          <button type="button" class="btn-launch-session" @click="openCreateDialog">
            + 立即新建任务
          </button>
        </div>
      </div>

      <div v-else class="task-detail-pane">
        <!-- Top Info Header -->
        <header class="detail-header-panel">
          <div class="title-section">
            <div class="title-row">
              <span class="flow-badge" :class="activeTask.flowType">
                {{ activeTask.flowType === 'transactional' ? '事务型流程' : '开发类流程' }}
              </span>
              <h1 class="task-main-title">{{ activeTask.title }}</h1>
              <span class="status-badge-lg" :class="getStatusBadgeClass(activeTask.status)">
                {{ getStatusLabel(activeTask.status) }}
              </span>
            </div>
            <p v-if="activeTask.description" class="task-main-desc">
              {{ activeTask.description }}
            </p>

            <div class="meta-bar">
              <span class="meta-item">
                <span class="label">项目:</span>
                <strong>{{ getProjectName(activeTask.projectId) }}</strong>
              </span>
              <span class="meta-item">
                <span class="label">Agent:</span>
                <BackendTag :backend="activeTask.backend" />
              </span>
              <span class="meta-item">
                <span class="label">模型:</span>
                <code>{{ activeTask.model }}</code>
              </span>
              <span v-if="activeTask.scheduleEnabled" class="meta-item schedule-active">
                <span class="label">调度:</span>
                <strong v-if="activeTask.scheduleType === 'cron'">周期 ({{ activeTask.cronExpr }})</strong>
                <strong v-else>定时执行</strong>
              </span>
            </div>
          </div>

          <!-- Actions -->
          <div class="header-actions">
            <button
              type="button"
              class="btn-schedule"
              :class="{ active: activeTask.scheduleEnabled }"
              @click="openScheduleDialog(activeTask)"
              title="配置定时/周期调度"
            >
              ⏰ 调度配置
            </button>
            <button
              v-if="isFinalStep && activeTask.isConfirmed"
              type="button"
              class="btn-trigger"
              :disabled="triggering"
              @click="triggerExecutionNow"
              title="立即执行任务最终步骤"
            >
              ▶ 立即执行
            </button>
            <button
              type="button"
              class="btn-secondary"
              @click="openEditDialog(activeTask)"
              title="编辑任务基本信息"
            >
              编辑
            </button>
            <button
              type="button"
              class="btn-danger"
              @click="deleteTask(activeTask)"
              title="删除任务"
            >
              删除
            </button>
          </div>
        </header>

        <p v-if="actionError" class="action-error-banner" role="alert">{{ actionError }}</p>

        <!-- Orchestration Flow Stepper -->
        <section class="stepper-section">
          <FlowStepper
            :task="activeTask"
            :flow-def="activeFlowDef"
            @advance="handleAdvanceStep"
          />
        </section>

        <!-- Session Execution Area -->
        <section class="session-section">
          <div class="section-title-bar">
            <h3>任务执行会话</h3>
            <span v-if="activeTask.activeSessionId" class="session-id-tag">
              Session: {{ activeTask.activeSessionId }}
            </span>
          </div>

          <!-- If no session created yet -->
          <div v-if="!activeTask.activeSessionId" class="no-session-card">
            <div class="no-session-content">
              <h4>尚未创建执行会话</h4>
              <p>
                已为此任务配置了 <strong>{{ activeTask.backend }}</strong> Agent 与
                <strong>{{ activeTask.model }}</strong> 模型。
                点击下方按钮即可立即在任务界面中拉起专属会话，按照流程步骤指引开始交互与执行。
              </p>
              <button
                type="button"
                class="btn-launch-session"
                :disabled="creatingSession"
                @click="createSessionForTask"
              >
                {{ creatingSession ? '正在创建会话...' : '🚀 创建并启动任务会话' }}
              </button>
            </div>
          </div>

          <!-- Embedded Session Chat Panel -->
          <div v-else class="embedded-chat-panel">
            <SessionChatPanel
              :key="activeTask.activeSessionId"
              :project-id="activeTask.projectId"
              :session-id="activeTask.activeSessionId"
            />
          </div>
        </section>
      </div>
    </main>

    <!-- Dialogs -->
    <TaskManageDialog
      v-if="showManageDialog"
      :task="editingTask"
      @close="showManageDialog = false"
      @saved="(t) => { activeTaskId = t.id }"
    />

    <ScheduleConfigDialog
      v-if="showScheduleDialog && schedulingTask"
      :task="schedulingTask"
      @close="showScheduleDialog = false"
      @updated="(t) => { schedulingTask = t }"
    />
  </div>
</template>

<style scoped>
/* The workbench is the task center's version of the shell's own two columns:
   the list column matches the project sidebar's 272px and the detail column
   matches the pane. minmax(0, 1fr) on the row is what keeps a long task list
   scrolling inside its column instead of stretching the grid. */
.tasks-workbench {
  display: grid;
  grid-template-columns: 272px minmax(0, 1fr);
  grid-template-rows: minmax(0, 1fr);
  height: 100%;
  overflow: hidden;
  background: var(--color-bg);
}

/* --- List column (same metrics as the project sidebar) --- */

.tasks-sidebar {
  display: flex;
  flex-direction: column;
  min-width: 0;
  overflow: hidden;
  background: var(--color-surface);
  border-right: 1px solid var(--color-border);
}

.sidebar-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: calc(var(--space-unit) * 2.333) calc(var(--space-unit) * 2.333) calc(var(--space-unit) * 1.333);
}

.sidebar-head h2 {
  font-size: 1.071rem;
  margin: 0;
}

.head-actions {
  display: flex;
  gap: calc(var(--space-unit) * 0.667);
}

.head-actions button {
  padding: calc(var(--space-unit) * 0.5) calc(var(--space-unit) * 1.333);
  border: 1px solid var(--color-border-strong);
  border-radius: var(--radius);
  background: var(--color-surface);
  color: var(--color-text);
  cursor: pointer;
  font: inherit;
  font-size: 0.857rem;
}

.search {
  margin: 0 calc(var(--space-unit) * 2) calc(var(--space-unit) * 1.333);
  padding: calc(var(--space-unit) * 1) calc(var(--space-unit) * 1.667);
  border: 1px solid var(--color-border-strong);
  border-radius: var(--radius);
  background: var(--color-surface);
  color: var(--color-text);
  font: inherit;
  font-size: 0.929rem;
}

.filters {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: calc(var(--space-unit) * 0.667);
  padding: 0 calc(var(--space-unit) * 2) calc(var(--space-unit) * 1.333);
  border-bottom: 1px solid var(--color-border);
}

.filter-select {
  min-width: 0;
  height: var(--control-height);
  padding: 0 calc(var(--space-unit) * 1);
  border: 1px solid var(--color-border-strong);
  border-radius: var(--radius);
  background: var(--color-surface);
  color: var(--color-text);
  font: inherit;
  font-size: 0.857rem;
}

/* Three filters, two columns: the status filter takes the free row. */
.filter-select.status-filter {
  grid-column: 1 / -1;
}

.task-list {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: calc(var(--space-unit) * 1) calc(var(--space-unit) * 1.333) calc(var(--space-unit) * 2);
  display: flex;
  flex-direction: column;
  gap: calc(var(--space-unit) * 1);
}

.empty-list {
  text-align: center;
  color: var(--color-text-muted);
  font-size: 0.857rem;
  padding: calc(var(--space-unit) * 4) 0;
}

.task-card {
  display: flex;
  flex-direction: column;
  gap: calc(var(--space-unit) * 0.667);
  padding: calc(var(--space-unit) * 1.333);
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  background: var(--color-surface);
  cursor: pointer;
}

.task-card:hover {
  background: var(--color-surface-muted);
}

.task-card.active {
  border-color: var(--color-primary);
  background: var(--color-primary-soft);
}

.card-top {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: calc(var(--space-unit) * 0.5);
}

.flow-pill,
.flow-badge {
  font-weight: 600;
  border-radius: var(--radius);
  line-height: 1.7;
}

.flow-pill {
  font-size: 0.714rem;
  padding: 0 calc(var(--space-unit) * 0.667);
}

.flow-badge {
  font-size: 0.786rem;
  padding: 0 calc(var(--space-unit) * 0.833);
}

/* A flow type is a category, not a state: the accent marks the transactional
   flow and the info color the development one. */
.flow-pill.transactional,
.flow-badge.transactional {
  background: var(--color-accent-soft);
  color: var(--color-accent);
}

.flow-pill.development,
.flow-badge.development {
  background: var(--color-info-soft);
  color: var(--color-info);
}

.status-badge,
.status-badge-lg {
  border-radius: 999px;
  line-height: 1.7;
  white-space: nowrap;
}

.status-badge {
  font-size: 0.714rem;
  padding: 0 calc(var(--space-unit) * 0.667);
}

.status-badge-lg {
  font-size: 0.8rem;
  padding: 0 calc(var(--space-unit) * 1);
}

.badge-draft {
  background: var(--color-surface-muted);
  color: var(--color-text-muted);
}

.badge-progress {
  background: var(--color-warning-soft);
  color: var(--color-warning);
}

.badge-ready,
.badge-completed {
  background: var(--color-success-soft);
  color: var(--color-success);
}

.badge-executing {
  background: var(--color-info-soft);
  color: var(--color-info);
}

.badge-failed {
  background: var(--color-danger-soft);
  color: var(--color-danger);
}

.card-title {
  margin: 0;
  font-size: 0.929rem;
  font-weight: 600;
  color: var(--color-text);
  line-height: 1.3;
}

.card-meta {
  display: flex;
  justify-content: space-between;
  gap: calc(var(--space-unit) * 0.5);
  font-size: 0.786rem;
  color: var(--color-text-muted);
}

.schedule-tag {
  display: inline-flex;
  align-items: center;
  gap: calc(var(--space-unit) * 0.333);
  align-self: flex-start;
  font-size: 0.75rem;
  color: var(--color-warning);
  background: var(--color-warning-soft);
  padding: calc(var(--space-unit) * 0.25) calc(var(--space-unit) * 0.667);
  border-radius: var(--radius);
}

/* --- Detail column (same inset as an ordinary pane body) --- */

.tasks-content {
  min-width: 0;
  min-height: 0;
  overflow-y: auto;
}

.empty-workbench {
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: calc(var(--space-unit) * 3);
}

.empty-card {
  text-align: center;
  max-width: 27rem;
  padding: calc(var(--space-unit) * 5);
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
}

.empty-icon {
  font-size: 3rem;
  margin-bottom: calc(var(--space-unit) * 1.333);
}

.empty-card h3 {
  margin: 0 0 calc(var(--space-unit) * 1) 0;
  font-size: 1.25rem;
}

.empty-card p {
  color: var(--color-text-muted);
  font-size: 0.9rem;
  line-height: 1.5;
  margin-bottom: calc(var(--space-unit) * 3);
}

.task-detail-pane {
  display: flex;
  flex-direction: column;
  gap: calc(var(--space-unit) * 3);
  padding: calc(var(--space-unit) * 3) calc(var(--space-unit) * 3.333);
}

.detail-header-panel {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: calc(var(--space-unit) * 3);
  padding-bottom: calc(var(--space-unit) * 2);
  border-bottom: 1px solid var(--color-border);
}

.title-section {
  display: flex;
  flex-direction: column;
  gap: calc(var(--space-unit) * 1);
  min-width: 0;
}

.title-row {
  display: flex;
  align-items: center;
  gap: calc(var(--space-unit) * 1.333);
  flex-wrap: wrap;
}

.task-main-title {
  margin: 0;
  font-size: 1.25rem;
  font-weight: 700;
  word-break: break-word;
}

.task-main-desc {
  margin: 0;
  font-size: 0.929rem;
  color: var(--color-text-muted);
  line-height: 1.4;
}

.meta-bar {
  display: flex;
  align-items: center;
  gap: calc(var(--space-unit) * 2);
  font-size: 0.857rem;
  color: var(--color-text-muted);
  margin-top: calc(var(--space-unit) * 0.5);
  flex-wrap: wrap;
}

.meta-item {
  display: flex;
  align-items: center;
  gap: calc(var(--space-unit) * 0.667);
}

.meta-item code {
  background: var(--color-surface-muted);
  color: var(--color-text);
  padding: 0 calc(var(--space-unit) * 0.667);
  border-radius: var(--radius);
  font-family: var(--font-mono);
  font-size: 0.8rem;
}

.meta-item.schedule-active {
  color: var(--color-warning);
}

.header-actions {
  display: flex;
  align-items: center;
  gap: calc(var(--space-unit) * 0.667);
  flex-shrink: 0;
  flex-wrap: wrap;
}

.header-actions button {
  height: var(--control-height);
  padding: 0 calc(var(--space-unit) * 1.5);
  border: 1px solid var(--color-border-strong);
  border-radius: var(--radius);
  background: var(--color-surface);
  color: var(--color-text);
  cursor: pointer;
  font: inherit;
  font-size: 0.857rem;
}

.btn-schedule {
  background: var(--color-warning-soft);
  border-color: var(--color-warning-border);
  color: var(--color-warning);
}

.btn-schedule.active {
  background: var(--color-warning);
  border-color: var(--color-warning);
  color: var(--color-on-primary);
}

.btn-trigger {
  background: var(--color-success);
  border-color: var(--color-success);
  color: var(--color-on-primary);
}

.btn-danger {
  background: var(--color-surface);
  border-color: var(--color-danger-border);
  color: var(--color-danger);
}

.action-error-banner {
  background: var(--color-danger-soft);
  border: 1px solid var(--color-danger-border);
  color: var(--color-danger);
  padding: calc(var(--space-unit) * 1.333) calc(var(--space-unit) * 2.333);
  border-radius: var(--radius);
  margin: 0;
  font-size: 0.886rem;
}

.stepper-section {
  display: flex;
  flex-direction: column;
}

.session-section {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: calc(var(--space-unit) * 2);
  min-height: 34rem;
}

.section-title-bar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: calc(var(--space-unit) * 1);
}

.section-title-bar h3 {
  margin: 0;
  font-size: 1.071rem;
  font-weight: 600;
}

.session-id-tag {
  font-size: 0.786rem;
  font-family: var(--font-mono);
  color: var(--color-text-muted);
}

.no-session-card {
  background: var(--color-surface);
  border: 1px dashed var(--color-border-strong);
  border-radius: var(--radius);
  padding: calc(var(--space-unit) * 6) calc(var(--space-unit) * 4);
  display: flex;
  align-items: center;
  justify-content: center;
  text-align: center;
}

.no-session-content {
  max-width: 32rem;
}

.no-session-content h4 {
  margin: 0 0 calc(var(--space-unit) * 1) 0;
  font-size: 1.071rem;
}

.no-session-content p {
  color: var(--color-text-muted);
  font-size: 0.886rem;
  line-height: 1.5;
  margin-bottom: calc(var(--space-unit) * 3);
}

.btn-launch-session {
  height: var(--control-height);
  padding: 0 calc(var(--space-unit) * 3);
  background: var(--color-primary);
  border: 1px solid var(--color-primary);
  border-radius: var(--radius);
  color: var(--color-on-primary);
  font: inherit;
  font-size: 0.929rem;
  font-weight: 600;
  cursor: pointer;
}

.btn-launch-session:disabled {
  background: var(--color-info-border);
  border-color: var(--color-info-border);
  cursor: default;
}

.embedded-chat-panel {
  flex: 1;
  min-height: 24rem;
  height: 34rem;
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  overflow: hidden;
  background: var(--color-bg);
}
</style>
