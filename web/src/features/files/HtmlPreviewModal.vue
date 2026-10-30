<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'

const props = defineProps<{
  visible: boolean
  fileName: string
  previewUrl: string
  filePath?: string
}>()

const emit = defineEmits<{
  close: []
}>()

const isFullScreen = ref(false)
const iframeKey = ref(0)

function handleRefresh() {
  iframeKey.value++
}

function toggleFullScreen() {
  isFullScreen.value = !isFullScreen.value
}

function handleClose() {
  isFullScreen.value = false
  emit('close')
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape' && props.visible) {
    if (isFullScreen.value) {
      isFullScreen.value = false
    } else {
      handleClose()
    }
  }
}

watch(
  () => props.visible,
  (val) => {
    if (!val) {
      isFullScreen.value = false
    }
  },
)

onMounted(() => {
  window.addEventListener('keydown', onKeydown)
})

onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKeydown)
})
</script>

<template>
  <Teleport to="body">
    <div
      v-if="visible"
      class="html-modal-backdrop"
      role="dialog"
      aria-modal="true"
      :aria-label="`预览 ${props.fileName}`"
      data-test="html-preview-backdrop"
      @click.self="handleClose"
    >
      <div
        class="html-modal-dialog"
        :class="{ 'is-fullscreen': isFullScreen }"
        data-test="html-preview-dialog"
      >
        <!-- Modal header -->
        <header class="html-modal-header">
          <div class="header-left">
            <span class="preview-badge">HTML 预览</span>
            <span class="file-name" :title="props.filePath || props.fileName">{{ props.fileName }}</span>
          </div>

          <div class="header-actions">
            <button
              type="button"
              class="modal-btn"
              title="重新加载页面"
              data-test="modal-refresh-btn"
              @click="handleRefresh"
            >
              <span class="btn-icon">
                <svg viewBox="0 0 16 16" width="13" height="13" fill="none" stroke="currentColor" stroke-width="1.8">
                  <path d="M14 8A6 6 0 1 1 9.5 2.5" />
                  <polyline points="14 2.5 14 6 10.5 6" />
                </svg>
              </span>
              <span class="btn-text">刷新</span>
            </button>

            <button
              type="button"
              class="modal-btn"
              :title="isFullScreen ? '退出全屏' : '全屏显示'"
              data-test="modal-fullscreen-btn"
              @click="toggleFullScreen"
            >
              <span class="btn-icon">
                <svg v-if="isFullScreen" viewBox="0 0 16 16" width="13" height="13" fill="none" stroke="currentColor" stroke-width="1.6">
                  <polyline points="3 6 6 6 6 3" />
                  <polyline points="13 10 10 10 10 13" />
                </svg>
                <svg v-else viewBox="0 0 16 16" width="13" height="13" fill="none" stroke="currentColor" stroke-width="1.6">
                  <polyline points="2 6 2 2 6 2" />
                  <polyline points="14 10 14 14 10 14" />
                </svg>
              </span>
              <span class="btn-text">{{ isFullScreen ? '还原' : '全屏' }}</span>
            </button>

            <button
              type="button"
              class="modal-btn close-btn"
              title="关闭 (Esc)"
              aria-label="关闭预览"
              data-test="modal-close-btn"
              @click="handleClose"
            >
              <svg viewBox="0 0 16 16" width="12" height="12" fill="none" stroke="currentColor" stroke-width="1.8">
                <line x1="3" y1="3" x2="13" y2="13" />
                <line x1="13" y1="3" x2="3" y2="13" />
              </svg>
            </button>
          </div>
        </header>

        <!-- Modal body: sandboxed iframe -->
        <main class="html-modal-body">
          <iframe
            v-if="props.previewUrl"
            :key="iframeKey"
            class="html-preview-frame"
            :src="props.previewUrl"
            sandbox="allow-scripts allow-forms allow-modals"
            data-test="html-preview-frame"
          />
        </main>
      </div>
    </div>
  </Teleport>
</template>

<style scoped>
.html-modal-backdrop {
  position: fixed;
  inset: 0;
  z-index: 9999;
  background: rgba(10, 12, 16, 0.75);
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

.html-modal-dialog {
  position: relative;
  width: 80vw;
  height: 80vh;
  display: flex;
  flex-direction: column;
  background: var(--color-surface, #14171f);
  border: 1px solid var(--color-border-strong, #2e3545);
  border-radius: 4px;
  overflow: hidden;
  transition: width 0.18s ease-out, height 0.18s ease-out;
}

.html-modal-dialog.is-fullscreen {
  width: 100vw;
  height: 100vh;
  border-radius: 0;
  border: none;
  box-shadow: none;
}

.html-modal-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: 44px;
  padding: 0 16px;
  background: var(--color-surface-muted, #f8fafc);
  border-bottom: 1px solid var(--color-border, #e2e8f0);
  flex-shrink: 0;
}

.header-left {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
}

.preview-badge {
  display: inline-flex;
  align-items: center;
  padding: 2px 7px;
  border-radius: 4px;
  font-size: 0.72rem;
  font-weight: 600;
  background: rgba(37, 99, 235, 0.1);
  color: var(--color-primary, #2563eb);
  letter-spacing: 0.02em;
}

.file-name {
  font-size: 0.86rem;
  font-weight: 500;
  color: var(--color-text, #0f172a);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.header-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}

.modal-btn {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  height: 28px;
  padding: 0 10px;
  border: 1px solid var(--color-border, #e2e8f0);
  border-radius: 4px;
  background: var(--color-surface, #ffffff);
  color: var(--color-text, #0f172a);
  font-size: 0.78rem;
  cursor: pointer;
  transition: all 0.12s ease;
}

.modal-btn:hover {
  background: var(--color-surface-muted, #f1f5f9);
  border-color: var(--color-border-strong, #cbd5e1);
}

.btn-icon {
  font-size: 0.85rem;
  line-height: 1;
}

.close-btn {
  padding: 0 8px;
  font-size: 1rem;
  color: var(--color-text-muted, #64748b);
}

.close-btn:hover {
  background: rgba(239, 68, 68, 0.1);
  color: var(--color-danger, #b91c1c);
  border-color: rgba(239, 68, 68, 0.3);
}

.html-modal-body {
  flex: 1;
  min-height: 0;
  position: relative;
  background: #ffffff;
}

.html-preview-frame {
  width: 100%;
  height: 100%;
  border: none;
  display: block;
}
</style>
