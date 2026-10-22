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

const imageModeBlocked = computed(() => props.images.length > 0 && !props.supportsImage)
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
  const el = textarea.value
  if (!el) return
  el.style.height = 'auto'
  el.style.height = `${Math.min(el.scrollHeight, 240)}px`
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
  <form class="chat-composer" @submit.prevent="submit">
    <p v-if="props.editing" class="chat-edit-banner" data-test="chat-edit-banner" role="status">
      <span>正在编辑历史消息；发送后将删除该消息之后的内容。</span>
      <button type="button" class="chat-edit-cancel" @click="emit('cancel-edit')">取消</button>
    </p>

    <p v-if="!props.hasModels" class="chat-composer-note" role="alert">
      尚未配置可用模型，请先到「设置 → 模型管理」添加并启用模型。
    </p>
    <p v-else-if="imageModeBlocked" class="chat-composer-note" role="alert">
      当前模型不支持图片输入，请更换模型或移除图片。
    </p>
    <p v-if="notice" class="chat-composer-note" role="alert">{{ notice }}</p>

    <div
      class="chat-composer-box"
      @dragover.prevent
      @drop.prevent="onDrop"
    >
      <label class="sr-only" for="pure-chat-message">发送消息</label>
      <textarea
        id="pure-chat-message"
        ref="textarea"
        class="chat-composer-text"
        rows="2"
        :value="props.text"
        :placeholder="props.running ? '正在生成回复…' : '给 Chat 发送消息'"
        data-test="chat-input"
        @input="onInput"
        @keydown="onKeydown"
        @paste="onPaste"
      ></textarea>

      <ComposerImageTray :images="props.images" @remove="removeImage" @preview="openPreview" />

      <div class="chat-composer-bar">
        <div class="chat-composer-tools">
          <button
            type="button"
            class="chat-attach-btn"
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
            class="hidden-file-input"
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

        <button
          type="button"
          class="chat-send-btn"
          :class="{ stop: props.running }"
          :disabled="props.running ? false : !canSend"
          data-test="chat-send-btn"
          @click="submit"
        >
          {{ props.running ? '停止' : '发送' }}
        </button>
      </div>
    </div>

    <ImagePreviewModal :image="preview" :visible="previewVisible" @close="closePreview" />
  </form>
</template>

<style scoped>
.sr-only {
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

.chat-composer {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
  padding: 0.5rem 0.75rem 0.75rem;
}

.chat-edit-banner {
  margin: 0;
  padding: 0.4rem 0.6rem;
  border-radius: 6px;
  background: var(--color-warning-soft, #fef3c7);
  color: var(--color-text);
  font-size: 0.8rem;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem;
}

.chat-edit-cancel {
  border: none;
  background: transparent;
  color: var(--color-primary);
  cursor: pointer;
  font: inherit;
  font-size: 0.8rem;
}

.chat-composer-note {
  margin: 0;
  color: var(--color-warning, #b45309);
  font-size: 0.8rem;
}

.chat-composer-box {
  border: 1px solid var(--color-border-strong);
  border-radius: 12px;
  background: var(--color-surface);
  padding: 0.5rem 0.6rem 0.4rem;
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
}

.chat-composer-box:focus-within {
  border-color: var(--color-primary);
}

.chat-composer-text {
  width: 100%;
  min-height: 2.6rem;
  max-height: 240px;
  resize: none;
  border: none;
  outline: none;
  background: transparent;
  color: var(--color-text);
  font: inherit;
  line-height: 1.5;
  box-sizing: border-box;
}

.chat-composer-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.5rem;
}

.chat-composer-tools {
  display: flex;
  align-items: center;
  gap: 0.35rem;
  min-width: 0;
}

.chat-attach-btn {
  width: 28px;
  height: 28px;
  border: 1px solid var(--color-border-strong);
  border-radius: 50%;
  background: var(--color-surface-muted);
  color: var(--color-text);
  font-size: 1rem;
  line-height: 1;
  cursor: pointer;
}

.chat-attach-btn:disabled {
  opacity: 0.45;
  cursor: not-allowed;
}

.hidden-file-input {
  display: none;
}

.chat-send-btn {
  min-width: 4.5rem;
  height: 30px;
  padding: 0 0.9rem;
  border: none;
  border-radius: 8px;
  background: var(--color-primary);
  color: var(--color-surface);
  font: inherit;
  font-size: 0.85rem;
  cursor: pointer;
}

.chat-send-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.chat-send-btn.stop {
  background: var(--color-danger, #dc2626);
}
</style>
