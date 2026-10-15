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
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useAgentsStore } from '../stores/agents'
import { useSessionsStore, type ChatEntry } from '../stores/sessions'
import { useInteractionsStore } from '../stores/interactions'
import { useOpenTabsStore } from '../stores/openTabs'
import { getInstanceGitInfo, type GitInfo } from '../api/git'
import { cachedModels } from '../modelCache'
import { computeContextUsage } from '../contextUsage'
import { ellipsisMiddle, formatTokens } from '../format'
import BackendTag from './BackendTag.vue'
import MessageView from './MessageView.vue'
import CapabilityControls from './CapabilityControls.vue'
import ModelSelect from './ModelSelect.vue'
import QuestionQueue from './QuestionQueue.vue'

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
// The composer's model for this session: the store resolves it from the user's
// explicit choice, the model the session actually used, or the picker's own
// first-model fallback. An update:modelValue from the picker is that automatic
// fallback, so it is recorded as such; a deliberate pick is recorded separately
// via the picker's `picked` event so it is never mistaken for a fallback.
const selectedModel = computed({
  get: () => sessions.modelFor(props.sessionId),
  set: (value: string) => sessions.setAutoModel(props.sessionId, value),
})

function onModelPicked(id: string) {
  sessions.setModel(props.sessionId, id)
}
// Scrolling follows the newest entry only while the user is at the bottom.
// The log scrolls by hand in jsdom tests, so every read and write of the
// scroller geometry goes through the element itself.
const logScroll = ref<HTMLElement | null>(null)
const pinnedToBottom = ref(true)
const showJumpToLatest = ref(false)
// The right activity pane scrolls independently of the left message column,
// so tool output can never yank the message view. It follows the same
// follow-while-at-bottom rule with its own scroller state.
const activityScroll = ref<HTMLElement | null>(null)
const activityPinned = ref(true)
const showJumpActivity = ref(false)
// Whether the right activity pane is collapsed. Persisted in localStorage so
// a refresh keeps the user's choice; defaults to expanded.
const ACTIVITY_STORAGE_KEY = 'dev-platform-activity-pane-v1'
const activityCollapsed = ref(false)
try {
  activityCollapsed.value = localStorage.getItem(ACTIVITY_STORAGE_KEY) === 'collapsed'
} catch {
  activityCollapsed.value = false
}
function setActivityCollapsed(collapsed: boolean) {
  activityCollapsed.value = collapsed
  try {
    localStorage.setItem(ACTIVITY_STORAGE_KEY, collapsed ? 'collapsed' : 'expanded')
  } catch {
    // Persistence is best-effort only.
  }
  // Expanding returns to the newest activity when the pane was following it.
  if (!collapsed) {
    activityPinned.value = true
    showJumpActivity.value = false
    void nextTick(() => scrollActivityToBottom(false))
  }
}
// The activity pane width, adjustable by dragging the middle divider.
// Persisted in localStorage alongside the collapsed state.
const ACTIVITY_WIDTH_KEY = 'dev-platform-activity-width-v1'
const DEFAULT_ACTIVITY_WIDTH = 340
const MIN_ACTIVITY_WIDTH = 240
const MAX_ACTIVITY_WIDTH = 640
function clampActivityWidth(px: number): number {
  if (!Number.isFinite(px)) return DEFAULT_ACTIVITY_WIDTH
  return Math.min(MAX_ACTIVITY_WIDTH, Math.max(MIN_ACTIVITY_WIDTH, Math.round(px)))
}
const activityWidth = ref(DEFAULT_ACTIVITY_WIDTH)
try {
  const stored = localStorage.getItem(ACTIVITY_WIDTH_KEY)
  if (stored !== null) activityWidth.value = clampActivityWidth(Number(stored))
} catch {
  activityWidth.value = DEFAULT_ACTIVITY_WIDTH
}
function persistActivityWidth() {
  try {
    localStorage.setItem(ACTIVITY_WIDTH_KEY, String(activityWidth.value))
  } catch {
    // Persistence is best-effort only.
  }
}
function resetActivityWidth() {
  activityWidth.value = DEFAULT_ACTIVITY_WIDTH
  persistActivityWidth()
}
// Dragging the divider resizes the activity pane. The drag starts only from
// the divider surface itself (never from its toggle button), tracks the
// pointer on the window so fast moves don't lose it, and clamps to leave at
// least a usable message column.
const splitRef = ref<HTMLElement | null>(null)
const draggingDivider = ref(false)
const activityPaneStyle = computed(() =>
  activityCollapsed.value ? {} : { width: `${activityWidth.value}px`, flex: '0 0 auto' },
)
function onDividerMouseDown(event: MouseEvent) {
  if (event.button !== 0) return
  if ((event.target as HTMLElement).closest('button')) return
  const container = splitRef.value
  if (!container) return
  event.preventDefault()
  draggingDivider.value = true
  const startX = event.clientX
  const startWidth = activityWidth.value
  const containerWidth = container.clientWidth
  const maxWidth = Math.max(
    MIN_ACTIVITY_WIDTH,
    Math.min(MAX_ACTIVITY_WIDTH, containerWidth > 0 ? containerWidth - 320 : MAX_ACTIVITY_WIDTH),
  )
  const onMove = (move: MouseEvent) => {
    const delta = startX - move.clientX
    activityWidth.value = Math.min(maxWidth, Math.max(MIN_ACTIVITY_WIDTH, startWidth + delta))
  }
  const onUp = () => {
    draggingDivider.value = false
    persistActivityWidth()
    window.removeEventListener('mousemove', onMove)
    window.removeEventListener('mouseup', onUp)
  }
  window.addEventListener('mousemove', onMove)
  window.addEventListener('mouseup', onUp)
}
function onDividerKeydown(event: KeyboardEvent) {
  if (event.key === 'ArrowLeft') {
    event.preventDefault()
    activityWidth.value = clampActivityWidth(activityWidth.value + 16)
    persistActivityWidth()
  } else if (event.key === 'ArrowRight') {
    event.preventDefault()
    activityWidth.value = clampActivityWidth(activityWidth.value - 16)
    persistActivityWidth()
  } else if (event.key === 'Enter' || event.key === ' ') {
    event.preventDefault()
    setActivityCollapsed(!activityCollapsed.value)
  }
}
// "At the bottom" tolerates a small rounding gap so subpixel layouts, and a
// wheel that stops a hair short, still count as following the conversation.
const FOLLOW_THRESHOLD_PX = 24
// Whether the project's model list for the session's backend loaded. A failed
// list disables sending rather than falling back to a free-text model.
const modelOk = ref(true)
// The picker, so a send the backend refused can ask it to re-read the catalog.
const modelPicker = ref<{ reloadForces: () => Promise<void> } | null>(null)
const toolNotice = ref('')

