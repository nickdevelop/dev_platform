<script setup lang="ts">
import type { ImageAttachment } from '../../../api/sessions'

const props = defineProps<{
  images: ImageAttachment[]
}>()

const emit = defineEmits<{
  remove: [id: string]
  preview: [image: ImageAttachment]
}>()

function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${Math.round(bytes / 1024)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}
</script>

<template>
  <div v-if="images.length > 0" class="composer-image-tray" data-test="image-tray">
    <div
      v-for="img in images"
      :key="img.id"
      class="image-card"
      :title="`${img.name} (${formatSize(img.size)})`"
    >
      <div class="image-thumb" @click="emit('preview', img)">
        <img :src="img.data.startsWith('data:') ? img.data : `data:${img.mediaType};base64,${img.data}`" :alt="img.name" />
      </div>
      <div class="image-info">
        <span class="image-name">{{ img.name }}</span>
        <span class="image-size">{{ formatSize(img.size) }}</span>
      </div>
      <button
        type="button"
        class="image-remove"
        aria-label="移除图片"
        @click.stop="emit('remove', img.id)"
      >
        ×
      </button>
    </div>
  </div>
</template>

<style scoped>
.composer-image-tray {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.25rem 0.1rem;
  overflow-x: auto;
  min-height: 44px;
}

.image-card {
  position: relative;
  display: flex;
  align-items: center;
  gap: 0.4rem;
  padding: 0.25rem 0.45rem 0.25rem 0.25rem;
  background: var(--color-surface-muted, #f8fafc);
  border: 1px solid var(--color-border-strong, #cbd5e1);
  border-radius: 6px;
  max-width: 14rem;
  flex-shrink: 0;
  transition: border-color 0.15s ease;
}

.image-card:hover {
  border-color: var(--color-primary, #3b82f6);
}

.image-thumb {
  width: 34px;
  height: 34px;
  border-radius: 4px;
  overflow: hidden;
  cursor: pointer;
  background: #000;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.image-thumb img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.image-info {
  display: flex;
  flex-direction: column;
  min-width: 0;
  overflow: hidden;
}

.image-name {
  font-size: 0.76rem;
  font-weight: 500;
  color: var(--color-text, #0f172a);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 8rem;
}

.image-size {
  font-size: 0.68rem;
  color: var(--color-text-muted, #64748b);
}

.image-remove {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 18px;
  height: 18px;
  border: none;
  border-radius: 50%;
  background: transparent;
  color: var(--color-text-muted, #64748b);
  font-size: 1rem;
  line-height: 1;
  cursor: pointer;
  padding: 0;
  transition: all 0.12s ease;
  flex-shrink: 0;
}

.image-remove:hover {
  background: rgba(239, 68, 68, 0.15);
  color: #ef4444;
}
</style>
