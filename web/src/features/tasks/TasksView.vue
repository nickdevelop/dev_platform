<script setup lang="ts">
// Task detail pane: the third column's content while the tasks area is open.
// The task list itself lives in the shell's middle column (see
// ./TasksSidebar.vue); the URL's taskId — or, when no task is selected yet, the
// first task in the list — picks the task this pane shows. Its flow workspace
// (transactional stages or the development stepper) owns the pane; a task run's
// agent session opens as the full conversation dialog over the pane.
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import type { Task, TaskRun } from '../../api/tasks'
import BackendTag from '../../shared/components/BackendTag.vue'
import { useAgentsStore } from '../../stores/agents'
import { useProjectsStore } from '../../stores/projects'
import { useTasksStore } from '../../stores/tasks'
import SessionChatPanel from '../chat/SessionChatPanel.vue'
import FlowStepper from './FlowStepper.vue'
import ScheduleConfigDialog from './ScheduleConfigDialog.vue'
import TaskManageDialog from './TaskManageDialog.vue'
import TaskChatDialog from './TaskChatDialog.vue'
import TransactionalFlowView from './TransactionalFlowView.vue'
import { getStatusBadgeClass, getStatusLabel } from './status'

const route = useRoute()
const router = useRouter()
const tasksStore = useTasksStore()
const projectsStore = useProjectsStore()
const agentsStore = useAgentsStore()

const viewingSessionId = ref<string | null>(null)
const showChatDialog = ref(false)

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
})

// A task-detail URL selects that task; leaving the task center on the plain
// /tasks route falls back to the first task so the pane is never blank when
// tasks exist.
const activeTaskId = computed<number | null>(() => {
  if (route.params.taskId !== undefined) return Number(route.params.taskId)
  return tasksStore.tasks.length > 0 ? tasksStore.tasks[0].id : null
})

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
  if (route.params.taskId !== undefined && Number(route.params.taskId) === t.id) {
    void router.push({ name: 'tasks' })
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
    showChatDialog.value = true
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
</script>

<template>
  <div class="tasks-workbench">
    <main class="tasks-content">
      <div v-if="!activeTask" class="empty-workbench">
        <div class="empty-card">
          <div class="empty-icon">📋</div>
          <h3>选择任务或新建任务</h3>
          <p>从左侧列表中选择一个任务以查看其编排流程、会话与调度配置，或点击上方新建任务。</p>
          <p class="empty-hint-muted">暂无任务时，可先在左侧列表点击「新建」。</p>
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

            <div v-else class="session-active-card">
              <div class="session-active-content">
                <div class="session-active-badge">💬 Agent 会话已就绪</div>
                <h4>当前阶段 Agent 会话</h4>
                <p>
                  已连接至后端 <strong>{{ activeTask.backend }}</strong> · <strong>{{ activeTask.model }}</strong>
                </p>
                <button
                  type="button"
                  class="btn-open-session-dialog"
                  data-test="btn-open-session-dialog"
                  @click="showChatDialog = true"
                >
                  💬 打开 Agent 会话窗口
                </button>
              </div>
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
      @saved="() => (showManageDialog = false)"
    />

    <ScheduleConfigDialog
      v-if="showScheduleDialog && schedulingTask"
      :task="schedulingTask"
      @close="showScheduleDialog = false"
      @updated="(t) => (schedulingTask = t)"
    />

    <TaskChatDialog
      v-if="showChatDialog && activeTask && activeTask.activeSessionId"
      :title="`任务会话：${activeTask.title}`"
      :project-id="activeTask.projectId"
      :session-id="activeTask.activeSessionId"
      :backend="activeTask.backend"
      :model="activeTask.model"
      @close="showChatDialog = false"
    />
  </div>
</template>

<style scoped>
@import './tasks-view.css';
</style>
