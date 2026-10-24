<script setup lang="ts">
// Composer for the pure-chat area. It keeps only what a chat needs: text,
// optional images, a model picker, send and stop. Agent-only controls
// (approval mode, slash commands, workspace mentions, context meter) are
// deliberately absent.
import { computed, nextTick, ref, watch } from 'vue'
import type { ImageAttachment } from '../../../api/messageTypes'
import ComposerImageTray from '../composer/ComposerImageTray.vue'
import ImagePreviewModal from '../rendering/ImagePreviewModal.vue'
import ChatModelSelect from './ChatModelSelect.vue'
import {
  imageAttachmentFromFile,
  imageValidationError,
} from '../composer/imageFiles'
import { UiButton, UiAlert } from '../../../shared/ui'

const MAX_IMAGES = 4

const props = defineProps<{
  text: string
  images: ImageAttachment[]
  modelId: number | null
  running: boolean
  disabled?: boolean
  hasModels: boolean
  supportsImage: boolean
  editing?: boolean
  modelFallbackLabel?: string
  // compact hides the drag-to-resize handle and caps the textarea for a
  // constrained surface such as the quick-assistant popover.
  compact?: boolean
  // inputId keeps label/for association unique when two composers are mounted
  // at once (main Chat page + quick-assistant popover).
  inputId?: string
  placeholder?: string
}>()

const emit = defineEmits<{
  'update:text': [value: string]
  'update:images': [value: ImageAttachment[]]
  'update:modelId': [value: number]
  send: []
  stop: []
  'cancel-edit': []
}>()

const textarea = ref<HTMLTextAreaElement | null>(null)
const fileInput = ref<HTMLInputElement | null>(null)
const preview = ref<ImageAttachment | null>(null)
const previewVisible = ref(false)
const notice = ref('')

const textareaHeight = ref<number | null>(null)
let isResizing = false
let startY = 0
let startHeight = 0

