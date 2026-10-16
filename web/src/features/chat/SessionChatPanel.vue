<script setup lang="ts">
// Session conversation for the right pane of the shell. The session is an agent
// session of the project. Several sessions can stay open at once via the tab bar:
// each session keeps its own cached transcript and live streams, so switching
// tabs never loses state. The composer is the only place a turn is sent or
// stopped; the send action becomes 停止 while the session's turn is running.
// The pane is split in two: the left column shows only user/agent messages,
// confirmations and the composer, while the right pane shows only
// thinking/tool activity as a flat list with its own independent scroller
// (thinking/edit/shell/question fold and start expanded; other tools are a summary line).
// A divider between the columns carries the collapse toggle in its
// middle and drags to resize the activity pane (width persisted); collapsing
// hides the pane entirely so the messages take the full width. Below 900px the
// pane overlays the messages instead of squeezing them.
import { computed, nextTick, onBeforeUnmount, onMounted, ref, toRef, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useAgentsStore } from '../../stores/agents'
import { useSessionsStore, type ChatEntry } from '../../stores/sessions'
import { useInteractionsStore } from '../../stores/interactions'
import { useActivityPane } from './pane/useActivityPane'
import { useComposerContext } from './composer/useComposerContext'
import { useSessionInteractions } from './interactions/useSessionInteractions'
import { useBottomFollow } from '../../shared/scrollFollow'
import { useOpenTabsStore } from '../../stores/openTabs'
import type { GitInfo } from '../../api/git'
import { cachedModels } from '../../shared/modelCache'
import { computeContextUsage } from '../../shared/contextUsage'
import { ellipsisMiddle, formatTokens } from '../../shared/format'
import BackendTag from '../../shared/components/BackendTag.vue'
import MessageView from './rendering/MessageView.vue'
import CapabilityControls from './CapabilityControls.vue'
import ModelSelect from './composer/ModelSelect.vue'
import QuestionQueue from './interactions/QuestionQueue.vue'
import ComposerDropdown from './composer/dropdown/ComposerDropdown.vue'
import { useComposerDropdown } from './composer/dropdown/useComposerDropdown'
import { BuiltInSlashSource } from './composer/dropdown/sources/BuiltInSlashSource'
import { ProviderSlashSource } from './composer/dropdown/sources/ProviderSlashSource'
import { WorkspaceFileSource } from './composer/dropdown/sources/WorkspaceFileSource'
import { SessionMentionSource } from './composer/dropdown/sources/SessionMentionSource'
import { resolveSessionMentions } from './composer/sessionMentionResolver'
import { getSessionHistory } from '../../api/sessions'
import { detectBuiltInCommand } from './composer/builtInCommands'

const props = defineProps<{ projectId: number; sessionId: string }>()

const agents = useAgentsStore()
const sessions = useSessionsStore()
const interactions = useInteractionsStore()
const router = useRouter()

const ready = ref(false)
const loadError = ref('')
const sendError = ref('')
const sending = ref(false)
const busy = ref(false)
// The delete confirmation is a modal inside the pane: the session leaves the
// platform index only after the user confirms.
const confirmingDelete = ref(false)
const deleting = ref(false)
const deleteError = ref('')
// preparing covers the window in which the session is being re-attached and its
// history replayed, so the pane says what it is waiting for instead of looking
// like an empty conversation.
const preparing = ref(false)
const messageText = ref('')
const textareaRef = ref<HTMLTextAreaElement | null>(null)
const slashSource = new BuiltInSlashSource()
const providerSlashSource = new ProviderSlashSource(
  () => props.projectId,
  () => props.sessionId,
)
const fileSource = new WorkspaceFileSource(() => props.projectId)
const sessionSource = new SessionMentionSource(
  () => sessions.sessions,
  () => props.sessionId,
)

const dropdown = useComposerDropdown({
  sources: [slashSource, providerSlashSource, fileSource, sessionSource],
  text: messageText,
  onApplyText(newText, newCursor) {
    messageText.value = newText
    void nextTick(() => {
      if (textareaRef.value) {
        textareaRef.value.focus()
        textareaRef.value.setSelectionRange(newCursor, newCursor)
      }
    })
  },
})

function onTextareaInput() {
  if (textareaRef.value) {
    void dropdown.onInput(textareaRef.value)
  }
}

function onCompositionEnd() {
  if (textareaRef.value) {
    dropdown.onCompositionEnd(textareaRef.value)
  }
}

