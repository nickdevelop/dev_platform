<script setup lang="ts">
// New session form for the right pane of the shell, laid out like the opencode
// new-session view: a centered composer holds the first message, and the
// toolbar inside it carries the agent-backend switch, the model picker and
// the create action. A session names only an agent backend: the server reuses
// or starts the project's instance for that backend. The title is never
// entered here — the server derives it from the first message — so an
// optional message rides along with the create request and the waiting is
// visible on this form instead of a frozen composer. On success the shell
// navigates to the new session.
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { RouterLink, useRouter } from 'vue-router'
import { useAgentsStore } from '../../../stores/agents'
import { useProjectsStore } from '../../../stores/projects'
import { useSessionsStore } from '../../../stores/sessions'
import { getSessionHistory, type BackendId, type ImageAttachment, type ImageMediaType } from '../../../api/sessions'
import ModelSelect from './ModelSelect.vue'
import ComposerDropdown from './dropdown/ComposerDropdown.vue'
import { useComposerDropdown } from './dropdown/useComposerDropdown'
import { BuiltInSlashSource } from './dropdown/sources/BuiltInSlashSource'
import { WorkspaceFileSource } from './dropdown/sources/WorkspaceFileSource'
import { SessionMentionSource } from './dropdown/sources/SessionMentionSource'
import { resolveSessionMentions } from './sessionMentionResolver'
import ComposerImageTray from './ComposerImageTray.vue'
import ImagePreviewModal from '../rendering/ImagePreviewModal.vue'

const props = defineProps<{ projectId?: number }>()

const emit = defineEmits<{
  created: [sessionId: string, projectId?: number]
  manage: [kind: 'register' | 'create', project: null]
}>()

const agents = useAgentsStore()
const projectsStore = useProjectsStore()
const sessions = useSessionsStore()
const router = useRouter()

const selectedProjectId = ref<number | null>(props.projectId ? Number(props.projectId) : null)

watch(
  () => props.projectId,
  (id) => {
    if (id !== undefined && id !== null) {
      selectedProjectId.value = Number(id)
    }
  },
  { immediate: true },
)

watch(selectedProjectId, (newId) => {
  if (newId && Number(newId) !== Number(props.projectId)) {
    if (router && typeof router.replace === 'function') {
      void router.replace({
        name: 'session-new',
        params: { projectId: String(newId) },
      })
    }
  }
})

const selectedBackend = ref<BackendId | ''>('')
const messageText = ref('')
const selectedModel = ref('')
const modelOk = ref(false)
const creating = ref(false)
const composerBox = ref<HTMLElement | null>(null)
const textareaRef = ref<HTMLTextAreaElement | null>(null)
const toolNotice = ref('')

const selectedCapabilities = computed(() =>
  selectedBackend.value === '' ? null : agents.capabilitiesFor(selectedBackend.value),
)

const selectedEnabled = computed(
  () => selectedBackend.value !== '' && agents.backendEnabledFor(selectedBackend.value),
)

// Dropdown autocomplete for slash commands, workspace files (@) and session mentions (@)
const dropdown = useComposerDropdown({
  sources: [
    new BuiltInSlashSource(),
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
  if (dropdown.onKeydown(event)) return
  if (event.key === 'Enter' && !event.shiftKey) {
    event.preventDefault()
    void createSession()
  }
}

// Image and file attachment support
const attachedImages = ref<ImageAttachment[]>([])
const previewModalImage = ref<ImageAttachment | null>(null)
const fileInputRef = ref<HTMLInputElement | null>(null)

const MAX_IMAGE_SIZE_BYTES = 5 * 1024 * 1024 // 5MB
const MAX_TEXT_FILE_SIZE_BYTES = 1024 * 1024 // 1MB
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
    const mediaType = VALID_IMAGE_TYPES[file.type]
    if (!mediaType) {
      toolNotice.value = `不支持的文件类型: ${file.name} (仅支持 PNG/JPEG/WebP/GIF)`
      continue
    }
    if (file.size > MAX_IMAGE_SIZE_BYTES) {
      toolNotice.value = `图片超过 5MB 限制: ${file.name}`
      continue
    }

    try {
      const base64 = await readFileAsBase64(file)
      attachedImages.value.push({
        id: `img-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`,
        name: file.name,
        mediaType,
        data: base64,
        size: file.size,
      })
    } catch {
      toolNotice.value = `读取图片失败: ${file.name}`
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
      if (file) imageFiles.push(file)
    }
  }
  if (imageFiles.length > 0) {
    event.preventDefault()
    void addImageFiles(imageFiles)
  }
}

function onDragOver(event: DragEvent) {
  if (event.dataTransfer?.types?.includes('Files')) {
    event.dataTransfer.dropEffect = 'copy'
  }
}

function onDrop(event: DragEvent) {
  const files = event.dataTransfer?.files
  if (files && files.length > 0) {
    void handleUploadedFiles(files)
  }
}

const hasContent = computed(() => messageText.value.trim() !== '' || attachedImages.value.length > 0)

// A first message that needs a model (the backend supports choosing one) waits
// for the model list, which is always available: a backend's catalog is scoped to
// the project, not to a running instance, so creating the very first session of a
// project can still name a model. Until the list arrives the create is held back
// rather than silently sending on the backend's default model.
const firstMessageReady = computed(() => {
  if (!hasContent.value) return true
  if (!selectedCapabilities.value?.modelSelection) return true
  return modelOk.value && selectedModel.value !== ''
})