const activeSession = computed(() => sessions.sessionById(props.sessionId))
const instance = computed(() => {
  const session = activeSession.value
  if (!session) return null
  return agents.instances.find((entry) => entry.id === session.instanceId) ?? null
})

// The workdir row shows the session's real working directory on its own line:
// middle-ellipsized with the full path on hover and a copy button, plus the
// git branch (with a dirty marker) when the directory is a git work tree.
const shortDirectory = computed(() => ellipsisMiddle(instance.value?.directory ?? ''))
const gitInfo = ref<GitInfo | null>(null)
const gitCache = new Map<string, GitInfo>()
const copiedPath = ref(false)
async function loadGit(force = false) {
  const session = activeSession.value
  if (!session) {
    gitInfo.value = null
    return
  }
  const key = session.instanceId
  if (!force && gitCache.has(key)) {
    gitInfo.value = gitCache.get(key) ?? null
    return
  }
  try {
    const info = await getInstanceGitInfo(props.projectId, session.instanceId)
    gitCache.set(key, info)
    if (activeSession.value?.instanceId === key) gitInfo.value = info
  } catch {
    // Branch display is best-effort: a failed probe leaves the row without
    // the branch chip rather than breaking the session.
    if (activeSession.value?.instanceId === key) gitInfo.value = null
  }
}
const branchLabel = computed(() => {
  const info = gitInfo.value
  if (!info?.isGit || !info.branch) return ''
  return `⎇ ${info.branch}${info.dirty ? '*' : ''}`
})
const branchTitle = computed(() => {
  const info = gitInfo.value
  if (!info?.isGit) return ''
  const state = info.dirty ? '有未提交改动' : '干净'
  return `分支 ${info.branch ?? ''} · ${state}${info.root ? ` · ${info.root}` : ''}`
})
async function copyDirectory() {
  const directory = instance.value?.directory
  if (!directory) return
  try {
    await navigator.clipboard.writeText(directory)
  } catch {
    // Non-secure contexts have no async clipboard: fall back to execCommand.
    const area = document.createElement('textarea')
    area.value = directory
    document.body.appendChild(area)
    area.select()
    try {
      document.execCommand('copy')
    } catch {
      // Copy is convenience only; failure stays silent.
    }
    area.remove()
  }
  copiedPath.value = true
  setTimeout(() => {
    copiedPath.value = false
  }, 1500)
}

