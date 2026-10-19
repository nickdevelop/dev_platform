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
import { getSessionHistory, type ImageAttachment, type ImageMediaType } from '../../api/sessions'
import { detectBuiltInCommand } from './composer/builtInCommands'
import FileSidebar from '../files/FileSidebar.vue'
import FileViewer from '../files/FileViewer.vue'

const props = defineProps<{ projectId: number; sessionId: string }>()

const agents = useAgentsStore()
const sessions = useSessionsStore()
const interactions = useInteractionsStore()
const router = useRouter()

const RIGHT_PANE_TAB_KEY = 'dev-platform-right-pane-tab-v1'
const rightPaneMode = ref<'activity' | 'files'>('activity')
try {
  const stored = localStorage.getItem(RIGHT_PANE_TAB_KEY)
  if (stored === 'files' || stored === 'activity') {
    rightPaneMode.value = stored
  }
} catch {
  // best effort
}
function setRightPaneMode(mode: 'activity' | 'files') {
  rightPaneMode.value = mode
  try {
    localStorage.setItem(RIGHT_PANE_TAB_KEY, mode)
  } catch {
    // best effort
  }
}

const activeWorkspaceFile = ref<string>('')
const fileSubView = ref<'tree' | 'viewer'>('tree')

function onSelectWorkspaceFile(filePath: string) {
  activeWorkspaceFile.value = filePath
  fileSubView.value = 'viewer'
}

