<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import type { FlowType, Task, TaskRun, TaskStatus } from '../../api/tasks'
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
const viewingSessionId = ref<string | null>(null)

const showManageDialog = ref(false)
const editingTask = ref<Task | null>(null)
const showScheduleDialog = ref(false)
const schedulingTask = ref<Task | null>(null)

const creatingSession = ref(false)
const triggering = ref(false)
const actionError = ref('')

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

const activeTask = computed(() => {
  if (!activeTaskId.value) return null
  return tasksStore.byId(activeTaskId.value) ?? null
})

watch(
  () => activeTask.value?.id,
  (taskId) => {
    if (taskId) {
      viewingSessionId.value = activeTask.value?.activeSessionId || null
      void tasksStore.loadTaskRuns(taskId)
    }
  },
  { immediate: true },
)

const taskRuns = computed<TaskRun[]>(() => {
  if (!activeTaskId.value) return []
  return tasksStore.runsByTaskId(activeTaskId.value)
})

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

const activeFlowDef = computed(() => {
  if (!activeTask.value) return undefined
  return tasksStore.flowDefByType(activeTask.value.flowType)
})

const isFinalStep = computed(() => {
  if (!activeTask.value) return false
  const maxStep = activeTask.value.flowType === 'transactional' ? 2 : 4
  return activeTask.value.currentStep === maxStep
})

const isDevelopmentCompleted = computed(
  () => activeTask.value?.flowType === 'development' && activeTask.value?.status === 'completed',
)

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

async function handleAdvanceStep(
  confirmed: boolean,
  notes?: string,
  driverPrompt?: string,
  executionPrompt?: string,
) {
  if (!activeTask.value) return
  actionError.value = ''
  try {
    await tasksStore.advanceStep(activeTask.value.id, {
      confirmed,
      notes,
      driverPrompt,
      executionPrompt,
    })
  } catch (err: any) {
    actionError.value = err.message || '步骤推进失败'
  }
}

async function handleSavePrompt(prompt: string) {
  if (!activeTask.value) return
  actionError.value = ''
  try {
    await tasksStore.saveExecutionPrompt(activeTask.value.id, prompt)
  } catch (err: any) {
    actionError.value = err.message || '保存执行提示词失败'
  }
}

async function createSessionForTask() {
  if (!activeTask.value) return
  creatingSession.value = true
  actionError.value = ''
  try {
    const res = await tasksStore.createSession(activeTask.value.id)
    viewingSessionId.value = res.session.id
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
    const res = await tasksStore.triggerExecution(activeTask.value.id, 'manual')
    if (res.run?.sessionId) {
      viewingSessionId.value = res.run.sessionId
    }
  } catch (err: any) {
    actionError.value = err.message || '触发执行失败'
  } finally {
    triggering.value = false
  }
}

function switchViewingSession(sid: string) {
  viewingSessionId.value = sid
}

