<script setup lang="ts">
// GroupFlowChatDialog: Collaborative IM view for running, inspecting, or triggering a group task.
// Uses Teleport to body with ESC listener and backdrop click-to-close to guarantee seamless closure.
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import type { GroupTask, GroupMemberDetail } from '../../api/groups'
import type { FlowMessage, FlowRole, FlowRun } from '../../api/tasks'
import { useGroupsStore } from '../../stores/groups'
import { useProjectsStore } from '../../stores/projects'
import FlowMessageItem from '../tasks/flow/FlowMessageItem.vue'
import BackendIcon from '../../shared/components/BackendIcon.vue'
import TaskChatDialog from '../tasks/TaskChatDialog.vue'
import { UiButton, UiAlert } from '../../shared/ui'

const props = defineProps<{
  open: boolean
  groupId: number
  task: GroupTask
  runId?: number
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
}>()

const groupsStore = useGroupsStore()
const projectsStore = useProjectsStore()

const messagesContainer = ref<HTMLElement | null>(null)
const userInput = ref('')
const sending = ref(false)
const canceling = ref(false)
const starting = ref(false)
const flowError = ref('')

const viewingSessionId = ref<string | null>(null)
const showSessionDialog = ref(false)

const group = computed(() => groupsStore.activeGroupDetail)
const members = computed<GroupMemberDetail[]>(() => group.value?.members || [])

// Map GroupMemberDetail into FlowRole interface for FlowMessageItem
const memberFlowRoles = computed<FlowRole[]>(() => {
  return members.value.map((m) => ({
    id: m.roleId,
    taskId: props.task.id,
    name: m.name,
    description: m.description,
    backend: m.backend,
    model: m.model,
    promptId: m.promptId,
    systemPrompt: m.systemPrompt,
    icon: m.icon,
    color: m.color,
    createdAt: m.createdAt,
  }))
})

function getRoleByRoleId(roleId?: number): FlowRole | undefined {
  if (!roleId) return undefined
  return memberFlowRoles.value.find((r) => r.id === roleId)
}

const flowRuns = computed<FlowRun[]>(() => groupsStore.activeTaskRuns || [])

const currentRun = computed<FlowRun | null>(() => {
  if (props.runId) {
    return flowRuns.value.find((r) => r.id === props.runId) || groupsStore.activeRun
  }
  return groupsStore.activeRun || flowRuns.value[0] || null
})

const messages = computed<FlowMessage[]>(() => groupsStore.flowMessages)

const isRunning = computed(() => currentRun.value?.status === 'running')
const isWaitingUser = computed(() => currentRun.value?.status === 'waiting_user')
const isCompleted = computed(() => currentRun.value?.status === 'completed')
const isCancelled = computed(() => currentRun.value?.status === 'cancelled')
const isFailed = computed(() => currentRun.value?.status === 'failed')

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

watch(
  () => messages.value[messages.value.length - 1]?.content,
  () => {
    scrollToBottom()
  },
)

async function selectRun(runId: number) {
  const target = flowRuns.value.find((r) => r.id === runId)
  if (target) {
    groupsStore.activeRun = target
    await groupsStore.loadRunMessages(target.id)
    groupsStore.subscribeRunEvents(target.id)
    scrollToBottom()
  }
}

async function handleStartTask() {
  starting.value = true
  flowError.value = ''
  try {
    const run = await groupsStore.triggerTask(props.groupId, props.task.id)
    if (run) {
      await groupsStore.loadRunMessages(run.id)
      groupsStore.subscribeRunEvents(run.id)
      scrollToBottom()
    }
  } catch (err: any) {
    flowError.value = err?.message || '启动协同任务失败'
  } finally {
    starting.value = false
  }
}

async function handleSendMessage() {
  const text = userInput.value.trim()
  if (!text || !currentRun.value || sending.value) return

  sending.value = true
  flowError.value = ''
  try {
    await groupsStore.sendUserMessage(currentRun.value.id, text)
    userInput.value = ''
    scrollToBottom()
  } catch (err: any) {
    flowError.value = err?.message || '发送消息失败'
  } finally {
    sending.value = false
  }
}

async function handleCancelRun() {
  if (!currentRun.value || canceling.value) return
  canceling.value = true
  try {
    await groupsStore.cancelRun(currentRun.value.id)
  } catch (err: any) {
    flowError.value = err?.message || '中断执行失败'
  } finally {
    canceling.value = false
  }
}

function onViewSession(sessionId: string) {
  viewingSessionId.value = sessionId
  showSessionDialog.value = true
}

function handleClose() {
  groupsStore.resetFlowState()
  emit('update:open', false)
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape') {
    handleClose()
  }
}

watch(
  () => props.open,
  async (isOpen) => {
    if (!isOpen) {
      groupsStore.resetFlowState()
      return
    }

    await groupsStore.loadTaskRuns(props.groupId, props.task.id)
    if (currentRun.value) {
      groupsStore.subscribeRunEvents(currentRun.value.id)
    }
    scrollToBottom()
  },
  { immediate: true },
)