// The composer context line anchors on exact backend usage and estimates the
// turn in flight. The limit is read from the model picker's cached catalog
// for the composer's current model — the picker owns fetching, so this panel
// never issues its own catalog request; without a known limit no percentage
// is derived.
const modelLimit = ref<number | undefined>(undefined)
function refreshModelLimit() {
  const session = activeSession.value
  const model = selectedModel.value
  if (!session || !model) {
    modelLimit.value = undefined
    return
  }
  const cached = cachedModels(props.projectId, session.backend)
  const base = model.split('#')[0]
  const found = cached?.find((entry) => entry.id === model || entry.id === base)
  const limit = found?.contextLimit
  modelLimit.value = typeof limit === 'number' && limit > 0 ? limit : undefined
}
function onModelStatus(ok: boolean) {
  modelOk.value = ok
  // The picker just (re)loaded its catalog: the cached limit may have
  // appeared or changed.
  refreshModelLimit()
}
const contextUsage = computed(() =>
  computeContextUsage(activeMessages.value, sessions.lifecycleUsage[props.sessionId], modelLimit.value),
)
const showContextLine = computed(() => contextUsage.value.used > 0 || contextUsage.value.limit !== undefined)
const contextMain = computed(() => {
  const usage = contextUsage.value
  const used = `${usage.estimated ? '~' : ''}${formatTokens(usage.used)}`
  if (usage.limit === undefined || usage.percent === undefined) return `上下文 ${used}`
  return `上下文 ${used}/${formatTokens(usage.limit)}（${usage.percent}%）`
})
const contextDetail = computed(() => {
  const parts = contextUsage.value.parts
  const segments = [`↑${formatTokens(parts.input)}`, `↓${formatTokens(parts.output)}`]
  if (parts.cacheRead > 0 || parts.cacheWrite > 0) {
    segments.push(`缓存读${formatTokens(parts.cacheRead)}/写${formatTokens(parts.cacheWrite)}`)
  }
  if (parts.reasoning > 0) segments.push(`推理${formatTokens(parts.reasoning)}`)
  return segments.join(' ')
})
const contextBarClass = computed(() => {
  const percent = contextUsage.value.percent ?? 0
  if (percent >= 95) return 'context-bar-fill danger'
  if (percent >= 80) return 'context-bar-fill warn'
  return 'context-bar-fill'
})
const contextBarWidth = computed(() => `${Math.min(contextUsage.value.percent ?? 0, 100)}%`)

const activeMessages = computed<ChatEntry[]>(() => sessions.messagesFor(props.sessionId))
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

