<script setup lang="ts">
// FlowMessageItem: Renders one collaborative IM chat bubble in the group conversation
import { computed } from 'vue'
import type { FlowMessage, FlowRole } from '../../../api/tasks'
import { renderMarkdown } from '../../../shared/markdown'
import FlowRoleBadge from './FlowRoleBadge.vue'

const props = defineProps<{
  message: FlowMessage
  role?: FlowRole | null
  roles?: FlowRole[]
}>()

const emit = defineEmits<{
  openSession: [sessionId: string]
}>()

const isUser = computed(() => props.message.senderType === 'user')
const isOrchestrator = computed(() => props.message.senderType === 'orchestrator')
const isAgent = computed(() => props.message.senderType === 'agent')
const isSystem = computed(() => props.message.senderType === 'system')

// Check if message declares completion
const isCompletedFlag = computed(() => {
  const c = props.message.content || ''
  return c.includes('[任务完成]') || c.includes('[流程完成]') || c.includes('【任务完成】') || c.includes('【流程完成】')
})

// Highlight @mentions in raw markdown content
const formattedContent = computed(() => {
  let text = props.message.content || ''
  // If streaming and empty
  if (!text && props.message.status === 'streaming') {
    return '<span class="inline-block animate-pulse text-[var(--color-text-muted)] font-mono text-xs">正在思考中…</span>'
  }

  // Pre-process @role mentions into highlighted spans before markdown rendering
  if (props.roles && props.roles.length > 0) {
    for (const r of props.roles) {
      const regex = new RegExp(`@(${r.name})`, 'g')
      text = text.replace(regex, `<span class="mention-role-tag font-semibold text-[var(--color-primary)] bg-[var(--color-primary)]/10 px-1 py-0.5 rounded border border-[var(--color-primary)]/20">@$1</span>`)
    }
  }

  return renderMarkdown(text)
})

function formatTime(iso: string): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (isNaN(d.getTime())) return ''
  return d.toLocaleTimeString('zh-CN', { hour12: false })
}

function handleOpenSession() {
  if (props.message.sessionId) {
    emit('openSession', props.message.sessionId)
  }
}
</script>