function onResizerMouseDown(e: MouseEvent) {
  if (!textarea.value) return
  isResizing = true
  startY = e.clientY
  startHeight = textarea.value.getBoundingClientRect().height
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

const imageModeBlocked = computed(() => props.images.length > 0 && !props.supportsImage)
const resolvedInputId = computed(() => props.inputId ?? 'pure-chat-message')
const resolvedPlaceholder = computed(() =>
  props.running ? '正在生成回复…' : props.placeholder ?? '给 Chat 发送消息',
)
const maxTextareaHeight = computed(() => (props.compact ? 120 : 240))
const canSend = computed(
  () =>
    !props.running &&
    !props.disabled &&
    props.hasModels &&
    props.modelId !== null &&
    !imageModeBlocked.value &&
    (props.text.trim() !== '' || props.images.length > 0),
)

function submit() {
  if (props.running) {
    emit('stop')
    return
  }
  if (!canSend.value) return
  emit('send')
}

function onKeydown(event: KeyboardEvent) {
  if (event.key !== 'Enter' || event.isComposing) return
  if (event.shiftKey) return
  event.preventDefault()
  submit()
}

function autosize() {
  if (textareaHeight.value !== null) return
  const el = textarea.value
  if (!el) return
  el.style.height = 'auto'
  el.style.height = `${Math.min(el.scrollHeight, maxTextareaHeight.value)}px`
}

function onInput(event: Event) {
  emit('update:text', (event.target as HTMLTextAreaElement).value)
  void nextTick(autosize)
}

watch(
  () => props.text,
  () => {
    void nextTick(autosize)
  },
)

function triggerFileInput() {
  fileInput.value?.click()
}

async function addFiles(files: FileList | File[]) {
  notice.value = ''
  const next = [...props.images]
  for (const file of Array.from(files)) {
    if (next.length >= MAX_IMAGES) {
      notice.value = `单次最多添加 ${MAX_IMAGES} 张图片`
      break
    }
    if (!file.type.startsWith('image/')) {
      notice.value = `仅支持图片: ${file.name || '未命名文件'}`
      continue
    }
    const validationError = imageValidationError(file)
    if (validationError !== null) {
      notice.value = validationError
      continue
    }
    try {
      next.push(await imageAttachmentFromFile(file))
    } catch {
      notice.value = `读取图片失败: ${file.name || '未命名图片'}`
    }
  }
  emit('update:images', next)
}

function onFileInput(event: Event) {
  const target = event.target as HTMLInputElement
  if (target.files && target.files.length > 0) void addFiles(target.files)
  target.value = ''
}

function onPaste(event: ClipboardEvent) {
  const files = event.clipboardData?.files
  if (files && files.length > 0) {
    event.preventDefault()
    void addFiles(files)
  }
}

function onDrop(event: DragEvent) {
  if (event.dataTransfer?.files && event.dataTransfer.files.length > 0) {
    event.preventDefault()
    void addFiles(event.dataTransfer.files)
  }
}

function removeImage(id: string) {
  emit(
    'update:images',
    props.images.filter((image) => image.id !== id),
  )
}

function openPreview(image: ImageAttachment) {
  preview.value = image
  previewVisible.value = true
}

function closePreview() {
  previewVisible.value = false
  preview.value = null
}
</script>

<template>
  <form class="chat-composer flex flex-col gap-2" @submit.prevent="submit">
    <div
      v-if="props.editing"
      class="chat-edit-banner flex items-center justify-between gap-3 px-3 py-1.5 rounded bg-amber-50 dark:bg-amber-950/30 border border-amber-200 dark:border-amber-800 text-xs text-amber-800 dark:text-amber-200"
      data-test="chat-edit-banner"
      role="status"
    >
      <span>正在编辑历史消息；发送后将删除该消息之后的内容。</span>
      <button type="button" class="chat-edit-cancel font-medium text-blue-600 hover:underline" @click="emit('cancel-edit')">取消</button>
    </div>

    <UiAlert v-if="!props.hasModels" variant="warning" class="chat-composer-note">
      尚未配置可用模型，请先到「设置 → 模型管理」添加并启用模型。
    </UiAlert>
    <UiAlert v-else-if="imageModeBlocked" variant="warning" class="chat-composer-note">
      当前模型不支持图片输入，请更换模型或移除图片。
    </UiAlert>
    <UiAlert v-if="notice" variant="warning" class="chat-composer-note">
      {{ notice }}
    </UiAlert>

    <div
      class="chat-composer-box relative rounded border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-900 shadow-sm p-2.5 flex flex-col gap-2 transition-colors focus-within:border-blue-500 focus-within:ring-2 focus-within:ring-blue-500/20"
      @dragover.prevent
      @drop.prevent="onDrop"
    >
      <div
        v-if="!props.compact"
        class="composer-resizer h-2 w-full flex items-center justify-center cursor-ns-resize select-none group"
        data-test="composer-resizer"
        title="拖拽调整输入框高度，双击重置"
        @mousedown="onResizerMouseDown"
        @dblclick="resetComposerHeight"
      >
        <span class="resizer-handle w-9 h-1 rounded-full bg-slate-300 dark:bg-slate-700 group-hover:bg-blue-500 transition-colors"></span>
      </div>

      <label class="sr-only" :for="resolvedInputId">发送消息</label>
      <textarea
        :id="resolvedInputId"
        ref="textarea"
        class="chat-composer-text w-full min-h-[2.2rem] bg-transparent text-slate-800 dark:text-slate-100 placeholder-slate-400 dark:placeholder-slate-500 text-sm outline-none resize-none leading-relaxed"
        :rows="props.compact ? 2 : 3"
        :style="textareaHeight ? { height: `${textareaHeight}px`, maxHeight: 'none' } : undefined"
        :value="props.text"
        :placeholder="resolvedPlaceholder"
        data-test="chat-input"
        @input="onInput"
        @keydown="onKeydown"
        @paste="onPaste"
      ></textarea>

      <ComposerImageTray :images="props.images" @remove="removeImage" @preview="openPreview" />

      <div class="chat-composer-bar flex items-center justify-between gap-2 pt-2 border-t border-slate-100 dark:border-slate-800">
        <div class="chat-composer-tools flex items-center gap-2 flex-wrap">
          <button
            type="button"
            class="chat-attach-btn w-7 h-7 flex items-center justify-center rounded border border-slate-300 dark:border-slate-700 hover:bg-slate-50 dark:hover:bg-slate-800 text-slate-600 dark:text-slate-300 disabled:opacity-40 disabled:cursor-not-allowed text-base font-medium transition-colors"
            :disabled="!props.supportsImage || props.images.length >= MAX_IMAGES"
            :title="props.supportsImage ? '添加图片' : '当前模型不支持图片'"
            aria-label="添加图片"
            data-test="chat-attach-btn"
            @click="triggerFileInput"
          >
            +
          </button>
          <input
            ref="fileInput"
            type="file"
            accept="image/*"
            multiple
            class="hidden-file-input hidden"
            data-test="chat-file-input"
            @change="onFileInput"
          />
          <ChatModelSelect
            :model-value="props.modelId"
            :disabled="props.running"
            :fallback-label="props.modelFallbackLabel"
            @update:model-value="emit('update:modelId', $event)"
          />
        </div>

        <UiButton
          type="button"
          :variant="props.running ? 'danger' : 'primary'"
          size="sm"
          class="chat-send-btn"
          :class="{ stop: props.running }"
          :disabled="props.running ? false : !canSend"
          data-test="chat-send-btn"
          @click="submit"
        >
          {{ props.running ? '停止' : '发送' }}
        </UiButton>
      </div>
    </div>

    <ImagePreviewModal :image="preview" :visible="previewVisible" @close="closePreview" />
  </form>
</template>