onMounted(() => {
  window.addEventListener('keydown', onKeydown)
})

onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKeydown)
  groupsStore.resetFlowState()
})
</script>

<template>
  <Teleport to="body">
    <div
      v-if="open"
      class="group-flow-chat-backdrop fixed inset-0 z-50 flex items-center justify-center p-3 md:p-6 bg-slate-950/75 select-none"
      role="dialog"
      aria-modal="true"
      data-test="group-flow-chat-dialog"
      @click.self="handleClose"
    >
      <div
        class="group-flow-chat-panel flex flex-col w-full max-w-5xl h-[88vh] bg-[var(--color-surface)] border border-[var(--color-border)] rounded-[var(--radius,8px)] shadow-2xl overflow-hidden select-text"
        tabindex="-1"
      >
        <!-- 1. Header Bar: Task Info, Orchestrator Brain & Members -->
        <header class="chat-dialog-header px-5 py-3 border-b border-[var(--color-border)] bg-[var(--color-surface-muted)] flex items-center justify-between gap-4 shrink-0">
          <div class="flex items-center gap-3 min-w-0">
            <div class="flex items-center gap-2">
              <span
                class="w-2.5 h-2.5 rounded-full shrink-0"
                :class="[
                  isRunning ? 'bg-amber-500 animate-pulse' : isCompleted ? 'bg-emerald-500' : isFailed ? 'bg-red-500' : 'bg-gray-400'
                ]"
                :title="isRunning ? '执行中' : isCompleted ? '已完成' : isFailed ? '失败' : '空闲'"
              />
              <h2 class="text-sm font-semibold text-[var(--color-text)] truncate max-w-md">
                [协同群聊] {{ task.title }}
              </h2>
            </div>

            <!-- Brain Model Badge -->
            <span class="inline-flex items-center gap-1 text-2xs font-mono px-2 py-0.5 rounded bg-[var(--color-surface)] border border-[var(--color-border)] text-[var(--color-text-muted)] shrink-0">
              <svg viewBox="0 0 16 16" width="12" height="12" fill="none" stroke="currentColor" stroke-width="1.8" class="text-blue-500">
                <circle cx="8" cy="8" r="3" />
                <path d="M8 1v2M8 13v2M1 8h2M13 8h2" />
              </svg>
              <span>调度大脑: {{ group?.orchestratorModel || 'Default' }}</span>
            </span>

            <!-- Member Avatars -->
            <div v-if="members.length > 0" class="hidden lg:flex items-center gap-1 pl-2 border-l border-[var(--color-border)]">
              <span class="text-3xs text-[var(--color-text-muted)] mr-1">协同专家:</span>
              <span
                v-for="m in members"
                :key="m.id"
                class="inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-3xs font-medium bg-[var(--color-surface)] border border-[var(--color-border)] text-[var(--color-text)]"
                :title="`${m.name} (${m.backend} / ${m.model})`"
              >
                <span class="w-1.5 h-1.5 rounded-full" :style="{ backgroundColor: m.color || '#3b82f6' }" />
                <span>{{ m.name }}</span>
              </span>
            </div>
          </div>

          <!-- Header Right Actions: History Selector, Interrupt & Close Button -->
          <div class="flex items-center gap-2.5 shrink-0">
            <!-- Run Selector if multiple runs exist -->
            <div v-if="flowRuns.length > 1" class="inline-flex items-center gap-1.5 px-2 py-1 rounded bg-[var(--color-surface)] border border-[var(--color-border)] text-2xs text-[var(--color-text-muted)]">
              <span>历史轮次:</span>
              <select
                :value="currentRun?.id"
                class="bg-transparent text-[var(--color-text)] border-none text-2xs focus:outline-none font-mono cursor-pointer"
                @change="(e: any) => selectRun(Number(e.target.value))"
              >
                <option
                  v-for="r in flowRuns"
                  :key="r.id"
                  :value="r.id"
                >
                  #{{ r.id }} ({{ r.status }})
                </option>
              </select>
            </div>

            <!-- Interrupt button if running -->
            <UiButton
              v-if="isRunning"
              variant="secondary"
              class="text-2xs h-7 px-2.5 text-red-500 hover:text-red-600 hover:bg-red-50 dark:hover:bg-red-950/20"
              :loading="canceling"
              :disabled="canceling"
              @click="handleCancelRun"
            >
              中断协同
            </UiButton>

            <!-- Close Dialog Button (Esc) -->
            <button
              type="button"
              class="close-dialog-btn text-[var(--color-text-muted)] hover:text-[var(--color-text)] hover:bg-[var(--color-surface)] p-1.5 rounded transition-colors inline-flex items-center justify-center border border-transparent hover:border-[var(--color-border)]"
              title="关闭视窗 (Esc)"
              aria-label="关闭视窗"
              data-test="group-flow-chat-close"
              @click="handleClose"
            >
              <svg viewBox="0 0 16 16" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
                <line x1="3" y1="3" x2="13" y2="13" />
                <line x1="13" y1="3" x2="3" y2="13" />
              </svg>
            </button>
          </div>
        </header>

        <!-- 2. Chat Message Stream Body -->
        <main
          ref="messagesContainer"
          class="chat-stream-body flex-1 min-h-0 overflow-y-auto p-4 md:p-6 space-y-4 bg-[var(--color-bg)]"
        >
          <!-- Alert if any -->
          <UiAlert v-if="flowError" variant="danger" class="py-2 text-xs">
            {{ flowError }}
          </UiAlert>

          <!-- Empty State: Task has never run -->
          <div
            v-if="!currentRun && flowRuns.length === 0"
            class="h-full flex flex-col items-center justify-center text-center p-8 space-y-3.5 my-auto"
          >
            <div class="w-12 h-12 rounded-full bg-blue-500/10 text-blue-500 flex items-center justify-center">
              <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="1.8">
                <polygon points="5 3 19 12 5 21 5 3" />
              </svg>
            </div>
            <div class="space-y-1">
              <h3 class="text-sm font-semibold text-[var(--color-text)]">
                协同任务尚未启动
              </h3>
              <p class="text-xs text-[var(--color-text-muted)] max-w-md leading-relaxed">
                中控调度大脑将根据任务需求分析目标，并在群聊中 @指派各领域专家角色分工协同推进。
              </p>
            </div>
            <UiButton
              variant="primary"
              class="text-xs px-5 py-2 font-medium"
              :loading="starting"
              :disabled="starting"
              @click="handleStartTask"
            >
              立即启动协同任务
            </UiButton>
          </div>

          <!-- Empty State: Run started but waiting messages -->
          <div
            v-else-if="messages.length === 0"
            class="py-16 text-center text-[var(--color-text-muted)] space-y-2"
          >
            <div class="inline-block animate-spin text-blue-500 mb-1">
              <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2">
                <circle cx="12" cy="12" r="10" stroke-opacity="0.25" />
                <path d="M12 2a10 10 0 0 1 10 10" />
              </svg>
            </div>
            <div class="text-xs font-medium">中控调度大脑正在规划协同分工…</div>
            <p class="text-3xs">即将向团队角色成员分派子任务，请稍候。</p>
          </div>

          <!-- Message Bubbles Stream -->
          <template v-else>
            <FlowMessageItem
              v-for="msg in messages"
              :key="msg.id"
              :message="msg"
              :role="getRoleByRoleId(msg.roleId)"
              :roles="memberFlowRoles"
              @open-session="onViewSession"
            />
          </template>
        </main>

        <!-- 3. Bottom Intervention & Input Bar -->
        <footer class="chat-dialog-footer p-3.5 bg-[var(--color-surface)] border-t border-[var(--color-border)] shrink-0 space-y-2">
          <!-- Waiting user notice if coordinator suspended -->
          <div
            v-if="isWaitingUser"
            class="text-xs px-3 py-1.5 rounded bg-amber-500/10 border border-amber-500/30 text-amber-600 dark:text-amber-400 flex items-center justify-between gap-2"
          >
            <span>中控大脑已挂起，等待您的人工决策指令。请在下方输入意见唤醒中控。</span>
          </div>

          <form class="flex items-end gap-2.5" @submit.prevent="handleSendMessage">
            <div class="flex-1 relative">
              <textarea
                v-model="userInput"
                rows="2"
                placeholder="在协同群聊中输入补充需求、解答中枢大脑疑问或干预指令… (Enter 发送, Shift+Enter 换行)"
                class="w-full text-xs md:text-sm p-2.5 rounded-[var(--radius,4px)] bg-[var(--color-surface-muted)] border border-[var(--color-border)] text-[var(--color-text)] placeholder:text-[var(--color-text-muted)] focus:outline-none focus:border-[var(--color-primary)] resize-none leading-relaxed font-sans"
                :disabled="!currentRun || sending"
                @keydown.enter.exact.prevent="handleSendMessage"
              />
            </div>
            <UiButton
              type="submit"
              variant="primary"
              class="h-10 px-5 text-xs font-medium shrink-0"
              :disabled="!userInput.trim() || !currentRun || sending"
              :loading="sending"
            >
              发送
            </UiButton>
          </form>
        </footer>
      </div>

      <!-- Agent Session Inspect Dialog -->
      <TaskChatDialog
        v-if="showSessionDialog && viewingSessionId"
        :title="`协同子会话: ${viewingSessionId}`"
        :project-id="task.projectId"
        :session-id="viewingSessionId"
        @close="showSessionDialog = false"
      />
    </div>
  </Teleport>
</template>

<style scoped>
.chat-stream-body {
  scroll-behavior: smooth;
}
</style>