function onTextareaKeydown(event: KeyboardEvent) {
  if (dropdown.onKeydown(event)) return
  if (event.key === 'Enter' && !event.shiftKey) {
    event.preventDefault()
    void onSubmit()
  }
}
// Scrolling follows the newest entry only while the user is at the bottom;
// both panes share that rule through shared/scrollFollow. The log scrolls by
// hand in jsdom tests, so every read and write of the scroller geometry goes
// through the element itself.
const logScroll = ref<HTMLElement | null>(null)
const activityScroll = ref<HTMLElement | null>(null)
// Whether the right activity pane is collapsed. Persisted in localStorage so
// a refresh keeps the user's choice; defaults to expanded.
const pane = useActivityPane(() => {
  // Expanding returns to the newest activity when the pane was following it.
  activityPinned.value = true
  showJumpActivity.value = false
  void nextTick(() => scrollActivityToBottom(false))
})
const {
  activityCollapsed: activityCollapsedState,
  setActivityCollapsed,
  activityWidth,
  splitRef,
  draggingDivider,
  activityPaneStyle,
  onDividerMouseDown,
  onDividerKeydown,
  resetActivityWidth,
} = pane
const activityCollapsed = activityCollapsedState

// "At the bottom" tolerates a small rounding gap so subpixel layouts, and a
// wheel that stops a hair short, still count as following the conversation.
const FOLLOW_THRESHOLD_PX = 24
// The picker, so a send the backend refused can ask it to re-read the catalog.
const modelPicker = ref<{ reloadForces: () => Promise<void> } | null>(null)

const activeSession = computed(() => sessions.sessionById(props.sessionId))
const activeMessages = computed<ChatEntry[]>(() => sessions.messagesFor(props.sessionId))

// The composer context (model choice, workdir row, context meter) lives in the
// composer layer; the panel passes the session's resolved signals in.
const {
  selectedModel,
  onModelPicked,
  modelOk,
  instance,
  shortDirectory,
  gitInfo,
  copiedPath,
  loadGit,
  branchLabel,
  branchTitle,
  copyDirectory,
  modelLimit,
  refreshModelLimit,
  onModelStatus,
  contextUsage,
  showContextLine,
  contextMain,
  contextDetail,
  contextBarClass,
  contextBarWidth,
} = useComposerContext({
  projectId: toRef(props, 'projectId'),
  sessionId: toRef(props, 'sessionId'),
  activeSession,
  activeMessages,
})
// A cheap change signal for the log: a new entry changes the length, and a
// streaming delta changes the text of an entry still open. Settled entries are
// immutable, so watching only the open ones catches every visible change
// without deep-watching the whole transcript.
const logSignature = computed(() => {
  const list = activeMessages.value
  let open = 0
  for (const entry of list) {
    if (entry.streaming) open += entry.text.length
  }
  return `${props.sessionId}:${list.length}:${open}`
})
const streamStatus = computed(() => sessions.statusFor(props.sessionId))
const historyError = computed(() => sessions.historyError[props.sessionId] ?? '')
const historyTruncated = computed(() => sessions.historyTruncated[props.sessionId] === true)
// The activity pane's change signal mirrors the message log but counts only
// thinking/tool entries, so a new user/agent message never scrolls the right
// pane, and new activity never touches the left column's jump button.
const activitySignature = computed(() => {
  let count = 0
  let open = 0
  for (const entry of activeMessages.value) {
    if (entry.kind === 'thinking' || entry.kind === 'tool_call' || entry.kind === 'tool_result') {
      count += 1
      if (entry.streaming) open += entry.text.length
    }
  }
  return `${props.sessionId}:${count}:${open}`
})
const activityCount = computed(() => {
  let count = 0
  for (const entry of activeMessages.value) {
    if (entry.kind === 'thinking' || entry.kind === 'tool_call') count += 1
  }
  return count
})

// The message column and the activity pane each follow their own scroller:
// reaching the bottom resumes following and hides the jump button; scrolling
// away parks the view. See shared/scrollFollow.
const logFollow = useBottomFollow(logScroll, logSignature)
const activityFollow = useBottomFollow(activityScroll, activitySignature)
const onLogScroll = logFollow.onScroll
const scrollToBottom = logFollow.scrollToBottom
const onActivityScroll = activityFollow.onScroll
const scrollActivityToBottom = activityFollow.scrollToBottom
const showJumpToLatest = logFollow.showJump
const pinnedToBottom = logFollow.pinned
const showJumpActivity = activityFollow.showJump
const activityPinned = activityFollow.pinned

const modelSelection = computed(() => activeSession.value?.capabilities.modelSelection === true)
const isRunning = computed(() => activeSession.value?.turn === 'running')
const canInterrupt = computed(() => activeSession.value?.capabilities.interrupt === true)
// The composer is gated on the session itself. Opening a session repairs a
// stopped instance, so an instance's status is no longer a precondition the
// user has to fix from another pane.
const interactEnabled = computed(() => activeSession.value?.status === 'active')
// A session that is still not usable after opening says so, with the reason
// the backend gave, instead of leaving the user with a dead composer.
const unavailable = computed(
  () => activeSession.value !== null && activeSession.value.status === 'disconnected',
)
// For a session whose backend runs a machine-level service, "disconnected" has one
// common cause the user can act on: that service is not running. The platform
// will not start it on its own, so the panel names the page that can, and
// offers the reconnect the operator's start makes possible — never an automatic
// retry.
const serviceDown = computed(
  () =>
    activeSession.value !== null &&
    activeSession.value.status === 'disconnected' &&
    agents.backendFor(activeSession.value.backend)?.serviceSupported === true,
)
const unavailableMessage = computed(() => {
  if (serviceDown.value && activeSession.value) {
    const service = `${agents.backendLabel(activeSession.value.backend).toLowerCase()} service`
    return `无法连接 ${service}，请到平台设置 → Agent 检测并启动。`
  }
  return sessions.error || '会话当前不可用，请重试。'
})

