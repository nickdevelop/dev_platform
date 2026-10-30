<script setup lang="ts">
import { ref } from 'vue'

const props = defineProps<{
  rawUrl: string
  fileName: string
  fileSize?: number
}>()

const zoomMode = ref<'fit' | 'actual'>('fit')
const loaded = ref(false)
const error = ref(false)

function toggleZoom() {
  zoomMode.value = zoomMode.value === 'fit' ? 'actual' : 'fit'
}

function onLoad() {
  loaded.value = true
  error.value = false
}

function onError() {
  loaded.value = false
  error.value = true
}
</script>

<template>
  <div class="image-viewer" data-test="image-viewer">
    <div class="image-toolbar">
      <button
        type="button"
        class="zoom-btn"
        :title="zoomMode === 'fit' ? '适应窗口 (点击切换实际尺寸)' : '实际尺寸 (点击切换适应窗口)'"
        @click="toggleZoom"
      >
        <svg viewBox="0 0 16 16" width="13" height="13" fill="none" stroke="currentColor" stroke-width="1.8">
          <circle cx="7" cy="7" r="4.5" />
          <line x1="10.5" y1="10.5" x2="14" y2="14" />
        </svg>
        <span>{{ zoomMode === 'fit' ? '实际尺寸 (100%)' : '适应窗口' }}</span>
      </button>
      <span class="image-name">{{ props.fileName }}</span>
    </div>

    <div class="image-stage" :class="`zoom-${zoomMode}`">
      <div v-if="error" class="image-error">
        加载图片失败
      </div>
      <img
        v-show="!error"
        :src="props.rawUrl"
        :alt="props.fileName"
        class="preview-img"
        @load="onLoad"
        @error="onError"
        @click="toggleZoom"
      />
    </div>
  </div>
</template>

<style scoped>
.image-viewer {
  display: flex;
  flex-direction: column;
  height: 100%;
  background: var(--color-surface, #ffffff);
  overflow: hidden;
}

.image-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: 36px;
  padding: 0 12px;
  background: var(--color-surface-muted, #f8fafc);
  border-bottom: 1px solid var(--color-border, #e3e6eb);
  font-size: 0.78rem;
  flex-shrink: 0;
}

.zoom-btn {
  border: 1px solid var(--color-border, #e3e6eb);
  background: var(--color-surface, #ffffff);
  padding: 2px 8px;
  border-radius: 4px;
  font-size: 0.75rem;
  color: var(--color-text);
  cursor: pointer;
}

.zoom-btn:hover {
  background: var(--color-bg, #f4f5f7);
}

.image-name {
  color: var(--color-text-muted);
}

.image-stage {
  flex: 1;
  min-height: 0;
  overflow: auto;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 20px;
  background: var(--color-surface-muted, #14171f);
}

.zoom-fit .preview-img {
  max-width: 100%;
  max-height: 100%;
  object-fit: contain;
  cursor: zoom-in;
  border: 1px solid var(--color-border);
}

.zoom-actual .preview-img {
  cursor: zoom-out;
  border: 1px solid var(--color-border);
}

.image-error {
  color: var(--color-danger, #b91c1c);
  font-size: 0.85rem;
}
</style>
