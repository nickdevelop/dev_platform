<script setup lang="ts">
import { computed } from 'vue'
import BackendTag from '../../../shared/components/BackendTag.vue'
import type { AgentSession } from '../../../api/sessions'
import { sessionDebugExportUrl } from '../../../api/sessions'

const props = defineProps<{
  session: AgentSession
}>()

const emit = defineEmits<{
  delete: []
}>()

const canExportDebug = computed(() => {
  return props.session.capabilities?.debugExport === true || props.session.backend === 'builtin'
})

const debugExportUrl = computed(() => {
  return sessionDebugExportUrl(props.session.projectId, props.session.id)
})
</script>

<template>
  <header class="chat-header">
    <div class="title-row">
      <BackendTag :backend="session.backend" />
      <h2 class="session-heading">{{ session.title || session.id }}</h2>
      <a
        v-if="canExportDebug"
        :href="debugExportUrl"
        download
        class="export-debug-btn"
        data-test="export-debug-log"
        title="下载内置 Agent 交互调试日志"
      >
        导出调试
      </a>
      <button
        type="button"
        class="delete-session"
        data-test="delete-session"
        title="删除会话"
        @click="emit('delete')"
      >
        删除
      </button>
    </div>
  </header>
</template>

<style scoped>
.chat-header {
  display: flex;
  flex-direction: column;
  padding-bottom: 0.5rem;
  border-bottom: 1px solid var(--color-border);
  flex-shrink: 0;
}

.title-row {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  min-width: 0;
}

.title-row .session-heading {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 1.05rem;
  font-weight: 600;
}

.delete-session {
  padding: 0.15rem 0.55rem;
  border: 1px solid var(--color-danger-border);
  border-radius: 6px;
  background: var(--color-surface);
  color: var(--color-danger);
  font: inherit;
  font-size: 0.8rem;
  cursor: pointer;
  transition: all 0.15s ease;
}

.delete-session:hover {
  background: var(--color-danger-soft, #fee2e2);
}

.export-debug-btn {
  padding: 0.15rem 0.55rem;
  border: 1px solid var(--color-border-strong);
  border-radius: 6px;
  background: var(--color-surface);
  color: var(--color-text-muted);
  font: inherit;
  font-size: 0.8rem;
  text-decoration: none;
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  transition: all 0.15s ease;
}

.export-debug-btn:hover {
  color: var(--color-primary-hover);
  border-color: var(--color-primary);
}
</style>
