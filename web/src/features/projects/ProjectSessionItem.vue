<script setup lang="ts">
// Compact session item component inside the project tree.
// Row 1: Logo (simplified agent short logo) + running indicator + session title.
// Row 2: Model tag on the left, right-aligned time tag and reserved delete button on the right.
import { computed } from 'vue'
import type { AgentSession } from '../../api/sessions'
import { useAgentsStore } from '../../stores/agents'
import { useSessionsStore } from '../../stores/sessions'
import RunningIndicator from '../../shared/components/RunningIndicator.vue'

const props = withDefaults(
  defineProps<{
    session: AgentSession
    isSelected?: boolean
  }>(),
  {
    isSelected: false,
  },
)

const emit = defineEmits<{
  select: [sessionId: string]
  delete: [sessionId: string]
}>()

const agentsStore = useAgentsStore()
const sessionsStore = useSessionsStore()

const logo = computed(() => agentsStore.backendShortName(props.session.backend))
const declaredColor = computed(() => agentsStore.backendColor(props.session.backend))
const declaredTone = computed(() => agentsStore.backendTone(props.session.backend))

const logoTone = computed(() => {
  if (declaredTone.value) return `tone-${declaredTone.value}`
  return 'tone-neutral'
})

const logoStyle = computed<Record<string, string>>(() =>
  declaredColor.value ? { '--logo-base': declaredColor.value } : {},
)

const untitledLabel = '未命名会话'
const isRunning = computed(() => props.session.turn === 'running')

function formatSessionTime(isoString?: string): string {
  if (!isoString) return ''
  const date = new Date(isoString)
  if (isNaN(date.getTime())) return ''

  const now = new Date()
  const isToday =
    date.getFullYear() === now.getFullYear() &&
    date.getMonth() === now.getMonth() &&
    date.getDate() === now.getDate()

  if (isToday) {
    const hours = String(date.getHours()).padStart(2, '0')
    const minutes = String(date.getMinutes()).padStart(2, '0')
    const seconds = String(date.getSeconds()).padStart(2, '0')
    return `${hours}:${minutes}:${seconds}`
  } else {
    const year = date.getFullYear()
    const month = String(date.getMonth() + 1).padStart(2, '0')
    const day = String(date.getDate()).padStart(2, '0')
    return `${year}-${month}-${day}`
  }
}

function formatFullTime(isoString?: string): string {
  if (!isoString) return ''
  const date = new Date(isoString)
  if (isNaN(date.getTime())) return ''
  return date.toLocaleString()
}

const modelFull = computed(() => {
  if (props.session.model) return props.session.model
  return sessionsStore.modelFor(props.session.id)
})

const modelLabel = computed(() => {
  const full = modelFull.value
  if (!full) return ''
  const slash = full.lastIndexOf('/')
  return slash >= 0 ? full.slice(slash + 1) : full
})
</script>

<template>
  <div
    class="session-open px-2 py-1 rounded cursor-pointer transition-colors border group/item"
    :class="[
      isSelected
        ? 'bg-platform-surface-muted border-platform-primary shadow-[0_0_6px_var(--color-primary-soft,rgba(59,130,246,0.2))] font-medium text-platform-text'
        : 'border-transparent hover:bg-platform-surface-muted hover:border-platform-border text-platform-text',
    ]"
    role="button"
    tabindex="0"
    :aria-label="session.title || untitledLabel"
    @click="emit('select', session.id)"
    @keydown.enter.prevent="emit('select', session.id)"
    @keydown.space.prevent="emit('select', session.id)"
  >
    <!-- Row 1: Logo + Title -->
    <div class="session-row-top flex items-center gap-1.5 mb-0.5 min-w-0">
      <span
        class="agent-logo"
        :class="declaredColor ? 'has-color' : logoTone"
        :style="logoStyle"
        :data-test="`agent-logo-${session.backend}`"
      >
        {{ logo }}
      </span>
      <RunningIndicator v-if="isRunning" />
      <span
        class="title truncate text-sm font-medium text-platform-text flex-1"
        :title="session.title || untitledLabel"
      >
        {{ session.title || untitledLabel }}
      </span>
    </div>

    <!-- Row 2: Model tag on the left, right-aligned time and delete action on the right -->
    <div class="session-row-bottom flex items-center justify-between gap-1 text-[11px] text-platform-text-muted">
      <div class="session-bottom-left flex items-center gap-1.5 min-w-0">
        <span
          v-if="modelLabel"
          class="session-model font-mono text-[10px] px-1 py-0.2 rounded bg-platform-surface-muted text-platform-text-muted border border-platform-border/60 max-w-[90px] truncate"
          :title="modelFull"
          data-test="session-model"
        >
          {{ modelLabel }}
        </span>
      </div>
      <div class="session-bottom-right flex items-center gap-1 shrink-0 ml-auto">
        <span
          class="session-time font-mono text-[10px] shrink-0"
          :title="formatFullTime(session.updatedAt)"
        >
          {{ formatSessionTime(session.updatedAt) }}
        </span>
        <button
          type="button"
          class="session-delete opacity-0 group-hover/item:opacity-100 hover:text-red-500 p-0.5 rounded transition-opacity"
          title="删除会话"
          aria-label="删除会话"
          data-test="delete-session-btn"
          @click.stop="emit('delete', session.id)"
        >
          <svg
            class="delete-icon w-3 h-3"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            aria-hidden="true"
            focusable="false"
          >
            <polyline points="3 6 5 6 21 6" />
            <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2" />
          </svg>
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.agent-logo {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 10px;
  font-weight: 700;
  line-height: 1;
  padding: 1.5px 3.5px;
  border-radius: 3px;
  letter-spacing: 0.5px;
  user-select: none;
  flex-shrink: 0;
}

.agent-logo.has-color {
  --logo-base: var(--color-info);
  --logo-color: var(--logo-base);
  --logo-surface: color-mix(in srgb, var(--logo-base) 14%, var(--color-surface, #fff));
  background: var(--logo-surface);
  color: var(--logo-color);
  border: 1px solid color-mix(in srgb, var(--logo-base) 25%, transparent);
}

:global([data-theme='dark'] .agent-logo.has-color) {
  --logo-color: color-mix(in srgb, var(--logo-base) 75%, var(--color-text, #fff));
  --logo-surface: color-mix(in srgb, var(--logo-base) 22%, var(--color-surface, #1e293b));
  border-color: color-mix(in srgb, var(--logo-base) 35%, transparent);
}

.agent-logo.tone-primary,
.agent-logo.tone-info {
  background: var(--color-info-soft, rgba(59, 130, 246, 0.12));
  color: var(--color-info, #2563eb);
}

.agent-logo.tone-secondary {
  background: var(--color-warning-soft, rgba(245, 158, 11, 0.12));
  color: var(--color-warning, #d97706);
}

.agent-logo.tone-neutral {
  background: var(--color-surface-muted, #f1f5f9);
  color: var(--color-text-muted, #64748b);
}
</style>