// The tool call awaiting confirmation. A backend that declares tool
// confirmation gets it exclusively from its approval stream (REST list + raw
// permission events): a tool_call entry without a result is NOT a permission —
// replying with a tool call id can never succeed (the server answers 404/400,
// surfaced as 502), so the dangling-tool fallback must not render a control for
// such a backend. Other backends keep the fallback for display only; their
// confirm path still reports that no confirmation API exists.
const pendingToolCall = computed<{ callId: string; name: string } | null>(() => {
  const session = activeSession.value
  if (session?.capabilities.toolConfirmation) {
    const approval = interactions.pendingApprovalFor(props.sessionId)
    return approval ? { callId: approval.requestId, name: approval.action } : null
  }
  const resolved = new Set<string>()
  let pending: { callId: string; name: string } | null = null
  for (const entry of activeMessages.value) {
    if (entry.kind === 'tool_result' && entry.callId) resolved.add(entry.callId)
    if (entry.kind === 'tool_call' && entry.callId) {
      pending = resolved.has(entry.callId) ? null : { callId: entry.callId, name: entry.name ?? entry.text }
    }
  }
  return pending
})

// Pending question forms. They travel on the same reply extension as tool
// confirmation, so they are shown exactly when that capability is declared.
// Rendered as answerable cards below the confirmation control: without them an
// agent question blocks the turn forever with no way to answer.
const pendingForms = computed(() => {
  const session = activeSession.value
  if (!session?.capabilities.toolConfirmation) return []
  return interactions.formsFor(props.sessionId)
})

onMounted(open)
watch(() => [props.projectId, props.sessionId], open)
onBeforeUnmount(() => {
  // Tabs keep other sessions alive: unmounting this panel must not close other
  // sessions' streams. The session's own subscriptions stay open so returning
  // via a tab is instant; they are released when the tab closes or the session
  // is deleted.
})