function onBackToTree() {
  fileSubView.value = 'tree'
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
const messageText = ref('')
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

function onTextareaKeydown(event: KeyboardEvent) {
  if (dropdown.onKeydown(event)) return
  if (event.key === 'Enter' && !event.shiftKey) {
    event.preventDefault()
    void onSubmit()
  }
}

// Image attachments support
const attachedImages = ref<ImageAttachment[]>([])
const previewModalImage = ref<ImageAttachment | null>(null)
const fileInputRef = ref<HTMLInputElement | null>(null)

const currentModelSupportsImages = computed<boolean>(() => {
  const session = activeSession.value
  if (!session) return false
  const model = selectedModel.value
  if (!model) return false
  const cached = cachedModels(props.projectId, session.backend)
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

const MAX_IMAGE_SIZE_BYTES = 5 * 1024 * 1024 // 5MB
const MAX_IMAGES_COUNT = 10
const VALID_IMAGE_TYPES: Record<string, ImageMediaType> = {
  'image/jpeg': 'image/jpeg',
  'image/png': 'image/png',
  'image/webp': 'image/webp',
  'image/gif': 'image/gif',
}

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
    const cleanType = (file.type || '').split(';')[0].trim().toLowerCase()
    const mediaType = VALID_IMAGE_TYPES[cleanType]
    if (!mediaType) {
      toolNotice.value = `不支持的文件类型: ${file.name || '未命名图片'} (仅支持 PNG/JPEG/WebP/GIF)`
      continue
    }
    if (file.size > MAX_IMAGE_SIZE_BYTES) {
      toolNotice.value = `图片超过 5MB 限制: ${file.name || '未命名图片'}`
      continue
    }

    try {
      const base64 = await readFileAsBase64(file)
      const fileName = file.name || `image-${Date.now()}.png`
      attachedImages.value.push({
        id: `img-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`,
        name: fileName,
        mediaType,
        data: base64,
        size: file.size,
      })
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

function readFileAsBase64(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => {
      const result = reader.result as string
      const commaIndex = result.indexOf(',')
      resolve(commaIndex !== -1 ? result.slice(commaIndex + 1) : result)
    }
    reader.onerror = () => reject(reader.error)
    reader.readAsDataURL(file)
  })
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
  // Expanding returns to the newest activity when the pane was following it.
  activityPinned.value = true
  showJumpActivity.value = false
  void nextTick(() => scrollActivityToBottom(false))
})
const {
  activityCollapsed: activityCollapsedState,
  setActivityCollapsed,
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
        class="conversation-split"
        data-test="conversation-split"
        :class="{ 'activity-collapsed': activityCollapsed }"
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
                placeholder="向 Agent 发送消息… (输入 / 使用快捷命令，@ 引用文件或会话)"
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
                    ref="modelPicker"
                    v-model="selectedModel"
                    :project-id="props.projectId"
                    :backend="activeSession.backend"
                    :model-selection="activeSession.capabilities.modelSelection"
                    @status="onModelStatus"
                    @picked="onModelPicked"
                  />
                  <ContextUsageMeter
                    :usage="contextUsage"
                    :context-detail="contextDetail"
                  />
                </div>
                <div class="composer-actions">
                  <span class="composer-hint">Enter 发送 · Shift+Enter 换行</span>
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
              </div>
            </div>
          </form>
        </div>

        <aside
          v-if="!activityCollapsed"
          class="activity-pane"
          aria-label="右侧活动面板"
          data-test="activity-pane"
        >
          <header class="activity-head">
            <div class="activity-pane-tabs" role="tablist">
              <button
                type="button"
                class="pane-tab activity-title"
                :class="{ 'is-active': rightPaneMode === 'activity' }"
                role="tab"
                :aria-selected="rightPaneMode === 'activity'"
                data-test="tab-activity"
                @click="setRightPaneMode('activity')"
              >
                活动<span v-if="activityCount > 0" class="activity-count" data-test="activity-count">{{ activityCount }}</span>
              </button>
              <button
                type="button"
                class="pane-tab"
                :class="{ 'is-active': rightPaneMode === 'files' }"
                role="tab"
                :aria-selected="rightPaneMode === 'files'"
                data-test="tab-files"
                @click="setRightPaneMode('files')"
              >
                工作区文件
              </button>
            </div>

            <div class="head-actions">
              <button
                v-if="rightPaneMode === 'files' && fileSubView === 'viewer'"
                type="button"
                class="back-to-tree-btn"
                title="返回文件树"
                @click="onBackToTree"
              >
                ‹ 文件树
              </button>
              <button
                type="button"
                class="activity-toggle"
                data-test="activity-hide"
                data-test-collapse="activity-collapse"
                title="收起右侧栏"
                aria-label="收起右侧栏"
                @click="setActivityCollapsed(true)"
              >
                收起
              </button>
            </div>
          </header>

          <template v-if="rightPaneMode === 'activity'">
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
          </template>

          <template v-else-if="rightPaneMode === 'files'">
            <div class="files-pane-wrapper">
              <FileSidebar
                v-show="fileSubView === 'tree'"
                :project-id="props.projectId"
                :active-path="activeWorkspaceFile"
                @select="onSelectWorkspaceFile"
              />
              <FileViewer
                v-show="fileSubView === 'viewer'"
                :project-id="props.projectId"
                :active-path="activeWorkspaceFile"
                @select="(p) => (activeWorkspaceFile = p)"
                @close="() => {}"
              />
            </div>
          </template>
        </aside>

        <button
          v-if="activityCollapsed"
          type="button"
          class="activity-fab"
          data-test="activity-expand"
          data-test-narrow="activity-expand-narrow"
          :aria-label="`展开活动（共 ${activityCount} 条）`"
          @click="setActivityCollapsed(false)"
        >
          <span class="fab-label">活动</span>
          <span v-if="activityCount > 0" class="activity-count" data-test="activity-count">{{ activityCount }}</span>
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
  /* Conversation area fills the container height; activity pane floats over
     the tab as an in-tab drawer / popup both on wide and narrow viewports. */
  position: relative;
  flex: 1;
  min-height: 0;
  min-width: 0;
  display: flex;
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
  /* Horizontal content (long code lines, wide tables) scrolls inside its own
     block — a markdown <pre> keeps its own overflow — so the message log itself
     never grows a horizontal scrollbar. */
  overflow-y: auto;
  overflow-x: hidden;
}

.activity-pane {
  position: absolute;
  top: 0;
  right: 0;
  bottom: 0;
  width: min(32rem, 88vw);
  display: flex;
  flex-direction: column;
  min-height: 0;
  min-width: 0;
  z-index: 10;
  border: 1px solid var(--color-border);
  border-radius: 10px;
  background: var(--color-surface);
  box-shadow: -4px 0 24px rgba(15, 23, 42, 0.16);
  animation: activitySlideIn 0.18s ease-out;
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

@media (max-width: 900px) {
  .activity-pane {
    width: min(24rem, 88vw);
  }
}

.activity-fab {
  position: absolute;
  right: 1rem;
  bottom: 5.5rem;
  z-index: 9;
  display: inline-flex;
  align-items: center;
  gap: 0.4rem;
  padding: 0.45rem 0.85rem;
  border: 1px solid var(--color-border-strong);
  border-radius: 999px;
  background: var(--color-surface);
  color: var(--color-text);
  box-shadow: 0 4px 14px rgba(15, 23, 42, 0.15);
  cursor: pointer;
  font: inherit;
  font-size: 0.85rem;
  font-weight: 500;
  transition: transform 0.15s ease, box-shadow 0.15s ease, border-color 0.15s ease, color 0.15s ease;
}

.activity-fab:hover {
  transform: translateY(-1px);
  box-shadow: 0 6px 18px rgba(15, 23, 42, 0.2);
  border-color: var(--color-primary);
  color: var(--color-primary);
}

.activity-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.5rem;
  padding: 0.55rem 0.75rem;
  border-bottom: 1px solid var(--color-border);
  background: var(--color-surface);
  border-top-left-radius: 10px;
  border-top-right-radius: 10px;
}

.activity-pane-tabs {
  display: flex;
  gap: 2px;
  background: var(--color-bg, #f4f5f7);
  padding: 2px;
  border-radius: 5px;
  border: 1px solid var(--color-border, #e3e6eb);
}

.pane-tab {
  border: none;
  background: transparent;
  padding: 3px 8px;
  border-radius: 4px;
  font-size: 0.78rem;
  color: var(--color-text-muted);
  cursor: pointer;
  transition: all 0.15s ease;
  display: inline-flex;
  align-items: center;
  gap: 4px;
}

.pane-tab:hover {
  color: var(--color-text);
}

.pane-tab.is-active {
  background: var(--color-surface, #ffffff);
  color: var(--color-primary, #2563eb);
  font-weight: 600;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.05);
}

.head-actions {
  display: flex;
  align-items: center;
  gap: 6px;
}

.back-to-tree-btn {
  border: 1px solid var(--color-border, #e3e6eb);
  background: var(--color-surface, #ffffff);
  padding: 2px 8px;
  border-radius: 4px;
  font-size: 0.75rem;
  color: var(--color-primary, #2563eb);
  cursor: pointer;
  transition: all 0.15s ease;
}

.back-to-tree-btn:hover {
  background: var(--color-primary-soft, #e8effd);
}

.files-pane-wrapper {
  flex: 1;
  min-height: 0;
  height: 100%;
  position: relative;
  overflow: hidden;
}

.activity-title {
  margin: 0;
  font-size: 0.95rem;
  font-weight: 600;
  display: flex;
  align-items: center;
  gap: 0.45rem;
}

.activity-count {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 1.3rem;
  height: 1.3rem;
  padding: 0 0.35rem;
  border-radius: 999px;
  background: var(--color-border);
  font-size: 0.75rem;
  font-weight: 600;
  line-height: 1;
  color: var(--color-text);
}

.activity-scroll {
  flex: 1;
  min-height: 0;
  /* Thinking/tool cards wrap their text (overflow-wrap below), so only the
     vertical axis ever scrolls here — the pane never grows a horizontal
     scrollbar. */
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

/* Desktop-style composer: one rounded box holds the input and its toolbar,
   like the opencode desktop client. */
.composer-box {
  position: relative;
  border: 1px solid var(--color-border-strong);
  border-radius: 8px;
  background: var(--color-surface);
  padding: 0.45rem 0.65rem 0.4rem;
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
  box-shadow: 0 1px 2px rgba(15, 23, 42, 0.05);
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

.composer-hint {
  color: var(--color-text-muted);
  font-size: 0.72rem;
  white-space: nowrap;
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
