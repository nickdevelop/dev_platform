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
import TransactionalFlowView from './TransactionalFlowView.vue'

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
    if (taskId && activeTask.value) {
      viewingSessionId.value = activeTask.value.activeSessionId || null
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
      await tasksStore.loadTaskRuns(activeTask.value.id)
    }
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

        <!-- TRANSACTIONAL WORKFLOW: Modularized Stage-based view -->
        <template v-if="activeTask.flowType === 'transactional'">
          <TransactionalFlowView
            :task="activeTask"
            :task-runs="taskRuns"
            :creating-session="creatingSession"
            :triggering="triggering"
            @create-session="createSessionForTask"
            @trigger-execution="triggerExecutionNow"
            @action-error="(msg) => (actionError = msg)"
          />
        </template>

        <!-- DEVELOPMENT WORKFLOW: Stepper & Continuous Session -->
        <template v-else>
          <!-- Orchestration Flow Stepper -->
          <section class="stepper-section">
            <FlowStepper
              :task="activeTask"
              :flow-def="activeFlowDef"
              @advance-step="handleAdvanceStep"
              @save-prompt="handleSavePrompt"
            />
          </section>

          <!-- Workspace Body: Chat Session or Empty Prompt -->
          <section class="flow-workspace-body">
            <div v-if="!activeTask.activeSessionId" class="no-session-card">
              <div class="no-session-content">
                <h4>尚未拉起任务执行会话</h4>
                <p>
                  已为此任务配置了 <strong>{{ activeTask.backend }}</strong> Agent 与
                  <strong>{{ activeTask.model }}</strong> 模型。
                  点击下方按钮创建会话，Agent 将根据当前阶段目标介入并协助推进。
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

            <div v-else class="embedded-chat-panel full-height">
              <SessionChatPanel
                :key="activeTask.activeSessionId"
                :project-id="activeTask.projectId"
                :session-id="activeTask.activeSessionId"
              />
            </div>
          </section>
        </template>
      </div>
    </main>

    <!-- Modals -->
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
@import './tasks-view.css';
</style>
