<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import type { FlowType, ScheduleType, Task, TaskRun, TaskStatus } from '../../api/tasks'
import BackendTag from '../../shared/components/BackendTag.vue'
import { useAgentsStore } from '../../stores/agents'
import { useProjectsStore } from '../../stores/projects'
import { useSessionsStore } from '../../stores/sessions'
import { useTasksStore } from '../../stores/tasks'
import SessionChatPanel from '../chat/SessionChatPanel.vue'
import FlowStepper from './FlowStepper.vue'
import { extractExecutionPrompt } from './promptExtractor'
import ScheduleConfigDialog from './ScheduleConfigDialog.vue'
import TaskManageDialog from './TaskManageDialog.vue'

const route = useRoute()
const router = useRouter()
const tasksStore = useTasksStore()
const projectsStore = useProjectsStore()
const agentsStore = useAgentsStore()
const sessionsStore = useSessionsStore()

const searchKeyword = ref('')
const selectedProjectFilter = ref<number | undefined>(undefined)
const selectedFlowFilter = ref<FlowType | undefined>(undefined)
const selectedStatusFilter = ref<TaskStatus | undefined>(undefined)

const activeTaskId = ref<number | null>(null)
const viewingSessionId = ref<string | null>(null)

// Transactional stage: 1 = Analysis, 2 = Confirmation & Ready, 3 = Execution & Monitoring
const transactionalStage = ref<1 | 2 | 3>(1)
const extractingPrompt = ref(false)
const extractNotice = ref('')
const executionPromptInput = ref('')
const stage2ScheduleType = ref<ScheduleType>('none')
const stage2ScheduledAt = ref('')
const stage2CronExpr = ref('0 * * * *')
const savingStage2 = ref(false)
const showPromptDetails = ref(false)

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