// Only platform-enabled backends may be chosen for a new session. The server also
// enforces this, but hiding them keeps the UI honest.
const selectableBackends = computed(() => agents.backends.filter((entry) => entry.enabled !== false))

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
  await agents.loadBackends()
  if (Array.isArray(projectsStore.projects) && projectsStore.projects.length === 0) {
    await projectsStore.load()
  }
  if (!selectedProjectId.value && (projectsStore.projects ?? []).length > 0) {
    selectedProjectId.value = projectsStore.projects[0].id
  }
  if (selectedBackend.value === '' && selectableBackends.value.length > 0) {
    selectedBackend.value = selectableBackends.value[0].backend
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
  // The model is part of the session's identity, not only of its first turn, so
  // it rides the create request even when no message is sent: the session is
  // created on the model the user saw, and the conversation opens on it.
  const withModel =
    selectedCapabilities.value?.modelSelection === true &&
    modelOk.value &&
    selectedModel.value !== ''
  const created = await sessions.createSession(targetProjectId, {
    backend: selectedBackend.value,
    ...(textToSend !== '' ? { message: textToSend } : {}),
    ...(withModel ? { model: selectedModel.value } : {}),
    ...(attachedImages.value.length > 0 ? { images: attachedImages.value } : {}),
  })
  creating.value = false
  if (created) emit('created', created.id, targetProjectId)
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
          <select
            id="project-select"
            v-model="selectedProjectId"
            class="project-select"
            data-test="project-select"
          >
            <option :value="null" disabled>请选择项目…</option>
            <option
              v-for="p in projectsStore.projects ?? []"
              :key="p.id"
              :value="p.id"
            >
              {{ p.name }} ({{ p.path }})
            </option>
          </select>
          <div
            v-if="Array.isArray(projectsStore.projects) && projectsStore.projects.length === 0"
            class="no-projects-hint"
            data-test="no-projects-hint"
          >
            <span>暂无可用项目：</span>
            <button
              type="button"
              class="btn-proj-link"
              data-test="btn-register-project"
              @click="emit('manage', 'register', null)"
            >
              注册已有目录
            </button>
            <span class="divider">·</span>
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

        <div class="backend-switch" role="radiogroup" aria-label="Agent 后端">
          <button
            v-for="backend in selectableBackends"
            :key="backend.backend"
            type="button"
            role="radio"
            class="backend-option"
            :class="{ active: selectedBackend === backend.backend }"
            :aria-checked="selectedBackend === backend.backend"
            :data-backend="backend.backend"
            :title="backend.label"
            @click="selectBackend(backend.backend)"
          >
            <span class="backend-name">{{ backend.label || backend.backend }}</span>
          </button>
        </div>
        <p v-if="selectedCapabilities" class="hint">
          {{
            selectedCapabilities.modelSelection
              ? '支持模型选择；可在下方输入框内选定模型，之后也可在对话页更换。'
              : '不支持模型选择；模型将使用后端默认值。'
          }}
        </p>

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
            placeholder="描述会话，例如：帮我检查构建失败的原因… (输入 / 使用快捷命令，@ 引用文件或会话)"
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
                :key="`${selectedBackend}-${selectedProjectId}`"
                :project-id="selectedProjectId ?? props.projectId ?? 0"
                :backend="selectedBackend"
                :model-selection="true"
                @status="modelOk = $event"
              />
              <span class="composer-hint">Enter 创建 · Shift+Enter 换行</span>
            </div>
            <button type="submit" class="create send-button" :disabled="!canCreate">
              {{ createLabel }}
            </button>
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

.create-session {
  display: flex;
  flex-direction: column;
  gap: 0.6rem;
}

/* Segmented switch for the agent backend: one pill container, one option per
   backend, the active one raised. A project-disabled backend stays visible
   but unpressable so the user knows why it cannot be chosen. */
.backend-switch {
  display: flex;
  gap: 0.25rem;
  padding: 0.25rem;
  border: 1px solid var(--color-border-strong);
  border-radius: 999px;
  background: var(--color-surface-muted);
  overflow-x: auto;
}

.backend-option {
  flex: 1 1 0;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 0.4rem;
  white-space: nowrap;
  padding: 0.4rem 0.9rem;
  border: 1px solid transparent;
  border-radius: 999px;
  background: transparent;
  color: var(--color-text-muted);
  font: inherit;
  font-size: 0.9rem;
  cursor: pointer;
}

.backend-option:hover:not(:disabled):not(.active) {
  color: var(--color-text);
}

.backend-option.active {
  background: var(--color-surface);
  border-color: var(--color-border-strong);
  color: var(--color-text);
  font-weight: 600;
  box-shadow: 0 1px 2px rgba(15, 23, 42, 0.12);
}

.backend-option:disabled {
  opacity: 0.55;
  cursor: not-allowed;
}

/* Desktop-style composer: one rounded box holds the input and its toolbar,
   like the opencode new-session view and the conversation composer. */
.composer-box {
  position: relative;
  border: 1px solid var(--color-border-strong);
  border-radius: 12px;
  background: var(--color-surface);
  display: flex;
  flex-direction: column;
  transition: border-color 0.15s ease;
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

.composer-hint {
  font-size: 0.75rem;
  color: var(--color-text-muted);
  margin-left: 0.25rem;
}

.create {
  margin-left: auto;
}

.send-button {
  padding: 0.4rem 1rem;
  border: none;
  border-radius: 6px;
  background: var(--color-primary);
  color: #fff;
  font: inherit;
  font-size: 0.85rem;
  font-weight: 600;
  cursor: pointer;
  transition: background 0.15s ease;
}

.send-button:hover:not(:disabled) {
  background: var(--color-primary-hover);
}

.send-button:disabled {
  opacity: 0.5;
  cursor: not-allowed;
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
