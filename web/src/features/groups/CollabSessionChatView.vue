<script setup lang="ts">
// CollabSessionChatView: Embedded collaborative session IM view for the workbench.
// Replaces the single-agent conversation column with full multi-agent group flow IM,
// while preserving the right-hand WorkspacePreviewPane.
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import type { AgentSession } from '../../api/sessions'
import type { GroupTask, GroupMemberDetail } from '../../api/groups'
import type { FlowMessage, FlowRole, FlowRun } from '../../api/tasks'
import { useGroupsStore } from '../../stores/groups'
import { useProjectsStore } from '../../stores/projects'
import FlowMessageItem from '../tasks/flow/FlowMessageItem.vue'
import TaskChatDialog from '../tasks/TaskChatDialog.vue'
import { UiButton, UiAlert } from '../../shared/ui'

const props = defineProps<{
  projectId: number
  sessionId: string
  session: AgentSession
}>()

const emit = defineEmits<{
  delete: []
}>()

const groupsStore = useGroupsStore()
const projectsStore = useProjectsStore()

const messagesContainer = ref<HTMLElement | null>(null)
const userInput = ref('')
const sending = ref(false)
const canceling = ref(false)
const starting = ref(false)
const loadingContext = ref(true)
const flowError = ref('')

const viewingSessionId = ref<string | null>(null)
const showSessionDialog = ref(false)

const group = computed(() => groupsStore.activeGroupDetail)
const members = computed<GroupMemberDetail[]>(() => group.value?.members || [])
const currentTask = computed<GroupTask | null>(() => {
  if (groupsStore.activeTaskId) {
    return groupsStore.taskById(groupsStore.activeTaskId) || null
  }
  return null
})

// Map GroupMemberDetail into FlowRole interface for FlowMessageItem
const memberFlowRoles = computed<FlowRole[]>(() => {
  return members.value.map((m) => ({
    id: m.roleId,
    taskId: currentTask.value?.id || 0,
    name: m.name,
    description: m.description,
    backend: m.backend,
    model: m.model,
    promptId: m.promptId,
    systemPrompt: m.systemPrompt,
    icon: m.icon,
    color: m.color,
    createdAt: m.createdAt,
    updatedAt: m.createdAt,
  }))
})

function getRoleByRoleId(roleId?: number): FlowRole | undefined {
  if (!roleId) return undefined
  return memberFlowRoles.value.find((r) => r.id === roleId)
}

const flowRuns = computed<FlowRun[]>(() => groupsStore.activeTaskRuns || [])

const currentRun = computed<FlowRun | null>(() => groupsStore.activeRun || flowRuns.value[0] || null)

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

async function initSessionContext() {
  loadingContext.value = true
  flowError.value = ''
  try {
    const res = await groupsStore.loadCollabSessionContext(props.sessionId)
    if (res.activeRun) {
      await groupsStore.loadRunMessages(res.activeRun.id)
      groupsStore.subscribeRunEvents(res.activeRun.id)
      scrollToBottom()
    }
  } catch (err: any) {
    flowError.value = err?.message || '加载协同会话上下文失败'
  } finally {
    loadingContext.value = false
  }
}

watch(
  () => props.sessionId,
  () => {
    void initSessionContext()
  },
  { immediate: true },
)

onBeforeUnmount(() => {
  groupsStore.resetFlowState()
})

