<script setup lang="ts">
import { onMounted, onBeforeUnmount } from 'vue'
import type { ImageAttachment } from '../../../api/sessions'

const props = defineProps<{
  image: ImageAttachment | null
  visible: boolean
}>()

const emit = defineEmits<{
  close: []
}>()

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape' && props.visible) {
    emit('close')
  }
}

onMounted(() => {
  window.addEventListener('keydown', onKeydown)
})

onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKeydown)
})

function formatSize(bytes?: number): string {
  if (!bytes) return ''
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${Math.round(bytes / 1024)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}
</script>

<template>
  <Teleport to="body">
    <div
      v-if="visible && image"
      class="image-modal-backdrop"
      role="dialog"
      aria-modal="true"
      :aria-label="image.name"
      @click.self="emit('close')"
    >
      <div class="image-modal-content">
        <div class="image-modal-header">
          <span class="modal-title">{{ image.name }}</span>
          <span v-if="image.size" class="modal-meta">{{ formatSize(image.size) }}</span>
          <button
            type="button"
            class="modal-close"
            aria-label="关闭预览"
            @click="emit('close')"
          >
            ×
          </button>
        </div>
        <div class="image-modal-body">
          <img
            :src="image.data.startsWith('data:') ? image.data : `data:${image.mediaType};base64,${image.data}`"
            :alt="image.name"
          />
        </div>
      </div>
    </div>
  </Teleport>
</template>

<style scoped>
.image-modal-backdrop {
  position: fixed;
  inset: 0;
  z-index: 9999;
  background: rgba(15, 23, 42, 0.75);
  backdrop-filter: blur(4px);
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 1.5rem;
  animation: fadeIn 0.15s ease-out;
}

@keyframes fadeIn {
  from { opacity: 0; }
  to { opacity: 1; }
}

.image-modal-content {
  position: relative;
  max-width: min(90vw, 1000px);
  max-height: 90vh;
  display: flex;
  flex-direction: column;
  background: var(--color-surface, #ffffff);
  border: 1px solid var(--color-border-strong, #cbd5e1);
  border-radius: 8px;
  box-shadow: 0 20px 40px rgba(0, 0, 0, 0.35);
  overflow: hidden;
}

.image-modal-header {
  display: flex;
  align-items: center;
  gap: 0.6rem;
  padding: 0.5rem 0.8rem;
  border-bottom: 1px solid var(--color-border, #e2e8f0);
  background: var(--color-surface-muted, #f8fafc);
}

.modal-title {
  font-size: 0.85rem;
  font-weight: 500;
  color: var(--color-text, #0f172a);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.modal-meta {
  font-size: 0.75rem;
  color: var(--color-text-muted, #64748b);
  margin-right: auto;
}

.modal-close {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  border: none;
  border-radius: 4px;
  background: transparent;
  color: var(--color-text-muted, #64748b);
  font-size: 1.25rem;
  line-height: 1;
  cursor: pointer;
  transition: all 0.12s ease;
}

.modal-close:hover {
  background: var(--color-border, #e2e8f0);
  color: var(--color-text, #0f172a);
}

.image-modal-body {
  padding: 0.5rem;
  display: flex;
  align-items: center;
  justify-content: center;
  overflow: auto;
  max-height: calc(90vh - 45px);
  background: #000000;
}

.image-modal-body img {
  max-width: 100%;
  max-height: calc(90vh - 60px);
  object-fit: contain;
  border-radius: 4px;
}
</style>
