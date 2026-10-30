<script setup lang="ts">
import { onBeforeUnmount, onMounted } from 'vue'
import SessionChatPanel from '../chat/SessionChatPanel.vue'

const props = defineProps<{
  title: string
  projectId: number
  sessionId: string
  backend?: string
  model?: string
}>()

const emit = defineEmits<{
  close: []
}>()

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape') {
    emit('close')
  }
}

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
      class="task-chat-dialog-backdrop"
      role="dialog"
      aria-modal="true"
      data-test="task-chat-dialog"
      @click.self="emit('close')"
    >
      <div class="task-chat-dialog-panel">
        <div class="task-chat-dialog-head">
          <div class="dialog-title-wrap">
            <span class="dialog-icon inline-flex items-center text-platform-text-muted">
              <svg viewBox="0 0 16 16" width="14" height="14" fill="none" stroke="currentColor" stroke-width="1.6">
                <rect x="3" y="4" width="10" height="8" rx="2" />
                <line x1="8" y1="2" x2="8" y2="4" />
                <circle cx="6" cy="8" r="0.75" fill="currentColor" />
                <circle cx="10" cy="8" r="0.75" fill="currentColor" />
                <line x1="6" y1="10" x2="10" y2="10" />
              </svg>
            </span>
            <span class="dialog-title">{{ title }}</span>
            <span v-if="backend" class="dialog-meta-badge">{{ backend }} {{ model ? `· ${model}` : '' }}</span>
          </div>
          <button
            type="button"
            class="dialog-close-btn inline-flex items-center justify-center"
            title="关闭 (Esc)"
            aria-label="关闭"
            data-test="task-chat-dialog-close"
            @click="emit('close')"
          >
            <svg viewBox="0 0 16 16" width="12" height="12" fill="none" stroke="currentColor" stroke-width="1.8">
              <line x1="3" y1="3" x2="13" y2="13" />
              <line x1="13" y1="3" x2="3" y2="13" />
            </svg>
          </button>
        </div>
        <div class="task-chat-dialog-body">
          <SessionChatPanel
            :key="sessionId"
            :project-id="projectId"
            :session-id="sessionId"
          />
        </div>
      </div>
    </div>
  </Teleport>
</template>

<style scoped>
.task-chat-dialog-backdrop {
  position: fixed;
  inset: 0;
  z-index: 50;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(10, 12, 16, 0.75);
  padding: 1.5rem;
}

.task-chat-dialog-panel {
  display: flex;
  flex-direction: column;
  width: min(78rem, 95vw);
  height: 90vh;
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: 4px;
  overflow: hidden;
}

.task-chat-dialog-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0.65rem 1rem;
  border-bottom: 1px solid var(--color-border);
  background: var(--color-surface-muted);
}

.dialog-title-wrap {
  display: flex;
  align-items: center;
  gap: 0.6rem;
  min-width: 0;
}

.dialog-icon {
  font-size: 1.1rem;
}

.dialog-title {
  font-weight: 600;
  font-size: 0.95rem;
  color: var(--color-text);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.dialog-meta-badge {
  font-size: 0.78rem;
  color: var(--color-text-muted);
  background: var(--color-surface);
  padding: 0.15rem 0.5rem;
  border-radius: 4px;
  border: 1px solid var(--color-border);
}

.dialog-close-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border: none;
  background: transparent;
  color: var(--color-text-muted);
  border-radius: 6px;
  cursor: pointer;
  font-size: 0.9rem;
  transition: all 0.15s ease;
}

.dialog-close-btn:hover {
  background: var(--color-border);
  color: var(--color-text);
}

.task-chat-dialog-body {
  flex: 1;
  min-height: 0;
  overflow: hidden;
}
</style>
