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
// A slim rail on the right of the split carries both pane toggles: the file
// tree and the activity pane (with its count inside the pane title). Both
// toggles stay visible when their panes are collapsed, so the header only
// carries the session identity and the delete action. The preview divider drags
// to resize the activity pane (width persisted); collapsing hides the pane
// entirely so the messages take the full width. Below 900px the pane overlays
// the messages instead of squeezing them.
import { computed, nextTick, onActivated, onBeforeUnmount, onDeactivated, onMounted, ref, toRef, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useAgentsStore } from '../../stores/agents'
import { useProjectsStore } from '../../stores/projects'
import { useSessionsStore, type ChatEntry } from '../../stores/sessions'
import { useWorktreesStore } from '../../stores/worktrees'
import { useInteractionsStore } from '../../stores/interactions'
import { useActivityPane } from './pane/useActivityPane'
import { useComposerContext } from './composer/useComposerContext'
import { useSessionInteractions } from './interactions/useSessionInteractions'
import { useBottomFollow } from '../../shared/scrollFollow'
import { useOpenTabsStore } from '../../stores/openTabs'
import { useDraftsStore } from '../../stores/drafts'
import { useViewportStore } from '../../stores/viewport'
import { useFileTreeStateStore } from '../../stores/fileTreeState'
import type { GitInfo } from '../../api/git'
import { cachedModels } from '../../shared/modelCache'
import { computeContextUsage } from '../../shared/contextUsage'
import { ellipsisMiddle, formatTokens } from '../../shared/format'
import SessionHeader from './pane/SessionHeader.vue'
import WorkspacePreviewPane, { type OpenFileTab } from './pane/WorkspacePreviewPane.vue'
import QuickFileSearchDialog from './pane/QuickFileSearchDialog.vue'
import MessageView from './rendering/MessageView.vue'
import CapabilityControls from './CapabilityControls.vue'
import ModelSelect from './composer/ModelSelect.vue'
import ContextUsageMeter from './composer/ContextUsageMeter.vue'
import ComposerImageTray from './composer/ComposerImageTray.vue'
import ImagePreviewModal from './rendering/ImagePreviewModal.vue'
import QuestionQueue from './interactions/QuestionQueue.vue'
import ComposerDropdown from './composer/dropdown/ComposerDropdown.vue'
import { useComposerDropdown } from './composer/dropdown/useComposerDropdown'
import { ProviderSlashSource as PlatformSlashSource } from './composer/dropdown/sources/ProviderSlashSource'
import { WorkspaceFileSource } from './composer/dropdown/sources/WorkspaceFileSource'
import { SessionMentionSource } from './composer/dropdown/sources/SessionMentionSource'
import { resolveSessionMentions } from './composer/sessionMentionResolver'
import { dispatchSlashCommand } from './composer/slashActionDispatcher'
import { getSessionHistory, executeCommand, compactSession } from '../../api/sessions'
import type { ImageAttachment } from '../../api/messageTypes'
import {
  type SendShortcut,
} from './composer/sendShortcut'
import { useAttachmentUploader } from './composer/useAttachmentUploader'
import { useComposerKeyboard } from './composer/useComposerKeyboard'
import { UiButton, UiDialog } from '../../shared/ui'

const props = withDefaults(
  defineProps<{
    projectId: number
    sessionId: string
    hideSideRail?: boolean
  }>(),
  {
    hideSideRail: false,
  },
)

const agents = useAgentsStore()
const projectsStore = useProjectsStore()
const sessions = useSessionsStore()
const interactions = useInteractionsStore()
const worktreesStore = useWorktreesStore()
const draftsStore = useDraftsStore()
const viewportStore = useViewportStore()
const fileTreeState = useFileTreeStateStore()
const router = useRouter()

let currentSessionId = props.sessionId
let isRestoringDraft = false

const initialSessionDraft = draftsStore.getSessionDraft(props.sessionId)

function getFileName(filePath: string): string {
  const parts = filePath.replace(/\\/g, '/').split('/')
  return parts[parts.length - 1] || filePath
}

const FILE_TREE_TAB_KEY = '__filetree__'

const openFileTabs = ref<OpenFileTab[]>([])
const activePreviewTab = ref<string>(FILE_TREE_TAB_KEY)

const currentActivePreviewFile = computed(() =>
  openFileTabs.value.find((f) => f.path === activePreviewTab.value),
)

function selectPreviewTab(tabKey: string) {
  activePreviewTab.value = tabKey
}

function openFileInPreview(
  filePath: string,
  options?: { mode?: 'view' | 'diff'; commitRef?: string; commitMessage?: string },
) {
  let cleanPath = filePath.replace(/\\/g, '/')
  const isAbs = cleanPath.startsWith('/')
  if (!isAbs) {
    cleanPath = cleanPath.replace(/^\/+/, '')
  }
  if (!cleanPath) return
  const reqMode = options?.mode || 'view'
  const reqCommit = options?.commitRef
  const reqMsg = options?.commitMessage

  const existing = openFileTabs.value.find((f) => f.path === cleanPath)
  if (!existing) {
    openFileTabs.value.push({
      path: cleanPath,
      name: getFileName(cleanPath),
      mode: reqMode,
      commitRef: reqCommit,
      commitMessage: reqMsg,
    })
  } else {
    existing.mode = reqMode
    existing.commitRef = reqCommit
    existing.commitMessage = reqMsg
  }
  activePreviewTab.value = cleanPath
  // If preview pane is collapsed, expand it so the file is previewed immediately
  if (previewCollapsed.value) {
    setPreviewCollapsed(false)
  }
}

function closeFileTab(path: string, event?: MouseEvent) {
  event?.stopPropagation()
  const idx = openFileTabs.value.findIndex((f) => f.path === path)
  if (idx === -1) return
  openFileTabs.value.splice(idx, 1)

  if (activePreviewTab.value === path) {
    if (openFileTabs.value.length > 0) {
      const nextIdx = Math.min(idx, openFileTabs.value.length - 1)
      activePreviewTab.value = openFileTabs.value[nextIdx].path
    } else {
      activePreviewTab.value = FILE_TREE_TAB_KEY
    }
  }
}