function resetToMainSession() {
  if (activeTask.value?.activeSessionId) {
    viewingSessionId.value = activeTask.value.activeSessionId
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
    <!-- Middle Column: Task List & Filters -->
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
        aria-label="搜索任务名称或描述"
      />

      <div class="sidebar-filters">
        <select
          v-model="selectedProjectFilter"
          class="filter-select project-filter"
          aria-label="按项目筛选"
        >
          <option :value="undefined">全部项目</option>
          <option v-for="p in projectsStore.projects" :key="p.id" :value="p.id">
            {{ p.name }}
          </option>
        </select>

        <select
          v-model="selectedFlowFilter"
          class="filter-select flow-filter"
          aria-label="按流程筛选"
        >
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
              :disabled="isDevelopmentCompleted"
              @click="openScheduleDialog(activeTask)"
              title="配置定时/周期调度"
            >
              ⏰ 调度配置
            </button>

            <!-- Completed state for development task -->
            <button
              v-if="isDevelopmentCompleted"
              type="button"
              class="btn-completed"
              disabled
            >
              ✓ 已完成
            </button>
            <!-- Ready to execute state -->
            <button
              v-else-if="isFinalStep && activeTask.isConfirmed"
              type="button"
              class="btn-trigger"
              :disabled="triggering"
              @click="triggerExecutionNow"
              title="立即执行任务最终步骤"
            >
              {{ triggering ? '正在启动...' : '▶ 立即执行' }}
            </button>
            <!-- Failed state retry -->
            <button
              v-else-if="activeTask.status === 'failed'"
              type="button"
              class="btn-retry"
              :disabled="triggering"
              @click="triggerExecutionNow"
              title="在当前会话重试执行"
            >
              🔄 重试执行
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
            @save-prompt="handleSavePrompt"
          />
        </section>

        <!-- Runs History (For transactional tasks) -->
        <section v-if="activeTask.flowType === 'transactional' && taskRuns.length > 0" class="runs-history-box">
          <div class="runs-header">
            <h4>执行历史记录（共 {{ taskRuns.length }} 次运行）</h4>
            <span class="runs-hint">事务型任务每次执行均新启动独立 Agent 会话，点击记录可切换查看</span>
          </div>
          <div class="runs-chips">
            <button
              v-for="r in taskRuns"
              :key="r.id"
              type="button"
              class="run-chip"
              :class="{
                active: viewingSessionId === r.sessionId,
                success: r.status === 'success',
                failed: r.status === 'failed',
              }"
              @click="switchViewingSession(r.sessionId)"
            >
              <span class="run-trig">[{{ r.triggerType === 'cron' ? '周期' : r.triggerType === 'once' ? '定时' : '手动' }}]</span>
              <span class="run-time">{{ new Date(r.triggeredAt).toLocaleTimeString() }}</span>
              <span class="run-badge">{{ r.status }}</span>
            </button>
          </div>
        </section>

        <!-- Session Execution Area -->
        <section class="session-section">
          <div class="section-title-bar">
            <div class="session-label-group">
              <h3>
                {{
                  activeTask.flowType === 'transactional'
                    ? viewingSessionId === activeTask.activeSessionId
                      ? '任务分析会话（主会话）'
                      : '独立执行会话（执行实例）'
                    : '任务全流程执行会话'
                }}
              </h3>
              <button
                v-if="viewingSessionId && activeTask.activeSessionId && viewingSessionId !== activeTask.activeSessionId"
                type="button"
                class="btn-switch-main"
                @click="resetToMainSession"
              >
                ↩ 返回任务分析主会话
              </button>
            </div>
            <span v-if="viewingSessionId" class="session-id-tag">
              Session: {{ viewingSessionId }}
            </span>
          </div>

          <!-- If no session created yet -->
          <div v-if="!activeTask.activeSessionId" class="no-session-card">
            <div class="no-session-content">
              <h4>尚未创建任务会话</h4>
              <p>
                已为此任务配置了 <strong>{{ activeTask.backend }}</strong> Agent 与
                <strong>{{ activeTask.model }}</strong> 模型。
                {{
                  activeTask.flowType === 'transactional'
                    ? '点击下方按钮拉起【任务分析】会话，Agent 将对任务进行分析并识别歧义与不确定点。'
                    : '点击下方按钮拉起会话，Agent 将从第一步【需求分析】开始介入处理。'
                }}
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
          <div v-else-if="viewingSessionId" class="embedded-chat-panel">
            <SessionChatPanel
              :key="viewingSessionId"
              :project-id="activeTask.projectId"
              :session-id="viewingSessionId"
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
.tasks-workbench {
  display: grid;
  grid-template-columns: 272px minmax(0, 1fr);
  height: 100%;
  overflow: hidden;
  background: var(--color-bg);
  color: var(--color-text);
  font-family: var(--font-family);
}

/* Sidebar */
.tasks-sidebar {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 0;
  border-right: 1px solid var(--color-border);
  background: var(--color-surface);
  overflow: hidden;
}

.sidebar-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: calc(var(--space-unit) * 2.5) calc(var(--space-unit) * 3);
  border-bottom: 1px solid var(--color-border);
  gap: calc(var(--space-unit) * 1.5);
}

.sidebar-head h2 {
  margin: 0;
  font-size: 1.15rem;
  font-weight: 700;
  color: var(--color-text);
}

.new-task {
  background: var(--color-primary);
  color: #fff;
  border: none;
  padding: calc(var(--space-unit) * 1) calc(var(--space-unit) * 2);
  border-radius: var(--radius);
  font-size: 0.85rem;
  font-weight: 600;
  cursor: pointer;
}

.search {
  margin: calc(var(--space-unit) * 2) calc(var(--space-unit) * 3);
  padding: calc(var(--space-unit) * 1.5) calc(var(--space-unit) * 2);
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  background: var(--color-bg);
  color: var(--color-text);
  font-size: 0.88rem;
  outline: none;
}

.sidebar-filters {
  display: flex;
  flex-direction: column;
  gap: calc(var(--space-unit) * 1.5);
  padding: 0 calc(var(--space-unit) * 3) calc(var(--space-unit) * 2.5);
  border-bottom: 1px solid var(--color-border);
}

.filter-select {
  padding: calc(var(--space-unit) * 1.2) calc(var(--space-unit) * 1.5);
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  background: var(--color-bg);
  color: var(--color-text);
  font-size: 0.82rem;
  outline: none;
}

.task-list {
  flex: 1;
  overflow-y: auto;
  padding: calc(var(--space-unit) * 2) calc(var(--space-unit) * 3);
  display: flex;
  flex-direction: column;
  gap: calc(var(--space-unit) * 1.5);
}