const composerLabel = computed(() => (isRunning.value && canInterrupt.value ? '停止' : '发送'))
const composerDisabled = computed(() => {
  if (!activeSession.value || !interactEnabled.value) return true
  if (isRunning.value) return !canInterrupt.value || busy.value
  return sending.value || messageText.value.trim() === '' || !modelOk.value
})

// The tool call awaiting confirmation and the pending question forms are the
// interaction layer's concern (see ./interactions/useSessionInteractions.ts);
// the panel renders whatever that layer reports.

onMounted(open)
watch(() => [props.projectId, props.sessionId], open)
watch(
  () => props.sessionId,
  (newSessionId) => {
    providerSlashSource.clearCache(newSessionId)
  },
)
const {
  toolNotice,
  pendingToolCall,
  pendingForms,
  confirmTool: onConfirmTool,
  denyTool: onDenyTool,
  answerForm: onAnswerForm,
} = useSessionInteractions({
  projectId: toRef(props, 'projectId'),
  sessionId: toRef(props, 'sessionId'),
  activeSession,
  activeMessages,
  afterFormAnswer: () => {
    void nextTick(() => scrollToBottom(false))
  },
})
onBeforeUnmount(() => {
  // Tabs keep other sessions alive: unmounting this panel must not close other
  // sessions' streams. The session's own subscriptions stay open so returning
  // via a tab is instant; they are released when the tab closes or the session
  // is deleted.
})
// A finished turn may have switched branches (the agent commits) or consumed
// context, so the workdir row refreshes exactly then instead of polling.
watch(isRunning, (running, was) => {
  if (was === true && running === false) void loadGit(true)
})
// The limit follows the composer's current model: switching models re-reads
// the picker's cached catalog entry the new model reports.
watch([() => activeSession.value?.backend, selectedModel], () => {
  refreshModelLimit()
})
// The follow-scroll watches live inside shared/scrollFollow. A directly opened session URL arrives before the project's sessions are loaded,
// so the list and the instances are fetched first. A failed load is reported as
// such; an id absent from a successful list is never opened. The model choice
// survives session switches: it is kept per session in the store, not reset here.
//
// Opening goes through the store's openSession, which re-attaches the session
// (starting a backend instance when none is running), replays the agent-native
// history, and only then subscribes — in that order.
async function open() {
  ready.value = false
  loadError.value = ''
  modelOk.value = true
  pinnedToBottom.value = true
  showJumpToLatest.value = false
  activityPinned.value = true
  showJumpActivity.value = false
  // The instance list is loaded only to label the conversation's working
  // directory — the user never sees or operates an instance itself.
  await Promise.all([sessions.loadSessions(props.projectId), agents.loadInstances(props.projectId)])
  ready.value = true
  if (sessions.error) {
    loadError.value = sessions.error
    return
  }
  if (!sessions.sessionById(props.sessionId)) return
  preparing.value = true
  toolNotice.value = ''
  // A first message the create request could not admit keeps its text here:
  // the composer is pre-filled and the failure is reported, so sending again
  // needs no retyping.
  const failedFirstMessage = sessions.consumeFirstMessageFailure(props.sessionId)
  if (failedFirstMessage) {
    messageText.value = failedFirstMessage.text
    sendError.value = '首条消息未能随会话一起发送，已保留文本，请重新发送。'
  }
  try {
    // Opening subscribes the session's stream, whose first frame carries the
    // pending confirmations and forms; no separate approval load is needed.
    await sessions.openSession(props.projectId, props.sessionId)
    // The workdir row is per-session metadata: it loads with the session and
    // never blocks the conversation. The context limit refreshes from the
    // picker's cache once its catalog lands (via onModelStatus).
    void loadGit()
    refreshModelLimit()
    // Opening a session lands on its newest entry, not on the top of a replayed
    // history.
    await nextTick()
    scrollToBottom(false)
    scrollActivityToBottom(false)
  } finally {
    preparing.value = false
  }
}

// retryOpen re-runs the whole open path, which is what the unavailable note and
// the history retry need: both failures come from the same repair attempt.
function retryOpen() {
  void open()
}

