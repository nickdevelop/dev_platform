<script setup lang="ts">
// Lists a project's indexed sessions. Selecting one opens its workbench;
// status is reported as the backend derived it (active/stopped/disconnected).
// Each row is a keyboard-operable control: Enter and Space open the session,
// matching the pointer behavior.
import type { AgentSession } from '../api/sessions'
import { useAgentsStore } from '../stores/agents'

const props = defineProps<{
  sessions: AgentSession[]
  activeId?: string | null
}>()

const emit = defineEmits<{ select: [sessionId: string] }>()

// Display fallback for sessions with no title yet; the platform id must
// never surface as a name.
const untitledLabel = '未命名任务'

const statusLabels: Record<string, string> = {
  active: '进行中',
  stopped: '已停止',
  disconnected: '已断开',
}

const agents = useAgentsStore()

// A task whose backend runs a machine-level service is disconnected for one
// reason the user can act on: that service is not reachable. Saying "已断开"
// would suggest the conversation, not the service, is the thing that needs
// attention.
function statusLabel(session: AgentSession): string {
  if (session.status === 'disconnected' && agents.backendFor(session.backend)?.serviceSupported) {
    return '服务无法连接'
  }
  return statusLabels[session.status] ?? session.status
}

function select(sessionId: string) {
  emit('select', sessionId)
}
</script>

<template>
  <ul class="session-list">
    <li
      v-for="session in props.sessions"
      :key="session.id"
      class="session-item"
      :class="{ active: session.id === props.activeId }"
      :data-session-id="session.id"
    >
      <div
        class="session-open"
        role="button"
        tabindex="0"
        :aria-current="session.id === props.activeId ? 'true' : undefined"
        @click="select(session.id)"
        @keydown.enter.prevent="select(session.id)"
        @keydown.space.prevent="select(session.id)"
      >
        <span class="session-info">
          <span class="session-title">{{ session.title || untitledLabel }}</span>
          <span class="session-meta">
            <span class="backend">{{ session.backend }}</span>
            <span class="status" :class="`status-${session.status}`">
              {{ statusLabel(session) }}
            </span>
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
  border: 1px solid var(--color-border);
  border-radius: 8px;
}

.session-item.active {
  border-color: var(--color-primary);
  background: var(--color-info-soft);
}

.session-open {
  display: block;
  width: 100%;
  padding: 0.5rem 0.75rem;
  cursor: pointer;
  border-radius: 8px;
}

.session-open:focus-visible {
  outline: 2px solid var(--color-primary);
  outline-offset: 2px;
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
  color: var(--color-text-muted);
  font-size: 0.85rem;
}

.status-active {
  color: var(--color-success);
}

.status-disconnected,
.status-stopped {
  color: var(--color-warning);
}

.empty {
  color: var(--color-text-muted);
}
</style>