<template>
  <!-- 1. System notification bubble (centered) -->
  <div v-if="isSystem" class="flow-msg-system flex justify-center my-3" data-test="msg-system">
    <div class="inline-flex items-center gap-1.5 px-3 py-1 rounded-[var(--radius,4px)] bg-[var(--color-surface-muted)] border border-[var(--color-border)] text-xs text-[var(--color-text-muted)] font-mono">
      <svg viewBox="0 0 16 16" width="12" height="12" fill="none" stroke="currentColor" stroke-width="1.8" class="text-amber-500">
        <circle cx="8" cy="8" r="6" />
        <line x1="8" y1="5" x2="8" y2="8.5" />
        <circle cx="8" cy="11" r="0.8" fill="currentColor" />
      </svg>
      <span>{{ message.content }}</span>
      <span class="text-[10px] opacity-70 ml-1">{{ formatTime(message.createdAt) }}</span>
    </div>
  </div>

  <!-- 2. Standard Chat Message (User right, Orchestrator/Agent left) -->
  <div
    v-else
    class="flow-msg-item flex gap-3 my-3 text-sm"
    :class="[isUser ? 'flex-row-reverse justify-start' : 'flex-row justify-start']"
    :data-test="`msg-${message.senderType}`"
  >
    <!-- Sender Avatar -->
    <div class="msg-avatar shrink-0 select-none">
      <!-- User Avatar -->
      <div
        v-if="isUser"
        class="w-7 h-7 rounded bg-[var(--color-surface-muted)] border border-[var(--color-border)] flex items-center justify-center text-xs font-semibold text-[var(--color-text)]"
        title="用户"
      >
        我
      </div>

      <!-- Orchestrator Avatar -->
      <div
        v-else-if="isOrchestrator"
        class="w-7 h-7 rounded bg-blue-500/10 border border-blue-500/30 text-blue-500 flex items-center justify-center"
        title="中控调度大脑"
      >
        <svg viewBox="0 0 16 16" width="14" height="14" fill="none" stroke="currentColor" stroke-width="1.8">
          <circle cx="8" cy="8" r="3" />
          <path d="M8 1v2M8 13v2M1 8h2M13 8h2" />
        </svg>
      </div>

      <!-- Agent Role Avatar -->
      <div
        v-else
        class="w-7 h-7 rounded flex items-center justify-center text-xs font-bold text-white border border-white/20"
        :style="{ backgroundColor: role?.color || '#3b82f6' }"
        :title="role?.name || message.senderName"
      >
        {{ (role?.name || message.senderName).slice(0, 1) }}
      </div>
    </div>

    <!-- Message Body Container -->
    <div
      class="msg-bubble-wrap flex flex-col max-w-[85%] md:max-w-[78%]"
      :class="[isUser ? 'items-end' : 'items-start']"
    >
      <!-- Sender Metadata Header -->
      <div class="msg-meta-header flex items-center gap-2 mb-1 px-1 text-xs text-[var(--color-text-muted)] select-none">
        <template v-if="!isUser">
          <span v-if="isOrchestrator" class="font-semibold text-blue-500 flex items-center gap-1">
            <span>平台中控</span>
            <span class="text-[10px] px-1 py-0.2 rounded bg-blue-500/10 border border-blue-500/20 font-mono">调度中枢</span>
          </span>
          <FlowRoleBadge
            v-else
            :role="role"
            :name="message.senderName"
          />
        </template>
        <span class="time-stamp text-[11px] tabular-nums font-mono opacity-80">
          {{ formatTime(message.createdAt) }}
        </span>
        <span
          v-if="message.status === 'streaming'"
          class="inline-block w-1.5 h-1.5 rounded-full bg-blue-500 animate-ping"
          title="生成中"
        />
      </div>

      <!-- Bubble Box -->
      <div
        class="msg-bubble p-3.5 rounded-[var(--radius,6px)] border text-xs md:text-sm leading-relaxed"
        :class="[
          isUser
            ? 'bg-[var(--color-primary)] text-white border-[var(--color-primary)] rounded-tr-none'
            : isOrchestrator
              ? 'bg-[var(--color-surface)] border-blue-500/20 text-[var(--color-text)] rounded-tl-none shadow-sm'
              : 'bg-[var(--color-surface)] border-[var(--color-border)] text-[var(--color-text)] rounded-tl-none'
        ]"
      >
        <!-- Completion Announcement Pill if finished -->
        <div
          v-if="isCompletedFlag"
          class="mb-2.5 p-2 rounded bg-emerald-500/10 border border-emerald-500/30 text-emerald-600 dark:text-emerald-400 font-semibold text-xs flex items-center gap-2 select-none"
        >
          <svg viewBox="0 0 16 16" width="13" height="13" fill="none" stroke="currentColor" stroke-width="2">
            <polyline points="3 8.5 6.5 12 13 4.5" />
          </svg>
          <span>多 Agent 协同流程已执行达成总目标</span>
        </div>

        <!-- Markdown Content -->
        <div class="msg-content markdown-body select-text" v-html="formattedContent" />

        <!-- Attached Session Card if coding agent was spawned -->
        <div
          v-if="message.sessionId"
          class="mt-3 pt-2.5 border-t border-[var(--color-border)] flex items-center justify-between gap-3 flex-wrap"
        >
          <div class="flex items-center gap-1.5 text-xs text-[var(--color-text-muted)] font-mono">
            <svg viewBox="0 0 16 16" width="12" height="12" fill="none" stroke="currentColor" stroke-width="1.6">
              <rect x="2.5" y="2.5" width="11" height="11" rx="1.5" />
              <line x1="5.5" y1="6" x2="10.5" y2="6" />
            </svg>
            <span>工作区会话: {{ message.sessionId }}</span>
          </div>
          <button
            type="button"
            class="text-xs text-[var(--color-primary)] hover:underline inline-flex items-center gap-1 font-medium transition-colors"
            @click="handleOpenSession"
          >
            <span>查看执行日志与变更</span>
            <svg viewBox="0 0 16 16" width="11" height="11" fill="none" stroke="currentColor" stroke-width="2">
              <polyline points="6 3 11 8 6 13" />
            </svg>
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.msg-bubble :deep(pre) {
  margin: 0.5rem 0;
  padding: 0.75rem;
  background-color: var(--color-surface-muted, #1a1e27);
  border-radius: var(--radius, 4px);
  border: 1px solid var(--color-border, #282e3d);
  font-family: monospace;
  font-size: 11px;
  overflow-x: auto;
}

.msg-bubble :deep(code) {
  font-family: monospace;
  font-size: 11px;
}

.flow-msg-item.flex-row-reverse .msg-bubble :deep(p) {
  color: #fff;
}
</style>
