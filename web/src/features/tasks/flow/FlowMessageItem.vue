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

// Highlight @mentions in raw markdown content without corrupting code blocks
const formattedContent = computed(() => {
  let text = props.message.content || ''
  // If streaming and empty
  if (!text && props.message.status === 'streaming') {
    return '<span class="inline-block animate-pulse text-[var(--color-text-muted)] font-mono text-xs">正在思考中…</span>'
  }

  // Pre-process @role mentions into highlighted spans before markdown rendering,
  // but protect code blocks (both fenced ``` and inline `) from being replaced.
  if (props.roles && props.roles.length > 0) {
    const codeBlocks: string[] = []
    // 1. Temporarily stash fenced code blocks and inline code
    text = text.replace(/(```[\s\S]*?```|`[^`\n]+`)/g, (match) => {
      codeBlocks.push(match)
      return `__CODE_BLOCK_PLACEHOLDER_${codeBlocks.length - 1}__`
    })

    // 2. Replace @mentions in ordinary markdown text
    for (const r of props.roles) {
      const regex = new RegExp(`@(${r.name})`, 'g')
      text = text.replace(regex, `<span class="mention-role-tag font-semibold text-[var(--color-primary)] bg-[var(--color-primary)]/10 px-1 py-0.5 rounded border border-[var(--color-primary)]/20">@$1</span>`)
    }

    // 3. Restore code blocks verbatim
    text = text.replace(/__CODE_BLOCK_PLACEHOLDER_(\d+)__/g, (_, idx) => {
      return codeBlocks[Number(idx)] || ''
    })
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

async function handleMarkdownClick(event: MouseEvent) {
  const target = event.target as HTMLElement | null
  const copyBtn = target?.closest('.code-copy-btn') as HTMLButtonElement | null
  if (!copyBtn) return

  event.preventDefault()
  event.stopPropagation()

  const wrapper = copyBtn.closest('.code-block-wrapper')
  const pre = wrapper?.querySelector('pre')
  const code = pre?.querySelector('code')
  const text = (code?.textContent || pre?.textContent || '').replace(/\n$/, '')
  if (!text) return

  try {
    if (navigator.clipboard && navigator.clipboard.writeText) {
      await navigator.clipboard.writeText(text)
    } else {
      const textarea = document.createElement('textarea')
      textarea.value = text
      textarea.style.position = 'fixed'
      textarea.style.opacity = '0'
      document.body.appendChild(textarea)
      textarea.select()
      document.execCommand('copy')
      document.body.removeChild(textarea)
    }
    copyBtn.classList.add('is-copied')
    window.setTimeout(() => {
      copyBtn.classList.remove('is-copied')
    }, 2000)
  } catch (err) {
    console.error('Failed to copy code snippet:', err)
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
      <span class="text-3xs opacity-70 ml-1">{{ formatTime(message.createdAt) }}</span>
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
            <span class="text-3xs px-1 py-0.2 rounded bg-blue-500/10 border border-blue-500/20 font-mono">AI内置大脑</span>
          </span>
          <FlowRoleBadge
            v-else
            :role="role"
            :name="message.senderName"
          />
        </template>
        <span class="time-stamp text-2xs tabular-nums font-mono opacity-80">
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
        <div class="msg-content markdown-body select-text" v-html="formattedContent" @click="handleMarkdownClick" />

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
/* 1. Markdown General Typography */
.markdown-body :deep(p) {
  margin: 0 0 0.5rem;
  line-height: 1.6;
}

.markdown-body :deep(p:last-child) {
  margin-bottom: 0;
}

.markdown-body :deep(h1),
.markdown-body :deep(h2),
.markdown-body :deep(h3),
.markdown-body :deep(h4) {
  margin: 0.75rem 0 0.35rem;
  font-weight: 600;
  line-height: 1.35;
  color: var(--color-text);
}

.markdown-body :deep(h1) {
  font-size: 1.15rem;
  border-bottom: 1px solid var(--color-border);
  padding-bottom: 0.3rem;
}

.markdown-body :deep(h2) {
  font-size: 1.05rem;
}

.markdown-body :deep(h3) {
  font-size: 0.95rem;
}

.markdown-body :deep(h4) {
  font-size: 0.88rem;
}

/* 2. Lists */
.markdown-body :deep(ul),
.markdown-body :deep(ol) {
  margin: 0.25rem 0 0.5rem;
  padding-left: 1.35rem;
}

.markdown-body :deep(ul) {
  list-style-type: disc;
}

.markdown-body :deep(ol) {
  list-style-type: decimal;
}

.markdown-body :deep(li) {
  margin-bottom: 0.2rem;
  line-height: 1.5;
}

.markdown-body :deep(li > p) {
  margin-bottom: 0.2rem;
}

/* 3. Inline Code and Code Blocks */
.markdown-body :deep(code) {
  background: var(--color-surface-muted, rgba(125, 125, 125, 0.12));
  border: 1px solid var(--color-border, rgba(125, 125, 125, 0.2));
  border-radius: 4px;
  padding: 0.1rem 0.35rem;
  font-family: var(--font-mono, monospace);
  font-size: 0.85em;
  color: var(--color-text);
}

.markdown-body :deep(pre) {
  margin: 0.5rem 0;
  padding: 0.75rem;
  background-color: var(--color-surface-muted, #1a1e27);
  border-radius: var(--radius, 6px);
  border: 1px solid var(--color-border, #282e3d);
  font-family: var(--font-mono, monospace);
  font-size: var(--text-size-12);
  line-height: 1.45;
  overflow-x: auto;
}

.markdown-body :deep(pre code) {
  background: transparent;
  border: none;
  padding: 0;
  color: inherit;
  font-size: inherit;
  white-space: pre;
}

/* Code block wrapper with floating copy button */
.markdown-body :deep(.code-block-wrapper) {
  position: relative;
  margin: 0.5rem 0;
}

.markdown-body :deep(.code-block-wrapper pre) {
  margin: 0;
  padding-top: 1.85rem;
}

.markdown-body :deep(.code-copy-btn) {
  position: absolute;
  top: 0.35rem;
  right: 0.35rem;
  z-index: 2;
  display: inline-flex;
  align-items: center;
  gap: 0.25rem;
  padding: 0.2rem 0.45rem;
  font-size: var(--text-size-meta);
  line-height: 1;
  color: var(--color-text-muted);
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: 4px;
  cursor: pointer;
  transition: all 0.15s ease-in-out;
  user-select: none;
}

.markdown-body :deep(.code-copy-btn:hover) {
  background: var(--color-surface-muted);
  color: var(--color-text);
  border-color: var(--color-border-strong, var(--color-primary));
}

.markdown-body :deep(.code-copy-btn .code-copy-icon) {
  width: 0.82rem;
  height: 0.82rem;
  flex-shrink: 0;
}

.markdown-body :deep(.code-copy-btn .code-copy-icon-copied),
.markdown-body :deep(.code-copy-btn .code-copied-text) {
  display: none;
}

.markdown-body :deep(.code-copy-btn.is-copied) {
  background: var(--color-success-soft, rgba(22, 163, 74, 0.1));
  border-color: var(--color-success, #16a34a);
  color: var(--color-success, #16a34a);
}

.markdown-body :deep(.code-copy-btn.is-copied .code-copy-icon-idle),
.markdown-body :deep(.code-copy-btn.is-copied .code-copy-text) {
  display: none;
}

.markdown-body :deep(.code-copy-btn.is-copied .code-copy-icon-copied) {
  display: inline-block;
  color: var(--color-success, #16a34a);
}

.markdown-body :deep(.code-copy-btn.is-copied .code-copied-text) {
  display: inline;
}

/* 4. Blockquotes */
.markdown-body :deep(blockquote) {
  margin: 0.5rem 0;
  padding: 0.35rem 0.75rem;
  border-left: 3px solid var(--color-primary);
  background: var(--color-primary-soft, rgba(37, 99, 235, 0.05));
  color: var(--color-text-muted);
  border-radius: 0 4px 4px 0;
}

/* 5. Tables */
.markdown-body :deep(table) {
  width: 100%;
  border-collapse: collapse;
  margin: 0.5rem 0;
  font-size: 0.85em;
}

.markdown-body :deep(th),
.markdown-body :deep(td) {
  border: 1px solid var(--color-border);
  padding: 0.4rem 0.6rem;
  text-align: left;
}

.markdown-body :deep(th) {
  background: var(--color-surface-muted);
  font-weight: 600;
}

/* 6. Links */
.markdown-body :deep(a) {
  color: var(--color-primary);
  text-decoration: underline;
  text-underline-offset: 2px;
}

/* 7. User Bubble Color Overrides (white text on primary background) */
.flow-msg-item.flex-row-reverse .msg-bubble :deep(p),
.flow-msg-item.flex-row-reverse .msg-bubble :deep(li),
.flow-msg-item.flex-row-reverse .msg-bubble :deep(h1),
.flow-msg-item.flex-row-reverse .msg-bubble :deep(h2),
.flow-msg-item.flex-row-reverse .msg-bubble :deep(h3),
.flow-msg-item.flex-row-reverse .msg-bubble :deep(h4) {
  color: #ffffff;
}

.flow-msg-item.flex-row-reverse .msg-bubble :deep(code) {
  background: rgba(255, 255, 255, 0.2);
  border-color: rgba(255, 255, 255, 0.3);
  color: #ffffff;
}

.flow-msg-item.flex-row-reverse .msg-bubble :deep(a) {
  color: #ffffff;
  text-decoration: underline;
}
</style>