async function send() {
  const session = activeSession.value
  if (!session || composerDisabled.value || messageText.value.trim() === '') return
  sending.value = true
  sendError.value = ''
  const model = modelSelection.value && selectedModel.value !== '' ? selectedModel.value : undefined
  const textToSend = await resolveSessionMentions(props.projectId, messageText.value, (targetId) =>
    getSessionHistory(props.projectId, targetId),
  )
  const updated = await sessions.sendMessage(props.projectId, session.id, textToSend, model)
  // The text is kept on failure so the user can retry without retyping it.
  if (updated) {
    messageText.value = ''
    // Sending means "look here": the view returns to the turn just made even
    // when it had been parked mid-history.
    await nextTick()
    scrollToBottom(false)
  } else {
    sendError.value = sessions.error || '发送失败，请重试。'
    // The backend may have dropped the model the list offered it. Re-reading
    // the catalog costs the user nothing (the platform answers from its own
    // cache while the refresh runs) and the retry then chooses from what the
    // backend really runs.
    if (model) void modelPicker.value?.reloadForces()
  }
  sending.value = false
}

async function stop() {
  const session = activeSession.value
  if (!session || !canInterrupt.value) return
  busy.value = true
  await sessions.interrupt(props.projectId, session.id)
  busy.value = false
}

async function handleBuiltInCommand(input: string): Promise<boolean> {
  const builtIn = detectBuiltInCommand(input)
  if (!builtIn) return false

  switch (builtIn.command.action) {
    case 'stop':
      await stop()
      messageText.value = ''
      return true
    case 'new':
      messageText.value = ''
      await router.push({ name: 'project-new-session', params: { id: props.projectId } })
      return true
    case 'model':
      if (builtIn.args) {
        selectedModel.value = builtIn.args
        toolNotice.value = `已切换模型为: ${builtIn.args}`
      } else {
        toolNotice.value = `当前模型: ${selectedModel.value || '默认'}`
      }
      messageText.value = ''
      return true
    case 'help':
      toolNotice.value = '可用内置命令: /new (新建会话), /stop (停止任务), /model <名称> (切换模型), /help (帮助)'
      messageText.value = ''
      return true
    default:
      return false
  }
}

async function onSubmit() {
  if (await handleBuiltInCommand(messageText.value)) {
    return
  }
  if (isRunning.value) {
    await stop()
    return
  }
  await send()
}

function reconnect() {
  if (activeSession.value) sessions.resubscribe(props.projectId, activeSession.value.id)
}

// removeSession deletes the platform index entry and returns to the project. The
// agent-native transcript is untouched, which the confirmation says. The
// deleted session also leaves the tab bar and releases its streams.
async function removeSession() {
  const session = activeSession.value
  if (!session || deleting.value) return
  deleting.value = true
  deleteError.value = ''
  const removed = await sessions.deleteSession(props.projectId, session.id)
  deleting.value = false
  if (!removed) {
    deleteError.value = sessions.error || '删除失败，请重试。'
    return
  }
  confirmingDelete.value = false
  try {
    useOpenTabsStore().closeTab(session.id)
  } catch {
    // Tab cleanup is best-effort; navigation still happens.
  }
  void router.push({ name: 'home' })
}
</script>

