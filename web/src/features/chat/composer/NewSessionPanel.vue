<script setup lang="ts">
// New session form for the right pane of the shell, laid out like the opencode
// new-session view: the project and the agent backend each occupy their own row
// (the backend as a segmented switch), a centered composer holds the first
// message, and its toolbar carries the model picker and the create action. A
// session names only an agent backend: the server reuses
// or starts the project's instance for that backend. The title is never
// entered here — the server derives it from the first message — so an
// optional message rides along with the create request and the waiting is
// visible on this form instead of a frozen composer. On success the shell
// navigates to the new session.
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import { useAgentsStore } from '../../../stores/agents'
import { useProjectsStore } from '../../../stores/projects'
import { useSessionsStore } from '../../../stores/sessions'
import { useWorktreesStore } from '../../../stores/worktrees'
import { usePromptsStore } from '../../../stores/prompts'
import { useDraftsStore } from '../../../stores/drafts'
import { useGroupsStore } from '../../../stores/groups'
import { buildPromptMessage, promptKindLabel } from '../../../shared/utils/promptBlock'
import { getSessionHistory, type BackendId, type ImageAttachment } from '../../../api/sessions'
import { UiSelectMenu, type SelectMenuItem } from '../../../shared/ui'
import ModelSelect from './ModelSelect.vue'
import AgentSwitch from './AgentSwitch.vue'
import ComposerDropdown from './dropdown/ComposerDropdown.vue'
import { useComposerDropdown } from './dropdown/useComposerDropdown'
import { ProviderSlashSource } from './dropdown/sources/ProviderSlashSource'
import { WorkspaceFileSource } from './dropdown/sources/WorkspaceFileSource'
import { SessionMentionSource } from './dropdown/sources/SessionMentionSource'
import { resolveSessionMentions } from './sessionMentionResolver'
import ComposerImageTray from './ComposerImageTray.vue'
import ComposerCommentCards from './ComposerCommentCards.vue'
import WorkspacePreviewPane, { type OpenFileTab } from '../pane/WorkspacePreviewPane.vue'
import { useActivityPane } from '../pane/useActivityPane'
import { buildCodeCommentsMessage } from '../../../shared/utils/codeCommentBlock'
import type { CodeCommentCard } from '../../files/types'
import ImagePreviewModal from '../rendering/ImagePreviewModal.vue'
import { useAttachmentUploader } from './useAttachmentUploader'
import { useComposerKeyboard } from './useComposerKeyboard'

const props = defineProps<{ projectId?: number }>()

const emit = defineEmits<{
  created: [sessionId: string, projectId?: number]
  manage: [kind: 'register' | 'create', project: null]
}>()

const agents = useAgentsStore()
const projectsStore = useProjectsStore()
const sessions = useSessionsStore()
const worktreesStore = useWorktreesStore()
const promptsStore = usePromptsStore()
const draftsStore = useDraftsStore()
const groupsStore = useGroupsStore()
const router = useRouter()
const route = useRoute()

const resolveRouteProjectId = (): number | null => {
  if (props.projectId !== undefined && props.projectId !== null) {
    return Number(props.projectId)
  }
  const param = route?.params?.projectId
  if (param !== undefined && param !== null && param !== '') {
    const num = Number(param)
    return isNaN(num) ? null : num
  }
  return null
}

const initialProjectId =
  resolveRouteProjectId() ??
  (draftsStore.newSessionDraft.projectId !== null && draftsStore.newSessionDraft.projectId !== undefined
    ? Number(draftsStore.newSessionDraft.projectId)
    : null)

const initialDraft = draftsStore.getNewSessionDraft(initialProjectId)

let isRestoringDraft = false

const sessionMode = ref<'agent' | 'collab'>('agent')
const selectedGroupId = ref<number | null>(null)

const groupOptions = computed<SelectMenuItem[]>(() => {
  return (groupsStore.groups ?? []).map((g) => ({
    value: g.id,
    label: `${g.name} (${g.orchestratorModel || 'Default'})`,
    description: g.description || undefined,
  }))
})

const selectedGroup = computed(() => {
  if (!selectedGroupId.value) return null
  return groupsStore.groupById(selectedGroupId.value) ?? null
})

const selectedPromptId = ref<number | ''>(initialDraft.promptId ?? '')
// Both prompt kinds are valid here: a role prompt becomes a <Role> block and a
// flow prompt a <Flow> block, with the user's own message after it.
const selectedPrompt = computed(() => {
  if (selectedPromptId.value === '') return null
  return promptsStore.byId(Number(selectedPromptId.value)) ?? null
})

const {
  sendShortcut,
  sendMenuOpen,
  sendShortcutHint,
  toggleSendMenu,
  setShortcut: chooseSendShortcut,
  handleKeydown: onComposerKeydown,
} = useComposerKeyboard(() => void createSession())

const composerHint = computed(
  () => `输入 / 使用命令，@ 引用文件或会话 · ${sendShortcutHint.value}`,
)

const textareaPlaceholder = computed(() => {
  if (sessionMode.value === 'collab') {
    return `输入协同任务目标与需求描述，将自动交由中控调度大脑分解并分派各专家协同推进… (${sendShortcutHint.value})`
  }
  if (selectedPrompt.value) {
    return `输入的内容将追加在所选提示词之后发送给 Agent… (可选；${composerHint.value})`
  }
  return `描述会话，例如：帮我检查构建失败的原因… (${composerHint.value})`
})

