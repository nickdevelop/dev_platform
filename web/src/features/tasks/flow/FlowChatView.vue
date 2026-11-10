<script setup lang="ts">
// FlowChatView: Main IM collaborative group chat interface for multi-agent workflows
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import type { Task, FlowRole } from '../../../api/tasks'
import { useTasksStore } from '../../../stores/tasks'
import { useProjectsStore } from '../../../stores/projects'
import FlowMessageItem from './FlowMessageItem.vue'
import FlowRoleBadge from './FlowRoleBadge.vue'
import FlowRoleManager from './FlowRoleManager.vue'
import TaskChatDialog from '../TaskChatDialog.vue'
import { UiButton, UiDialog, UiAlert } from '../../../shared/ui'

const props = defineProps<{
  task: Task
  runId?: number
}>()

const tasksStore = useTasksStore()
const projectsStore = useProjectsStore()

const messagesContainer = ref<HTMLElement | null>(null)
const userInput = ref('')
const sending = ref(false)
const canceling = ref(false)
const showRoleManager = ref(false)

// Session Drawer state
const viewingSessionId = ref<string | null>(null)
const showSessionDialog = ref(false)

const roles = computed<FlowRole[]>(() => tasksStore.rolesByTaskId(props.task.id))
const flowRuns = computed(() => tasksStore.flowRunsByTaskId(props.task.id))
const activeRun = computed(() => {
  const run = tasksStore.activeFlowRun
  if (!run || run.taskId !== props.task.id) {
    return null
  }
  return run
})
const messages = computed(() => {
  if (!activeRun.value) return []
  return tasksStore.messagesByRunId(activeRun.value.id)
})

const projectName = computed(() => {
  return projectsStore.byId(props.task.projectId)?.name || `项目 #${props.task.projectId}`
})

const isRunning = computed(() => activeRun.value?.status === 'running')
const isWaitingUser = computed(() => activeRun.value?.status === 'waiting_user')
const isCompleted = computed(() => activeRun.value?.status === 'completed')
const isCancelled = computed(() => activeRun.value?.status === 'cancelled')
const isFailed = computed(() => activeRun.value?.status === 'failed')

function scrollToBottom() {
  nextTick(() => {
    if (messagesContainer.value) {
      messagesContainer.value.scrollTop = messagesContainer.value.scrollHeight
    }
  })
}

watch(
  () => messages.value.length,
  () => {
    scrollToBottom()
  },
)

// Also scroll on content chunks
watch(
  () => messages.value[messages.value.length - 1]?.content,
  () => {
    scrollToBottom()
  },
)

async function initializeRun(runId?: number) {
  if (tasksStore.activeFlowRun?.id) {
    tasksStore.disconnectFlowEvents(tasksStore.activeFlowRun.id)
  }
  await tasksStore.loadRoles(props.task.id)
  const runs = await tasksStore.loadFlowRuns(props.task.id)

  let targetId = runId
  if (!targetId && runs.length > 0) {
    targetId = runs[0].id
  }

  if (targetId) {
    await selectRun(targetId)
  } else {
    tasksStore.resetActiveFlowRun()
  }
}

async function selectRun(runId: number) {
  if (tasksStore.activeFlowRun?.id) {
    tasksStore.disconnectFlowEvents(tasksStore.activeFlowRun.id)
  }
  const run = await tasksStore.loadFlowRun(runId)
  if (run && run.taskId === props.task.id) {
    await tasksStore.loadFlowMessages(run.id)
    tasksStore.connectFlowEvents(run.id)
    scrollToBottom()
  }
}

watch(
  () => props.task.id,
  async (newTaskId, oldTaskId) => {
    if (newTaskId && newTaskId !== oldTaskId) {
      userInput.value = ''
      await initializeRun()
    }
  },
)

onMounted(async () => {
  await initializeRun(props.runId)
})

onBeforeUnmount(() => {
  if (tasksStore.activeFlowRun?.id) {
    tasksStore.disconnectFlowEvents(tasksStore.activeFlowRun.id)
  }
})