function onSelectWorkspaceFile(
  filePath: string,
  options?: { mode?: 'view' | 'diff'; commitRef?: string; commitMessage?: string },
) {
  openFileInPreview(filePath, options)
}

function onOpenFileFromMarkdown(href: string) {
  if (!href) return
  const [urlWithoutHash] = href.split('#')
  let path = urlWithoutHash.trim()

  // Decode URI encoding if present (e.g. %20, non-ASCII characters)
  try {
    path = decodeURIComponent(path)
  } catch {
    // Keep raw path if decoding fails
  }

  // Strip file:// or file:/// protocol if present
  path = path.replace(/^file:\/\/\/?/i, '/')
  path = path.replace(/\\/g, '/')

  // Normalize candidate root directories
  const candidates: string[] = []
  if (instance.value?.directory) {
    candidates.push(instance.value.directory.replace(/\\/g, '/'))
  }
  if (activeWorktreeId.value) {
    const wt = worktreesStore.byId(props.projectId, activeWorktreeId.value)
    if (wt?.path) {
      candidates.push(wt.path.replace(/\\/g, '/'))
    }
  }
  const proj = projectsStore.byId(props.projectId)
  if (proj?.path) {
    candidates.push(proj.path.replace(/\\/g, '/'))
  }

  // Strip project directory or instance workdir prefix if absolute path
  let isUnderProject = false
  for (const dir of candidates) {
    const cleanDir = dir.replace(/\/+$/, '')
    if (cleanDir && (path === cleanDir || path.startsWith(cleanDir + '/'))) {
      path = path.slice(cleanDir.length)
      isUnderProject = true
      break
    }
    const strippedCandidate = cleanDir.replace(/^\/+/, '')
    const strippedPath = path.replace(/^\/+/, '')
    if (strippedCandidate && (strippedPath === strippedCandidate || strippedPath.startsWith(strippedCandidate + '/'))) {
      path = strippedPath.slice(strippedCandidate.length)
      isUnderProject = true
      break
    }
  }

  if (isUnderProject) {
    // Project-relative path: strip leading slashes and relative prefix
    path = path.replace(/^[/\\]+/, '').replace(/^\.\//, '')
  } else {
    // For external absolute paths, preserve leading slash; for relative paths, strip ./
    path = path.replace(/^\.\//, '')
  }
  if (!path) return

  openFileInPreview(path)
}

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
const messageText = ref(initialSessionDraft.messageText || '')
const textareaRef = ref<HTMLTextAreaElement | null>(null)
const textareaHeight = ref<number | null>(null)
let isResizing = false
let startY = 0
let startHeight = 0

function onResizerMouseDown(e: MouseEvent) {
  if (!textareaRef.value) return
  isResizing = true
  startY = e.clientY
  startHeight = textareaRef.value.getBoundingClientRect().height
  document.body.style.cursor = 'ns-resize'
  document.body.style.userSelect = 'none'

  const onMouseMove = (ev: MouseEvent) => {
    if (!isResizing) return
    const delta = startY - ev.clientY
    const newHeight = Math.max(36, Math.min(600, startHeight + delta))
    textareaHeight.value = newHeight
  }

  const onMouseUp = () => {
    isResizing = false
    document.body.style.cursor = ''
    document.body.style.userSelect = ''
    window.removeEventListener('mousemove', onMouseMove)
    window.removeEventListener('mouseup', onMouseUp)
  }

  window.addEventListener('mousemove', onMouseMove)
  window.addEventListener('mouseup', onMouseUp)
}

function resetComposerHeight() {
  textareaHeight.value = null
}
const platformSlashSource = new PlatformSlashSource(
  () => props.projectId,
  () => props.sessionId,
  () => activeSession.value?.backend,
)
const fileSource = new WorkspaceFileSource(
  () => props.projectId,
  () => sessions.sessionById(props.sessionId)?.worktreeId ?? 0,
)

const sessionSource = new SessionMentionSource(
  () => sessions.sessions,
  () => props.sessionId,
)

const dropdown = useComposerDropdown({
  sources: [platformSlashSource, fileSource, sessionSource],
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

const {
  sendShortcut,
  sendMenuOpen,
  toggleSendMenu,
  setShortcut,
  onWindowClick,
  handleKeydown: onComposerKeydown,
} = useComposerKeyboard(() => void onSubmit())

function onTextareaKeydown(event: KeyboardEvent) {
  onComposerKeydown(event, dropdown.onKeydown(event))
}

const composerPlaceholder = computed(() => {
  if (sendShortcut.value === 'ctrl-enter') {
    return '向 Agent 发送消息… (Ctrl+Enter 发送，Enter 换行，输入 / 使用命令，@ 引用)'
  }
  return '向 Agent 发送消息… (Enter 发送，Shift+Enter 换行，输入 / 使用命令，@ 引用)'
})

const sendButtonTitle = computed(() => {
  if (isRunning.value) return '停止'
  return sendShortcut.value === 'ctrl-enter' ? '发送 (Ctrl+Enter)' : '发送 (Enter)'
})

// Image attachments support
const {
  attachedImages,
  previewModalImage,
  fileInputRef,
  removeImage,
  openImagePreview,
  triggerFileInput,
  handleUploadedFiles,
  onFileInput: handleFileInput,
  onPaste,
  onDragOver,
  onDrop: handleDrop,
} = useAttachmentUploader({
  initialImages: initialSessionDraft.attachedImages,
  onNotice: (msg) => {
    toolNotice.value = msg
  },
})

watch(messageText, (val) => {
  if (isRestoringDraft) return
  if (!currentSessionId) return
  draftsStore.setSessionDraft(currentSessionId, { messageText: val })
})

watch(
  attachedImages,
  (val) => {
    if (isRestoringDraft) return
    if (!currentSessionId) return
    draftsStore.setSessionDraft(currentSessionId, { attachedImages: val })
  },
  { deep: true },
)

const currentModelSupportsImages = computed<boolean>(() => {
  const session = activeSession.value
  if (!session) return false
  const model = selectedModel.value
  if (!model) return false
  const cached = cachedModels(session.backend)
  const base = model.split('#')[0]
  const found = cached?.find((entry) => entry.id === model || entry.id === base)
  if (found && typeof found.supportsImages === 'boolean') {
    return found.supportsImages
  }
  const lower = model.toLowerCase()
  return (
    lower.includes('gpt-4o') ||
    lower.includes('gpt-4-turbo') ||
    lower.includes('claude-3') ||
    lower.includes('gemini') ||
    lower.includes('vision') ||
    lower.includes('vl') ||
    lower.includes('llava') ||
    lower.includes('pixtral')
  )
})

function onFileInput(event: Event) {
  handleFileInput(event, (snippet) => {
    const prefix = messageText.value.trim() ? '\n\n' : ''
    messageText.value += `${prefix}${snippet}`
  })
}

function onDrop(event: DragEvent) {
  const textData = event.dataTransfer?.getData('text/plain')
  if (textData && textData.startsWith('@')) {
    event.preventDefault()
    const prefix = messageText.value.length > 0 && !messageText.value.endsWith(' ') ? ' ' : ''
    messageText.value += `${prefix}${textData} `
    return
  }

  handleDrop(event, (snippet) => {
    const prefix = messageText.value.trim() ? '\n\n' : ''
    messageText.value += `${prefix}${snippet}`
  })
}
// Scrolling follows the newest entry only while the user is at the bottom;
// both panes share that rule through shared/scrollFollow. The log scrolls by
// hand in jsdom tests, so every read and write of the scroller geometry goes
// through the element itself.
const logScroll = ref<HTMLElement | null>(null)
// Whether the activity pane is collapsed (persisted in localStorage).
// Defaults to collapsed, presenting a floating FAB that opens an in-tab drawer.
const pane = useActivityPane()
const {
  previewCollapsed,
  setPreviewCollapsed,
  previewWidth,
  setPreviewWidth,
  activityCollapsed,
  setActivityCollapsed,
} = pane

const isPreviewResizing = ref(false)
let previewStartX = 0
let previewStartWidth = 0

function onPreviewResizerMouseDown(e: MouseEvent) {
  if (e.button !== 0) return
  e.preventDefault()
  isPreviewResizing.value = true
  previewStartX = e.clientX
  previewStartWidth = previewWidth.value

  document.body.style.cursor = 'col-resize'
  document.body.style.userSelect = 'none'

  const onMouseMove = (ev: MouseEvent) => {
    if (!isPreviewResizing.value) return
    const delta = previewStartX - ev.clientX
    const maxWidth = Math.max(400, Math.min(window.innerWidth * 0.7, window.innerWidth - 360))
    const newWidth = Math.round(Math.max(320, Math.min(maxWidth, previewStartWidth + delta)))
    previewWidth.value = newWidth
  }

  const onMouseUp = () => {
    if (!isPreviewResizing.value) return
    isPreviewResizing.value = false
    document.body.style.cursor = ''
    document.body.style.userSelect = ''
    window.removeEventListener('mousemove', onMouseMove)
    window.removeEventListener('mouseup', onMouseUp)
    setPreviewWidth(previewWidth.value)
  }

  window.addEventListener('mousemove', onMouseMove)
  window.addEventListener('mouseup', onMouseUp)
}

watch(
  () => [props.projectId, props.sessionId],
  () => {
    openFileTabs.value = []
    activePreviewTab.value = FILE_TREE_TAB_KEY
  },
)

// "At the bottom" tolerates a small rounding gap so subpixel layouts, and a
// wheel that stops a hair short, still count as following the conversation.
const FOLLOW_THRESHOLD_PX = 24
// The picker, so a send the backend refused can ask it to re-read the catalog.


const activeSession = computed(() => sessions.sessionById(props.sessionId))
const activeWorktreeId = computed(() => activeSession.value?.worktreeId ?? 0)

watch(
  () => props.projectId,
  (projectId) => {
    if (projectId) void worktreesStore.load(projectId)
  },
  { immediate: true },
)

watch(activeWorktreeId, () => {
  if (activeSession.value) void loadGit(true)
})

function onFileTreeBranchChanged() {
  void worktreesStore.load(props.projectId, true)
  void loadGit(true)
}
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
  worktreeId: activeWorktreeId,
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
const historyHasMore = computed(() => sessions.historyHasMore[props.sessionId] === true)
const historyOlderLoading = computed(() => sessions.historyOlderLoading[props.sessionId] === true)
const historyOlderError = computed(() => sessions.historyOlderError[props.sessionId] ?? '')
// The message column follows its scroller: reaching the bottom resumes
// following and hides the jump button; scrolling away parks the view. See shared/scrollFollow.
const logFollow = useBottomFollow(logScroll, logSignature)

async function loadOlderHistory() {
  const scroller = logScroll.value
  const previousHeight = scroller?.scrollHeight ?? 0
  const previousTop = scroller?.scrollTop ?? 0
  await sessions.loadOlderHistory(props.projectId, props.sessionId)
  await nextTick()
  if (scroller) {
    // Keep the user's reading position: the prepended page adds height above.
    scroller.scrollTop = previousTop + (scroller.scrollHeight - previousHeight)
  }
}

let isDeactivated = false

function restoreScrollPosition() {
  const apply = () => {
    if (!props.sessionId || !logScroll.value) return
    const savedLogState = viewportStore.getSessionScroll(props.sessionId)
    if (savedLogState && !savedLogState.pinned) {
      logScroll.value.scrollTop = savedLogState.scrollTop
      pinnedToBottom.value = false
      showJumpToLatest.value = true
    } else {
      scrollToBottom(false)
    }
  }

  apply()
  if (typeof window !== 'undefined' && typeof window.requestAnimationFrame === 'function') {
    window.requestAnimationFrame(() => {
      apply()
      window.requestAnimationFrame(apply)
    })
  }
}

function onLogScroll() {
  if (isDeactivated) return
  logFollow.onScroll()
  if (props.sessionId && logScroll.value && logScroll.value.clientHeight > 0 && logScroll.value.scrollHeight > 0) {
    viewportStore.saveSessionScroll(props.sessionId, logScroll.value.scrollTop, pinnedToBottom.value)
  }
}

// The activity pane's change signal mirrors the message log but counts only
const scrollToBottom = logFollow.scrollToBottom
const showJumpToLatest = logFollow.showJump
const pinnedToBottom = logFollow.pinned

function onJumpToLatest() {
  scrollToBottom(true)
  if (props.sessionId && logScroll.value) {
    viewportStore.saveSessionScroll(props.sessionId, logScroll.value.scrollHeight, true)
  }
}

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

const quickFileSearchOpen = ref(false)

function onSessionKeydown(e: KeyboardEvent) {
  if (isDeactivated) return

  // Ctrl+P / Cmd+P: Quick open file search
  if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'p') {
    e.preventDefault()
    quickFileSearchOpen.value = !quickFileSearchOpen.value
    return
  }

  // Alt+2: Toggle preview/activity pane
  if (e.altKey && (e.key === '2' || e.code === 'Digit2')) {
    e.preventDefault()
    setPreviewCollapsed(!previewCollapsed.value)
    return
  }
}

let isInitialMount = true

onMounted(() => {
  window.addEventListener('click', onWindowClick)
  window.addEventListener('keydown', onSessionKeydown)
  if (!projectsStore.byId(props.projectId)) {
    void projectsStore.load()
  }
  void open()
  void nextTick(() => {
    isInitialMount = false
  })
})

onActivated(() => {
  if (isInitialMount) return
  isDeactivated = false
  // If the session data was evicted (e.g. tab closed and reopened while kept alive)
  // or stream is not open, re-open to reload history and re-subscribe.
  if (
    !sessions.historyLoaded[props.sessionId] ||
    !sessions.messages[props.sessionId] ||
    sessions.streamStatus[props.sessionId] !== 'open'
  ) {
    void open()
  } else {
    sessions.activeSessionId = props.sessionId
    void nextTick(() => {
      restoreScrollPosition()
    })
  }
})

onDeactivated(() => {
  isDeactivated = true
  if (currentSessionId && logScroll.value && logScroll.value.clientHeight > 0 && logScroll.value.scrollHeight > 0) {
    viewportStore.saveSessionScroll(currentSessionId, logScroll.value.scrollTop, pinnedToBottom.value)
  }
  if (currentSessionId && !isRestoringDraft) {
    draftsStore.setSessionDraft(currentSessionId, {
      messageText: messageText.value,
      attachedImages: attachedImages.value,
    })
  }
})

watch(() => [props.projectId, props.sessionId], open)
watch(
  () => props.sessionId,
  (newSessionId, oldSessionId) => {
    const prevId = oldSessionId || currentSessionId
    if (prevId && prevId !== newSessionId) {
      draftsStore.setSessionDraft(prevId, {
        messageText: messageText.value,
        attachedImages: attachedImages.value,
      })
    }
    currentSessionId = newSessionId
    if (newSessionId) {
      isRestoringDraft = true
      const draft = draftsStore.getSessionDraft(newSessionId)
      messageText.value = draft.messageText || ''
      attachedImages.value = Array.isArray(draft.attachedImages) ? [...draft.attachedImages] : []
      void nextTick(() => {
        isRestoringDraft = false
      })
    }
    platformSlashSource.clearCache(newSessionId)
  },
)
const {
  toolNotice,
  pendingToolCall,
  pendingForms,
  answeringForm,
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
  window.removeEventListener('click', onWindowClick)
  window.removeEventListener('keydown', onSessionKeydown)
  // Tabs keep other sessions alive: unmounting this panel must not close other
  // sessions' streams. The session's own subscriptions stay open so returning
  // via a tab is instant; they are released when the tab closes or the session
  // is deleted.
  if (currentSessionId) {
    if (logScroll.value) {
      viewportStore.saveSessionScroll(currentSessionId, logScroll.value.scrollTop, pinnedToBottom.value)
    }
  }
  if (currentSessionId && !isRestoringDraft) {
    draftsStore.setSessionDraft(currentSessionId, {
      messageText: messageText.value,
      attachedImages: attachedImages.value,
    })
  }
})
// A finished turn may have switched branches (the agent commits) or consumed
// context, so the workdir row refreshes exactly then instead of polling.
watch(isRunning, (running, was) => {
  if (was === true && running === false) {
    void loadGit(true)
    // The agent may have changed files: the next open of the tree loads them again.
    if (currentSessionId) fileTreeState.invalidateData(currentSessionId)
  }
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
  sendError.value = ''
  sessions.error = ''
  modelOk.value = true

  const savedLogState = viewportStore.getSessionScroll(props.sessionId)

  if (savedLogState && !savedLogState.pinned) {
    pinnedToBottom.value = false
    showJumpToLatest.value = true
  } else {
    pinnedToBottom.value = true
    showJumpToLatest.value = false
  }

  // The instance list is loaded only to label the conversation's working
  // directory — the user never sees or operates an instance itself.
  // Avoid re-fetching sessions and instances if already populated for this project.
  const needsProjectSessions =
    !sessions.sessionsByProject[props.projectId] ||
    !sessions.sessionById(props.sessionId)
  const needsInstances =
    agents.projectId !== props.projectId || agents.instances.length === 0

  const preloads: Promise<unknown>[] = []
  if (needsProjectSessions) preloads.push(sessions.loadSessions(props.projectId))
  if (needsInstances) preloads.push(agents.loadInstances(props.projectId))
  if (preloads.length > 0) {
    await Promise.all(preloads)
  }
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
    if (sessions.historyLoaded[props.sessionId] && sessions.streamStatus[props.sessionId] === 'open') {
      sessions.activeSessionId = props.sessionId
    } else {
      await sessions.openSession(props.projectId, props.sessionId)
    }
    // The workdir row is per-session metadata: it loads with the session and
    // never blocks the conversation. The context limit refreshes from the
    // picker's cache once its catalog lands (via onModelStatus).
    void loadGit()
    refreshModelLimit()
    // Opening a session lands on its newest entry, or restores previous scroll
    // position if the user previously scrolled away from bottom.
    await nextTick()
    restoreScrollPosition()
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
  if (!session || composerDisabled.value || (messageText.value.trim() === '' && attachedImages.value.length === 0)) return
  sending.value = true
  sendError.value = ''
  const model = modelSelection.value && selectedModel.value !== '' ? selectedModel.value : undefined
  const textToSend = await resolveSessionMentions(props.projectId, messageText.value, (targetId, options) =>
    getSessionHistory(props.projectId, targetId, options),
  )
  const updated = await sessions.sendMessage(props.projectId, session.id, textToSend, model, attachedImages.value)
  // The text is kept on failure so the user can retry without retyping it.
  if (updated) {
    messageText.value = ''
    attachedImages.value = []
    draftsStore.clearSessionDraft(session.id)
    // Sending means "look here": the view returns to the turn just made even
    // when it had been parked mid-history.
    await nextTick()
    scrollToBottom(false)
  } else {
    sendError.value = sessions.error || '发送失败，请重试。'
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

async function handleSlashCommand(input: string): Promise<boolean> {
  const session = activeSession.value
  if (!session) return false

  busy.value = true
  try {
    const res = await dispatchSlashCommand(input, {
      projectId: props.projectId,
      sessionId: session.id,
      findCommand: (cmdName) => platformSlashSource.findCommand(cmdName),
      compactSession,
    })
    if (!res.handled) {
      busy.value = false
      return false
    }
    if (res.notice) {
      toolNotice.value = res.notice
    }
    if (res.clearInput) {
      messageText.value = ''
    }
    busy.value = false
    return true
  } catch (err: unknown) {
    toolNotice.value = `执行失败: ${err instanceof Error ? err.message : String(err)}`
    busy.value = false
    return true
  }
}

async function onSubmit() {
  if (await handleSlashCommand(messageText.value)) {
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
  draftsStore.clearSessionDraft(session.id)
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
      <div
        class="conversation-split"
        data-test="conversation-split"
        :class="{
          'activity-collapsed': previewCollapsed,
          'preview-collapsed': previewCollapsed,
        }"
      >
        <div class="message-column">
          <SessionHeader
            :session="activeSession"
            :instance="instance"
            :short-directory="shortDirectory"
            :copied-path="copiedPath"
            :branch-label="branchLabel"
            :branch-title="branchTitle"
            :is-running="isRunning"
            @delete="confirmingDelete = true"
            @copy-directory="copyDirectory"
          />

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

          <div class="conversation-area">
            <div
              class="conversation-scroll"
              data-test="conversation-scroll"
              ref="logScroll"
              @scroll.passive="onLogScroll"
            >
              <button
                v-if="historyHasMore"
                type="button"
                class="load-older"
                data-test="load-older-history"
                :disabled="historyOlderLoading"
                @click="loadOlderHistory"
              >
                {{ historyOlderLoading ? '正在加载更早的消息…' : '加载更早的消息' }}
              </button>
              <p v-if="historyOlderError" class="history-error" data-test="history-older-error" role="alert">
                {{ historyOlderError }}
              </p>

              <MessageView
                :messages="activeMessages"
                :capabilities="activeSession.capabilities"
                :stream-status="streamStatus"
                :running="isRunning"
                display="messages"
                @open-file="onOpenFileFromMarkdown"
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
              @click="onJumpToLatest"
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
            <div
              class="composer-box"
              @dragover.prevent="onDragOver"
              @drop.prevent="onDrop"
            >
              <div
                class="composer-resizer"
                data-test="composer-resizer"
                title="拖拽调整输入框高度，双击重置"
                @mousedown="onResizerMouseDown"
                @dblclick="resetComposerHeight"
              >
                <span class="resizer-handle"></span>
              </div>
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
                rows="2"
                :style="textareaHeight ? { height: `${textareaHeight}px`, maxHeight: 'none' } : undefined"
                :placeholder="composerPlaceholder"
                @input="onTextareaInput"
                @keydown="onTextareaKeydown"
                @paste="onPaste"
                @compositionstart="dropdown.onCompositionStart"
                @compositionend="onCompositionEnd"
              ></textarea>
              <ComposerImageTray
                :images="attachedImages"
                @remove="removeImage"
                @preview="openImagePreview"
              />
              <div class="composer-bar">
                <div class="composer-tools">
                  <button
                    type="button"
                    class="composer-tool-btn composer-plus-btn"
                    title="添加图片或文件"
                    aria-label="添加图片或文件"
                    data-test="attach-image-btn"
                    @click="triggerFileInput"
                  >
                    +
                  </button>
                  <input
                    ref="fileInputRef"
                    type="file"
                    multiple
                    style="display: none;"
                    data-test="file-input"
                    @change="onFileInput"
                  />
                  <ModelSelect
                    :key="`${activeSession.backend}-${activeSession.id}`"
                    v-model="selectedModel"
                    :backend="activeSession.backend"
                    :model-selection="activeSession.capabilities?.modelSelection"
                    @status="onModelStatus"
                    @picked="onModelPicked"
                  />
                  <ContextUsageMeter
                    :usage="contextUsage"
                    :context-detail="contextDetail"
                  />
                </div>
                <div class="composer-actions">
                  <div class="send-split-group" data-test="send-split-group">
                    <button
                      type="button"
                      class="composer-action send-main-btn"
                      :class="isRunning ? 'stop-button' : 'send-button'"
                      :disabled="composerDisabled"
                      :title="sendButtonTitle"
                      data-test="composer-send-btn"
                      @click="onSubmit"
                    >
                      {{ composerLabel }}
                    </button>
                    <button
                      type="button"
                      class="send-dropdown-trigger"
                      :class="[
                        isRunning ? 'stop-trigger' : 'send-trigger',
                        { 'is-open': sendMenuOpen },
                      ]"
                      data-test="send-shortcut-toggle"
                      aria-label="选择发送快捷键"
                      title="选择发送快捷键"
                      @click.stop="toggleSendMenu"
                    >
                      <svg class="dropdown-arrow" viewBox="0 0 24 24" width="10" height="10" fill="currentColor">
                        <path d="M7 10l5 5 5-5z" />
                      </svg>
                    </button>
                    <div v-if="sendMenuOpen" class="send-dropdown-menu" data-test="send-shortcut-menu">
                      <button
                        type="button"
                        class="send-menu-item"
                        :class="{ selected: sendShortcut === 'enter' }"
                        data-test="shortcut-enter"
                        @click="setShortcut('enter')"
                      >
                        <span class="menu-check">
                          <svg v-if="sendShortcut === 'enter'" viewBox="0 0 16 16" width="12" height="12" fill="none" stroke="currentColor" stroke-width="2">
                            <polyline points="3 8 6 11 13 4" />
                          </svg>
                        </span>
                        <div class="menu-content">
                          <span class="menu-label">Enter 发送</span>
                          <span class="menu-hint">Shift+Enter 换行</span>
                        </div>
                      </button>
                      <button
                        type="button"
                        class="send-menu-item"
                        :class="{ selected: sendShortcut === 'ctrl-enter' }"
                        data-test="shortcut-ctrl-enter"
                        @click="setShortcut('ctrl-enter')"
                      >
                        <span class="menu-check">
                          <svg v-if="sendShortcut === 'ctrl-enter'" viewBox="0 0 16 16" width="12" height="12" fill="none" stroke="currentColor" stroke-width="2">
                            <polyline points="3 8 6 11 13 4" />
                          </svg>
                        </span>
                        <div class="menu-content">
                          <span class="menu-label">Ctrl+Enter 发送</span>
                          <span class="menu-hint">Enter 换行</span>
                        </div>
                      </button>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </form>
        </div>

        <!-- Column 2: Unified Right Workspace Panel (FileTree and Previews as Tabs) -->
        <WorkspacePreviewPane
          :project-id="props.projectId"
          :session-id="props.sessionId"
          :worktree-id="activeWorktreeId"
          :hide-side-rail="props.hideSideRail"
          :preview-collapsed="previewCollapsed"
          :preview-width="previewWidth"
          :is-preview-resizing="isPreviewResizing"
          :active-preview-tab="activePreviewTab"
          :open-file-tabs="openFileTabs"
          :current-active-preview-file="currentActivePreviewFile"
          :file-tree-tab-key="FILE_TREE_TAB_KEY"
          @collapse="setPreviewCollapsed"
          @resizer-mousedown="onPreviewResizerMouseDown"
          @select-tab="selectPreviewTab"
          @close-tab="closeFileTab"
          @select-workspace-file="onSelectWorkspaceFile"
          @branch-changed="onFileTreeBranchChanged"
          @open-file-preview="openFileInPreview"
        />
      </div>

      <!-- Delete Session Confirmation Dialog (Unified with UiDialog) -->
      <UiDialog
        v-if="confirmingDelete"
        title="删除会话"
        width-class="w-full max-w-md"
        data-test="delete-session-dialog"
        @close="confirmingDelete = false"
      >
        <div class="confirm-content flex flex-col gap-3 py-1">
          <p class="confirm-title text-sm font-medium text-platform-text">
            确定删除会话「{{ activeSession.title || activeSession.id }}」吗？
          </p>
          <p class="confirm-hint text-xs text-platform-text-muted">
            仅从平台列表移除；Agent 原生会话仍保留，重新扫描可能再次出现。
          </p>
          <p v-if="deleteError" class="confirm-error text-xs text-red-500" role="alert">{{ deleteError }}</p>
          <div class="confirm-actions flex items-center justify-end gap-2 pt-2">
            <UiButton variant="secondary" size="sm" class="confirm-cancel" @click="confirmingDelete = false">
              取消
            </UiButton>
            <UiButton
              variant="danger"
              size="sm"
              class="confirm-delete"
              data-test="confirm-delete"
              :disabled="deleting"
              :loading="deleting"
              @click="removeSession"
            >
              {{ deleting ? '删除中…' : '删除' }}
            </UiButton>
          </div>
        </div>
      </UiDialog>
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

    <ImagePreviewModal
      :image="previewModalImage"
      :visible="previewModalImage !== null"
      @close="previewModalImage = null"
    />

    <QuickFileSearchDialog
      :open="quickFileSearchOpen"
      :project-id="props.projectId"
      :worktree-id="worktreeId"
      @close="quickFileSearchOpen = false"
      @select="(path) => { quickFileSearchOpen = false; openFileInPreview(path) }"
    />
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
  position: relative;
  /* One source of truth for the rail: its fixed width plus the flex gap gives
     the offset used by the floating drawer and the preview overlay. */
  --chat-rail-width: 20px;
  --chat-rail-gap: 0.5rem;
  --chat-rail-offset: calc(var(--chat-rail-width));
  --chat-filetree-min: 210px;
  /* Allows the preview to overlay when both docked panes would not fit. */
  container-type: inline-size;
  flex: 1;
  min-height: 0;
  min-width: 0;
  display: flex;
  align-items: stretch;
  overflow: hidden;
}

.message-column {
  flex: 1;
  min-width: 300px;
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  min-height: 0;
  padding: calc(var(--space-unit) * 2);
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
  /* Horizontal content (long code lines, wide tables) scrolls inside its own
     block — a markdown <pre> keeps its own overflow — so the message log itself
     never grows a horizontal scrollbar. */
  overflow-y: auto;
  overflow-x: hidden;
}

/* Second Column: Preview Workspace Dock (Locks total width to previewWidth) */
.preview-dock-wrapper {
  display: flex;
  align-items: stretch;
  position: relative;
  min-width: 320px;
  max-width: 80vw;
  min-height: 0;
  transition: width 0.18s ease-out;
  z-index: 10;
}

.preview-dock-wrapper.is-resizing {
  transition: none !important;
}

.preview-dock-wrapper .preview-pane {
  flex: 1;
  min-width: 0;
  height: 100%;
  border: 1px solid var(--color-border);
  border-radius: 4px;
  background: var(--color-surface);
  overflow: hidden;
  position: relative;
}

.filetree-in-preview {
  width: 240px;
  flex: 0 0 240px;
  min-width: 180px;
  max-width: 300px;
  height: 100%;
  margin-left: 0.25rem;
  border: 1px solid var(--color-border);
  border-radius: 4px;
  background: var(--color-surface);
  overflow: hidden;
}

.preview-empty-state {
  flex: 1;
  height: 100%;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 0.5rem;
  padding: 2rem 1.5rem;
  text-align: center;
  user-select: none;
}

/* Unified Workspace Panel */
.preview-pane {
  min-width: 300px;
  max-width: 80vw;
  display: flex;
  flex-direction: column;
  min-height: 0;
  border: 1px solid var(--color-border);
  border-radius: 4px;
  background: var(--color-surface);
  overflow: hidden;
  position: relative;
  z-index: 10;
  transition: width 0.18s ease-out;
}

/* Header */
.activity-title {
  margin: 0;
  font-size: 0.82rem;
  font-weight: 500;
  display: flex;
  align-items: center;
  gap: 0.45rem;
}

.activity-count {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 1.2rem;
  height: 1.2rem;
  padding: 0 0.3rem;
  border-radius: 999px;
  background: var(--color-border);
  font-size: var(--text-size-meta);
  font-weight: 600;
  line-height: 1;
  color: var(--color-text);
}

.activity-scroll {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  overflow-x: hidden;
  padding: 0.5rem 0.6rem;
}

.activity-toggle {
  padding: 0.2rem 0.65rem;
  border: 1px solid var(--color-border-strong);
  border-radius: 6px;
  background: var(--color-surface);
  color: var(--color-text-muted);
  cursor: pointer;
  font: inherit;
  font-size: 0.8rem;
  white-space: nowrap;
  transition: color 0.15s ease, border-color 0.15s ease;
}

.activity-toggle:hover {
  color: var(--color-primary);
  border-color: var(--color-primary);
}

.jump-latest {
  position: absolute;
  bottom: 0.75rem;
  left: 50%;
  transform: translateX(-50%);
  z-index: 5;
  padding: 0.35rem 0.85rem;
  border: 1px solid var(--color-border-strong);
  border-radius: 999px;
  background: var(--color-surface);
  font: inherit;
  font-size: 0.82rem;
  font-weight: 500;
  cursor: pointer;
  transition: color 0.12s ease-out, border-color 0.12s ease-out;
}

.jump-latest:hover {
  border-color: var(--color-accent);
  color: var(--color-accent);
}

.confirm-backdrop {
  position: fixed;
  inset: 0;
  background: rgba(10, 12, 16, 0.75);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 50;
}

.confirm-panel {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
  width: min(26rem, 90vw);
  padding: 1.25rem;
  border-radius: 4px;
  background: var(--color-surface);
  border: 1px solid var(--color-border-strong);
}

.confirm-title {
  margin: 0;
  font-weight: 600;
  font-size: 0.95rem;
  color: var(--color-text);
}

.confirm-hint {
  margin: 0;
  color: var(--color-text-muted);
  font-size: 0.82rem;
  line-height: 1.5;
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
  margin-top: 0.25rem;
}

.confirm-actions button {
  padding: 0.35rem 0.8rem;
  border: 1px solid var(--color-border-strong);
  border-radius: 6px;
  background: var(--color-surface);
  cursor: pointer;
  font: inherit;
  font-size: 0.85rem;
  font-medium: 500;
  transition: all 0.15s ease;
}

.confirm-actions .confirm-cancel:hover {
  background: var(--color-surface-muted);
}

.confirm-actions .confirm-delete {
  background: var(--color-danger);
  border-color: var(--color-danger);
  color: #fff;
}

.confirm-actions .confirm-delete:hover:not(:disabled) {
  opacity: 0.9;
}

.confirm-actions .confirm-delete:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.session-heading {
  margin: 0;
  font-size: 1.05rem;
}

.chip {
  flex: none;
  color: var(--color-text-muted);
  background: var(--color-surface-muted);
  border: 1px solid var(--color-border);
  border-radius: 4px;
  padding: 0.05rem 0.4rem;
  font-size: 0.75rem;
  white-space: nowrap;
}

.chip.branch {
  font-family: var(--font-mono, monospace);
}

.turn-status {
  color: var(--color-primary-hover);
  font-size: 0.8rem;
  font-weight: 500;
}

.load-older {
  display: block;
  margin: 0.6rem auto;
  padding: 0.3rem 0.8rem;
  border: 1px solid var(--color-border-strong);
  border-radius: 999px;
  background: var(--color-surface);
  color: var(--color-text-muted);
  cursor: pointer;
  font-size: 0.8rem;
}

.load-older:hover:not(:disabled) {
  border-color: var(--color-primary);
  color: var(--color-primary);
}

.load-older:disabled {
  cursor: progress;
  opacity: 0.6;
}

.history-note {
  margin: 0;
  padding: 0.4rem 0.6rem;
  border-radius: 6px;
  background: var(--color-surface-muted);
  color: var(--color-text-muted);
  font-size: 0.82rem;
}

.history-error {
  margin: 0;
  padding: 0.4rem 0.6rem;
  border-radius: 6px;
  background: var(--color-warning-soft);
  color: var(--color-warning);
  font-size: 0.82rem;
}

.retry {
  margin-left: 0.4rem;
  padding: 0.1rem 0.5rem;
  border: 1px solid var(--color-border-strong);
  border-radius: 6px;
  background: var(--color-surface);
  cursor: pointer;
  font: inherit;
  font-size: 0.78rem;
  transition: all 0.15s ease;
}

.retry:hover {
  border-color: var(--color-primary);
  color: var(--color-primary);
}

.send {
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
}

.composer-resizer {
  height: 10px;
  margin-top: -0.25rem;
  margin-bottom: 0.1rem;
  cursor: ns-resize;
  display: flex;
  align-items: center;
  justify-content: center;
  user-select: none;
}

.composer-resizer .resizer-handle {
  width: 38px;
  height: 3px;
  border-radius: 2px;
  background: var(--color-border-strong);
  opacity: 0.6;
  transition: opacity 0.15s ease, background-color 0.15s ease;
}

.composer-resizer:hover .resizer-handle {
  opacity: 1;
  background: var(--color-primary);
}

.composer-box {
  position: relative;
  border: 1px solid var(--color-border-strong);
  border-radius: 4px;
  background: var(--color-surface);
  padding: 0.45rem 0.65rem 0.4rem;
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
  transition: border-color 0.15s ease-out;
}

.composer-box:focus-within {
  border-color: var(--color-accent);
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
  padding: 0.1rem 0;
  border: none;
  outline: none;
  resize: none;
  font: inherit;
  font-size: 0.88rem;
  line-height: 1.4;
  background: transparent;
  color: var(--color-text);
  min-height: 2.2rem;
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
  padding-top: 0.3rem;
  border-top: 1px solid var(--color-border);
  min-height: 28px;
}

.composer-tools {
  display: flex;
  align-items: center;
  gap: 0.45rem;
  min-width: 0;
  flex-wrap: wrap;
}

.composer-tool-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  height: 28px;
  width: 28px;
  padding: 0;
  border: 1px solid var(--color-border-strong);
  border-radius: 6px;
  background: var(--color-surface-muted);
  color: var(--color-text);
  font-size: 0.88rem;
  cursor: pointer;
  transition: all 0.15s ease;
}

.composer-plus-btn {
  font-size: 1.15rem;
  font-weight: 500;
  line-height: 1;
}

.composer-tool-btn:hover:not(:disabled) {
  border-color: var(--color-primary);
  background: var(--color-surface);
}

.composer-tool-btn:disabled,
.composer-tool-btn.disabled {
  opacity: 0.45;
  cursor: not-allowed;
  filter: grayscale(1);
}

.composer-actions {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  flex-shrink: 0;
}

.send-split-group {
  position: relative;
  display: inline-flex;
  align-items: center;
}

.send-split-group .send-main-btn {
  border-top-right-radius: 0;
  border-bottom-right-radius: 0;
  border-right: none;
}

.send-dropdown-trigger {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  height: 28px;
  width: 20px;
  padding: 0;
  border: 1px solid var(--color-border-strong);
  border-top-right-radius: 6px;
  border-bottom-right-radius: 6px;
  cursor: pointer;
  transition: all 0.15s ease;
}

.send-dropdown-trigger.send-trigger {
  background: var(--color-primary);
  border-color: var(--color-primary);
  color: var(--color-surface);
  border-left: 1px solid rgba(255, 255, 255, 0.3);
}

.send-dropdown-trigger.stop-trigger {
  background: var(--color-surface);
  border-color: var(--color-danger-border);
  color: var(--color-danger);
  border-left: 1px solid var(--color-danger-border);
}

.send-dropdown-menu {
  position: absolute;
  bottom: calc(100% + 4px);
  right: 0;
  min-width: 175px;
  background: var(--color-surface);
  border: 1px solid var(--color-border-strong);
  border-radius: 4px;
  padding: 4px;
  z-index: 100;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.send-menu-item {
  display: flex;
  align-items: center;
  gap: 6px;
  width: 100%;
  padding: 6px 8px;
  border: none;
  border-radius: 4px;
  background: transparent;
  color: var(--color-text);
  font: inherit;
  font-size: 0.82rem;
  cursor: pointer;
  text-align: left;
  transition: background 0.15s ease;
}

.send-menu-item:hover {
  background: var(--color-surface-muted);
}

.send-menu-item.selected {
  font-weight: 600;
  color: var(--color-primary);
}

.menu-check {
  width: 12px;
  display: inline-block;
  color: var(--color-primary);
  font-size: 0.85rem;
}

.menu-content {
  display: flex;
  flex-direction: column;
}

.menu-label {
  font-size: 0.82rem;
}

.menu-hint {
  font-size: var(--text-size-meta);
  color: var(--color-text-muted);
}

.composer-action {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  height: 28px;
  min-width: 3.2rem;
  padding: 0 0.75rem;
  border: 1px solid var(--color-border-strong);
  border-radius: 6px;
  background: var(--color-surface);
  cursor: pointer;
  font: inherit;
  font-size: 0.82rem;
  font-weight: 500;
  transition: all 0.15s ease;
}

.reconnect {
  align-self: center;
  min-width: 3.4rem;
  padding: 0.35rem 0.9rem;
  border: 1px solid var(--color-border-strong);
  border-radius: 6px;
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

.tool-notice {
  color: var(--color-warning);
  font-size: 0.85rem;
  margin: 0;
  padding: 0.4rem 0.6rem;
  border-radius: 6px;
  background: var(--color-warning-soft);
}

.instance-note {
  color: var(--color-warning);
  font-size: 0.85rem;
  margin: 0;
  padding: 0.4rem 0.6rem;
  border-radius: 6px;
  background: var(--color-warning-soft);
}

.send-error {
  color: var(--color-danger);
  font-size: 0.85rem;
  margin: 0;
  padding: 0.4rem 0.6rem;
  border-radius: 6px;
  background: var(--color-danger-soft);
}

.empty,
.loading {
  color: var(--color-text-muted);
}
</style>