.empty-list {
  text-align: center;
  color: var(--color-text-muted);
  font-size: 0.85rem;
  padding: calc(var(--space-unit) * 4) 0;
}

.task-card {
  background: var(--color-surface-muted);
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  padding: calc(var(--space-unit) * 2);
  cursor: pointer;
  transition: all 0.2s;
  display: flex;
  flex-direction: column;
  gap: calc(var(--space-unit) * 1);
}

.task-card:hover {
  border-color: var(--color-primary);
  background: var(--color-surface-muted);
}

.task-card.active {
  border-color: var(--color-primary);
  background: var(--color-surface-muted);
  box-shadow: 0 0 10px rgba(59, 130, 246, 0.2);
}

.card-top {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.flow-pill {
  font-size: 0.72rem;
  padding: 2px 6px;
  border-radius: 4px;
  font-weight: 600;
}

.flow-pill.transactional {
  background: rgba(139, 92, 246, 0.15);
  color: #a78bfa;
}

.flow-pill.development {
  background: rgba(14, 165, 233, 0.15);
  color: #38bdf8;
}

.status-badge {
  font-size: 0.72rem;
  padding: 2px 6px;
  border-radius: 10px;
}

.badge-draft {
  background: rgba(156, 163, 175, 0.2);
  color: #9ca3af;
}

.badge-progress {
  background: rgba(245, 158, 11, 0.2);
  color: #fbbf24;
}

.badge-ready {
  background: rgba(16, 185, 129, 0.2);
  color: #34d399;
}

.badge-executing {
  background: rgba(59, 130, 246, 0.2);
  color: #60a5fa;
}

.badge-completed {
  background: rgba(16, 185, 129, 0.25);
  color: #10b981;
}

.badge-failed {
  background: rgba(239, 68, 68, 0.2);
  color: #f87171;
}

.card-title {
  margin: 0;
  font-size: 0.92rem;
  font-weight: 600;
  color: var(--color-text);
  line-height: 1.3;
}

.card-meta {
  display: flex;
  justify-content: space-between;
  font-size: 0.78rem;
  color: var(--color-text-muted);
}

.schedule-tag {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: 0.74rem;
  color: #fbbf24;
  background: rgba(245, 158, 11, 0.1);
  padding: 3px 6px;
  border-radius: 4px;
}

/* Right Content */
.tasks-content {
  overflow-y: auto;
  display: flex;
  flex-direction: column;
}

.empty-workbench {
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
}

.empty-card {
  text-align: center;
  max-width: 440px;
  padding: 40px;
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
}

.empty-icon {
  font-size: 3rem;
  margin-bottom: 12px;
}

.empty-card h3 {
  margin: 0 0 8px 0;
  font-size: 1.25rem;
}

.empty-card p {
  color: var(--color-text-muted);
  font-size: 0.9rem;
  line-height: 1.5;
  margin-bottom: 20px;
}

.task-detail-pane {
  display: flex;
  flex-direction: column;
  gap: calc(var(--space-unit) * 3);
  padding: calc(var(--space-unit) * 4);
  max-width: 1360px;
  margin: 0 auto;
  width: 100%;
}

.detail-header-panel {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 20px;
  padding-bottom: calc(var(--space-unit) * 3);
  border-bottom: 1px solid var(--color-border);
}

.title-section {
  display: flex;
  flex-direction: column;
  gap: calc(var(--space-unit) * 1.5);
}

.title-row {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}

.flow-badge {
  font-size: 0.76rem;
  font-weight: 700;
  padding: 3px 8px;
  border-radius: 4px;
}

.flow-badge.transactional {
  background: rgba(139, 92, 246, 0.2);
  color: #c4b5fd;
}

.flow-badge.development {
  background: rgba(14, 165, 233, 0.2);
  color: #7dd3fc;
}

.task-main-title {
  margin: 0;
  font-size: 1.45rem;
  font-weight: 700;
}

.status-badge-lg {
  font-size: 0.8rem;
  padding: 3px 10px;
  border-radius: 12px;
  font-weight: 600;
}

.task-main-desc {
  margin: 0;
  font-size: 0.92rem;
  color: var(--color-text-muted);
  line-height: 1.4;
}

.meta-bar {
  display: flex;
  align-items: center;
  gap: 16px;
  font-size: 0.85rem;
  color: var(--color-text-muted);
  margin-top: 4px;
  flex-wrap: wrap;
}

.meta-item {
  display: flex;
  align-items: center;
  gap: 6px;
}

.meta-item .label {
  color: var(--color-text-muted);
}

.meta-item code {
  background: rgba(255, 255, 255, 0.08);
  padding: 2px 6px;
  border-radius: 4px;
  font-size: 0.8rem;
}

.meta-item.schedule-active {
  color: #fbbf24;
}

.header-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
}

