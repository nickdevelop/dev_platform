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
        <header class="task-chat-dialog-head">
          <div class="dialog-title-wrap">
            <span class="dialog-icon inline-flex items-center text-[var(--color-text-muted)]">
              <svg viewBox="0 0 16 16" width="14" height="14" fill="none" stroke="currentColor" stroke-width="1.6" aria-hidden="true">
                <rect x="2.5" y="3.5" width="11" height="9" rx="1.5" />
                <line x1="8" y1="1.5" x2="8" y2="3.5" />
                <circle cx="6" cy="7.5" r="0.75" fill="currentColor" />
                <circle cx="10" cy="7.5" r="0.75" fill="currentColor" />
                <line x1="6" y1="9.5" x2="10" y2="9.5" />
              </svg>
            </span>
            <span class="dialog-title text-sm font-semibold text-[var(--color-text)]">{{ title }}</span>
            <span v-if="backend" class="dialog-meta-badge font-mono text-[11px] text-[var(--color-text-muted)] bg-[var(--color-surface)] px-2 py-0.5 rounded border border-[var(--color-border)]">
              {{ backend }} {{ model ? `· ${model}` : '' }}
            </span>
          </div>
          <button
            type="button"
            class="dialog-close-btn inline-flex items-center justify-center p-1.5 rounded text-[var(--color-text-muted)] hover:text-[var(--color-text)] hover:bg-[var(--color-surface-muted)] transition-colors"
            title="关闭 (Esc)"
            aria-label="关闭"
            data-test="task-chat-dialog-close"
            @click="emit('close')"
          >
            <svg viewBox="0 0 16 16" width="14" height="14" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true">
              <line x1="3" y1="3" x2="13" y2="13" />
              <line x1="13" y1="3" x2="3" y2="13" />
            </svg>
          </button>
        </header>

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
  background: rgba(15, 23, 42, 0.7);
  padding: 1.5rem;
}

.task-chat-dialog-panel {
  display: flex;
  flex-direction: column;
  width: min(80rem, 95vw);
  height: 90vh;
  background: var(--color-surface, #ffffff);
  border: 1px solid var(--color-border, #e2e8f0);
  border-radius: var(--radius, 6px);
  overflow: hidden;
}

.task-chat-dialog-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0.75rem 1rem;
  border-bottom: 1px solid var(--color-border, #e2e8f0);
  background: var(--color-surface-muted, #f8fafc);
  gap: 1rem;
}

.dialog-title-wrap {
  display: flex;
  align-items: center;
  gap: 0.6rem;
  min-width: 0;
}

.dialog-title {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.task-chat-dialog-body {
  flex: 1;
  min-height: 0;
  overflow: hidden;
}
</style>
