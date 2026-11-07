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
import { usePromptsStore } from '../../../stores/prompts'
import { useDraftsStore } from '../../../stores/drafts'
import { applyUserPromptTemplate, hasUserPromptPlaceholder } from '../../prompts/promptTemplate'
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
const promptsStore = usePromptsStore()
const draftsStore = useDraftsStore()
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

const selectedPromptId = ref<number | ''>(initialDraft.promptId ?? '')
// A new project session only offers flow prompts (role prompts belong to the
// role library and the pure-chat assistants). The type check is the safety net
// against a draft that still names a role prompt from before this restriction.
const selectedPrompt = computed(() => {
  if (selectedPromptId.value === '') return null
  const prompt = promptsStore.byId(Number(selectedPromptId.value))
  return prompt && prompt.type === 'flow' ? prompt : null
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
  if (selectedPrompt.value) {
    if (hasUserPromptPlaceholder(selectedPrompt.value.content)) {
      return `输入的内容将替换模版中的 {user_prompt} 并发送给 Agent… (可选，留空则替换为空；${composerHint.value})`
    }
    return `输入的内容将追加在模版后发送给 Agent… (可选；${composerHint.value})`
  }
  return `描述会话，例如：帮我检查构建失败的原因… (${composerHint.value})`
})

const selectedProjectId = ref<number | null>(initialProjectId)

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
  for (const p of promptsStore.flowPrompts) {
    items.push({
      value: p.id,
      label: p.name,
      sublabel: p.description || undefined,
    })
  }
  return items
})

// Drops a restored draft selection that is no longer offered here. Keeping it
// would silently inject a role prompt into the first message while the picker
// no longer shows it; an unknown id (prompt list not loaded yet) is left alone
// because selectedPrompt already refuses to apply it.
function clearInvalidPromptSelection() {
  if (selectedPromptId.value === '') return
  const prompt = promptsStore.byId(Number(selectedPromptId.value))
  if (!prompt || prompt.type === 'flow') return
  selectedPromptId.value = ''
  draftsStore.setNewSessionDraft({ promptId: '' }, selectedProjectId.value)
}

watch(
  () => [props.projectId, route?.params?.projectId],
  ([propId, routeId]) => {
    const candidate = propId ?? routeId
    if (candidate !== undefined && candidate !== null && candidate !== '') {
      const num = Number(candidate)
      if (!isNaN(num)) {
        selectedProjectId.value = num
      }
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
      },
      oldId,
    )
  }

  if (newId) {
    isRestoringDraft = true
    const targetDraft = draftsStore.getNewSessionDraft(newId)
    messageText.value = targetDraft.messageText || ''
    const preferredBackend =
      targetDraft.backend ||
      projectsStore.byId(Number(newId))?.defaultBackend ||
      firstSelectableBackend()
    selectedBackend.value = selectableBackends.value.some(
      (entry) => entry.backend === preferredBackend,
    )
      ? preferredBackend
      : firstSelectableBackend()
    selectedModel.value = targetDraft.model || ''
    selectedPromptId.value = targetDraft.promptId || ''
    clearInvalidPromptSelection()
    attachedImages.value = Array.isArray(targetDraft.attachedImages) ? [...targetDraft.attachedImages] : []
    void nextTick(() => {
      isRestoringDraft = false
    })
  }

  if (newId && Number(newId) !== Number(props.projectId)) {
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
    ),
    new WorkspaceFileSource({
      projectId: () => selectedProjectId.value ?? 0,
    }),
    new SessionMentionSource({
      projectId: () => selectedProjectId.value ?? 0,
    }),
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
    selectedPrompt.value !== null,
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
// enforces this, but showing them disabled keeps the UI honest.
const selectableBackends = computed(() => agents.backends.filter((entry) => entry.enabled !== false))

