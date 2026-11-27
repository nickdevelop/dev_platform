<script setup lang="ts">
// Compact session item component inside the project tree.
// Row 1: Agent logo (official icon, or short name) + running indicator + session title.
// Row 2: Model tag on the left, right-aligned time tag and reserved delete button on the right.
import { computed } from 'vue'
import type { AgentSession } from '../../api/sessions'
import { useSessionsStore } from '../../stores/sessions'
import BackendIcon from '../../shared/components/BackendIcon.vue'
import RunningIndicator from '../../shared/components/RunningIndicator.vue'

const props = withDefaults(
  defineProps<{
    session: AgentSession
    isSelected?: boolean
    worktreeLabel?: string
  }>(),
  {
    isSelected: false,
    worktreeLabel: '',
  },
)

const emit = defineEmits<{
  select: [sessionId: string]
  delete: [sessionId: string]
}>()

const sessionsStore = useSessionsStore()

const untitledLabel = '未命名会话'
const isRunning = computed(() => {
  if (sessionsStore.sessionTurns[props.session.id] !== undefined) {
    return sessionsStore.sessionTurns[props.session.id] === 'running'
  }
  return props.session.turn === 'running'
})

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

const isCollab = computed(() => props.session.backend === 'group-flow')

const modelFull = computed(() => {
  return sessionsStore.sessionDisplayModel(props.session.id, props.session.model)
})

const modelLabel = computed(() => {
  const full = modelFull.value
  if (!full) return isCollab.value ? '协同' : ''
  const slash = full.lastIndexOf('/')
  return slash >= 0 ? full.slice(slash + 1) : full
})
</script>

<template>
  <div
    class="session-open px-2.5 py-1.5 rounded-[3px] cursor-pointer transition-colors border group/item"
    :class="[
      isSelected
        ? 'selected bg-platform-surface-muted border-platform-primary border-l-2 border-l-[var(--color-accent,#4a8ad6)] border-t-transparent border-r-transparent border-b-transparent font-medium text-platform-text'
        : 'border-transparent hover:bg-platform-surface-muted/60 text-platform-text',
    ]"
    role="button"
    tabindex="0"
    :aria-label="session.title || untitledLabel"
    @click="emit('select', session.id)"
    @keydown.enter.prevent="emit('select', session.id)"
    @keydown.space.prevent="emit('select', session.id)"
  >
    <!-- Row 1: Logo + Title + Optional BOT source badge -->
    <div class="session-row-top flex items-center gap-1.5 mb-0.5 min-w-0">
      <BackendIcon :backend="session.backend" />
      <RunningIndicator v-if="isRunning" />
      <span
        class="title truncate text-sm font-medium text-platform-text flex-1"
        :title="session.title || untitledLabel"
      >
        {{ session.title || untitledLabel }}
      </span>
      <span
        v-if="session.source === 'bot'"
        class="entity-chip entity-chip--wechat shrink-0"
        title="由外部消息渠道触发并自动放行"
        data-test="session-bot-badge"
      >
        BOT
      </span>
    </div>

    <!-- Row 2: Model tag on the left, right-aligned time and delete action on the right -->
    <div class="session-row-bottom flex items-center justify-between gap-1 text-2xs text-platform-text-muted">
      <div class="session-bottom-left flex items-center gap-1.5 min-w-0">
        <span
          v-if="props.worktreeLabel"
          class="session-worktree flex items-center gap-1 text-2xs px-1.5 py-0.5 rounded bg-platform-surface-muted text-platform-text-muted border border-platform-border/60 truncate"
          :title="`工作树：${props.worktreeLabel}`"
          data-test="session-worktree"
        >
          <svg viewBox="0 0 16 16" width="9" height="9" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
            <circle cx="4.5" cy="3.5" r="1.4" />
            <circle cx="4.5" cy="12.5" r="1.4" />
            <circle cx="11.5" cy="6.5" r="1.4" />
            <path d="M4.5 5v6M6 6.5h3.5a2 2 0 0 1 2 2" />
          </svg>
          <span class="truncate">{{ props.worktreeLabel }}</span>
        </span>
        <span
          v-if="modelLabel"
          class="session-model text-2xs px-1.5 py-0.5 rounded bg-platform-surface-muted text-platform-text-muted border border-platform-border/60 truncate"
          :title="modelFull"
          data-test="session-model"
        >
          {{ modelLabel }}
        </span>
      </div>
      <div class="session-bottom-right flex items-center gap-1 shrink-0 ml-auto">
        <span
          class="session-time font-mono text-2xs shrink-0"
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
