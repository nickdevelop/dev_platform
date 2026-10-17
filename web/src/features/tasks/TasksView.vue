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
    <!-- Middle Column: Task List & Filters -->
    <aside class="tasks-sidebar">
      <div class="sidebar-header">
        <h2>任务管理</h2>
        <button type="button" class="btn-create-task" @click="openCreateDialog">
          + 新建任务
        </button>
      </div>

      <div class="sidebar-filters">
        <input
          v-model="searchKeyword"
          type="search"
          placeholder="搜索任务名称或描述..."
          class="filter-search"
        />

        <div class="filter-row">
          <select v-model="selectedProjectFilter" class="filter-select">
            <option :value="undefined">全部项目</option>
            <option v-for="p in projectsStore.projects" :key="p.id" :value="p.id">
              {{ p.name }}
            </option>
          </select>

          <select v-model="selectedFlowFilter" class="filter-select">
            <option :value="undefined">全部流程</option>
            <option value="transactional">事务型</option>
            <option value="development">开发类</option>
          </select>
        </div>
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
          <button type="button" class="btn-create-task-lg" @click="openCreateDialog">
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
.tasks-workbench {
  display: grid;
  grid-template-columns: 320px 1fr;
  height: 100%;
  overflow: hidden;
  background: var(--color-bg, #18181f);
}

/* Sidebar */
.tasks-sidebar {
  border-right: 1px solid var(--color-border, #2d2d38);
  display: flex;
  flex-direction: column;
  background: var(--color-sidebar-bg, #1e1e26);
  overflow: hidden;
}

.sidebar-header {
  padding: 16px;
  display: flex;
  justify-content: space-between;
  align-items: center;
  border-bottom: 1px solid var(--color-border, #2d2d38);
}

.sidebar-header h2 {
  margin: 0;
  font-size: 1.15rem;
  font-weight: 700;
}

.btn-create-task {
  background: var(--color-primary, #3b82f6);
  color: #fff;
  border: none;
  padding: 6px 12px;
  border-radius: 6px;
  font-size: 0.82rem;
  font-weight: 600;
  cursor: pointer;
}

.sidebar-filters {
  padding: 12px 16px;
  display: flex;
  flex-direction: column;
  gap: 8px;
  border-bottom: 1px solid var(--color-border, #2d2d38);
}

.filter-search {
  background: var(--color-input-bg, #282834);
  border: 1px solid var(--color-border, #3a3a48);
  color: #fff;
  padding: 7px 10px;
  border-radius: 6px;
  font-size: 0.85rem;
  outline: none;
}

.filter-row {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 8px;
}

.filter-select {
  background: var(--color-input-bg, #282834);
  border: 1px solid var(--color-border, #3a3a48);
  color: #ddd;
  padding: 6px 8px;
  border-radius: 6px;
  font-size: 0.8rem;
  outline: none;
}

.task-list {
  flex: 1;
  overflow-y: auto;
  padding: 12px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.empty-list {
  text-align: center;
  color: var(--color-text-muted, #777);
  font-size: 0.85rem;
  padding: 24px 0;
}

.task-card {
  background: var(--color-card-bg, #242430);
  border: 1px solid var(--color-border, #333342);
  border-radius: 8px;
  padding: 12px 14px;
  cursor: pointer;
  transition: all 0.2s;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.task-card:hover {
  border-color: var(--color-primary, #3b82f6);
  background: rgba(59, 130, 246, 0.05);
}

.task-card.active {
  border-color: var(--color-primary, #3b82f6);
  background: rgba(59, 130, 246, 0.12);
  box-shadow: 0 0 12px rgba(59, 130, 246, 0.2);
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
  color: var(--color-text, #fff);
  line-height: 1.3;
}

.card-meta {
  display: flex;
  justify-content: space-between;
  font-size: 0.78rem;
  color: var(--color-text-muted, #888);
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

/* Right Workbench Pane */
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
  background: var(--color-card-bg, #202028);
  border: 1px solid var(--color-border, #333342);
  border-radius: 12px;
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
  color: var(--color-text-muted, #888);
  font-size: 0.9rem;
  line-height: 1.5;
  margin-bottom: 20px;
}

.btn-create-task-lg {
  background: var(--color-primary, #3b82f6);
  color: #fff;
  border: none;
  padding: 10px 20px;
  border-radius: 6px;
  font-weight: 600;
  cursor: pointer;
}

.task-detail-pane {
  display: flex;
  flex-direction: column;
  gap: 18px;
  padding: 24px;
  max-width: 1280px;
  margin: 0 auto;
  width: 100%;
  height: 100%;
}

.detail-header-panel {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 20px;
  padding-bottom: 16px;
  border-bottom: 1px solid var(--color-border, #2d2d38);
}

.title-section {
  display: flex;
  flex-direction: column;
  gap: 8px;
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
  color: var(--color-text-muted, #aaa);
  line-height: 1.4;
}

.meta-bar {
  display: flex;
  align-items: center;
  gap: 16px;
  font-size: 0.85rem;
  color: var(--color-text-muted, #999);
  margin-top: 4px;
  flex-wrap: wrap;
}

.meta-item {
  display: flex;
  align-items: center;
  gap: 6px;
}

.meta-item .label {
  color: var(--color-text-muted, #777);
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
  border-radius: 6px;
  font-size: 0.85rem;
  font-weight: 600;
  cursor: pointer;
  transition: all 0.2s;
}

.btn-schedule.active {
  background: #f59e0b;
  color: #000;
}

.btn-trigger {
  background: #10b981;
  color: #fff;
  border: none;
  padding: 7px 14px;
  border-radius: 6px;
  font-size: 0.85rem;
  font-weight: 600;
  cursor: pointer;
}

.btn-secondary {
  background: transparent;
  border: 1px solid var(--color-border, #444);
  color: var(--color-text, #ddd);
  padding: 7px 12px;
  border-radius: 6px;
  font-size: 0.85rem;
  cursor: pointer;
}

.btn-danger {
  background: transparent;
  border: 1px solid rgba(239, 68, 68, 0.4);
  color: #f87171;
  padding: 7px 12px;
  border-radius: 6px;
  font-size: 0.85rem;
  cursor: pointer;
}

.action-error-banner {
  background: rgba(239, 68, 68, 0.15);
  border: 1px solid rgba(239, 68, 68, 0.4);
  color: #ff6b6b;
  padding: 8px 14px;
  border-radius: 6px;
  margin: 0;
  font-size: 0.88rem;
}

.stepper-section {
  display: flex;
  flex-direction: column;
}

.session-section {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 12px;
  min-height: 550px;
}

.section-title-bar {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.section-title-bar h3 {
  margin: 0;
  font-size: 1.1rem;
  font-weight: 600;
}

.session-id-tag {
  font-size: 0.78rem;
  font-family: monospace;
  color: var(--color-text-muted, #888);
}

.no-session-card {
  background: var(--color-card-bg, #22222a);
  border: 1px dashed var(--color-border, #444);
  border-radius: 8px;
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
  color: var(--color-text-muted, #aaa);
  font-size: 0.88rem;
  line-height: 1.5;
  margin-bottom: 20px;
}

.btn-launch-session {
  background: var(--color-primary, #3b82f6);
  color: #fff;
  border: none;
  padding: 10px 24px;
  border-radius: 6px;
  font-size: 0.92rem;
  font-weight: 600;
  cursor: pointer;
  box-shadow: 0 4px 14px rgba(59, 130, 246, 0.4);
}

.embedded-chat-panel {
  flex: 1;
  min-height: 520px;
  height: 600px;
  border: 1px solid var(--color-border, #333342);
  border-radius: 8px;
  overflow: hidden;
  background: var(--color-bg, #18181f);
}
</style>