const sortedBackends = computed(() =>
  [...agents.backends].sort((a, b) => a.order - b.order),
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

const canCreate = computed(
  () =>
    selectedProjectId.value !== null &&
    selectedBackend.value !== '' &&
    selectedEnabled.value &&
    firstMessageReady.value &&
    !creating.value,
)

const createLabel = computed(() => {
  if (creating.value) return '创建中…'
  return hasContent.value ? '创建并开始对话' : '新建会话'
})

onMounted(async () => {
  void promptsStore.loadPrompts().then(() => {
    // The prompt list arrives asynchronously, so a role prompt in the restored
    // draft can only be recognised and cleared here.
    clearInvalidPromptSelection()
  })
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

async function createSession() {
  const targetProjectId = selectedProjectId.value
  if (!targetProjectId) {
    toolNotice.value = '请先选择关联的项目'
    return
  }
  if (selectedBackend.value === '' || creating.value || !selectedEnabled.value) return
  if (!firstMessageReady.value) return
  creating.value = true
  toolNotice.value = ''
  const rawText = messageText.value.trim()
  const textToSend =
    rawText !== ''
      ? await resolveSessionMentions(targetProjectId, rawText, (targetId) =>
          getSessionHistory(targetProjectId, targetId),
        )
      : ''
  const finalMessage = selectedPrompt.value
    ? applyUserPromptTemplate(selectedPrompt.value.content, textToSend)
    : textToSend
  // The model is part of the session's identity, not only of its first turn, so
  // it rides the create request even when no message is sent: the session is
  // created on the model the user saw, and the conversation opens on it.
  const withModel =
    selectedCapabilities.value?.modelSelection === true &&
    modelOk.value &&
    selectedModel.value !== ''
  const created = await sessions.createSession(targetProjectId, {
    backend: selectedBackend.value,
    ...(finalMessage !== '' ? { message: finalMessage } : {}),
    ...(withModel ? { model: selectedModel.value } : {}),
    ...(attachedImages.value.length > 0 ? { images: attachedImages.value } : {}),
  })
  creating.value = false
  if (created) {
    draftsStore.clearNewSessionDraft(targetProjectId)
    emit('created', created.id, targetProjectId)
  }
}
</script>

<template>
  <section class="new-session" aria-label="新建会话">
    <div class="new-session-inner">
      <h1>新建会话</h1>
      <p class="subtitle">选择 Agent 后端，描述你要做的事</p>

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

        <div class="agent-selector-group">
          <span class="agent-label">Agent</span>
          <AgentSwitch v-model="selectedBackend" :backends="sortedBackends" />
        </div>

        <div class="prompt-selector-group">
          <div class="prompt-selector-header">
            <label for="prompt-select" class="prompt-label">提示词模板</label>
            <span
              v-if="selectedPrompt && hasUserPromptPlaceholder(selectedPrompt.content)"
              class="prompt-placeholder-hint"
            >
              输入内容将填充到模版的 {user_prompt} 处
            </span>
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
            <div class="snippet-title">模板预览：</div>
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
                v-if="selectedCapabilities?.modelSelection && selectedBackend !== ''"
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
                    <span class="menu-check">{{ sendShortcut === 'enter' ? '✓' : '' }}</span>
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
                    <span class="menu-check">{{ sendShortcut === 'ctrl-enter' ? '✓' : '' }}</span>
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
            :to="{ name: 'settings-agents' }"
            data-test="new-session-service-link"
            >前往平台设置 → Agent</RouterLink
          >
        </p>
      </form>
    </div>

    <ImagePreviewModal
      v-if="previewModalImage"
      :image="previewModalImage"
      @close="previewModalImage = null"
    />
  </section>
</template>

<style scoped>
.new-session {
  display: flex;
  justify-content: center;
  padding: 3rem 1rem;
}

.new-session-inner {
  width: min(48rem, 100%);
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.new-session h1 {
  font-size: 1.5rem;
  margin: 0;
  text-align: center;
}

.subtitle {
  margin: 0 0 0.75rem;
  text-align: center;
  color: var(--color-text-muted);
  font-size: 0.9rem;
}

.project-selector-group {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  margin-bottom: 0.5rem;
  padding: 0.5rem 0.75rem;
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: 8px;
}

.project-label {
  font-size: 0.88rem;
  font-weight: 600;
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
  border-radius: 8px;
}

.prompt-selector-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.prompt-label {
  font-size: 0.88rem;
  font-weight: 600;
  color: var(--color-text-muted);
}

.prompt-placeholder-hint {
  font-size: 0.75rem;
  color: var(--color-primary);
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
  align-items: center;
  gap: 0.75rem;
  padding: 0.5rem 0.75rem;
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: 8px;
}

.agent-label {
  font-size: 0.88rem;
  font-weight: 600;
  color: var(--color-text-muted);
  white-space: nowrap;
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