// Send user message in the collaborative chat
async function handleSendMessage() {
  const text = userInput.value.trim()
  if (!text || !activeRun.value || sending.value) return

  sending.value = true
  userInput.value = ''
  try {
    await tasksStore.postFlowMessage(activeRun.value.id, text)
    scrollToBottom()
  } catch (err: any) {
    alert(err.message || '发送消息失败')
  } finally {
    sending.value = false
  }
}

function handleKeydown(e: KeyboardEvent) {
  if (e.key === 'Enter' && (e.ctrlKey || e.metaKey || !e.shiftKey)) {
    e.preventDefault()
    handleSendMessage()
  }
}

// Cancel flow run
async function handleCancel() {
  if (!activeRun.value || canceling.value) return
  if (!confirm('确定要中止当前正在运行的协同流程吗？')) return

  canceling.value = true
  try {
    await tasksStore.cancelFlowRun(activeRun.value.id)
  } catch (err: any) {
    alert(err.message || '中止流程失败')
  } finally {
    canceling.value = false
  }
}

// Trigger a fresh execution
async function handleRestart() {
  if (isRunning.value) return
  try {
    const res = await tasksStore.triggerExecution(props.task.id, 'manual')
    if (res.run) {
      await tasksStore.loadFlowRuns(props.task.id)
      const latestRuns = tasksStore.flowRunsByTaskId(props.task.id)
      if (latestRuns.length > 0) {
        await selectRun(latestRuns[0].id)
      }
    }
  } catch (err: any) {
    alert(err.message || '启动协同流程失败')
  }
}

function handleOpenSession(sessionId: string) {
  viewingSessionId.value = sessionId
  showSessionDialog.value = true
}

function getRoleById(roleId?: number): FlowRole | null {
  if (!roleId) return null
  return roles.value.find((r) => r.id === roleId) || null
}
</script>

