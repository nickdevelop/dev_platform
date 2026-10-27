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
import { useDraftsStore } from '../../stores/drafts'
import type { GitInfo } from '../../api/git'
import { cachedModels } from '../../shared/modelCache'
import { computeContextUsage } from '../../shared/contextUsage'
import { ellipsisMiddle, formatTokens } from '../../shared/format'
import BackendTag from '../../shared/components/BackendTag.vue'
import MessageView from './rendering/MessageView.vue'
import CapabilityControls from './CapabilityControls.vue'
import ModelSelect from './composer/ModelSelect.vue'
import ApprovalModeSelect from './composer/ApprovalModeSelect.vue'
import ContextUsageMeter from './composer/ContextUsageMeter.vue'
import ComposerImageTray from './composer/ComposerImageTray.vue'
import ImagePreviewModal from './rendering/ImagePreviewModal.vue'
import QuestionQueue from './interactions/QuestionQueue.vue'
import ComposerDropdown from './composer/dropdown/ComposerDropdown.vue'
import { useComposerDropdown } from './composer/dropdown/useComposerDropdown'
import { BuiltInSlashSource } from './composer/dropdown/sources/BuiltInSlashSource'
import { ProviderSlashSource } from './composer/dropdown/sources/ProviderSlashSource'
import { WorkspaceFileSource } from './composer/dropdown/sources/WorkspaceFileSource'
import { SessionMentionSource } from './composer/dropdown/sources/SessionMentionSource'
import { resolveSessionMentions } from './composer/sessionMentionResolver'
import { getSessionHistory } from '../../api/sessions'
import type { ImageAttachment } from '../../api/messageTypes'
import {
  VALID_IMAGE_TYPES,
  imageAttachmentFromFile,
  imageValidationError,
} from './composer/imageFiles'
import { detectBuiltInCommand } from './composer/builtInCommands'
import {
  type ApprovalMode,
  getPersistedApprovalMode,
  setPersistedApprovalMode,
} from './interactions/autoApproval'
import { useAutoApproval } from './interactions/useAutoApproval'
import {
  type SendShortcut,
  getPersistedSendShortcut,
  setPersistedSendShortcut,
  shouldSubmitOnKeydown,
} from './composer/sendShortcut'
import FileSidebar from '../files/FileSidebar.vue'
import FileViewer from '../files/FileViewer.vue'
import { UiButton, UiDialog } from '../../shared/ui'

const props = defineProps<{ projectId: number; sessionId: string }>()

const agents = useAgentsStore()
const sessions = useSessionsStore()
const interactions = useInteractionsStore()
const draftsStore = useDraftsStore()
const router = useRouter()

let currentSessionId = props.sessionId
let isRestoringDraft = false

const initialSessionDraft = draftsStore.getSessionDraft(props.sessionId)

interface OpenFileTab {
  path: string
  name: string
}

function getFileName(filePath: string): string {
  const parts = filePath.replace(/\\/g, '/').split('/')
  return parts[parts.length - 1] || filePath
}

const openFileTabs = ref<OpenFileTab[]>([])
const activePreviewTab = ref<string>('activity')

function selectPreviewTab(tabKey: string) {
  activePreviewTab.value = tabKey
  if (tabKey === 'activity') {
    scrollActivityToBottomReliably(false)
  }
}

function openFileInPreview(filePath: string) {
  const cleanPath = filePath.replace(/\\/g, '/').replace(/^\/+/, '')
  if (!cleanPath) return
  const exists = openFileTabs.value.some((f) => f.path === cleanPath)
  if (!exists) {
    openFileTabs.value.push({
      path: cleanPath,
      name: getFileName(cleanPath),
    })
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
      activePreviewTab.value = 'activity'
      scrollActivityToBottomReliably(false)
    }
  }
}