<template>
  <section
    class="session-chat"
    aria-label="会话对话"
    data-test="conversation"
    :data-stream-session="activeSession ? props.sessionId : ''"
    :data-stream-status="streamStatus"
  >
    <template v-if="activeSession">
      <header class="chat-header">
        <div class="title-row">
          <BackendTag :backend="activeSession.backend" />
          <h2 class="session-heading">{{ activeSession.title || activeSession.id }}</h2>
          <button
            type="button"
            class="delete-session"
            data-test="delete-session"
            title="删除会话"
            @click="confirmingDelete = true"
          >
            删除
          </button>
        </div>
        <div v-if="instance" class="workdir-row" data-test="workdir-row">
          <span class="workdir-path" data-test="workdir-path" :title="instance.directory">{{
            shortDirectory
          }}</span>
          <button
            type="button"
            class="workdir-copy"
            data-test="workdir-copy"
            :title="copiedPath ? '已复制' : '复制完整路径'"
            @click="copyDirectory"
          >
            {{ copiedPath ? '已复制' : '复制' }}
          </button>
          <span
            v-if="branchLabel"
            class="chip branch"
            data-test="git-branch"
            :title="branchTitle"
            >{{ branchLabel }}</span
          >
          <span v-if="isRunning" class="turn-status" role="status" aria-live="polite">运行中</span>
        </div>
        <div v-else-if="isRunning" class="workdir-row">
          <span class="turn-status" role="status" aria-live="polite">运行中</span>
        </div>
      </header>

      <p v-if="preparing" class="history-note" data-test="preparing" role="status" aria-live="polite">
        正在准备 agent…
      </p>

      <p v-if="historyTruncated" class="history-note" data-test="history-truncated">
        仅显示最近的历史记录。
      </p>

      <p v-if="historyError" class="history-error" role="alert" data-test="history-error">
        {{ historyError }}
        <button type="button" class="retry" data-test="history-retry" @click="retryOpen">重试</button>
      </p>

      <p v-if="unavailable" class="instance-note" role="alert" data-test="session-unavailable">
        {{ unavailableMessage }}
        <button
          v-if="serviceDown"
          type="button"
          class="retry"
          data-test="session-reconnect"
          @click="retryOpen"
        >
          重新连接
        </button>
        <button v-else type="button" class="retry" data-test="session-retry" @click="retryOpen">重试</button>
      </p>

      <div
        ref="splitRef"
        class="conversation-split"
        :class="{ 'activity-collapsed': activityCollapsed, dragging: draggingDivider }"
      >
        <div class="message-column">
          <div class="conversation-area">
            <div
              class="conversation-scroll"
              data-test="conversation-scroll"
              ref="logScroll"
              @scroll.passive="onLogScroll"
            >
              <MessageView
                :messages="activeMessages"
                :capabilities="activeSession.capabilities"
                :stream-status="streamStatus"
                display="messages"
              />

              <button
                v-if="streamStatus === 'error' || streamStatus === 'reconnecting'"
                type="button"
                class="reconnect"
                data-test="stream-reconnect"
                @click="reconnect"
              >
                重新连接事件流
              </button>
            </div>

            <button
              v-if="showJumpToLatest"
              type="button"
              class="jump-latest"
              data-test="jump-latest"
              @click="scrollToBottom(true)"
            >
              ↓ 滚动到最新消息
            </button>
          </div>

          <CapabilityControls
            :capabilities="activeSession.capabilities"
            :pending-tool-call="pendingToolCall"
            @confirm-tool="onConfirmTool"
            @deny-tool="onDenyTool"
          />

          <QuestionQueue
            v-if="pendingForms.length > 0"
            :forms="pendingForms"
            :busy="answeringForm"
            @submit="(formId, answer) => onAnswerForm(formId, answer)"
          />

          <p v-if="toolNotice" class="tool-notice" role="alert">{{ toolNotice }}</p>

          <p v-if="sendError" class="send-error" role="alert">{{ sendError }}</p>

          <form class="send composer" @submit.prevent="onSubmit">
            <div class="composer-box">
              <ComposerDropdown
                v-if="dropdown.isOpen.value"
                :items="dropdown.items.value"
                :selected-index="dropdown.selectedIndex.value"
                :loading="dropdown.loading.value"
                @select="dropdown.selectItem"
              />
              <label class="composer-label" for="message-text">发送消息</label>
              <textarea
                id="message-text"
                ref="textareaRef"
                v-model="messageText"
                rows="3"
                placeholder="向 Agent 发送消息… (输入 / 使用快捷命令，@ 引用文件或会话)"
                @input="onTextareaInput"
                @keydown="onTextareaKeydown"
                @compositionstart="dropdown.onCompositionStart"
                @compositionend="onCompositionEnd"
              ></textarea>
              <div class="composer-bar">
                <div class="composer-tools">
                  <ModelSelect
                    ref="modelPicker"
                    v-model="selectedModel"
                    :project-id="props.projectId"
                    :backend="activeSession.backend"
                    :model-selection="activeSession.capabilities.modelSelection"
                    @status="onModelStatus"
                    @picked="onModelPicked"
                  />
                  <span class="composer-hint">Enter 发送 · Shift+Enter 换行</span>
                </div>
                <button
                  type="button"
                  class="composer-action"
                  :class="isRunning ? 'stop-button' : 'send-button'"
                  :disabled="composerDisabled"
                  :title="isRunning ? '停止' : '发送'"
                  @click="onSubmit"
                >
                  {{ composerLabel }}
                </button>
              </div>
              <div
                v-if="showContextLine"
                class="context-line"
                data-test="context-usage"
                :title="contextDetail"
              >
                <span class="context-main">{{ contextMain }}</span>
                <span
                  v-if="contextUsage.percent !== undefined"
                  class="context-bar"
                  role="progressbar"
                  :aria-valuenow="contextUsage.percent"
                  aria-valuemin="0"
                  aria-valuemax="100"
                >
                  <span :class="contextBarClass" :style="{ width: contextBarWidth }"></span>
                </span>
                <span class="context-detail">{{ contextDetail }}</span>
                <span v-if="contextUsage.estimated" class="context-est" data-test="context-estimated"
                  >估算</span
                >
              </div>
            </div>
          </form>
        </div>

        <div
          class="split-divider"
          data-test="split-divider"
          role="separator"
          aria-orientation="vertical"
          aria-label="活动栏分隔条，拖拽调整宽度，双击恢复默认"
          tabindex="0"
          @mousedown="onDividerMouseDown"
          @dblclick="resetActivityWidth"
          @keydown="onDividerKeydown"
        >
          <button
            type="button"
            class="divider-toggle"
            :data-test="activityCollapsed ? 'activity-expand' : 'activity-collapse'"
            :title="activityCollapsed ? '展开活动栏' : '收起活动栏'"
            :aria-label="activityCollapsed ? '展开思考与工具栏' : '收起思考与工具栏'"
            :aria-expanded="(!activityCollapsed).toString()"
            @click="setActivityCollapsed(!activityCollapsed)"
          >
            {{ activityCollapsed ? '«' : '»' }}
          </button>
        </div>

        <aside
          v-if="!activityCollapsed"
          class="activity-pane"
          aria-label="思考与工具"
          data-test="activity-pane"
          :style="activityPaneStyle"
        >
          <header class="activity-head">
            <h3 class="activity-title">思考与工具<span v-if="activityCount > 0" class="activity-count" data-test="activity-count">{{ activityCount }}</span></h3>
            <button
              type="button"
              class="activity-toggle narrow-only"
              data-test="activity-hide"
              title="收起活动栏"
              aria-label="收起思考与工具栏"
              @click="setActivityCollapsed(true)"
            >
              收起
            </button>
          </header>
          <div
            class="activity-scroll"
            data-test="activity-scroll"
            ref="activityScroll"
            @scroll.passive="onActivityScroll"
          >
            <MessageView
              :messages="activeMessages"
              :capabilities="activeSession.capabilities"
              :stream-status="streamStatus"
              display="activity"
            />
          </div>
          <button
            v-if="showJumpActivity"
            type="button"
            class="jump-latest"
            data-test="jump-activity"
            @click="scrollActivityToBottom(true)"
          >
            ↓ 最新活动
          </button>
        </aside>

        <button
          v-if="activityCollapsed"
          type="button"
          class="activity-fab narrow-only"
          data-test="activity-expand-narrow"
          @click="setActivityCollapsed(false)"
        >
          活动<span v-if="activityCount > 0" class="activity-count">{{ activityCount }}</span>
        </button>
      </div>

      <div
        v-if="confirmingDelete"
        class="confirm-backdrop"
        role="dialog"
        aria-modal="true"
        aria-label="删除会话"
        data-test="delete-session-dialog"
      >
        <div class="confirm-panel">
          <p class="confirm-title">确定删除会话「{{ activeSession.title || activeSession.id }}」吗？</p>
          <p class="confirm-hint">仅从平台列表移除；Agent 原生会话仍保留，重新扫描可能再次出现。</p>
          <p v-if="deleteError" class="confirm-error" role="alert">{{ deleteError }}</p>
          <div class="confirm-actions">
            <button type="button" class="confirm-cancel" @click="confirmingDelete = false">取消</button>
            <button
              type="button"
              class="confirm-delete"
              data-test="confirm-delete"
              :disabled="deleting"
              @click="removeSession"
            >
              删除
            </button>
          </div>
        </div>
      </div>
    </template>
    <template v-else-if="loadError">
      <p class="empty" role="alert">{{ loadError }}</p>
      <RouterLink :to="{ name: 'home' }">返回工作台</RouterLink>
    </template>
    <template v-else-if="ready">
      <p class="empty">会话不存在或已被移除。</p>
      <RouterLink :to="{ name: 'home' }">返回工作台</RouterLink>
    </template>
    <p v-else class="loading">加载中…</p>
  </section>