async function handleStartTask() {
  if (!group.value || !currentTask.value) return
  starting.value = true
  flowError.value = ''
  try {
    const run = await groupsStore.triggerTask(group.value.id, currentTask.value.id)
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
  userInput.value = ''
  flowError.value = ''
  try {
    await groupsStore.sendUserMessage(currentRun.value.id, text)
    scrollToBottom()
  } catch (err: any) {
    userInput.value = text
    flowError.value = err?.message || '发送干预指令失败'
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
</script>

<template>
  <div class="collab-session-chat-view flex flex-col flex-1 min-w-0 h-full bg-[var(--color-bg)]" data-test="collab-session-view">
    <!-- Header Bar: Collab Info, Brain Model & Controls -->
    <header class="collab-header px-4 py-2.5 border-b border-[var(--color-border)] bg-[var(--color-surface)] flex items-center justify-between gap-3 shrink-0">
      <div class="flex items-center gap-2.5 min-w-0">
        <span
          class="w-2.5 h-2.5 rounded-full shrink-0"
          :class="[
            isRunning ? 'bg-amber-500 animate-pulse' : isCompleted ? 'bg-emerald-500' : isFailed ? 'bg-red-500' : 'bg-gray-400'
          ]"
          :title="isRunning ? '协同执行中' : isCompleted ? '已完成' : isFailed ? '执行失败' : '空闲'"
        />
        <h2 class="text-sm font-semibold text-[var(--color-text)] truncate max-w-sm" :title="props.session.title || '团队协同会话'">
          {{ props.session.title || '团队协同会话' }}
        </h2>

        <!-- Group Name Badge -->
        <span v-if="group" class="inline-flex items-center gap-1 text-2xs px-2 py-0.5 rounded bg-[var(--color-surface-muted)] border border-[var(--color-border)] text-[var(--color-text-muted)] shrink-0">
          <svg viewBox="0 0 16 16" width="11" height="11" fill="none" stroke="currentColor" stroke-width="1.8" class="text-accent">
            <circle cx="8" cy="8" r="3" />
            <path d="M8 1v2M8 13v2M1 8h2M13 8h2" />
          </svg>
          <span>{{ group.name }}</span>
        </span>

        <!-- Brain Model Badge -->
        <span class="hidden sm:inline-flex items-center gap-1 text-2xs font-mono px-2 py-0.5 rounded bg-[var(--color-surface-muted)] text-[var(--color-text-muted)] shrink-0">
          <span>中控大脑: {{ group?.orchestratorModel || props.session.model || 'Default' }}</span>
        </span>

        <!-- Member Roles Badges -->
        <div v-if="members.length > 0" class="hidden xl:flex items-center gap-1 pl-2 border-l border-[var(--color-border)]">
          <span
            v-for="m in members"
            :key="m.id"
            class="inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-2xs font-medium bg-[var(--color-surface-muted)] border border-[var(--color-border)] text-[var(--color-text)]"
            :title="`${m.name} (${m.backend} / ${m.model})`"
          >
            <span class="w-1.5 h-1.5 rounded-full" :style="{ backgroundColor: m.color || '#3b82f6' }" />
            <span>{{ m.name }}</span>
          </span>
        </div>
      </div>

      <!-- Header Right: Run Selector, Interrupt & Delete Action -->
      <div class="flex items-center gap-2 shrink-0">
        <div v-if="flowRuns.length > 1" class="inline-flex items-center gap-1 px-2 py-1 rounded bg-[var(--color-surface-muted)] border border-[var(--color-border)] text-2xs text-[var(--color-text-muted)]">
          <span>轮次:</span>
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

        <UiButton
          v-if="isRunning"
          variant="secondary"
          size="sm"
          class="text-2xs h-7 px-2.5 text-red-500 hover:text-red-600 hover:bg-red-50 dark:hover:bg-red-950/20"
          :loading="canceling"
          :disabled="canceling"
          @click="handleCancelRun"
        >
          中断协同
        </UiButton>

        <button
          type="button"
          class="delete-session-btn text-[var(--color-text-muted)] hover:text-red-500 p-1 rounded transition-colors"
          title="删除会话"
          aria-label="删除会话"
          data-test="delete-session-btn"
          @click="emit('delete')"
        >
          <svg viewBox="0 0 16 16" width="13" height="13" fill="none" stroke="currentColor" stroke-width="1.8">
            <path d="M2.5 4h11M5.5 4V2.5a1 1 0 0 1 1-1h3a1 1 0 0 1 1 1V4M6 7v5M10 7v5" />
          </svg>
        </button>
      </div>
    </header>

    <!-- Message Stream Body -->
    <main
      ref="messagesContainer"
      class="collab-stream-body flex-1 min-h-0 overflow-y-auto p-4 md:p-6 space-y-4"
    >
      <UiAlert v-if="flowError" variant="error" class="py-2 text-xs">
        {{ flowError }}
      </UiAlert>

      <!-- Loading state -->
      <div v-if="loadingContext" class="py-16 text-center text-xs text-[var(--color-text-muted)]">
        正在加载协同会话数据…
      </div>

      <!-- Empty State: Never run -->
      <div
        v-else-if="!currentRun && flowRuns.length === 0"
        class="h-full flex flex-col items-center justify-center text-center p-8 space-y-3.5 my-auto"
      >
        <div class="w-12 h-12 rounded-full bg-accent/10 text-accent flex items-center justify-center">
          <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="1.8">
            <polygon points="5 3 19 12 5 21 5 3" />
          </svg>
        </div>
        <div class="space-y-1">
          <h3 class="text-sm font-semibold text-[var(--color-text)]">
            协同任务准备就绪
          </h3>
          <p class="text-xs text-[var(--color-text-muted)] max-w-md leading-relaxed">
            中控调度大脑已绑定团队群组「{{ group?.name }}」，可立即启动协同或发送干预指令。
          </p>
        </div>
        <UiButton
          variant="primary"
          class="text-xs px-5 py-2 font-medium"
          :loading="starting"
          :disabled="starting"
          @click="handleStartTask"
        >
          立即启动协同
        </UiButton>
      </div>

      <!-- Empty State: Started and planning -->
      <div
        v-else-if="messages.length === 0"
        class="py-16 text-center text-[var(--color-text-muted)] space-y-2"
      >
        <div class="inline-block animate-spin text-accent mb-1">
          <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2">
            <circle cx="12" cy="12" r="10" stroke-opacity="0.25" />
            <path d="M12 2a10 10 0 0 1 10 10" />
          </svg>
        </div>
        <div class="text-xs font-medium">中控调度大脑正在规划协同分工…</div>
        <p class="text-2xs text-muted-foreground">即将向团队角色成员分派子任务，请稍候。</p>
      </div>

      <!-- Messages Stream -->
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

    <!-- Bottom Intervention & Input Bar -->
    <footer class="collab-footer p-3 bg-[var(--color-surface)] border-t border-[var(--color-border)] shrink-0 space-y-2">
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
            placeholder="输入补充需求、解答中枢大脑疑问或人工干预指令… (Enter 发送, Shift+Enter 换行)"
            class="w-full text-xs md:text-sm p-2.5 rounded bg-[var(--color-surface-muted)] border border-[var(--color-border)] text-[var(--color-text)] placeholder:text-[var(--color-text-muted)] focus:outline-none focus:border-[var(--color-primary)] resize-none leading-relaxed font-sans"
            :disabled="!currentRun || sending"
            @keydown.enter.exact.prevent="handleSendMessage"
          />
        </div>
        <UiButton
          type="submit"
          variant="primary"
          class="h-10 px-5 text-xs font-medium shrink-0"
          data-test="btn-send-message"
          :disabled="!userInput.trim() || !currentRun || sending"
          :loading="sending"
        >
          发送
        </UiButton>
      </form>
    </footer>

    <!-- Child Agent Session Inspector Dialog -->
    <TaskChatDialog
      v-if="showSessionDialog && viewingSessionId"
      :title="`协同子会话: ${viewingSessionId}`"
      :project-id="props.projectId"
      :session-id="viewingSessionId"
      @close="showSessionDialog = false"
    />
  </div>
</template>

<style scoped>
.collab-stream-body {
  scroll-behavior: smooth;
}
</style>