const selectedProjectId = ref<number | null>(initialProjectId)
const selectedWorktreeId = ref<number>(initialDraft.worktreeId ?? 0)

const selectedProject = computed(() =>
  selectedProjectId.value === null ? undefined : projectsStore.byId(Number(selectedProjectId.value)),
)

const projectIsGit = computed(() => selectedProject.value?.isGit === true)

// Code Comment Cards in composer
const commentCards = ref<CodeCommentCard[]>(
  initialDraft.commentCards ? [...initialDraft.commentCards] : [],
)

watch(
  commentCards,
  (cards) => {
    if (isRestoringDraft) return
    draftsStore.setNewSessionDraft({ commentCards: cards }, selectedProjectId.value)
  },
  { deep: true },
)

function onAddCodeComment(card: CodeCommentCard) {
  commentCards.value.push(card)
}

function onRemoveCodeComment(id: string) {
  commentCards.value = commentCards.value.filter((c) => c.id !== id)
}

function onLocateCodeComment(card: CodeCommentCard) {
  openFileInPreview(card.filePath, { initialLine: card.startLine })
}

// Workspace Preview Pane (FileTree & Previews) for New Session
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
  options?: { mode?: 'view' | 'diff'; commitRef?: string; commitMessage?: string; initialLine?: number },
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
  const reqLine = options?.initialLine

  const existing = openFileTabs.value.find((f) => f.path === cleanPath)
  if (existing) {
    existing.mode = reqMode
    existing.commitRef = reqCommit
    existing.commitMessage = reqMsg
    existing.initialLine = reqLine
  } else {
    openFileTabs.value.push({
      path: cleanPath,
      name: getFileName(cleanPath),
      mode: reqMode,
      commitRef: reqCommit,
      commitMessage: reqMsg,
      initialLine: reqLine,
    })
  }
  activePreviewTab.value = cleanPath
  setPreviewCollapsed(false)
}

