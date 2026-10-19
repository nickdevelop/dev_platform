<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(
  defineProps<{
    mode?: 'preview' | 'source'
    size?: number
    lines?: number
    filePath?: string
    canSwitchMode?: boolean
  }>(),
  {
    mode: 'preview',
    canSwitchMode: true,
  },
)

const emit = defineEmits<{
  'update:mode': [mode: 'preview' | 'source']
  'copy-path': []
}>()

function formatBytes(bytes?: number): string {
  if (bytes === undefined || bytes === null || isNaN(bytes)) return ''
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

const formattedSize = computed(() => formatBytes(props.size))
</script>

<template>
  <div class="artifact-toolbar" data-test="artifact-toolbar">
    <!-- Mode switch buttons -->
    <div v-if="props.canSwitchMode" class="mode-switch">
      <button
        type="button"
        class="mode-btn"
        :class="{ 'is-active': props.mode === 'preview' }"
        @click="emit('update:mode', 'preview')"
      >
        预览
      </button>
      <button
        type="button"
        class="mode-btn"
        :class="{ 'is-active': props.mode === 'source' }"
        @click="emit('update:mode', 'source')"
      >
        源码
      </button>
    </div>

    <!-- Right info & metadata -->
    <div class="toolbar-meta">
      <span v-if="props.lines !== undefined && props.lines > 0" class="meta-item">
        {{ props.lines }} 行
      </span>
      <span v-if="formattedSize" class="meta-item">
        {{ formattedSize }}
      </span>
      <span v-if="props.filePath" class="meta-item path-item" :title="props.filePath">
        {{ props.filePath }}
      </span>
    </div>
  </div>
</template>

<style scoped>
.artifact-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: 36px;
  padding: 0 12px;
  background: var(--color-surface-muted, #f8fafc);
  border-bottom: 1px solid var(--color-border, #e3e6eb);
  flex-shrink: 0;
  font-size: 0.78rem;
}

.mode-switch {
  display: flex;
  background: var(--color-bg, #f4f5f7);
  padding: 2px;
  border-radius: 5px;
  border: 1px solid var(--color-border, #e3e6eb);
}

.mode-btn {
  border: none;
  background: transparent;
  padding: 2px 10px;
  border-radius: 4px;
  font-size: 0.75rem;
  color: var(--color-text-muted);
  cursor: pointer;
  transition: all 0.15s ease;
}

.mode-btn.is-active {
  background: var(--color-surface, #ffffff);
  color: var(--color-primary, #2563eb);
  font-weight: 600;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.05);
}

.toolbar-meta {
  display: flex;
  align-items: center;
  gap: 10px;
  color: var(--color-text-muted);
  font-size: 0.75rem;
  overflow: hidden;
}

.meta-item {
  white-space: nowrap;
}

.path-item {
  max-width: 200px;
  overflow: hidden;
  text-overflow: ellipsis;
  direction: rtl;
  text-align: left;
}
</style>