.btn-schedule {
  background: rgba(245, 158, 11, 0.15);
  border: 1px solid rgba(245, 158, 11, 0.4);
  color: #fbbf24;
  padding: 7px 12px;
  border-radius: var(--radius);
  font-size: 0.85rem;
  font-weight: 600;
  cursor: pointer;
}

.btn-schedule:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

.btn-completed {
  background: rgba(16, 185, 129, 0.2);
  border: 1px solid rgba(16, 185, 129, 0.5);
  color: #10b981;
  padding: 7px 14px;
  border-radius: var(--radius);
  font-size: 0.85rem;
  font-weight: 600;
  cursor: default;
}

.btn-trigger {
  background: #10b981;
  color: #fff;
  border: none;
  padding: 7px 14px;
  border-radius: var(--radius);
  font-size: 0.85rem;
  font-weight: 600;
  cursor: pointer;
}

.btn-retry {
  background: #f59e0b;
  color: #000;
  border: none;
  padding: 7px 14px;
  border-radius: var(--radius);
  font-size: 0.85rem;
  font-weight: 600;
  cursor: pointer;
}

.btn-secondary {
  background: transparent;
  border: 1px solid var(--color-border);
  color: var(--color-text);
  padding: 7px 12px;
  border-radius: var(--radius);
  font-size: 0.85rem;
  cursor: pointer;
}

.btn-danger {
  background: transparent;
  border: 1px solid rgba(239, 68, 68, 0.4);
  color: #f87171;
  padding: 7px 12px;
  border-radius: var(--radius);
  font-size: 0.85rem;
  cursor: pointer;
}

.action-error-banner {
  background: rgba(239, 68, 68, 0.15);
  border: 1px solid rgba(239, 68, 68, 0.4);
  color: #ff6b6b;
  padding: 8px 14px;
  border-radius: var(--radius);
  margin: 0;
  font-size: 0.88rem;
}

/* Runs History Box */
.runs-history-box {
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  padding: calc(var(--space-unit) * 2) calc(var(--space-unit) * 2.5);
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.runs-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.runs-header h4 {
  margin: 0;
  font-size: 0.95rem;
  font-weight: 600;
}

.runs-hint {
  font-size: 0.78rem;
  color: var(--color-text-muted);
}

.runs-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.run-chip {
  background: var(--color-surface-muted);
  border: 1px solid var(--color-border);
  border-radius: 6px;
  padding: 6px 10px;
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 0.8rem;
  cursor: pointer;
  color: var(--color-text);
}

.run-chip.active {
  border-color: var(--color-primary);
  background: rgba(59, 130, 246, 0.15);
}

.run-trig {
  font-weight: 600;
  color: #60a5fa;
}

.run-badge {
  font-size: 0.7rem;
  padding: 1px 4px;
  border-radius: 4px;
  background: rgba(255, 255, 255, 0.1);
}

.run-chip.success .run-badge {
  color: #34d399;
}

.run-chip.failed .run-badge {
  color: #f87171;
}

/* Session Section */
.session-section {
  display: flex;
  flex-direction: column;
  gap: calc(var(--space-unit) * 1.5);
}

.section-title-bar {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.session-label-group {
  display: flex;
  align-items: center;
  gap: 12px;
}

.session-label-group h3 {
  margin: 0;
  font-size: 1.1rem;
  font-weight: 600;
}

.btn-switch-main {
  background: rgba(59, 130, 246, 0.15);
  border: 1px solid rgba(59, 130, 246, 0.3);
  color: #60a5fa;
  font-size: 0.8rem;
  padding: 3px 8px;
  border-radius: 4px;
  cursor: pointer;
}

.session-id-tag {
  font-size: 0.78rem;
  font-family: monospace;
  color: var(--color-text-muted);
}

.no-session-card {
  background: var(--color-surface);
  border: 1px dashed var(--color-border);
  border-radius: var(--radius);
  padding: 40px 24px;
  display: flex;
  align-items: center;
  justify-content: center;
  text-align: center;
}

.no-session-content {
  max-width: 500px;
}

.no-session-content h4 {
  margin: 0 0 8px 0;
  font-size: 1.1rem;
}

.no-session-content p {
  color: var(--color-text-muted);
  font-size: 0.88rem;
  line-height: 1.5;
  margin-bottom: 20px;
}

.btn-launch-session {
  background: var(--color-primary);
  color: #fff;
  border: none;
  padding: 10px 24px;
  border-radius: var(--radius);
  font-size: 0.92rem;
  font-weight: 600;
  cursor: pointer;
  box-shadow: 0 4px 14px rgba(59, 130, 246, 0.4);
}

.embedded-chat-panel {
  min-height: 540px;
  height: 640px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  overflow: hidden;
  background: var(--color-bg);
}
</style>