<template>
  <div class="flow-chat-view flex flex-col h-full bg-[var(--color-bg)] overflow-hidden" data-test="flow-chat-view">
    <!-- 1. Top Bar: Title, Step Progress, Status and Actions -->
    <header class="chat-header px-4 py-2.5 border-b border-[var(--color-border)] bg-[var(--color-surface)] flex items-center justify-between gap-4 shrink-0">
      <div class="flex items-center gap-3 min-w-0">
        <!-- Back button if triggered from list: not rendered — the only current
             usage embeds FlowChatView directly inside TaskDetailView, which has
             no 'back' listener, and emit() cannot report whether one exists. -->
        <div class="min-w-0">
          <div class="flex items-center gap-2 flex-wrap">
            <span class="text-xs px-2 py-0.5 rounded bg-blue-500/10 text-blue-500 font-semibold border border-blue-500/20">
              多 Agent 协同群聊
            </span>
            <h2 class="text-sm font-semibold text-[var(--color-text)] truncate">
              {{ task.title }}
            </h2>
          </div>
          <div class="flex items-center gap-3 text-2xs text-[var(--color-text-muted)] font-mono">
            <span>工作区: {{ projectName }}</span>
            <span>中控模型: {{ activeRun?.orchestratorModel || task.orchestratorModel || task.model || '-' }}</span>
            <span v-if="activeRun" class="text-amber-500 font-semibold">
              步骤: {{ activeRun.currentStep }} / {{ activeRun.maxSteps }}
            </span>
            <span v-else class="text-slate-400">
              步骤: 0 / 20
            </span>
          </div>
        </div>
      </div>

      <!-- Action buttons & status indicator -->
      <div class="flex items-center gap-2.5 shrink-0">
        <!-- Status indicator badge -->
        <span
          v-if="activeRun"
          class="status-pill text-xs px-2 py-0.5 rounded font-mono font-medium flex items-center gap-1.5 border"
          :class="{
            'bg-blue-500/10 text-blue-500 border-blue-500/30': isRunning,
            'bg-amber-500/10 text-amber-500 border-amber-500/30': isWaitingUser,
            'bg-emerald-500/10 text-emerald-500 border-emerald-500/30': isCompleted,
            'bg-rose-500/10 text-rose-500 border-rose-500/30': isFailed,
            'bg-slate-500/10 text-slate-400 border-slate-500/30': isCancelled,
          }"
        >
          <span
            class="w-1.5 h-1.5 rounded-full"
            :class="{
              'bg-blue-500 animate-ping': isRunning,
              'bg-amber-500': isWaitingUser,
              'bg-emerald-500': isCompleted,
              'bg-rose-500': isFailed,
              'bg-slate-400': isCancelled,
            }"
          />
          <span v-if="isRunning">运行推进中…</span>
          <span v-else-if="isWaitingUser">等待用户介入</span>
          <span v-else-if="isCompleted">流程已完成</span>
          <span v-else-if="isFailed">执行失败</span>
          <span v-else-if="isCancelled">已手动中止</span>
        </span>
        <span
          v-else
          class="status-pill text-xs px-2 py-0.5 rounded font-mono font-medium flex items-center gap-1.5 border bg-slate-500/10 text-slate-500 border-slate-500/30"
          data-test="flow-status-idle"
        >
          <span class="w-1.5 h-1.5 rounded-full bg-slate-400" />
          <span>未执行</span>
        </span>

        <!-- Cancel button when running -->
        <UiButton
          v-if="isRunning"
          variant="danger"
          class="text-xs px-2.5 py-1"
          data-test="btn-cancel-flow"
          :loading="canceling"
          @click="handleCancel"
        >
          中止执行
        </UiButton>

        <!-- Rerun or Start button when not running -->
        <UiButton
          v-else
          variant="primary"
          class="text-xs px-2.5 py-1"
          data-test="btn-rerun-flow"
          @click="handleRestart"
        >
          {{ activeRun ? '重新启动' : '启动协同流程' }}
        </UiButton>

        <!-- Role management button -->
        <UiButton
          variant="secondary"
          class="text-xs px-2 py-1"
          title="管理参与角色"
          @click="showRoleManager = true"
        >
          <svg viewBox="0 0 16 16" width="12" height="12" fill="none" stroke="currentColor" stroke-width="1.8" class="mr-1 inline-block">
            <path d="M11 14v-2a3 3 0 0 0-3-3H4a3 3 0 0 0-3 3v2" />
            <circle cx="6" cy="4" r="3" />
            <path d="M15 14v-1.5a2.5 2.5 0 0 0-2-2.45" />
            <path d="M11.5 1.5a2.5 2.5 0 0 1 0 4.9" />
          </svg>
          角色 ({{ roles.length }})
        </UiButton>
      </div>
    </header>

    <!-- 2. Roles Sub-header: Quick access strip -->
    <section class="roles-strip px-4 py-2 bg-[var(--color-surface)]/70 border-b border-[var(--color-border)] flex items-center gap-2 overflow-x-auto shrink-0 select-none">
      <span class="text-2xs text-[var(--color-text-muted)] shrink-0 font-medium">团队成员:</span>
      <!-- Orchestrator Pill -->
      <span class="inline-flex items-center gap-1 px-2 py-0.5 rounded text-2xs font-semibold bg-blue-500/10 text-blue-500 border border-blue-500/20">
        <svg viewBox="0 0 16 16" width="10" height="10" fill="none" stroke="currentColor" stroke-width="2">
          <circle cx="8" cy="8" r="3" />
        </svg>
        中控调度大脑
      </span>

      <!-- Configured Agent Roles -->
      <FlowRoleBadge
        v-for="r in roles"
        :key="r.id"
        :role="r"
        interactive
        @click="showRoleManager = true"
      />

      <button
        type="button"
        class="text-2xs text-[var(--color-primary)] hover:underline ml-1 shrink-0 font-medium"
        @click="showRoleManager = true"
      >
        + 配置角色
      </button>
    </section>

    <!-- 3. IM Conversation Scroll Area -->
    <main
      ref="messagesContainer"
      class="chat-messages-area flex-1 min-h-0 overflow-y-auto p-4 md:p-6 space-y-1"
      data-test="flow-messages-area"
    >
      <!-- History Runs Selector if multiple runs exist -->
      <div v-if="flowRuns.length > 1" class="flex justify-center mb-4">
        <div class="inline-flex items-center gap-2 px-3 py-1 rounded bg-[var(--color-surface)] border border-[var(--color-border)] text-xs text-[var(--color-text-muted)]">
          <span>历史轮次:</span>
          <select
            :value="activeRun?.id"
            class="bg-transparent text-[var(--color-text)] border-none text-xs focus:outline-none font-mono cursor-pointer"
            @change="(e: any) => selectRun(Number(e.target.value))"
          >
            <option
              v-for="r in flowRuns"
              :key="r.id"
              :value="r.id"
            >
              轮次 #{{ r.id }} ({{ r.status }} / 步骤 {{ r.currentStep }})
            </option>
          </select>
        </div>
      </div>

      <!-- Messages list -->
      <template v-if="messages.length > 0">
        <FlowMessageItem
          v-for="msg in messages"
          :key="msg.id"
          :message="msg"
          :role="getRoleById(msg.roleId)"
          :roles="roles"
          @open-session="handleOpenSession"
        />
      </template>

      <!-- Empty state when no run exists -->
      <div
        v-else-if="!activeRun"
        class="h-full flex flex-col items-center justify-center text-center p-8 space-y-3"
      >
        <div class="w-10 h-10 rounded-full bg-blue-500/10 text-blue-500 flex items-center justify-center">
          <svg viewBox="0 0 16 16" width="20" height="20" fill="none" stroke="currentColor" stroke-width="1.8">
            <polygon points="5 3 13 8 5 13 5 3" />
          </svg>
        </div>
        <h3 class="text-sm font-semibold text-[var(--color-text)]">协同流程尚未启动</h3>
        <p class="text-xs text-[var(--color-text-muted)] max-w-sm leading-relaxed">
          点击下方按钮启动多 Agent 协同流程。中控大脑将自动分析目标，并在群聊中 @指派专业角色分工完成。
        </p>
        <UiButton
          variant="primary"
          class="text-xs px-4 py-2 font-medium"
          @click="handleRestart"
        >
          立即开始协同流程
        </UiButton>
      </div>
    </main>

    <!-- 4. Bottom Input Bar: User Participation & Intervention -->
    <footer class="chat-footer p-3 border-t border-[var(--color-border)] bg-[var(--color-surface)] shrink-0 space-y-2">
      <!-- Waiting user notice if paused -->
      <div
        v-if="isWaitingUser"
        class="text-xs px-3 py-1.5 rounded bg-amber-500/10 border border-amber-500/30 text-amber-600 dark:text-amber-400 flex items-center justify-between gap-2"
      >
        <span>中控已达到最大协同轮次挂起，请在下方输入意见或调整指令唤醒中控。</span>
      </div>

      <div class="input-row flex items-end gap-2.5">
        <div class="flex-1 relative">
          <textarea
            v-model="userInput"
            rows="2"
            class="w-full text-xs md:text-sm p-2.5 rounded-[var(--radius,4px)] bg-[var(--color-surface-muted)] border border-[var(--color-border)] text-[var(--color-text)] placeholder:text-[var(--color-text-muted)] focus:outline-none focus:border-[var(--color-primary)] resize-none leading-relaxed"
            placeholder="在群聊中输入补充需求、调整意见或干预指令… (Enter 发送, Shift+Enter 换行)"
            :disabled="!activeRun"
            @keydown="handleKeydown"
          />
        </div>

        <UiButton
          variant="primary"
          class="h-10 px-4 text-xs font-semibold shrink-0"
          :disabled="!userInput.trim() || !activeRun || sending"
          :loading="sending"
          @click="handleSendMessage"
        >
          发送
        </UiButton>
      </div>
    </footer>

    <!-- Role Manager Dialog -->
    <UiDialog
      v-if="showRoleManager"
      title="配置 Agent 团队角色"
      width="680px"
      @close="showRoleManager = false"
    >
      <FlowRoleManager
        :task-id="task.id"
        :roles="roles"
        @updated="tasksStore.loadRoles(task.id)"
      />
    </UiDialog>

    <!-- Coding Agent Workspace Session Dialog -->
    <TaskChatDialog
      v-if="showSessionDialog && viewingSessionId"
      :title="`协同子会话: ${viewingSessionId}`"
      :project-id="task.projectId"
      :session-id="viewingSessionId"
      @close="showSessionDialog = false"
    />
  </div>
</template>

<style scoped>
.chat-messages-area {
  scroll-behavior: smooth;
}
</style>