function onSelectWorkspaceFile(filePath: string) {
  openFileInPreview(filePath)
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

const approvalMode = ref<ApprovalMode>(
  (initialSessionDraft.approvalMode as ApprovalMode | undefined) || getPersistedApprovalMode(),
)
const sendShortcut = ref<SendShortcut>(getPersistedSendShortcut())
const sendMenuOpen = ref(false)

function toggleSendMenu() {
  sendMenuOpen.value = !sendMenuOpen.value
}

function setShortcut(shortcut: SendShortcut) {
  sendShortcut.value = shortcut
  setPersistedSendShortcut(shortcut)
  sendMenuOpen.value = false
}

function onWindowClick(event: MouseEvent) {
  if (!sendMenuOpen.value) return
  const target = event.target as Element | null
  if (target && !target.closest('.send-split-group')) {
    sendMenuOpen.value = false
  }
}

function onTextareaKeydown(event: KeyboardEvent) {
  if (dropdown.onKeydown(event)) return
  if (shouldSubmitOnKeydown(event, sendShortcut.value)) {
    event.preventDefault()
    void onSubmit()
  }
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
const attachedImages = ref<ImageAttachment[]>(
  Array.isArray(initialSessionDraft.attachedImages) ? [...initialSessionDraft.attachedImages] : [],
)

watch(approvalMode, (val) => {
  setPersistedApprovalMode(val)
  if (isRestoringDraft) return
  if (!currentSessionId) return
  draftsStore.setSessionDraft(currentSessionId, { approvalMode: val })
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

const previewModalImage = ref<ImageAttachment | null>(null)
const fileInputRef = ref<HTMLInputElement | null>(null)

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

const MAX_IMAGES_COUNT = 10

function removeImage(id: string) {
  attachedImages.value = attachedImages.value.filter((img) => img.id !== id)
}

function openImagePreview(img: ImageAttachment) {
  previewModalImage.value = img
}

function triggerFileInput() {
  fileInputRef.value?.click()
}

const MAX_TEXT_FILE_SIZE_BYTES = 1024 * 1024 // 1MB

function readFileAsText(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve((reader.result as string) || '')
    reader.onerror = () => reject(reader.error)
    reader.readAsText(file)
  })
}

async function addImageFiles(files: FileList | File[]) {
  const list = Array.from(files)
  for (const file of list) {
    if (attachedImages.value.length >= MAX_IMAGES_COUNT) {
      toolNotice.value = `单次最多添加 ${MAX_IMAGES_COUNT} 张图片`
      break
    }
    const validationError = imageValidationError(file)
    if (validationError !== null) {
      toolNotice.value = validationError
      continue
    }
    try {
      attachedImages.value.push(await imageAttachmentFromFile(file))
    } catch {
      toolNotice.value = `读取图片失败: ${file.name || '未命名图片'}`
    }
  }
}

async function handleUploadedFiles(files: FileList | File[]) {
  const list = Array.from(files)
  const imageFiles: File[] = []
  const textFiles: File[] = []

  for (const file of list) {
    if (file.type.startsWith('image/') || VALID_IMAGE_TYPES[file.type]) {
      imageFiles.push(file)
    } else {
      textFiles.push(file)
    }
  }

  if (imageFiles.length > 0) {
    await addImageFiles(imageFiles)
  }

  for (const file of textFiles) {
    if (file.size > MAX_TEXT_FILE_SIZE_BYTES) {
      toolNotice.value = `文本文件超过 1MB 限制: ${file.name}`
      continue
    }
    try {
      const text = await readFileAsText(file)
      const prefix = messageText.value.trim() ? '\n\n' : ''
      messageText.value += `${prefix}--- 文件: ${file.name} ---\n${text}\n---`
    } catch {
      toolNotice.value = `读取文件失败: ${file.name}`
    }
  }
}

function onFileInput(event: Event) {
  const target = event.target as HTMLInputElement
  if (target.files && target.files.length > 0) {
    void handleUploadedFiles(target.files)
    target.value = ''
  }
}

function onPaste(event: ClipboardEvent) {
  const items = event.clipboardData?.items
  if (!items) return
  const imageFiles: File[] = []
  for (const item of Array.from(items)) {
    if (item.kind === 'file' && item.type.startsWith('image/')) {
      const file = item.getAsFile()
      if (file) {
        const ext = file.type.split('/')[1]?.split('+')[0] || 'png'
        const rawName = file.name || ''
        const isGeneric = !rawName || rawName === 'image.png' || rawName === 'blob' || !rawName.includes('.')
        const name = isGeneric ? `pasted-image-${Date.now()}.${ext}` : rawName
        imageFiles.push(new File([file], name, { type: file.type }))
      }
    }
  }
  if (imageFiles.length > 0) {
    event.preventDefault()
    void addImageFiles(imageFiles)
  }
}

function onDragOver(event: DragEvent) {
  if (
    event.dataTransfer?.types?.includes('Files') ||
    event.dataTransfer?.types?.includes('text/plain')
  ) {
    event.dataTransfer.dropEffect = 'copy'
  }
}

function onDrop(event: DragEvent) {
  const textData = event.dataTransfer?.getData('text/plain')
  if (textData && textData.startsWith('@')) {
    event.preventDefault()
    const prefix = messageText.value.length > 0 && !messageText.value.endsWith(' ') ? ' ' : ''
    messageText.value += `${prefix}${textData} `
    return
  }

  const files = event.dataTransfer?.files
  if (files && files.length > 0) {
    void handleUploadedFiles(files)
  }
}
// Scrolling follows the newest entry only while the user is at the bottom;
// both panes share that rule through shared/scrollFollow. The log scrolls by
// hand in jsdom tests, so every read and write of the scroller geometry goes
// through the element itself.
const logScroll = ref<HTMLElement | null>(null)
const activityScroll = ref<HTMLElement | null>(null)
// Whether the activity pane is collapsed (persisted in localStorage).
// Defaults to collapsed, presenting a floating FAB that opens an in-tab drawer.
const pane = useActivityPane(() => {
  // Expanding returns to the newest activity when the pane is showing activity.
  if (activePreviewTab.value === 'activity') {
    scrollActivityToBottomReliably(false)
  }
})
const {
  previewCollapsed,
  setPreviewCollapsed,
  previewWidth,
  setPreviewWidth,
  activityCollapsed,
  setActivityCollapsed,
  fileTreePinned,
  fileTreeCollapsed,
  fileTreeFloatingOpen,
  isFileTreeDocked,
  isFileTreeFloating,
  isFileTreeVisible,
  setFileTreePinned,
  toggleFileTreePinned,
  setFileTreeCollapsed,
  toggleFileTree,
  openFloatingFileTree,
  closeFloatingFileTree,
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
    activePreviewTab.value = 'activity'
  },
)

// "At the bottom" tolerates a small rounding gap so subpixel layouts, and a
// wheel that stops a hair short, still count as following the conversation.
const FOLLOW_THRESHOLD_PX = 24
// The picker, so a send the backend refused can ask it to re-read the catalog.


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

function scrollActivityToBottomReliably(smooth = false) {
  activityPinned.value = true
  showJumpActivity.value = false
  void nextTick(() => {
    scrollActivityToBottom(smooth)
    if (typeof window !== 'undefined' && typeof window.requestAnimationFrame === 'function') {
      window.requestAnimationFrame(() => {
        scrollActivityToBottom(smooth)
      })
    }
  })
}

watch(activePreviewTab, (newTab) => {
  if (newTab === 'activity') {
    scrollActivityToBottomReliably(false)
  }
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

// The tool call awaiting confirmation and the pending question forms are the
// interaction layer's concern (see ./interactions/useSessionInteractions.ts);
// the panel renders whatever that layer reports.

onMounted(() => {
  window.addEventListener('click', onWindowClick)
  void open()
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
        approvalMode: approvalMode.value,
      })
    }
    currentSessionId = newSessionId
    if (newSessionId) {
      isRestoringDraft = true
      const draft = draftsStore.getSessionDraft(newSessionId)
      messageText.value = draft.messageText || ''
      attachedImages.value = Array.isArray(draft.attachedImages) ? [...draft.attachedImages] : []
      if (draft.approvalMode) {
        approvalMode.value = draft.approvalMode
      }
      void nextTick(() => {
        isRestoringDraft = false
      })
    }
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

useAutoApproval({
  mode: approvalMode,
  pendingToolCall,
  pendingForms,
  confirmTool: onConfirmTool,
  answerForm: onAnswerForm,
})

onBeforeUnmount(() => {
  window.removeEventListener('click', onWindowClick)
  // Tabs keep other sessions alive: unmounting this panel must not close other
  // sessions' streams. The session's own subscriptions stay open so returning
  // via a tab is instant; they are released when the tab closes or the session
  // is deleted.
  if (currentSessionId && !isRestoringDraft) {
    draftsStore.setSessionDraft(currentSessionId, {
      messageText: messageText.value,
      attachedImages: attachedImages.value,
      approvalMode: approvalMode.value,
    })
  }
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
  sendError.value = ''
  sessions.error = ''
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
    scrollActivityToBottomReliably(false)
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
  const textToSend = await resolveSessionMentions(props.projectId, messageText.value, (targetId) =>
    getSessionHistory(props.projectId, targetId),
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
      await router.push({ name: 'session-new', params: { projectId: String(props.projectId) } })
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
      <header class="chat-header">
        <div class="title-row">
          <BackendTag :backend="activeSession.backend" />
          <h2 class="session-heading">{{ activeSession.title || activeSession.id }}</h2>
          <button
            v-if="previewCollapsed"
            type="button"
            class="header-tool-btn preview-btn"
            data-test="toggle-preview-btn"
            title="展开预览面板"
            aria-label="展开预览面板"
            @click="setPreviewCollapsed(false)"
          >
            <span>预览</span>
            <span v-if="activityCount > 0" class="mini-count">{{ activityCount }}</span>
          </button>
          <button
            type="button"
            class="header-tool-btn filetree-btn"
            :class="{ 'is-active': isFileTreeVisible }"
            data-test="toggle-filetree-btn"
            :title="isFileTreeVisible ? '收起文件树' : '打开文件树'"
            :aria-label="isFileTreeVisible ? '收起文件树' : '打开文件树'"
            @click="toggleFileTree"
          >
            <svg class="header-tool-icon" viewBox="0 0 24 24" width="13" height="13" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M3 7v10a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2V9a2 2 0 0 0-2-2h-6l-2-2H5a2 2 0 0 0-2 2z" />
            </svg>
            <span>文件树</span>
          </button>
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
        class="conversation-split"
        data-test="conversation-split"
        :class="{
          'activity-collapsed': previewCollapsed,
          'preview-collapsed': previewCollapsed,
          'filetree-docked': isFileTreeDocked,
        }"
      >
        <!-- Floating file tree backdrop -->
        <div
          v-if="isFileTreeFloating"
          class="filetree-backdrop"
          data-test="filetree-backdrop"
          @click="closeFloatingFileTree"
        />
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
                :running="isRunning"
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
            :autopilot="approvalMode === 'autopilot'"
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
                  <ApprovalModeSelect v-model="approvalMode" />
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
                        <span class="menu-check">{{ sendShortcut === 'enter' ? '✓' : '' }}</span>
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
                        <span class="menu-check">{{ sendShortcut === 'ctrl-enter' ? '✓' : '' }}</span>
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

        <!-- Column 2: Preview Tab Panel -->
        <aside
          v-if="!previewCollapsed"
          class="preview-pane activity-pane"
          :class="{ 'is-resizing': isPreviewResizing }"
          :style="{ width: `${previewWidth}px`, flex: `0 0 ${previewWidth}px` }"
          aria-label="预览面板"
          data-test="activity-pane"
          data-test-preview="preview-pane"
        >
          <div
            class="preview-resizer"
            data-test="preview-resizer"
            title="拖拽调整预览栏宽度"
            @mousedown="onPreviewResizerMouseDown"
          >
            <span class="preview-resizer-line"></span>
          </div>
          <header class="preview-head activity-head">
            <div class="preview-tabs-list" role="tablist">
              <!-- Tab 0: Fixed 思考与工具 -->
              <button
                type="button"
                class="pane-tab activity-title"
                :class="{ 'is-active': activePreviewTab === 'activity' }"
                role="tab"
                :aria-selected="activePreviewTab === 'activity'"
                data-test="tab-activity"
                title="思考与工具"
                @click="selectPreviewTab('activity')"
              >
                <span>思考与工具</span>
                <span v-if="activityCount > 0" class="activity-count" data-test="activity-count">{{ activityCount }}</span>
              </button>

              <!-- Dynamic Tabs: Opened files -->
              <div
                v-for="fileTab in openFileTabs"
                :key="fileTab.path"
                class="pane-tab preview-file-tab"
                :class="{ 'is-active': activePreviewTab === fileTab.path }"
                role="tab"
                :aria-selected="activePreviewTab === fileTab.path"
                :title="fileTab.path"
                data-test="tab-file"
                @click="selectPreviewTab(fileTab.path)"
                @mousedown.middle.prevent="closeFileTab(fileTab.path, $event)"
              >
                <span class="file-tab-name">{{ fileTab.name }}</span>
                <button
                  type="button"
                  class="tab-close-icon"
                  title="关闭标签"
                  aria-label="关闭标签"
                  data-test="close-file-tab"
                  @click="closeFileTab(fileTab.path, $event)"
                >
                  ×
                </button>
              </div>
            </div>

            <div class="head-actions">
              <button
                type="button"
                class="activity-toggle"
                data-test="activity-hide"
                data-test-collapse="activity-collapse"
                title="折叠预览栏"
                aria-label="折叠预览栏"
                @click="setPreviewCollapsed(true)"
              >
                收起
              </button>
            </div>
          </header>

          <div class="preview-content">
            <!-- Content 1: 思考与工具 -->
            <div
              v-show="activePreviewTab === 'activity'"
              class="activity-pane-view"
            >
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
                  :running="isRunning"
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
            </div>

            <!-- Content 2: 文件预览 -->
            <div
              v-show="activePreviewTab !== 'activity'"
              class="preview-file-viewer"
            >
              <FileViewer
                :project-id="props.projectId"
                :active-path="activePreviewTab !== 'activity' ? activePreviewTab : ''"
                :hide-tabs="true"
                @select="openFileInPreview"
              />
            </div>
          </div>
        </aside>

        <!-- Column 3: File Tree Drawer (Docked or Floating) -->
        <aside
          v-if="isFileTreeVisible"
          class="filetree-drawer"
          :class="{
            'filetree-docked': isFileTreeDocked,
            'filetree-floating': isFileTreeFloating,
          }"
          aria-label="项目文件树"
          data-test="filetree-drawer"
        >
          <header class="filetree-head">
            <div class="filetree-title">
              <span class="title-text">工作区文件</span>
            </div>
            <div class="filetree-controls">
              <button
                type="button"
                class="dock-btn pin-btn"
                data-test="filetree-pin-btn"
                :class="{ pinned: isFileTreeDocked }"
                :title="isFileTreeDocked ? '取消固定（浮出抽屉）' : '固定文件树'"
                :aria-label="isFileTreeDocked ? '取消固定' : '固定文件树'"
                :aria-pressed="isFileTreeDocked.toString()"
                @click="toggleFileTreePinned"
              >
                <svg class="dock-icon" viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2">
                  <line x1="12" y1="17" x2="12" y2="22" />
                  <path d="M5 17h14v-1.76a2 2 0 0 0-1.11-1.79l-1.78-.9A2 2 0 0 1 15 10.76V6h1a1 1 0 0 0 1-1V3a1 1 0 0 0-1-1H8a1 1 0 0 0-1 1v2a1 1 0 0 0 1 1h1v4.76a2 2 0 0 1-1.11 1.79l-1.78.9A2 2 0 0 0 5 15.24Z" />
                </svg>
              </button>
              <button
                type="button"
                class="dock-btn collapse-btn"
                data-test="filetree-collapse-btn"
                title="收起文件树"
                aria-label="收起文件树"
                @click="setFileTreeCollapsed(true)"
              >
                <svg class="dock-icon" viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2">
                  <polyline points="9 18 15 12 9 6" />
                </svg>
              </button>
            </div>
          </header>

          <div class="filetree-body">
            <FileSidebar
              :project-id="props.projectId"
              :active-path="activePreviewTab !== 'activity' ? activePreviewTab : ''"
              @select="onSelectWorkspaceFile"
            />
          </div>
        </aside>
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
  flex: 1;
  min-height: 0;
  min-width: 0;
  display: flex;
  align-items: stretch;
  gap: 0.5rem;
  overflow: hidden;
}

.message-column {
  flex: 1;
  min-width: 300px;
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
  /* Horizontal content (long code lines, wide tables) scrolls inside its own
     block — a markdown <pre> keeps its own overflow — so the message log itself
     never grows a horizontal scrollbar. */
  overflow-y: auto;
  overflow-x: hidden;
}

/* Second Column: Preview Pane (Tab Panel) */
.preview-pane {
  min-width: 320px;
  max-width: 80vw;
  display: flex;
  flex-direction: column;
  min-height: 0;
  border: 1px solid var(--color-border);
  border-radius: 8px;
  background: var(--color-surface);
  box-shadow: 0 1px 3px rgba(15, 23, 42, 0.06);
  overflow: hidden;
  position: relative;
  z-index: 10;
  transition: width 0.18s ease;
}

.preview-pane.is-resizing {
  transition: none !important;
}

.preview-resizer {
  position: absolute;
  left: 0;
  top: 0;
  bottom: 0;
  width: 6px;
  cursor: col-resize;
  z-index: 20;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: background 0.15s ease;
}

.preview-resizer:hover,
.preview-pane.is-resizing .preview-resizer {
  background: var(--color-primary-soft, rgba(59, 130, 246, 0.15));
}

.preview-resizer-line {
  width: 2px;
  height: 24px;
  border-radius: 1px;
  background: var(--color-border-strong, #cbd5e1);
  transition: background 0.15s ease, height 0.15s ease;
}

.preview-resizer:hover .preview-resizer-line,
.preview-pane.is-resizing .preview-resizer-line {
  background: var(--color-primary, #3b82f6);
  height: 36px;
}

@media (max-width: 900px) {
  .preview-pane {
    position: absolute;
    top: 0;
    right: 0;
    bottom: 0;
    width: min(32rem, 90vw) !important;
    max-width: 90vw;
    z-index: 25;
    box-shadow: -4px 0 24px rgba(15, 23, 42, 0.16);
    animation: activitySlideIn 0.18s ease-out;
  }

  .preview-resizer {
    display: none;
  }
}

@keyframes activitySlideIn {
  from {
    transform: translateX(12px);
    opacity: 0.8;
  }
  to {
    transform: translateX(0);
    opacity: 1;
  }
}

.preview-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.5rem;
  padding: 0.4rem 0.6rem;
  border-bottom: 1px solid var(--color-border);
  background: var(--color-surface-muted, #f8fafc);
  flex-shrink: 0;
}

.preview-tabs-list {
  display: flex;
  align-items: center;
  gap: 3px;
  overflow-x: auto;
  flex: 1;
  min-width: 0;
  scrollbar-width: thin;
}

.preview-tabs-list::-webkit-scrollbar {
  height: 3px;
}

.pane-tab {
  border: 1px solid transparent;
  background: transparent;
  padding: 3px 8px;
  border-radius: 5px;
  font-size: 0.78rem;
  color: var(--color-text-muted);
  cursor: pointer;
  transition: all 0.15s ease;
  display: inline-flex;
  align-items: center;
  gap: 4px;
  white-space: nowrap;
  user-select: none;
}

.pane-tab:hover {
  color: var(--color-text);
  background: var(--color-surface);
}

.pane-tab.is-active {
  background: var(--color-surface);
  color: var(--color-primary);
  border-color: var(--color-border);
  font-weight: 600;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.05);
}

.preview-file-tab {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  max-width: 160px;
}

.file-tab-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.tab-close-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 14px;
  height: 14px;
  border-radius: 50%;
  border: none;
  background: transparent;
  color: var(--color-text-muted);
  font-size: 0.85rem;
  line-height: 1;
  cursor: pointer;
  padding: 0;
  margin-left: 2px;
  transition: all 0.12s ease;
}

.tab-close-icon:hover {
  background: var(--color-danger-border, #fee2e2);
  color: var(--color-danger, #ef4444);
}

.preview-content {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  position: relative;
  overflow: hidden;
}

.activity-pane-view {
  flex: 1;
  min-height: 0;
  height: 100%;
  display: flex;
  flex-direction: column;
  position: relative;
  overflow: hidden;
}

.preview-file-viewer {
  flex: 1;
  min-height: 0;
  height: 100%;
  overflow: hidden;
}

/* Third Column: File Tree Drawer */
.filetree-backdrop {
  position: absolute;
  inset: 0;
  background: rgba(15, 23, 42, 0.28);
  z-index: 35;
  animation: backdropFadeIn 0.15s ease-out;
}

@keyframes backdropFadeIn {
  from {
    opacity: 0;
  }
  to {
    opacity: 1;
  }
}

.filetree-drawer {
  display: flex;
  flex-direction: column;
  min-height: 0;
  border: 1px solid var(--color-border);
  border-radius: 8px;
  background: var(--color-surface);
  box-shadow: 0 1px 3px rgba(15, 23, 42, 0.06);
  overflow: hidden;
}

.filetree-drawer.filetree-docked {
  flex: 0 0 260px;
  min-width: 210px;
  max-width: 320px;
  height: 100%;
  position: relative;
  z-index: 5;
}

.filetree-drawer.filetree-floating {
  position: absolute;
  top: 0;
  right: 0;
  bottom: 0;
  width: 290px;
  z-index: 40;
  border-radius: 8px 0 0 8px;
  box-shadow: -4px 0 24px rgba(15, 23, 42, 0.18);
  animation: drawerSlideIn 0.18s ease-out;
}

@keyframes drawerSlideIn {
  from {
    transform: translateX(100%);
  }
  to {
    transform: translateX(0);
  }
}

.filetree-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.5rem;
  padding: 0.45rem 0.65rem;
  border-bottom: 1px solid var(--color-border);
  background: var(--color-surface-muted, #f8fafc);
  flex-shrink: 0;
}

.filetree-title {
  font-size: 0.82rem;
  font-weight: 600;
  color: var(--color-text);
}

.filetree-controls {
  display: flex;
  align-items: center;
  gap: 4px;
}

.filetree-controls .dock-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 22px;
  height: 22px;
  padding: 0;
  border: 1px solid var(--color-border);
  border-radius: 4px;
  background: var(--color-surface);
  color: var(--color-text-muted);
  cursor: pointer;
  transition: all 0.15s ease;
}

.filetree-controls .dock-btn:hover {
  color: var(--color-primary);
  border-color: var(--color-primary);
}

.filetree-controls .pin-btn.pinned {
  color: var(--color-primary);
  border-color: var(--color-primary);
  background: var(--color-primary-soft, #e8effd);
}

.filetree-body {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  overflow-x: hidden;
}

/* Header tool buttons */
.header-tool-btn {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 0.15rem 0.55rem;
  border: 1px solid var(--color-border-strong);
  border-radius: 6px;
  background: var(--color-surface);
  color: var(--color-text-muted);
  font: inherit;
  font-size: 0.8rem;
  cursor: pointer;
  transition: all 0.15s ease;
}

.header-tool-btn:hover {
  color: var(--color-primary);
  border-color: var(--color-primary);
}

.header-tool-btn.is-active {
  color: var(--color-primary);
  border-color: var(--color-primary);
  background: var(--color-primary-soft, #e8effd);
}

.header-tool-btn .mini-count {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 1.1rem;
  height: 1.1rem;
  padding: 0 0.25rem;
  border-radius: 999px;
  background: var(--color-border);
  font-size: 0.7rem;
  font-weight: 600;
  color: var(--color-text);
}

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
  font-size: 0.72rem;
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
  box-shadow: 0 4px 12px rgba(15, 23, 42, 0.15);
  font: inherit;
  font-size: 0.82rem;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.15s ease;
}

.jump-latest:hover {
  border-color: var(--color-primary);
  color: var(--color-primary);
}

.chat-header {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
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
  font-size: 1.05rem;
  font-weight: 600;
}

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
  transition: all 0.15s ease;
}

.workdir-copy:hover {
  color: var(--color-primary-hover);
  border-color: var(--color-primary);
}

.delete-session {
  padding: 0.15rem 0.55rem;
  border: 1px solid var(--color-danger-border);
  border-radius: 6px;
  background: var(--color-surface);
  color: var(--color-danger);
  font: inherit;
  font-size: 0.8rem;
  cursor: pointer;
  transition: all 0.15s ease;
}

.delete-session:hover {
  background: var(--color-danger-soft, #fee2e2);
}

.confirm-backdrop {
  position: fixed;
  inset: 0;
  background: rgba(15, 23, 42, 0.45);
  backdrop-filter: blur(1px);
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
  border-radius: 6px;
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  box-shadow: 0 10px 30px rgba(15, 23, 42, 0.2);
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
  border-radius: 6px;
  background: var(--color-surface);
  padding: 0.45rem 0.65rem 0.4rem;
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
  box-shadow: 0 1px 2px rgba(15, 23, 42, 0.05);
  transition: border-color 0.15s ease, box-shadow 0.15s ease;
}

.composer-box:focus-within {
  border-color: var(--color-primary);
  box-shadow: 0 0 0 2px var(--color-primary-soft, rgba(59, 130, 246, 0.2));
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
  border-radius: 6px;
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.12);
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
  font-size: 0.72rem;
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
