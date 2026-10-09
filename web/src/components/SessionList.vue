<script setup lang="ts">
// Lists a project's indexed sessions. Selecting one opens its workbench;
// status is reported as the backend derived it (active/stopped/disconnected).
import type { AgentSession } from '../api/sessions'

const props = defineProps<{
  sessions: AgentSession[]
  activeId?: string | null
}>()

const emit = defineEmits<{ select: [sessionId: string] }>()

const statusLabels: Record<string, string> = {
  active: '进行中',
  stopped: '已停止',
  disconnected: '已断开',
}
</script>

<template>
  <ul class="session-list">
    <li
      v-for="session in props.sessions"
      :key="session.id"
      class="session-item"
      :class="{ active: session.id === props.activeId }"
      @click="emit('select', session.id)"
    >
      <div class="session-info">
        <span class="session-title">{{ session.title || session.id }}</span>
        <span class="session-meta">
          <span class="backend">{{ session.backend }}</span>
          <span class="status" :class="`status-${session.status}`">
            {{ statusLabels[session.status] ?? session.status }}
          </span>
        </span>
      </div>
    </li>
    <li v-if="props.sessions.length === 0" class="empty">该项目还没有会话。</li>
  </ul>
</template>

<style scoped>
.session-list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
}

.session-item {
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  padding: 0.5rem 0.75rem;
  cursor: pointer;
}

.session-item.active {
  border-color: #2563eb;
  background: #eff6ff;
}

.session-info {
  display: flex;
  justify-content: space-between;
  gap: 0.75rem;
}

.session-title {
  font-weight: 600;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.session-meta {
  display: flex;
  gap: 0.5rem;
  flex-shrink: 0;
  color: #64748b;
  font-size: 0.85rem;
}

.status-active {
  color: #15803d;
}

.status-disconnected,
.status-stopped {
  color: #b45309;
}

.empty {
  color: #64748b;
}
</style>