const cronPresets = [
  { label: '每小时整点', expr: '0 * * * *' },
  { label: '工作日早 09:00', expr: '0 9 * * 1-5' },
  { label: '每天凌晨 02:00', expr: '0 2 * * *' },
  { label: '每周一凌晨 03:00', expr: '0 3 * * 1' },
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

function formatDateForInput(isoString?: string): string {
  if (!isoString) {
    const d = new Date(Date.now() + 3600 * 1000)
    d.setMinutes(d.getMinutes() - d.getTimezoneOffset())
    return d.toISOString().slice(0, 16)
  }
  const d = new Date(isoString)
  d.setMinutes(d.getMinutes() - d.getTimezoneOffset())
  return d.toISOString().slice(0, 16)
}

function syncTaskToStageState(t: Task | null) {
  if (!t) return
  executionPromptInput.value = t.executionPrompt || ''
  stage2ScheduleType.value = t.scheduleEnabled ? t.scheduleType || 'none' : 'none'
  stage2CronExpr.value = t.cronExpr || '0 * * * *'
  stage2ScheduledAt.value = formatDateForInput(t.scheduledAt)
  extractNotice.value = ''

  if (t.flowType === 'transactional') {
    if (t.isConfirmed || t.status === 'ready_for_execution' || t.status === 'executing' || t.status === 'completed') {
      transactionalStage.value = 3
    } else {
      transactionalStage.value = 1
    }
  }
}

watch(
  () => activeTask.value?.id,
  (taskId) => {
    if (taskId && activeTask.value) {
      viewingSessionId.value = activeTask.value.activeSessionId || null
      syncTaskToStageState(activeTask.value)
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

// Transactional flow: Step 1 -> Step 2 with automatic prompt extraction
async function extractAndGoToStage2() {
  if (!activeTask.value) return
  extractingPrompt.value = true
  actionError.value = ''

  try {
    if (activeTask.value.activeSessionId) {
      await sessionsStore.loadHistory(activeTask.value.projectId, activeTask.value.activeSessionId)
      const messages = sessionsStore.messagesFor(activeTask.value.activeSessionId)
      const extracted = extractExecutionPrompt(messages, {
        title: activeTask.value.title,
        description: activeTask.value.description,
      })

      if (extracted) {
        executionPromptInput.value = extracted
        extractNotice.value = '✓ 已从分析会话中自动识别并提取执行方案，请核对与微调'
      } else {
        executionPromptInput.value = `【执行方案】${activeTask.value.title}\n需求与目标：${activeTask.value.description || ''}\n1. `
        extractNotice.value = '会话中暂未检测到完整的 Agent 输出方案，已生成初始骨架，可直接补充编写'
      }
    } else {
      executionPromptInput.value = `【执行方案】${activeTask.value.title}\n需求与目标：${activeTask.value.description || ''}\n1. `
      extractNotice.value = '尚未启动会话，已基于任务需求生成初始执行方案模板'
    }

    transactionalStage.value = 2
  } catch (err: any) {
    actionError.value = err.message || '提取执行方案失败'
  } finally {
    extractingPrompt.value = false
  }
}

// Transactional flow: Re-extract prompt from active session while in Stage 2
async function reExtractPrompt() {
  if (!activeTask.value || !activeTask.value.activeSessionId) return
  extractingPrompt.value = true
  try {
    await sessionsStore.loadHistory(activeTask.value.projectId, activeTask.value.activeSessionId)
    const messages = sessionsStore.messagesFor(activeTask.value.activeSessionId)
    const extracted = extractExecutionPrompt(messages, {
      title: activeTask.value.title,
      description: activeTask.value.description,
    })
    if (extracted) {
      executionPromptInput.value = extracted
      extractNotice.value = '✓ 已重新从分析会话中提取最新执行方案'
    } else {
      extractNotice.value = '会话中暂未找到新的 Agent 方案输出'
    }
  } catch (err: any) {
    actionError.value = err.message || '重新提取失败'
  } finally {
    extractingPrompt.value = false
  }
}

// Transactional flow: Confirm Stage 2 and switch to Stage 3 (ready & execution)
async function confirmStage2AndAdvance() {
  if (!activeTask.value) return
  const prompt = executionPromptInput.value.trim()
  if (!prompt) {
    actionError.value = '请填写或确认可直接执行的方案提示词'
    return
  }

  savingStage2.value = true
  actionError.value = ''

  try {
    // 1. Save execution prompt (moves task to ready_for_execution state)
    await tasksStore.saveExecutionPrompt(activeTask.value.id, prompt)

    // 2. Save schedule if configured
    const scheduleEnabled = stage2ScheduleType.value !== 'none'
    let scheduledAtIso: string | undefined = undefined
    if (stage2ScheduleType.value === 'once' && stage2ScheduledAt.value) {
      scheduledAtIso = new Date(stage2ScheduledAt.value).toISOString()
    }

    await tasksStore.updateSchedule(activeTask.value.id, {
      scheduleType: scheduleEnabled ? stage2ScheduleType.value : 'none',
      scheduleEnabled,
      cronExpr: stage2ScheduleType.value === 'cron' ? stage2CronExpr.value.trim() : undefined,
      scheduledAt: scheduledAtIso,
    })

    // 3. Switch smoothly to Stage 3: Execution & Monitoring
    transactionalStage.value = 3
  } catch (err: any) {
    actionError.value = err.message || '确认并就绪失败'
  } finally {
    savingStage2.value = false
  }
}

function switchTransactionalStage(stage: 1 | 2 | 3) {
  transactionalStage.value = stage
  if (stage === 1 && activeTask.value?.activeSessionId) {
    viewingSessionId.value = activeTask.value.activeSessionId
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

        <!-- =================================================================== -->
        <!-- TRANSACTIONAL WORKFLOW: Stage-based views                          -->
        <!-- =================================================================== -->
        <template v-if="activeTask.flowType === 'transactional'">
          <!-- Stage Stepper Bar -->
          <div class="stage-stepper-bar">
            <div
              class="stage-step-item"
              :class="{
                active: transactionalStage === 1,
                completed: activeTask.isConfirmed || transactionalStage > 1,
              }"
              @click="switchTransactionalStage(1)"
            >
              <div class="step-num">
                <span v-if="activeTask.isConfirmed || transactionalStage > 1">✓</span>
                <span v-else>1</span>
              </div>
              <div class="step-label">
                <span class="step-title-text">任务分析与澄清</span>
                <span class="step-status-sub">{{ activeTask.activeSessionId ? '对话分析中' : '待启动' }}</span>
              </div>
            </div>

            <div class="stage-step-arrow">➔</div>

            <div
              class="stage-step-item"
              :class="{
                active: transactionalStage === 2,
                completed: activeTask.isConfirmed || transactionalStage > 2,
              }"
              @click="switchTransactionalStage(2)"
            >
              <div class="step-num">
                <span v-if="activeTask.isConfirmed || transactionalStage > 2">✓</span>
                <span v-else>2</span>
              </div>
              <div class="step-label">
                <span class="step-title-text">方案确认与就绪</span>
                <span class="step-status-sub">{{ activeTask.isConfirmed ? '已就绪' : '待确认' }}</span>
              </div>
            </div>

            <div class="stage-step-arrow">➔</div>

            <div
              class="stage-step-item"
              :class="{
                active: transactionalStage === 3,
                completed: activeTask.status === 'completed',
              }"
              @click="switchTransactionalStage(3)"
            >
              <div class="step-num">
                <span v-if="activeTask.status === 'completed'">✓</span>
                <span v-else>3</span>
              </div>
              <div class="step-label">
                <span class="step-title-text">任务执行与监控</span>
                <span class="step-status-sub">
                  {{ activeTask.isConfirmed ? `${taskRuns.length} 次运行` : '未就绪' }}
                </span>
              </div>
            </div>
          </div>

          <!-- STAGE 1: Task Analysis Dedicated View -->
          <section v-if="transactionalStage === 1" class="stage-view stage-analysis-view">
            <div class="stage-banner">
              <span class="banner-badge">💡 步骤 1 · 任务分析与澄清</span>
              <p class="banner-desc">
                Agent 正在对任务需求进行分析，重点识别信息歧义、执行过程不确定点与潜在风险。在对话确认完毕后，点击右下方按钮系统将<strong>自动识别并提炼执行方案</strong>，无缝切换至下一步。
              </p>
            </div>

            <!-- Analysis Chat Session Container -->
            <div class="analysis-session-wrap">
              <div v-if="!activeTask.activeSessionId" class="no-session-card">
                <div class="no-session-content">
                  <h4>尚未启动任务分析会话</h4>
                  <p>
                    已为此任务配置了 <strong>{{ activeTask.backend }}</strong> Agent 与
                    <strong>{{ activeTask.model }}</strong> 模型。
                    点击下方按钮拉起【任务分析】会话，Agent 将对任务进行分析并识别歧义与不确定点。
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
            </div>

            <!-- Floating / Bottom Extraction Action Bar -->
            <div class="stage-bottom-bar">
              <div class="bottom-bar-info">
                <span class="tip-icon">✨</span>
                <span>分析对话完成后，点击右侧按钮系统将自动提取 Agent 总结的执行方案并进入下一步确认。</span>
              </div>
              <button
                type="button"
                class="btn-extract-next"
                :disabled="extractingPrompt"
                @click="extractAndGoToStage2"
              >
                {{ extractingPrompt ? '正在提取方案...' : '🪄 提取方案并进入下一步 ➔' }}
              </button>
            </div>
          </section>

          <!-- STAGE 2: Plan Confirmation & Ready Dedicated View -->
          <section v-else-if="transactionalStage === 2" class="stage-view stage-confirm-view">
            <div class="stage-confirm-container">
              <!-- Notice banner -->
              <div class="confirmation-notice">
                <div class="notice-badge">⚠️ 核心免打断要求</div>
                <p class="notice-text">
                  在【任务分析】阶段，Agent 需分析任务需求，识别信息是否有歧义、执行过程是否存在不确定点与潜在风险，全面与用户确认清楚，并输出可直接执行的执行计划，作为第二步执行的提示词。后续独立执行阶段将不再向用户提问。
                </p>
              </div>

              <!-- Execution Prompt Box -->
              <div class="execution-prompt-box">
                <div class="prompt-box-header">
                  <div class="prompt-box-title-wrap">
                    <span class="prompt-box-title">📝 提炼第二步执行提示词（Execution Prompt）</span>
                    <button
                      type="button"
                      class="btn-re-extract"
                      :disabled="extractingPrompt"
                      @click="reExtractPrompt"
                    >
                      {{ extractingPrompt ? '正在提取...' : '🔄 从分析会话重新提取' }}
                    </button>
                  </div>
                  <span class="prompt-box-hint">
                    任务分析完成后，请在此确认可直接执行的方案提示词，确认后任务即刻就绪。
                  </span>
                </div>

                <div v-if="extractNotice" class="extract-notice-bar" role="status">
                  {{ extractNotice }}
                </div>

                <textarea
                  v-model="executionPromptInput"
                  rows="9"
                  placeholder="例如：请按以下已确认方案执行部署：1.拉取镜像 2.重启容器 3.健康检查..."
                  class="execution-prompt-textarea"
                ></textarea>
              </div>

              <!-- Integrated Schedule Configuration -->
              <div class="schedule-config-card">
                <div class="card-head">
                  <span class="card-head-title">⏰ 调度与触发方式</span>
                  <span class="card-head-sub">配置任务就绪后的执行策略（支持立即手动执行、定时单次或周期循环）</span>
                </div>

                <div class="schedule-options">
                  <label class="schedule-opt-item" :class="{ selected: stage2ScheduleType === 'none' }">
                    <input v-model="stage2ScheduleType" type="radio" value="none" />
                    <div class="opt-content">
                      <strong>仅手动触发</strong>
                      <span>随时在执行界面点击【▶ 立即执行】拉起独立会话</span>
                    </div>
                  </label>

                  <label class="schedule-opt-item" :class="{ selected: stage2ScheduleType === 'once' }">
                    <input v-model="stage2ScheduleType" type="radio" value="once" />
                    <div class="opt-content">
                      <strong>定时单次执行</strong>
                      <span>在指定的未来时间到达时，由系统自动执行一次</span>
                    </div>
                  </label>

                  <label class="schedule-opt-item" :class="{ selected: stage2ScheduleType === 'cron' }">
                    <input v-model="stage2ScheduleType" type="radio" value="cron" />
                    <div class="opt-content">
                      <strong>周期循环调度 (Cron)</strong>
                      <span>按设定的 Cron 表达式周期性自动启动独立 Agent 会话执行</span>
                    </div>
                  </label>
                </div>

                <!-- Schedule inputs depending on type -->
                <div v-if="stage2ScheduleType === 'once'" class="schedule-input-row">
                  <label class="input-label" for="stage2-datetime">执行时刻：</label>
                  <input
                    id="stage2-datetime"
                    v-model="stage2ScheduledAt"
                    type="datetime-local"
                    class="datetime-input"
                  />
                </div>

                <div v-else-if="stage2ScheduleType === 'cron'" class="schedule-input-row">
                  <label class="input-label" for="stage2-cron">Cron 表达式：</label>
                  <input
                    id="stage2-cron"
                    v-model="stage2CronExpr"
                    type="text"
                    placeholder="0 * * * *"
                    class="cron-input"
                  />
                  <div class="preset-chips">
                    <button
                      v-for="p in cronPresets"
                      :key="p.expr"
                      type="button"
                      class="preset-chip"
                      @click="stage2CronExpr = p.expr"
                    >
                      {{ p.label }}
                    </button>
                  </div>
                </div>
              </div>

              <!-- Bottom Confirmation Actions -->
              <div class="confirm-actions-bar">
                <button
                  type="button"
                  class="btn-secondary"
                  @click="switchTransactionalStage(1)"
                >
                  ⬅ 返回分析会话修改
                </button>
                <button
                  type="button"
                  class="btn-save-prompt btn-confirm-ready btn-primary"
                  :disabled="savingStage2"
                  @click="confirmStage2AndAdvance"
                >
                  {{ savingStage2 ? '正在保存就绪...' : '✓ 确认执行方案并就绪（进入执行） ➔' }}
                </button>
              </div>
            </div>
          </section>

          <!-- STAGE 3: Execution & Monitoring Dedicated View -->
          <section v-else class="stage-view stage-execution-view">
            <!-- Ready & Control Panel -->
            <div class="execution-control-panel">
              <div class="control-left">
                <div class="ready-status-badge">
                  <span class="status-indicator"></span>
                  <strong>任务已就绪</strong>
                </div>

                <div v-if="activeTask.scheduleEnabled" class="schedule-status-chip">
                  <span class="chip-icon">⏰</span>
                  <span v-if="activeTask.scheduleType === 'cron'">周期调度：<code>{{ activeTask.cronExpr }}</code></span>
                  <span v-else>定时执行</span>
                </div>

                <div class="plan-summary-toggle" @click="showPromptDetails = !showPromptDetails">
                  <span>{{ showPromptDetails ? '▼ 收起执行方案提示词' : '▶ 查看当前执行方案提示词' }}</span>
                </div>
              </div>

              <div class="control-right">
                <button
                  type="button"
                  class="btn-secondary btn-sm"
                  @click="switchTransactionalStage(2)"
                  title="重新核对或微调执行提示词"
                >
                  📝 调整方案提示词
                </button>
                <button
                  type="button"
                  class="btn-secondary btn-sm"
                  @click="switchTransactionalStage(1)"
                  title="回看任务分析主会话"
                >
                  💬 查看分析记录
                </button>
                <button
                  type="button"
                  class="btn-trigger btn-lg"
                  :disabled="triggering"
                  @click="triggerExecutionNow"
                  title="立即启动一次独立执行会话"
                >
                  {{ triggering ? '正在启动...' : '▶ 立即执行一次' }}
                </button>
              </div>
            </div>

            <!-- Collapsible Prompt Preview -->
            <div v-if="showPromptDetails" class="prompt-preview-drawer">
              <div class="drawer-header">
                <strong>执行方案提示词（每次独立执行依据）：</strong>
              </div>
              <pre class="drawer-content">{{ activeTask.executionPrompt || '（未设置）' }}</pre>
            </div>

            <!-- Execution Runs Section -->
            <div class="runs-workbench-wrap">
              <div class="runs-header-bar">
                <div class="runs-header-title">
                  <h4>独立执行记录（共 {{ taskRuns.length }} 次）</h4>
                  <span class="runs-sub-hint">事务型任务每次执行均新启动独立 Agent 会话免打断执行</span>
                </div>

                <div v-if="taskRuns.length > 0" class="runs-tab-scroll">
                  <button
                    v-for="r in taskRuns"
                    :key="r.id"
                    type="button"
                    class="run-tab-chip"
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
              </div>

              <!-- Execution Session Display -->
              <div v-if="taskRuns.length === 0" class="empty-runs-card">
                <div class="empty-runs-icon">🚀</div>
                <h3>等待首次执行</h3>
                <p>任务方案已确认就绪，点击上方【▶ 立即执行一次】启动免打断运行，或等待设定的定时/周期调度自动触发。</p>
              </div>

              <div v-else-if="viewingSessionId" class="embedded-chat-panel full-height">
                <SessionChatPanel
                  :key="viewingSessionId"
                  :project-id="activeTask.projectId"
                  :session-id="viewingSessionId"
                />
              </div>
            </div>
          </section>
        </template>

        <!-- =================================================================== -->
        <!-- DEVELOPMENT WORKFLOW: Stepper & Continuous Session                 -->
        <!-- =================================================================== -->
        <template v-else>
          <!-- Orchestration Flow Stepper -->
          <section class="stepper-section">
            <FlowStepper
              :task="activeTask"
              :flow-def="activeFlowDef"
              @advance="handleAdvanceStep"
              @save-prompt="handleSavePrompt"
            />
          </section>

          <!-- Session Execution Area -->
          <section class="session-section">
            <div class="section-title-bar">
              <div class="session-label-group">
                <h3>任务全流程执行会话</h3>
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
                  点击下方按钮拉起会话，Agent 将从第一步【需求分析】开始介入处理。
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
        </template>
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
  height: 100%;
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
  margin: 0 0 20px 0;
  line-height: 1.5;
}

.task-detail-pane {
  display: flex;
  flex-direction: column;
  height: 100%;
  padding: 20px 24px;
  gap: 16px;
  overflow-y: auto;
}

/* Header */
.detail-header-panel {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 20px;
  padding-bottom: 16px;
  border-bottom: 1px solid var(--color-border);
}

.title-section {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.title-row {
  display: flex;
  align-items: center;
  gap: 10px;
}

.flow-badge {
  font-size: 0.76rem;
  padding: 3px 8px;
  border-radius: 4px;
  font-weight: 600;
}

.flow-badge.transactional {
  background: rgba(139, 92, 246, 0.2);
  color: #a78bfa;
}

.flow-badge.development {
  background: rgba(14, 165, 233, 0.2);
  color: #38bdf8;
}

.task-main-title {
  margin: 0;
  font-size: 1.35rem;
  font-weight: 700;
  color: var(--color-text);
}

.status-badge-lg {
  font-size: 0.78rem;
  padding: 3px 10px;
  border-radius: 12px;
  font-weight: 600;
}

.task-main-desc {
  margin: 0;
  font-size: 0.88rem;
  color: var(--color-text-muted);
  line-height: 1.4;
}

.meta-bar {
  display: flex;
  flex-wrap: wrap;
  gap: 14px;
  font-size: 0.82rem;
  color: var(--color-text-muted);
  align-items: center;
}

.meta-item {
  display: flex;
  align-items: center;
  gap: 4px;
}

.meta-item strong {
  color: var(--color-text);
}

.meta-item code {
  font-family: monospace;
  background: var(--color-surface);
  padding: 1px 4px;
  border-radius: 3px;
  color: var(--color-text);
}

.schedule-active {
  color: #fbbf24;
}

.schedule-active strong {
  color: #fbbf24;
}

.header-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
}

.btn-schedule {
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  color: var(--color-text);
  padding: 6px 12px;
  border-radius: var(--radius);
  font-size: 0.82rem;
  cursor: pointer;
  transition: all 0.2s;
}

.btn-schedule:hover {
  border-color: #fbbf24;
  color: #fbbf24;
}

.btn-schedule.active {
  border-color: #fbbf24;
  color: #fbbf24;
  background: rgba(245, 158, 11, 0.1);
}

.btn-trigger {
  background: #10b981;
  color: #fff;
  border: none;
  padding: 6px 14px;
  border-radius: var(--radius);
  font-size: 0.84rem;
  font-weight: 600;
  cursor: pointer;
  transition: all 0.2s;
}

.btn-trigger:hover:not(:disabled) {
  background: #059669;
}

.btn-trigger:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

.btn-retry {
  background: #f59e0b;
  color: #fff;
  border: none;
  padding: 6px 14px;
  border-radius: var(--radius);
  font-size: 0.84rem;
  font-weight: 600;
  cursor: pointer;
}

.btn-completed {
  background: rgba(16, 185, 129, 0.2);
  color: #34d399;
  border: 1px solid #10b981;
  padding: 6px 14px;
  border-radius: var(--radius);
  font-size: 0.84rem;
  font-weight: 600;
}

.btn-secondary {
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  color: var(--color-text);
  padding: 6px 12px;
  border-radius: var(--radius);
  font-size: 0.82rem;
  cursor: pointer;
}

.btn-danger {
  background: transparent;
  border: 1px solid rgba(239, 68, 68, 0.4);
  color: #ef4444;
  padding: 6px 12px;
  border-radius: var(--radius);
  font-size: 0.82rem;
  cursor: pointer;
}

.btn-danger:hover {
  background: rgba(239, 68, 68, 0.1);
}

.action-error-banner {
  background: rgba(239, 68, 68, 0.15);
  border: 1px solid #ef4444;
  color: #f87171;
  padding: 8px 14px;
  border-radius: 6px;
  font-size: 0.85rem;
  margin: 0;
}

/* ========================================================================== */
/* STAGE STEPPER BAR (Transactional)                                          */
/* ========================================================================== */
.stage-stepper-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: 8px;
  padding: 12px 20px;
  gap: 12px;
}

.stage-step-item {
  display: flex;
  align-items: center;
  gap: 10px;
  cursor: pointer;
  padding: 6px 12px;
  border-radius: 6px;
  transition: all 0.2s;
  flex: 1;
}

.stage-step-item:hover {
  background: rgba(255, 255, 255, 0.04);
}

.stage-step-item.active {
  background: rgba(59, 130, 246, 0.12);
  border: 1px solid rgba(59, 130, 246, 0.35);
}

.stage-step-item.completed:not(.active) .step-num {
  background: #10b981;
  border-color: #10b981;
  color: #fff;
}

.step-num {
  width: 28px;
  height: 28px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 0.82rem;
  font-weight: 700;
  border: 2px solid var(--color-border);
  color: var(--color-text-muted);
  background: var(--color-bg);
  flex-shrink: 0;
}

.stage-step-item.active .step-num {
  background: var(--color-primary);
  border-color: var(--color-primary);
  color: #fff;
  box-shadow: 0 0 8px rgba(59, 130, 246, 0.5);
}

.step-label {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.step-title-text {
  font-size: 0.88rem;
  font-weight: 600;
  color: var(--color-text);
}

.stage-step-item.active .step-title-text {
  color: #60a5fa;
}

.step-status-sub {
  font-size: 0.72rem;
  color: var(--color-text-muted);
}

.stage-step-arrow {
  color: var(--color-border);
  font-size: 0.85rem;
  user-select: none;
}

/* ========================================================================== */
/* STAGE 1: ANALYSIS VIEW                                                     */
/* ========================================================================== */
.stage-view {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 0;
  gap: 12px;
}

.stage-banner {
  background: rgba(59, 130, 246, 0.06);
  border-left: 3px solid var(--color-primary);
  padding: 10px 14px;
  border-radius: 0 6px 6px 0;
}

.banner-badge {
  font-size: 0.8rem;
  font-weight: 700;
  color: #60a5fa;
  display: block;
  margin-bottom: 2px;
}

.banner-desc {
  margin: 0;
  font-size: 0.82rem;
  color: var(--color-text-muted);
  line-height: 1.4;
}

.banner-desc strong {
  color: var(--color-text);
}

.analysis-session-wrap {
  flex: 1;
  min-height: 380px;
  display: flex;
  flex-direction: column;
}

.stage-bottom-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: 8px;
  padding: 12px 18px;
  gap: 16px;
}

.bottom-bar-info {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 0.82rem;
  color: var(--color-text-muted);
}

.tip-icon {
  font-size: 1rem;
}

.btn-extract-next {
  background: linear-gradient(135deg, #3b82f6 0%, #2563eb 100%);
  color: #fff;
  border: none;
  padding: 8px 18px;
  border-radius: 6px;
  font-size: 0.88rem;
  font-weight: 600;
  cursor: pointer;
  box-shadow: 0 2px 8px rgba(37, 99, 235, 0.35);
  transition: all 0.2s;
  white-space: nowrap;
}

.btn-extract-next:hover:not(:disabled) {
  background: linear-gradient(135deg, #2563eb 0%, #1d4ed8 100%);
  transform: translateY(-1px);
}

.btn-extract-next:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

/* ========================================================================== */
/* STAGE 2: CONFIRMATION VIEW                                                 */
/* ========================================================================== */
.stage-confirm-container {
  display: flex;
  flex-direction: column;
  gap: 16px;
  max-width: 960px;
  margin: 0 auto;
  width: 100%;
}

.confirmation-notice {
  background: rgba(245, 158, 11, 0.12);
  border-left: 3px solid #f59e0b;
  padding: 10px 14px;
  border-radius: 0 6px 6px 0;
}

.notice-badge {
  font-size: 0.8rem;
  font-weight: 700;
  color: #f59e0b;
  margin-bottom: 2px;
}

.notice-text {
  margin: 0;
  font-size: 0.84rem;
  color: var(--color-text);
  line-height: 1.45;
}

.execution-prompt-box {
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: 8px;
  padding: 16px 18px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.prompt-box-header {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.prompt-box-title-wrap {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.prompt-box-title {
  font-size: 0.95rem;
  font-weight: 700;
  color: var(--color-text);
}

.prompt-box-hint {
  font-size: 0.8rem;
  color: var(--color-text-muted);
}

.btn-re-extract {
  background: rgba(59, 130, 246, 0.15);
  color: #60a5fa;
  border: 1px solid rgba(59, 130, 246, 0.35);
  border-radius: 4px;
  padding: 4px 10px;
  font-size: 0.78rem;
  font-weight: 600;
  cursor: pointer;
  transition: all 0.2s;
}

.btn-re-extract:hover:not(:disabled) {
  background: rgba(59, 130, 246, 0.25);
}

.extract-notice-bar {
  background: rgba(16, 185, 129, 0.12);
  border: 1px solid rgba(16, 185, 129, 0.3);
  color: #34d399;
  padding: 6px 12px;
  border-radius: 4px;
  font-size: 0.82rem;
}

.execution-prompt-textarea {
  width: 100%;
  box-sizing: border-box;
  background: var(--color-bg);
  border: 1px solid var(--color-border);
  border-radius: 6px;
  color: var(--color-text);
  font-family: inherit;
  font-size: 0.88rem;
  line-height: 1.5;
  padding: 12px;
  resize: vertical;
  outline: none;
}

.execution-prompt-textarea:focus {
  border-color: var(--color-primary);
}

.schedule-config-card {
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: 8px;
  padding: 16px 18px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.card-head {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.card-head-title {
  font-size: 0.95rem;
  font-weight: 700;
  color: var(--color-text);
}

.card-head-sub {
  font-size: 0.8rem;
  color: var(--color-text-muted);
}

.schedule-options {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 12px;
}

.schedule-opt-item {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  background: var(--color-bg);
  border: 1px solid var(--color-border);
  border-radius: 6px;
  padding: 10px 12px;
  cursor: pointer;
  transition: all 0.2s;
}

.schedule-opt-item.selected {
  border-color: var(--color-primary);
  background: rgba(59, 130, 246, 0.08);
}

.opt-content {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.opt-content strong {
  font-size: 0.84rem;
  color: var(--color-text);
}

.opt-content span {
  font-size: 0.74rem;
  color: var(--color-text-muted);
  line-height: 1.3;
}

.schedule-input-row {
  display: flex;
  align-items: center;
  gap: 10px;
  padding-top: 6px;
  border-top: 1px dashed var(--color-border);
}

.input-label {
  font-size: 0.84rem;
  color: var(--color-text-muted);
  min-width: 90px;
}

.datetime-input,
.cron-input {
  background: var(--color-bg);
  border: 1px solid var(--color-border);
  border-radius: 4px;
  padding: 6px 10px;
  color: var(--color-text);
  font-size: 0.84rem;
  outline: none;
}

.cron-input {
  width: 140px;
  font-family: monospace;
}

.preset-chips {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
}

.preset-chip {
  background: var(--color-surface-muted);
  border: 1px solid var(--color-border);
  border-radius: 4px;
  padding: 3px 8px;
  font-size: 0.76rem;
  color: var(--color-text-muted);
  cursor: pointer;
}

.preset-chip:hover {
  color: var(--color-text);
  border-color: var(--color-primary);
}

.confirm-actions-bar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding-top: 6px;
}

.btn-confirm-ready {
  background: #10b981;
  color: #fff;
  border: none;
  padding: 10px 22px;
  border-radius: 6px;
  font-size: 0.9rem;
  font-weight: 700;
  cursor: pointer;
  box-shadow: 0 2px 8px rgba(16, 185, 129, 0.35);
  transition: all 0.2s;
}

.btn-confirm-ready:hover:not(:disabled) {
  background: #059669;
}

/* ========================================================================== */
/* STAGE 3: EXECUTION & MONITORING VIEW                                      */
/* ========================================================================== */
.execution-control-panel {
  display: flex;
  align-items: center;
  justify-content: space-between;
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: 8px;
  padding: 12px 18px;
  gap: 16px;
}

.control-left {
  display: flex;
  align-items: center;
  gap: 14px;
  flex-wrap: wrap;
}

.ready-status-badge {
  display: flex;
  align-items: center;
  gap: 6px;
  color: #34d399;
  font-size: 0.88rem;
}

.status-indicator {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: #10b981;
  box-shadow: 0 0 6px #10b981;
}

.schedule-status-chip {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: 0.8rem;
  color: #fbbf24;
  background: rgba(245, 158, 11, 0.12);
  padding: 3px 8px;
  border-radius: 4px;
}

.plan-summary-toggle {
  font-size: 0.8rem;
  color: var(--color-primary);
  cursor: pointer;
  user-select: none;
}

.plan-summary-toggle:hover {
  text-decoration: underline;
}

.control-right {
  display: flex;
  align-items: center;
  gap: 8px;
}

.btn-sm {
  padding: 5px 10px;
  font-size: 0.8rem;
}

.btn-lg {
  padding: 8px 18px;
  font-size: 0.88rem;
  font-weight: 700;
}

.prompt-preview-drawer {
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: 6px;
  padding: 12px 16px;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.drawer-header {
  font-size: 0.82rem;
  color: var(--color-text-muted);
}

.drawer-content {
  margin: 0;
  font-family: inherit;
  font-size: 0.84rem;
  color: var(--color-text);
  white-space: pre-wrap;
  max-height: 160px;
  overflow-y: auto;
  line-height: 1.45;
  background: var(--color-bg);
  padding: 8px 10px;
  border-radius: 4px;
}

.runs-workbench-wrap {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-height: 380px;
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: 8px;
  overflow: hidden;
}

.runs-header-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 16px;
  border-bottom: 1px solid var(--color-border);
  background: rgba(255, 255, 255, 0.02);
  gap: 16px;
}

.runs-header-title {
  display: flex;
  align-items: center;
  gap: 10px;
}

.runs-header-title h4 {
  margin: 0;
  font-size: 0.92rem;
  font-weight: 700;
}

.runs-sub-hint {
  font-size: 0.76rem;
  color: var(--color-text-muted);
}

.runs-tab-scroll {
  display: flex;
  gap: 6px;
  overflow-x: auto;
  max-width: 60%;
}

.run-tab-chip {
  display: flex;
  align-items: center;
  gap: 4px;
  background: var(--color-bg);
  border: 1px solid var(--color-border);
  border-radius: 12px;
  padding: 3px 10px;
  font-size: 0.76rem;
  color: var(--color-text-muted);
  cursor: pointer;
  white-space: nowrap;
  transition: all 0.2s;
}

.run-tab-chip.active {
  background: rgba(59, 130, 246, 0.15);
  border-color: var(--color-primary);
  color: #60a5fa;
  font-weight: 600;
}

.run-tab-chip.success .run-badge {
  color: #34d399;
}

.run-tab-chip.failed .run-badge {
  color: #f87171;
}

.empty-runs-card {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 40px;
  text-align: center;
}

.empty-runs-icon {
  font-size: 2.5rem;
  margin-bottom: 10px;
}

.empty-runs-card h3 {
  margin: 0 0 6px 0;
  font-size: 1.15rem;
}

.empty-runs-card p {
  margin: 0;
  font-size: 0.85rem;
  color: var(--color-text-muted);
  max-width: 480px;
  line-height: 1.5;
}

/* ========================================================================== */
/* COMMON SESSION & DEVELOPMENT STYLES                                       */
/* ========================================================================== */
.stepper-section {
  flex-shrink: 0;
}

.session-section {
  flex: 1;
  min-height: 380px;
  display: flex;
  flex-direction: column;
}

.section-title-bar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 10px 0;
}

.session-label-group h3 {
  margin: 0;
  font-size: 1rem;
}

.session-id-tag {
  font-family: monospace;
  font-size: 0.78rem;
  color: var(--color-text-muted);
}

.no-session-card {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--color-surface);
  border: 1px dashed var(--color-border);
  border-radius: 8px;
  padding: 30px;
}

.no-session-content {
  text-align: center;
  max-width: 480px;
}

.no-session-content h4 {
  margin: 0 0 8px 0;
  font-size: 1.1rem;
}

.no-session-content p {
  color: var(--color-text-muted);
  font-size: 0.86rem;
  margin: 0 0 16px 0;
  line-height: 1.45;
}

.btn-launch-session {
  background: var(--color-primary);
  color: #fff;
  border: none;
  padding: 8px 18px;
  border-radius: var(--radius);
  font-size: 0.88rem;
  font-weight: 600;
  cursor: pointer;
}

.embedded-chat-panel {
  flex: 1;
  min-height: 380px;
  border: 1px solid var(--color-border);
  border-radius: 8px;
  overflow: hidden;
  display: flex;
  flex-direction: column;
}

.embedded-chat-panel.full-height {
  height: 100%;
}
</style>