function closeFileTab(path: string, event?: MouseEvent) {
  if (event) event.stopPropagation()
  const idx = openFileTabs.value.findIndex((f) => f.path === path)
  if (idx === -1) return
  openFileTabs.value.splice(idx, 1)
  if (activePreviewTab.value === path) {
    if (openFileTabs.value.length > 0) {
      const nextIdx = Math.max(0, idx - 1)
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

const {
  previewCollapsed,
  setPreviewCollapsed,
  previewWidth,
  setPreviewWidth,
} = useActivityPane()

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
  [selectedProjectId, selectedWorktreeId],
  () => {
    openFileTabs.value = []
    activePreviewTab.value = FILE_TREE_TAB_KEY
  },
)

const projectWorktrees = computed(() =>
  selectedProjectId.value === null ? [] : worktreesStore.forProject(Number(selectedProjectId.value)),
)

const worktreeOptions = computed<SelectMenuItem<number>[]>(() => {
  const project = selectedProject.value
  const options: SelectMenuItem<number>[] = [
    {
      value: 0,
      label: '主工作区',
      sublabel: project?.path,
    },
  ]
  for (const worktree of projectWorktrees.value) {
    options.push({
      value: worktree.id,
      label: worktree.branch,
      sublabel: worktree.available ? worktree.path : `${worktree.path}（路径缺失）`,
      disabled: !worktree.available,
    })
  }
  return options
})

// A restored draft may name a worktree that was removed or belongs to a
// non-git project: fall back to the main checkout rather than sending an id
// the server must reject.
function clampWorktreeSelection() {
  if (!projectIsGit.value || selectedWorktreeId.value === 0) {
    selectedWorktreeId.value = 0
    return
  }
  const exists = projectWorktrees.value.some(
    (entry) => entry.id === selectedWorktreeId.value && entry.available,
  )
  if (!exists) selectedWorktreeId.value = 0
}

watch(selectedProjectId, (id) => {
  if (id === null) {
    selectedWorktreeId.value = 0
    return
  }
  if (projectsStore.byId(Number(id))?.isGit) {
    void worktreesStore.load(Number(id)).then(clampWorktreeSelection)
  } else {
    selectedWorktreeId.value = 0
  }
})

watch(projectWorktrees, clampWorktreeSelection)

watch(projectIsGit, (isGit) => {
  if (!isGit) {
    selectedWorktreeId.value = 0
    return
  }
  if (selectedProjectId.value !== null) {
    void worktreesStore.load(Number(selectedProjectId.value)).then(clampWorktreeSelection)
  }
})

const projectOptions = computed<SelectMenuItem<number | null>[]>(() => {
  return (projectsStore.projects ?? []).map((p) => ({
    value: p.id,
    label: p.name,
    sublabel: p.path,
  }))
})

const promptOptions = computed<SelectMenuItem<number | ''>[]>(() => {
  const items: SelectMenuItem<number | ''>[] = [
    { value: '', label: '不使用提示词（默认）' },
  ]
  for (const p of promptsStore.prompts ?? []) {
    items.push({
      value: p.id,
      label: p.name,
      sublabel: p.description || undefined,
      tag: promptKindLabel(p.type),
    })
  }
  return items
})

// Drops a restored draft selection whose prompt was deleted. A list that has
// not loaded yet is left alone so a valid draft is never cleared prematurely.
function clearMissingPromptSelection() {
  if (selectedPromptId.value === '') return
  const list = promptsStore.prompts
  if (!Array.isArray(list) || list.length === 0) return
  if (list.some((p) => p.id === Number(selectedPromptId.value))) return
  selectedPromptId.value = ''
  draftsStore.setNewSessionDraft({ promptId: '' }, selectedProjectId.value)
}

watch(
  () => promptsStore.prompts,
  () => clearMissingPromptSelection(),
)

// A kept-alive panel keeps its watchers while another page is shown; it must
// not follow route changes made for that page.
const isNewSessionRouteActive = () =>
  route?.name === 'session-new' || route?.name === 'session-new-global'

watch(
  () => [props.projectId, route?.params?.projectId],
  ([propId, routeId], oldVal) => {
    if (!isNewSessionRouteActive()) return
    if (!oldVal) {
      const candidate = propId ?? routeId
      if (candidate !== undefined && candidate !== null && candidate !== '') {
        const num = Number(candidate)
        if (!isNaN(num)) {
          selectedProjectId.value = num
        }
      }
      return
    }
    const [oldPropId, oldRouteId] = oldVal
    if (propId !== oldPropId && propId !== undefined && propId !== null) {
      const num = Number(propId)
      if (!isNaN(num)) selectedProjectId.value = num
    } else if (routeId !== oldRouteId && routeId !== undefined && routeId !== null && routeId !== '') {
      const num = Number(routeId)
      if (!isNaN(num)) selectedProjectId.value = num
    }
  },
  { immediate: true },
)

watch(
  () => projectsStore.projects,
  (list) => {
    if (selectedProjectId.value === null && Array.isArray(list) && list.length > 0) {
      selectedProjectId.value = list[0].id
    }
  },
)

const selectedBackend = ref<BackendId | ''>(initialDraft.backend ?? '')
const messageText = ref(initialDraft.messageText ?? '')
const selectedModel = ref(initialDraft.model ?? '')
const modelOk = ref(false)
const modelFailed = ref(false)
const creating = ref(false)
const composerBox = ref<HTMLElement | null>(null)
const textareaRef = ref<HTMLTextAreaElement | null>(null)
const toolNotice = ref('')

watch(selectedProjectId, (newId, oldId) => {
  if (oldId && oldId !== newId) {
    draftsStore.setNewSessionDraft(
      {
        projectId: oldId,
        backend: selectedBackend.value,
        model: selectedModel.value,
        promptId: selectedPromptId.value,
        messageText: messageText.value,
        attachedImages: attachedImages.value,
        worktreeId: selectedWorktreeId.value,
        commentCards: commentCards.value,
      },
      oldId,
    )
  }

  if (newId) {
    isRestoringDraft = true
    const targetDraft = draftsStore.getNewSessionDraft(newId)
    messageText.value = targetDraft.messageText || ''
    commentCards.value = Array.isArray(targetDraft.commentCards) ? [...targetDraft.commentCards] : []
    // Drafts carry the backend explicitly; the project entity itself is
    // backend-agnostic, so fall straight to the first selectable backend.
    const preferredBackend = targetDraft.backend || firstSelectableBackend()
    selectedBackend.value = selectableBackends.value.some(
      (entry) => entry.backend === preferredBackend,
    )
      ? preferredBackend
      : firstSelectableBackend()
    selectedModel.value = targetDraft.model || ''
    selectedPromptId.value = targetDraft.promptId || ''
    selectedWorktreeId.value = targetDraft.worktreeId ?? 0
    if (projectsStore.byId(Number(newId))?.isGit) {
      void worktreesStore.load(Number(newId)).then(clampWorktreeSelection)
    } else {
      selectedWorktreeId.value = 0
    }
    clearMissingPromptSelection()
    attachedImages.value = Array.isArray(targetDraft.attachedImages) ? [...targetDraft.attachedImages] : []
    void nextTick(() => {
      isRestoringDraft = false
    })
  }

  if (newId && Number(newId) !== Number(props.projectId) && isNewSessionRouteActive()) {
    if (router && typeof router.replace === 'function') {
      void router.replace({
        name: 'session-new',
        params: { projectId: String(newId) },
      })
    }
  }
})

watch(messageText, (val) => {
  if (isRestoringDraft) return
  draftsStore.setNewSessionDraft({ messageText: val }, selectedProjectId.value)
})

watch(selectedBackend, (val) => {
  if (isRestoringDraft) return
  draftsStore.setNewSessionDraft({ backend: val }, selectedProjectId.value)
})

watch(selectedModel, (val) => {
  if (isRestoringDraft) return
  draftsStore.setNewSessionDraft({ model: val }, selectedProjectId.value)
})

watch(selectedPromptId, (val) => {
  if (isRestoringDraft) return
  draftsStore.setNewSessionDraft({ promptId: val }, selectedProjectId.value)
})

watch(selectedWorktreeId, (val) => {
  if (isRestoringDraft) return
  draftsStore.setNewSessionDraft({ worktreeId: val }, selectedProjectId.value)
})

const selectedCapabilities = computed(() =>
  selectedBackend.value === '' ? null : agents.capabilitiesFor(selectedBackend.value),
)

const selectedEnabled = computed(
  () => selectedBackend.value !== '' && agents.backendEnabledFor(selectedBackend.value),
)

// Dropdown autocomplete for slash commands, workspace files (@) and session mentions (@)
const dropdown = useComposerDropdown({
  sources: [
    new ProviderSlashSource(
      () => selectedProjectId.value ?? 0,
      () => '',
      () => selectedBackend.value || '',
      () => selectedWorktreeId.value,
    ),
    new WorkspaceFileSource(
      () => selectedProjectId.value ?? 0,
      () => selectedWorktreeId.value,
    ),
    // The panel has no open session yet, so mentions resolve against the
    // project's previously loaded session index.
    new SessionMentionSource(
      () => (selectedProjectId.value ? sessions.sessionsByProject[selectedProjectId.value] ?? [] : []),
      () => '',
    ),
  ],
  text: messageText,
  onApplyText: (newText, newCursor) => {
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
  onComposerKeydown(event, dropdown.onKeydown(event))
}

// Image and file attachment support
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
  initialImages: initialDraft.attachedImages,
  onNotice: (msg) => {
    toolNotice.value = msg
  },
})

watch(
  attachedImages,
  (val) => {
    if (isRestoringDraft) return
    draftsStore.setNewSessionDraft({ attachedImages: val }, selectedProjectId.value)
  },
  { deep: true },
)

function onFileInput(event: Event) {
  handleFileInput(event, (snippet) => {
    const prefix = messageText.value.trim() ? '\n\n' : ''
    messageText.value += `${prefix}${snippet}`
  })
}

function onDrop(event: DragEvent) {
  handleDrop(event, (snippet) => {
    const prefix = messageText.value.trim() ? '\n\n' : ''
    messageText.value += `${prefix}${snippet}`
  })
}

const hasContent = computed(
  () =>
    messageText.value.trim() !== '' ||
    attachedImages.value.length > 0 ||
    selectedPrompt.value !== null ||
    commentCards.value.length > 0,
)

// A first message that needs a model (the backend supports choosing one) waits
// for the model list, which is always available: a backend's catalog is scoped to
// the project, not to a running instance, so creating the very first session of a
// project can still name a model. Until the list arrives the create is held back
// rather than silently sending on the backend's default model.
const firstMessageReady = computed(() => {
  if (!hasContent.value) return true
  if (!selectedCapabilities.value?.modelSelection) return true
  if (modelOk.value && selectedModel.value !== '') return true
  if (modelFailed.value) return true
  return false
})

// Only platform-enabled backends may be chosen for a new session. The server also
// enforces this, and disabled backends are not offered in the switch at all.
const selectableBackends = computed(() => agents.backends.filter((entry) => entry.enabled !== false))

const sortedBackends = computed(() =>
  [...selectableBackends.value].sort((a, b) => a.order - b.order),
)

function firstSelectableBackend(): BackendId | '' {
  return selectableBackends.value[0]?.backend ?? ''
}

// A restored draft may name a backend the operator has since disabled: fall
// back to the first selectable one rather than showing an unpressable choice.
watch(selectableBackends, (list) => {
  if (selectedBackend.value === '') return
  if (!list.some((entry) => entry.backend === selectedBackend.value)) {
    selectedBackend.value = list[0]?.backend ?? ''
    selectedModel.value = ''
    modelOk.value = false
  }
})

const canCreate = computed(() => {
  if (selectedProjectId.value === null || creating.value) return false
  if (sessionMode.value === 'collab') {
    return selectedGroupId.value !== null && hasContent.value
  }
  return (
    selectedBackend.value !== '' &&
    selectedEnabled.value &&
    firstMessageReady.value
  )
})

const createLabel = computed(() => {
  if (creating.value) return sessionMode.value === 'collab' ? '启动中…' : '创建中…'
  if (sessionMode.value === 'collab') return '创建并启动协同'
  return hasContent.value ? '创建并开始对话' : '新建会话'
})

onMounted(async () => {
  void promptsStore.loadPrompts().then(() => {
    // The prompt list arrives asynchronously; a draft naming a prompt since
    // deleted can only be recognised here.
    clearMissingPromptSelection()
  })
  void groupsStore.loadGroups().then(() => {
    if (!selectedGroupId.value && (groupsStore.groups ?? []).length > 0) {
      selectedGroupId.value = groupsStore.groups[0].id
    }
  }).catch(() => {})
  await agents.loadBackends()
  if (Array.isArray(projectsStore.projects) && projectsStore.projects.length === 0) {
    await projectsStore.load()
  }
  if (!selectedProjectId.value && (projectsStore.projects ?? []).length > 0) {
    selectedProjectId.value = projectsStore.projects[0].id
  }
  if (
    selectedBackend.value === '' ||
    !selectableBackends.value.some((entry) => entry.backend === selectedBackend.value)
  ) {
    selectedBackend.value = firstSelectableBackend()
  }
  textareaRef.value?.focus()
})

function selectBackend(backend: BackendId) {
  if (!agents.backendEnabledFor(backend)) return
  if (selectedBackend.value === backend) return
  selectedBackend.value = backend
  selectedModel.value = ''
  modelOk.value = false
}

// The panel can stay mounted after a session is created (kept alive), so the
// composer's own state is cleared too, not only the stored draft.
function resetComposer() {
  isRestoringDraft = true
  messageText.value = ''
  attachedImages.value = []
  commentCards.value = []
  selectedPromptId.value = ''
  void nextTick(() => {
    isRestoringDraft = false
  })
}

async function createSession() {
  const targetProjectId = selectedProjectId.value
  if (!targetProjectId) {
    toolNotice.value = '请先选择关联的项目'
    return
  }

  if (sessionMode.value === 'collab') {
    if (!selectedGroupId.value) {
      toolNotice.value = '请先选择协同团队'
      return
    }
    const rawText = messageText.value.trim()
    if (!rawText) {
      toolNotice.value = '请输入协同任务目标'
      return
    }
    creating.value = true
    toolNotice.value = ''
    try {
      const textToSend = await resolveSessionMentions(targetProjectId, rawText, (targetId, options) =>
        getSessionHistory(targetProjectId, targetId, options),
      )
      const textWithCards = buildCodeCommentsMessage(commentCards.value, textToSend)
      const finalPrompt = selectedPrompt.value
        ? buildPromptMessage(selectedPrompt.value, textWithCards)
        : textWithCards

      const res = await groupsStore.createCollabSession(selectedGroupId.value, {
        projectId: targetProjectId,
        worktreeId: projectIsGit.value && selectedWorktreeId.value > 0 ? selectedWorktreeId.value : 0,
        prompt: finalPrompt,
      })
      creating.value = false
      if (res && res.session) {
        const sessionId = res.session.id || (res.session as any).ID
        draftsStore.clearNewSessionDraft(targetProjectId)
        resetComposer()
        emit('created', sessionId, targetProjectId)
      }
    } catch (err: any) {
      creating.value = false
      toolNotice.value = err?.message || '创建协同会话失败'
    }
    return
  }

  if (selectedBackend.value === '' || creating.value || !selectedEnabled.value) return
  if (!firstMessageReady.value) return
  creating.value = true
  toolNotice.value = ''
  const rawText = messageText.value.trim()
  if (rawText.startsWith('/')) {
    const spaceIdx = rawText.indexOf(' ')
    const cmdName = (spaceIdx === -1 ? rawText.slice(1) : rawText.slice(1, spaceIdx)).toLowerCase()
    if (cmdName === 'compact') {
      toolNotice.value = '会话尚未创建，无法执行上下文压缩命令'
      creating.value = false
      return
    }
  }
  const textToSend =
    rawText !== ''
      ? await resolveSessionMentions(targetProjectId, rawText, (targetId, options) =>
          getSessionHistory(targetProjectId, targetId, options),
        )
      : ''
  const textWithCards = buildCodeCommentsMessage(commentCards.value, textToSend)
  const finalMessage = selectedPrompt.value
    ? buildPromptMessage(selectedPrompt.value, textWithCards)
    : textWithCards
  // The model is part of the session's identity, not only of its first turn, so
  // it rides the create request even when no message is sent: the session is
  // created on the model the user saw, and the conversation opens on it.
  const withModel =
    selectedCapabilities.value?.modelSelection === true &&
    modelOk.value &&
    selectedModel.value !== ''
  const created = await sessions.createSession(
    targetProjectId,
    {
      backend: selectedBackend.value,
      ...(projectIsGit.value && selectedWorktreeId.value > 0
        ? { worktreeId: selectedWorktreeId.value }
        : {}),
      ...(finalMessage !== '' ? { message: finalMessage } : {}),
      ...(withModel ? { model: selectedModel.value } : {}),
      ...(attachedImages.value.length > 0 ? { images: attachedImages.value } : {}),
    },
    // A failed first message re-opens the composer with the user's own text
    // only, never the prompt block that rides in front of it.
    { userMessage: textToSend },
  )
  creating.value = false
  if (created) {
    draftsStore.clearNewSessionDraft(targetProjectId)
    resetComposer()
    emit('created', created.id, targetProjectId)
  }
}
</script>

<template>
  <div class="new-session-layout" data-test="new-session-layout">
    <div class="new-session-main">
      <section class="new-session" aria-label="新建会话">
    <div class="new-session-inner">
      <div class="new-session-header flex items-center justify-between mb-4">
        <div>
          <h1>新建会话</h1>
          <p class="subtitle">选择会话模式与后端，描述你要做的事</p>
        </div>
        <div class="session-mode-switch-group">
          <div class="inline-flex p-0.5 rounded-lg bg-[var(--color-surface-muted)] border border-[var(--color-border)] select-none">
            <button
              type="button"
              class="px-3 py-1 text-xs font-medium rounded transition-colors"
              :class="sessionMode === 'agent' ? 'bg-[var(--color-surface)] text-[var(--color-text)] shadow-sm' : 'text-[var(--color-text-muted)] hover:text-[var(--color-text)]'"
              data-test="mode-agent"
              @click="sessionMode = 'agent'"
            >
              单 Agent
            </button>
            <button
              type="button"
              class="px-3 py-1 text-xs font-medium rounded transition-colors flex items-center gap-1.5"
              :class="sessionMode === 'collab' ? 'bg-[var(--color-surface)] text-[var(--color-text)] shadow-sm' : 'text-[var(--color-text-muted)] hover:text-[var(--color-text)]'"
              data-test="mode-collab"
              @click="sessionMode = 'collab'"
            >
              <svg viewBox="0 0 16 16" width="12" height="12" fill="none" stroke="currentColor" stroke-width="1.8">
                <circle cx="8" cy="8" r="3" />
                <path d="M8 1v2M8 13v2M1 8h2M13 8h2" />
              </svg>
              <span>团队协同</span>
            </button>
          </div>
        </div>
      </div>

      <form class="create-session" @submit.prevent="createSession">
        <div class="project-selector-group">
          <label for="project-select" class="project-label">关联项目</label>
          <UiSelectMenu
            id="project-select"
            v-model="selectedProjectId"
            class="project-select"
            data-test="project-select"
            :options="projectOptions"
            placeholder="请选择项目…"
            block
          />
          <div
            v-if="Array.isArray(projectsStore.projects) && projectsStore.projects.length === 0"
            class="no-projects-hint"
            data-test="no-projects-hint"
          >
            <span>暂无可用项目：</span>
            <button
              type="button"
              class="btn-proj-link"
              data-test="btn-create-project"
              @click="emit('manage', 'create', null)"
            >
              新建项目
            </button>
          </div>
        </div>

        <div v-if="projectIsGit" class="worktree-selector-group" data-test="worktree-selector">
          <div class="worktree-selector-head">
            <label for="worktree-select" class="worktree-label">工作树</label>
            <RouterLink
              v-if="projectWorktrees.length === 0 && !worktreesStore.isLoading(Number(selectedProjectId))"
              class="worktree-settings-link"
              :to="{ name: 'project-settings', params: { projectId: String(selectedProjectId) } }"
            >
              前往项目设置创建工作树
            </RouterLink>
          </div>
          <UiSelectMenu
            id="worktree-select"
            v-model="selectedWorktreeId"
            class="worktree-select"
            data-test="worktree-select"
            :options="worktreeOptions"
            placeholder="主工作区"
            block
          />
        </div>

        <div class="agent-selector-group">
          <template v-if="sessionMode === 'agent'">
            <span class="agent-label">Agent</span>
            <AgentSwitch v-model="selectedBackend" :backends="sortedBackends" :loaded="agents.backendsLoaded" />
          </template>
          <template v-else>
            <span class="agent-label">协同团队</span>
            <div class="flex-1">
              <UiSelectMenu
                id="group-select"
                v-model="selectedGroupId"
                class="group-select"
                data-test="group-select"
                :options="groupOptions"
                placeholder="请选择协同团队…"
                block
              />
              <div v-if="(groupsStore.groups ?? []).length === 0 && !groupsStore.loading" class="text-xs text-[var(--color-text-muted)] mt-1">
                暂无协同团队，可前往 <RouterLink class="text-accent underline" to="/groups">团队中心</RouterLink> 创建。
              </div>
              <div v-else-if="selectedGroup" class="mt-2 p-2.5 rounded bg-[var(--color-surface-muted)] border border-[var(--color-border)] text-xs flex items-center justify-between">
                <div class="flex items-center gap-2">
                  <span class="w-1.5 h-1.5 rounded-full bg-accent" />
                  <span class="font-medium text-[var(--color-text)]">{{ selectedGroup.name }}</span>
                  <span class="text-[var(--color-text-muted)] text-2xs font-mono">中枢大脑: {{ selectedGroup.orchestratorModel }}</span>
                </div>
                <div class="text-[var(--color-text-muted)] text-2xs font-mono tabular-nums">
                  {{ selectedGroup.memberCount }} 位协同专家 · 自动执行
                </div>
              </div>
            </div>
          </template>
        </div>

        <div class="prompt-selector-group">
          <div class="prompt-selector-header">
            <label for="prompt-select" class="prompt-label">提示词</label>
          </div>
          <div class="prompt-select-row">
            <UiSelectMenu
              id="prompt-select"
              v-model="selectedPromptId"
              class="prompt-select"
              data-test="prompt-select"
              :options="promptOptions"
              placeholder="不使用提示词（默认）"
              clearable
              block
            />
            <button
              v-if="selectedPromptId !== ''"
              type="button"
              class="btn-clear-prompt"
              title="清除选择"
              data-test="btn-clear-prompt"
              @click="selectedPromptId = ''"
            >
              <svg viewBox="0 0 16 16" width="12" height="12" fill="none" stroke="currentColor" stroke-width="1.8">
                <line x1="3" y1="3" x2="13" y2="13" />
                <line x1="13" y1="3" x2="3" y2="13" />
              </svg>
            </button>
          </div>
          <div v-if="selectedPrompt" class="prompt-preview-snippet" data-test="prompt-preview-snippet">
            <div class="snippet-title">提示词预览：</div>
            <pre class="snippet-text">{{ selectedPrompt.content }}</pre>
          </div>
        </div>

        <div
          ref="composerBox"
          class="composer-box"
          @dragover.prevent="onDragOver"
          @drop.prevent="onDrop"
        >
          <ComposerDropdown
            v-if="dropdown.isOpen.value"
            :items="dropdown.items.value"
            :selected-index="dropdown.selectedIndex.value"
            :loading="dropdown.loading.value"
            @select="dropdown.selectItem"
          />
          <ComposerCommentCards
            :cards="commentCards"
            @remove="onRemoveCodeComment"
            @locate="onLocateCodeComment"
          />
          <label class="composer-label" for="first-message">会话描述</label>
          <textarea
            id="first-message"
            ref="textareaRef"
            v-model="messageText"
            rows="5"
            :placeholder="textareaPlaceholder"
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
                v-if="sessionMode === 'agent' && selectedCapabilities?.modelSelection && selectedBackend !== ''"
                v-model="selectedModel"
                :key="selectedBackend"
                :backend="selectedBackend"
                :model-selection="true"
                @status="modelOk = $event"
                @failed="modelFailed = $event"
              />
            </div>

            <div class="composer-actions">
              <div class="send-split-group" data-test="send-split-group">
                <button
                  type="submit"
                  class="composer-action send-main-btn send-button create"
                  :disabled="!canCreate"
                  :title="canCreate ? (sendShortcut === 'enter' ? '发送 (Enter)' : '发送 (Ctrl+Enter)') : '请先选择项目与 Agent 并输入内容'"
                  data-test="composer-send-btn"
                >
                  {{ creating ? '创建中…' : '发送' }}
                </button>
                <button
                  type="button"
                  class="send-dropdown-trigger send-trigger"
                  :class="{ 'is-open': sendMenuOpen }"
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
                    :class="{ active: sendShortcut === 'enter' }"
                    data-test="send-shortcut-enter"
                    @click="chooseSendShortcut('enter')"
                  >
                    <span class="menu-check">
                      <svg v-if="sendShortcut === 'enter'" viewBox="0 0 16 16" width="10" height="10" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
                        <polyline points="3 8.5 6.5 12 13 4.5" />
                      </svg>
                    </span>
                    <span class="menu-label">按 Enter 发送</span>
                    <span class="menu-hint">Shift+Enter 换行</span>
                  </button>
                  <button
                    type="button"
                    class="send-menu-item"
                    :class="{ active: sendShortcut === 'ctrl-enter' }"
                    data-test="send-shortcut-ctrl-enter"
                    @click="chooseSendShortcut('ctrl-enter')"
                  >
                    <span class="menu-check">
                      <svg v-if="sendShortcut === 'ctrl-enter'" viewBox="0 0 16 16" width="10" height="10" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
                        <polyline points="3 8.5 6.5 12 13 4.5" />
                      </svg>
                    </span>
                    <span class="menu-label">按 Ctrl+Enter 发送</span>
                    <span class="menu-hint">Enter 换行</span>
                  </button>
                </div>
              </div>
            </div>
          </div>
        </div>
        <p v-if="toolNotice" class="notice-bar" role="status">{{ toolNotice }}</p>
        <p v-if="hasContent && !firstMessageReady" class="hint" data-test="model-required-hint">
          模型列表尚未就绪，暂时无法发送首条消息。
        </p>

        <!-- A refused session usually means the machine-wide service is not running.
             The platform will not start it for you, so the refusal says where to. -->
        <p v-if="sessions.error" class="page-error" role="alert" data-test="new-session-error">
          {{ sessions.error }}
          <RouterLink
            v-if="sessions.errorCode === 'service_not_running'"
            :to="{ name: 'engines-agents' }"
            data-test="new-session-service-link"
            >前往引擎中心 → Agent配置</RouterLink
          >
        </p>
      </form>
    </div>

    <ImagePreviewModal
      v-if="previewModalImage"
      :image="previewModalImage"
      :visible="true"
      @close="previewModalImage = null"
    />
      </section>
    </div>

    <!-- Column 2: Unified Right Workspace Panel (FileTree and Previews as Tabs) -->
    <WorkspacePreviewPane
      v-if="selectedProjectId !== null"
      :project-id="Number(selectedProjectId)"
      :worktree-id="selectedWorktreeId"
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
      @open-file-preview="openFileInPreview"
      @add-comment="onAddCodeComment"
    />
  </div>
</template>

<style scoped>
.new-session-layout {
  display: flex;
  flex-direction: row;
  height: 100%;
  width: 100%;
  overflow: hidden;
  position: relative;
}

.new-session-main {
  flex: 1 1 0%;
  min-width: 0;
  height: 100%;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
}

.new-session {
  display: flex;
  justify-content: center;
  padding: 3rem 1rem;
  min-height: 100%;
  box-sizing: border-box;
}

.new-session-inner {
  width: min(48rem, 100%);
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.new-session h1 {
  font-size: 1.25rem;
  font-weight: 600;
  letter-spacing: -0.02em;
  margin: 0;
  text-align: left;
}

.subtitle {
  margin: 0.25rem 0 0.75rem;
  text-align: left;
  color: var(--color-text-muted);
  font-size: 0.75rem;
}

.project-selector-group {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  margin-bottom: 0.5rem;
  padding: 0.5rem 0.75rem;
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: 6px;
}

.project-label {
  font-size: 0.75rem;
  font-weight: 500;
  color: var(--color-text-muted);
  white-space: nowrap;
}

.project-select {
  flex: 1;
  min-width: 0;
  padding: 0.35rem 0.6rem;
  font: inherit;
  font-size: 0.88rem;
  color: var(--color-text);
  background: var(--color-surface-muted);
  border: 1px solid var(--color-border-strong);
  border-radius: 6px;
  outline: none;
  cursor: pointer;
}

.project-select:focus {
  border-color: var(--color-primary);
}

.worktree-selector-group {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
  margin-bottom: 0.5rem;
  padding: 0.5rem 0.75rem;
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: 6px;
}

.worktree-selector-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem;
}

.worktree-label {
  font-size: 0.75rem;
  font-weight: 500;
  color: var(--color-text-muted);
  white-space: nowrap;
}

.worktree-settings-link {
  font-size: 0.75rem;
  color: var(--color-primary);
  text-decoration: none;
  white-space: nowrap;
}

.worktree-settings-link:hover {
  text-decoration: underline;
}

.worktree-select {
  flex: 1;
  min-width: 0;
}

.no-projects-hint {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  font-size: 0.82rem;
  color: var(--color-text-muted);
  white-space: nowrap;
}

.btn-proj-link {
  background: none;
  border: none;
  color: var(--color-primary);
  cursor: pointer;
  padding: 0;
  font: inherit;
  font-size: 0.82rem;
  text-decoration: underline;
}

.btn-proj-link:hover {
  color: var(--color-primary-hover);
}

.prompt-selector-group {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
  padding: 0.5rem 0.75rem;
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: 6px;
}

.prompt-selector-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.prompt-label {
  font-size: 0.75rem;
  font-weight: 500;
  color: var(--color-text-muted);
}

.prompt-select-row {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.prompt-select {
  flex: 1;
  min-width: 0;
  padding: 0.35rem 0.6rem;
  font: inherit;
  font-size: 0.88rem;
  color: var(--color-text);
  background: var(--color-surface-muted);
  border: 1px solid var(--color-border-strong);
  border-radius: 6px;
  outline: none;
  cursor: pointer;
}

.prompt-select:focus {
  border-color: var(--color-primary);
}

.btn-clear-prompt {
  background: transparent;
  border: none;
  color: var(--color-text-muted);
  cursor: pointer;
  padding: 0.2rem 0.4rem;
  font-size: 0.85rem;
  border-radius: 4px;
}

.btn-clear-prompt:hover {
  color: var(--color-danger);
  background: var(--color-surface-muted);
}

.prompt-preview-snippet {
  margin-top: 0.25rem;
  padding: 0.4rem 0.6rem;
  background: var(--color-bg);
  border: 1px dashed var(--color-border);
  border-radius: 6px;
}

.snippet-title {
  font-size: 0.75rem;
  font-weight: 500;
  color: var(--color-text-muted);
  margin-bottom: 0.2rem;
}

.snippet-text {
  margin: 0;
  font-size: 0.8rem;
  font-family: var(--font-mono);
  line-height: 1.4;
  color: var(--color-text);
  white-space: pre-wrap;
  word-break: break-word;
  max-height: 6rem;
  overflow-y: auto;
}

.create-session {
  display: flex;
  flex-direction: column;
  gap: 0.6rem;
}

.agent-selector-group {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
  padding: 0.5rem 0.75rem;
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: 6px;
}

.agent-label {
  font-size: 0.75rem;
  font-weight: 500;
  color: var(--color-text-muted);
}

/* Desktop-style composer: one rounded box holds the input and its toolbar,
   like the opencode new-session view and the conversation composer. */
.composer-box {
  position: relative;
  border: 1px solid var(--color-border-strong);
  border-radius: 4px;
  background: var(--color-surface);
  display: flex;
  flex-direction: column;
  transition: border-color 0.15s ease-out;
}

.composer-box:focus-within {
  border-color: var(--color-primary);
}

.composer-label {
  position: absolute;
  width: 1px;
  height: 1px;
  padding: 0;
  margin: -1px;
  overflow: hidden;
  clip: rect(0, 0, 0, 0);
  white-space: nowrap;
  border: 0;
}

.composer-box textarea {
  border: none;
  background: transparent;
  color: var(--color-text);
  font: inherit;
  font-size: 0.95rem;
  line-height: 1.5;
  padding: 0.75rem 0.85rem 0.35rem;
  resize: vertical;
  min-height: 5.5rem;
  outline: none;
}

.composer-box textarea::placeholder {
  color: var(--color-text-muted);
}

.composer-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem;
  padding: 0.4rem 0.6rem 0.5rem;
  border-top: 1px solid var(--color-border);
}

.composer-tools {
  display: flex;
  align-items: center;
  gap: 0.4rem;
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

.composer-tool-btn:hover:not(:disabled) {
  border-color: var(--color-primary);
  background: var(--color-surface);
}

.composer-plus-btn {
  font-size: 1.15rem;
  font-weight: 500;
  line-height: 1;
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
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
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

.send-menu-item.active {
  font-weight: 600;
  color: var(--color-primary);
}

.menu-check {
  width: 12px;
  display: inline-block;
  color: var(--color-primary);
  font-size: 0.85rem;
}

.menu-label {
  font-size: 0.82rem;
}

.menu-hint {
  font-size: var(--text-size-meta);
  color: var(--color-text-muted);
  margin-left: auto;
}

.hint {
  margin: 0;
  font-size: 0.8rem;
  color: var(--color-text-muted);
}

.notice-bar {
  margin: 0;
  padding: 0.35rem 0.75rem;
  border-radius: 6px;
  background: var(--color-warning-soft, rgba(234, 179, 8, 0.15));
  color: var(--color-warning, #ca8a04);
  font-size: 0.82rem;
}

.page-error {
  margin: 0;
  padding: 0.6rem 0.8rem;
  border-radius: 8px;
  background: var(--color-danger-soft, #fee2e2);
  color: var(--color-danger, #b91c1c);
  font-size: 0.85rem;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.5rem;
}

.page-error a {
  color: inherit;
  font-weight: 600;
  text-decoration: underline;
}
</style>