</template>

<style scoped>
.session-chat {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  /* The pane is one viewport tall, so the conversation scrolls inside it and
     the composer stays put at the bottom. */
  height: 100%;
  min-height: 0;
}

.conversation-split {
  /* Left messages + right activity share the pane height; each column scrolls
     inside itself so tool output can never push the composer out of view. */
  position: relative;
  flex: 1;
  min-height: 0;
  display: flex;
  gap: 0.75rem;
  align-items: stretch;
}

.message-column {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  min-height: 0;
}

.conversation-area {
  /* The anchor for the floating jump button, which sits over the bottom of
     the log and above the composer. */
  position: relative;
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

.conversation-scroll {
  flex: 1;
  min-height: 0;
  overflow: auto;
}

.activity-pane {
  display: flex;
  flex-direction: column;
  min-height: 0;
  min-width: 0;
  position: relative;
  border: 1px solid var(--color-border);
  border-radius: 10px;
  background: var(--color-surface-muted);
}

/* The middle divider: a slim drag handle with the collapse toggle riding in
   its vertical center. Collapsing hides the pane entirely, so the divider is
   the only width the right side keeps. */
.split-divider {
  flex: 0 0 14px;
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: col-resize;
  border-radius: 7px;
  outline-offset: 1px;
}

.split-divider::before {
  content: '';
  position: absolute;
  top: 4px;
  bottom: 4px;
  left: 50%;
  width: 2px;
  transform: translateX(-50%);
  border-radius: 2px;
  background: var(--color-border);
}

.split-divider:hover::before,
.conversation-split.dragging .split-divider::before {
  background: var(--color-primary);
}

.conversation-split.dragging,
.conversation-split.dragging .split-divider {
  user-select: none;
}

.divider-toggle {
  position: relative;
  z-index: 1;
  width: 1.15rem;
  height: 2.6rem;
  padding: 0;
  border: 1px solid var(--color-border-strong);
  border-radius: 999px;
  background: var(--color-surface);
  color: var(--color-text-muted);
  cursor: pointer;
  font: inherit;
  font-size: 0.7rem;
  line-height: 1;
  box-shadow: 0 1px 4px rgba(15, 23, 42, 0.12);
}

.divider-toggle:hover {
  color: var(--color-primary-hover);
  border-color: var(--color-primary);
}

.narrow-only {
  display: none;
}

.activity-fab {
  position: absolute;
  right: 0.75rem;
  bottom: 5.5rem;
  z-index: 9;
  padding: 0.4rem 0.8rem;
  border: 1px solid var(--color-border-strong);
  border-radius: 999px;
  background: var(--color-surface);
  box-shadow: 0 4px 12px rgba(15, 23, 42, 0.15);
  cursor: pointer;
  font: inherit;
  font-size: 0.85rem;
}

/* Narrow screens: the activity pane becomes an overlay drawer so it never
   squeezes the message column; the divider steps aside for a floating button
   and the pane's own header close. */
@media (max-width: 900px) {
  .split-divider {
    display: none;
  }

  .narrow-only {
    display: inline-block;
  }

  .activity-fab.narrow-only {
    display: inline-block;
  }

  .activity-pane {
    position: absolute;
    top: 0;
    right: 0;
    bottom: 0;
    width: min(22rem, 86vw) !important;
    flex: none;
    z-index: 8;
    box-shadow: 0 8px 24px rgba(15, 23, 42, 0.18);
  }
}

.activity-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.5rem;
  padding: 0.45rem 0.6rem;
  border-bottom: 1px solid var(--color-border);
}