// Approval polling. opencode pushes permission cards on its raw event stream;
// a backend that declares tool confirmation without raw events (pi over RPC)
// has no push channel, so the workbench polls while a turn runs and stops when
// it settles. Replying already refreshes the list through the store.
let approvalPoll: ReturnType<typeof setInterval> | null = null
let approvalPollBusy = false
function stopApprovalPoll() {
  if (approvalPoll !== null) {
    clearInterval(approvalPoll)
    approvalPoll = null
  }
}
function startApprovalPoll() {
  stopApprovalPoll()
  const session = activeSession.value
  if (!session?.capabilities.toolConfirmation || session.capabilities.rawEvents) return
  approvalPoll = setInterval(() => {
    if (approvalPollBusy) return
    approvalPollBusy = true
    void interactions.loadApprovals(props.projectId, props.sessionId).finally(() => {
      approvalPollBusy = false
    })
  }, 1500)
}
watch(isRunning, (running) => {
  if (running) startApprovalPoll()
  else stopApprovalPoll()
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
onBeforeUnmount(stopApprovalPoll)

// onLogScroll tracks where the user is: reaching the bottom resumes following
// the conversation and retires the jump button; scrolling away stops following
// so a new entry can never yank the view.
function onLogScroll() {
  const el = logScroll.value
  if (!el) return
  const distance = el.scrollHeight - el.scrollTop - el.clientHeight
  const atBottom = distance <= FOLLOW_THRESHOLD_PX
  pinnedToBottom.value = atBottom
  if (atBottom) showJumpToLatest.value = false
}

function scrollToBottom(smooth: boolean) {
  const el = logScroll.value
  if (!el) return
  // The click animates; following the stream lands each delta immediately, so
  // a fast token stream never queues a pile of smooth scrolls.
  if (smooth && typeof el.scrollTo === 'function') {
    el.scrollTo({ top: el.scrollHeight, behavior: 'smooth' })
  } else {
    el.scrollTop = el.scrollHeight
  }
  pinnedToBottom.value = true
  showJumpToLatest.value = false
}

// onActivityScroll mirrors onLogScroll for the right pane: it tracks only the
// activity scroller, so parking the activity history never affects the message
// column's follow state, and vice versa.
function onActivityScroll() {
  const el = activityScroll.value
  if (!el) return
  const distance = el.scrollHeight - el.scrollTop - el.clientHeight
  const atBottom = distance <= FOLLOW_THRESHOLD_PX
  activityPinned.value = atBottom
  if (atBottom) showJumpActivity.value = false
}

function scrollActivityToBottom(smooth: boolean) {
  const el = activityScroll.value
  if (!el) return
  if (smooth && typeof el.scrollTo === 'function') {
    el.scrollTo({ top: el.scrollHeight, behavior: 'smooth' })
  } else {
    el.scrollTop = el.scrollHeight
  }
  activityPinned.value = true
  showJumpActivity.value = false
}

watch(logSignature, () => {
  // Wait for the entry to be in the DOM before measuring it.
  void nextTick(() => {
    const el = logScroll.value
    if (!el) return
    if (pinnedToBottom.value) {
      el.scrollTop = el.scrollHeight
      showJumpToLatest.value = false
    } else {
      showJumpToLatest.value = true
    }
  })
})

watch(activitySignature, () => {
  void nextTick(() => {
    const el = activityScroll.value
    if (!el) return
    if (activityPinned.value) {
      el.scrollTop = el.scrollHeight
      showJumpActivity.value = false
    } else {
      showJumpActivity.value = true
    }
  })
})

// A directly opened session URL arrives before the project's sessions are loaded,
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
    await sessions.openSession(props.projectId, props.sessionId)
    const session = sessions.sessionById(props.sessionId)
    if (session?.capabilities.toolConfirmation) {
      await interactions.loadApprovals(props.projectId, props.sessionId)
      if (interactions.error) toolNotice.value = interactions.error
    }
    // Approvals arrive on the raw event stream, so it is opened exactly when the
    // backend declares it.
    if (session?.capabilities.rawEvents) {
      interactions.subscribeRaw(props.projectId, props.sessionId, session.nativeSessionId)
    }
    // A turn may already be running when the session is opened; start polling so
    // its permission cards appear even without a push channel.
    startApprovalPoll()
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

// Tool decisions for a backend that declares tool confirmation go through its
// reply API (stores/interactions.ts); other backends still expose no
// confirmation method, so the limitation is surfaced instead.
async function onToolDecision(callId: string, decision: 'once' | 'reject') {
  const session = activeSession.value
  if (session?.capabilities.toolConfirmation) {
    toolNotice.value = ''
    const ok = await interactions.replyPermission(props.projectId, props.sessionId, callId, decision)
    if (!ok) toolNotice.value = interactions.error || '确认失败，请重试。'
    return
  }
  toolNotice.value = '此后端未提供工具确认接口，工作台无法批准或拒绝工具调用。'
}

function onConfirmTool(callId: string) {
  void onToolDecision(callId, 'once')
}

function onDenyTool(callId: string) {
  void onToolDecision(callId, 'reject')
}

const answeringForm = ref(false)

// Submits one question-form answer. A settled/expired form drops its card
// with the server's message instead of leaving an unanswerable question.
async function onAnswerForm(formId: string, answer: Record<string, unknown>) {
  if (answeringForm.value) return
  answeringForm.value = true
  toolNotice.value = ''
  try {
    const ok = await interactions.replyForm(props.projectId, props.sessionId, formId, answer)
    if (!ok) toolNotice.value = interactions.error || '提交失败，请重试。'
    else {
      await nextTick()
      scrollToBottom(false)
    }
  } finally {
    answeringForm.value = false
  }
}

async function send() {
  const session = activeSession.value
  if (!session || composerDisabled.value || messageText.value.trim() === '') return
  sending.value = true
  sendError.value = ''
  const model = modelSelection.value && selectedModel.value !== '' ? selectedModel.value : undefined
  const updated = await sessions.sendMessage(props.projectId, session.id, messageText.value, model)
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

async function onSubmit() {
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
    useInteractionsStore().unsubscribeRaw(props.projectId, session.id)
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
              <label class="composer-label" for="message-text">发送消息</label>
              <textarea
                id="message-text"
                v-model="messageText"
                rows="3"
                placeholder="向 Agent 发送消息…"
                @keydown.enter.exact.prevent="onSubmit"
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