.activity-title {
  margin: 0;
  font-size: 0.9rem;
  display: flex;
  align-items: center;
  gap: 0.4rem;
}

.activity-count {
  display: inline-block;
  min-width: 1.4rem;
  text-align: center;
  padding: 0.05rem 0.4rem;
  border-radius: 999px;
  background: var(--color-border);
  font-size: 0.75rem;
  color: var(--color-text);
}

.activity-scroll {
  flex: 1;
  min-height: 0;
  overflow: auto;
  padding: 0.5rem 0.6rem;
}

.activity-toggle {
  padding: 0.2rem 0.6rem;
  border: 1px solid var(--color-border-strong);
  border-radius: 6px;
  background: var(--color-surface);
  cursor: pointer;
  font: inherit;
  font-size: 0.8rem;
  white-space: nowrap;
}

.jump-latest {
  position: absolute;
  bottom: 0.75rem;
  left: 50%;
  transform: translateX(-50%);
  z-index: 5;
  padding: 0.3rem 0.8rem;
  border: 1px solid var(--color-border-strong);
  border-radius: 999px;
  background: var(--color-surface);
  box-shadow: 0 4px 12px rgba(15, 23, 42, 0.15);
  font: inherit;
  font-size: 0.85rem;
  cursor: pointer;
}

.chat-header {
  display: flex;
  flex-direction: column;
  gap: 0.3rem;
  /* A rule under the title separates the session heading from the
     conversation below it. */
  padding-bottom: 0.5rem;
  border-bottom: 1px solid var(--color-border);
}

.title-row {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  min-width: 0;
}

.title-row .session-heading {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* The workdir row owns the second header line so a long path never pushes
   the title: the path ellipsizes in the middle (see ellipsisMiddle) with
   the full directory on hover, beside the copy control and branch chip. */
.workdir-row {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  min-width: 0;
  font-size: 0.8rem;
  color: var(--color-text-muted);
}

.workdir-path {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-family: var(--font-mono, monospace);
}

.workdir-copy {
  flex: none;
  padding: 0.05rem 0.45rem;
  border: 1px solid var(--color-border-strong);
  border-radius: 6px;
  background: var(--color-surface);
  color: var(--color-text-muted);
  cursor: pointer;
  font: inherit;
  font-size: 0.75rem;
  white-space: nowrap;
}

.workdir-copy:hover {
  color: var(--color-primary-hover);
  border-color: var(--color-primary);
}

.delete-session {
  padding: 0.15rem 0.5rem;
  border: 1px solid var(--color-danger-border);
  border-radius: 6px;
  background: var(--color-surface);
  color: var(--color-danger);
  font: inherit;
  font-size: 0.8rem;
  cursor: pointer;
}

.confirm-backdrop {
  position: fixed;
  inset: 0;
  background: rgba(15, 23, 42, 0.35);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 20;
}

.confirm-panel {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  width: min(26rem, 90vw);
  padding: 1rem 1.1rem;
  border-radius: 10px;
  background: var(--color-surface);
  box-shadow: 0 10px 30px rgba(15, 23, 42, 0.2);
}

.confirm-title {
  margin: 0;
  font-weight: 600;
}

.confirm-hint {
  margin: 0;
  color: var(--color-text-muted);
  font-size: 0.85rem;
}

.confirm-error {
  margin: 0;
  color: var(--color-danger);
  font-size: 0.85rem;
}

.confirm-actions {
  display: flex;
  justify-content: flex-end;
  gap: 0.5rem;
}

.confirm-actions button {
  padding: 0.35rem 0.7rem;
  border: 1px solid var(--color-border-strong);
  border-radius: 6px;
  background: var(--color-surface);
  cursor: pointer;
  font: inherit;
}

.confirm-actions .confirm-delete {
  background: var(--color-danger);
  border-color: var(--color-danger);
  color: var(--color-surface);
}

.confirm-actions .confirm-delete:disabled {
  background: var(--color-danger-border);
  border-color: var(--color-danger-border);
  cursor: default;
}

.session-heading {
  margin: 0;
  font-size: 1.05rem;
}

.chip {
  flex: none;
  color: var(--color-text-muted);
  background: var(--color-surface-muted);
  border-radius: 999px;
  padding: 0.1rem 0.5rem;
  font-size: 0.8rem;
  white-space: nowrap;
}

.chip.branch {
  font-family: var(--font-mono, monospace);
}

.turn-status {
  color: var(--color-primary-hover);
  font-size: 0.85rem;
}

.history-note {
  margin: 0;
  color: var(--color-text-muted);
  font-size: 0.85rem;
}

.history-error {
  margin: 0;
  color: var(--color-warning);
  font-size: 0.85rem;
}

.retry {
  margin-left: 0.4rem;
  padding: 0.1rem 0.5rem;
  border: 1px solid var(--color-border-strong);
  border-radius: 6px;
  background: var(--color-surface);
  cursor: pointer;
  font: inherit;
}

.send {
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
}

/* Desktop-style composer: one rounded box holds the input and its toolbar,
   like the opencode desktop client. */
.composer-box {
  position: relative;
  border: 1px solid var(--color-border-strong);
  border-radius: 12px;
  background: var(--color-surface);
  padding: 0.6rem 0.7rem 0.5rem;
  display: flex;
  flex-direction: column;
  gap: 0.45rem;
  box-shadow: 0 1px 2px rgba(15, 23, 42, 0.06);
}

.composer-box:focus-within {
  border-color: var(--color-primary);
  box-shadow: 0 0 0 3px var(--color-info-soft);
}

.composer-label {
  position: absolute;
  width: 1px;
  height: 1px;
  overflow: hidden;
  clip: rect(0 0 0 0);
  white-space: nowrap;
}

.send textarea {
  padding: 0.15rem 0.1rem;
  border: none;
  outline: none;
  resize: none;
  font: inherit;
  line-height: 1.5;
  background: transparent;
  color: var(--color-text);
  min-height: 3.2rem;
  max-height: 12rem;
}

.send textarea::placeholder {
  color: var(--color-text-muted);
}

.composer-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.5rem;
  padding-top: 0.35rem;
  border-top: 1px solid var(--color-border);
}

.composer-tools {
  display: flex;
  align-items: center;
  gap: 0.6rem;
  min-width: 0;
}

.composer-hint {
  color: var(--color-text-muted);
  font-size: 0.75rem;
  white-space: nowrap;
}

.composer-action,
.reconnect {
  align-self: center;
  min-width: 3.4rem;
  padding: 0.35rem 0.9rem;
  border: 1px solid var(--color-border-strong);
  border-radius: 999px;
  background: var(--color-surface);
  cursor: pointer;
  font: inherit;
  font-weight: 600;
}

.composer-action.send-button {
  background: var(--color-primary);
  border-color: var(--color-primary);
  color: var(--color-surface);
}

.composer-action.send-button:disabled {
  background: var(--color-info-border);
  border-color: var(--color-info-border);
  cursor: default;
}

.composer-action.stop-button {
  color: var(--color-danger);
  border-color: var(--color-danger-border);
}

/* The composer context line: exact usage anchored on backend reports with
   the in-flight turn estimated (marked 估算). The bar warns at 80% and
   alarms at 95%; without a model limit only the used figure shows. */
.context-line {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding-top: 0.35rem;
  border-top: 1px solid var(--color-border);
  font-size: 0.75rem;
  color: var(--color-text-muted);
  min-width: 0;
}

.context-main {
  flex: none;
  white-space: nowrap;
  font-variant-numeric: tabular-nums;
}

.context-bar {
  flex: 1 1 4rem;
  min-width: 3rem;
  max-width: 12rem;
  height: 0.35rem;
  border-radius: 999px;
  background: var(--color-border);
  overflow: hidden;
}

.context-bar-fill {
  display: block;
  height: 100%;
  border-radius: 999px;
  background: var(--color-primary);
}

.context-bar-fill.warn {
  background: var(--color-warning);
}

.context-bar-fill.danger {
  background: var(--color-danger);
}

.context-detail {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-variant-numeric: tabular-nums;
}

.context-est {
  flex: none;
  border: 1px solid var(--color-border-strong);
  border-radius: 4px;
  padding: 0 0.3rem;
  font-size: 0.7rem;
  white-space: nowrap;
}

.tool-notice {
  color: var(--color-warning);
  font-size: 0.85rem;
  margin: 0;
}

.instance-note {
  color: var(--color-warning);
  font-size: 0.85rem;
  margin: 0;
}

.send-error {
  color: var(--color-danger);
  font-size: 0.85rem;
  margin: 0;
}

.empty,
.loading {
  color: var(--color-text-muted);
}
</style>
