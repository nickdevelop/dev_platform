<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import type { Prompt } from '../../api/prompts'
import { usePromptsStore } from '../../stores/prompts'
import PromptManageDialog from './PromptManageDialog.vue'
import { hasUserPromptPlaceholder } from './promptTemplate'
import { UiButton } from '../../shared/ui'

const route = useRoute()
const router = useRouter()
const promptsStore = usePromptsStore()

const showEditDialog = ref(false)
const copiedTemplate = ref(false)

onMounted(async () => {
  if (promptsStore.prompts.length === 0) {
    await promptsStore.loadPrompts()
  }
})

const activePromptId = computed<number | null>(() => {
  if (route.params.promptId !== undefined) {
    return Number(route.params.promptId)
  }
  return promptsStore.prompts.length > 0 ? promptsStore.prompts[0].id : null
})

const activePrompt = computed<Prompt | null>(() => {
  if (!activePromptId.value) return null
  return promptsStore.byId(activePromptId.value) ?? null
})

watch(
  () => activePromptId.value,
  async (id) => {
    if (id && !promptsStore.byId(id)) {
      try {
        await promptsStore.loadPrompt(id)
      } catch {
        // If not found, stay null
      }
    }
  },
  { immediate: true },
)

async function deleteCurrentPrompt() {
  if (!activePrompt.value) return
  if (!confirm(`确定要删除提示词「${activePrompt.value.name}」吗？`)) return
  const idToDelete = activePrompt.value.id
  await promptsStore.deletePrompt(idToDelete)
  void router.push({ name: 'prompts' })
}

function onSaved(updated: Prompt) {
  showEditDialog.value = false
  if (activePromptId.value !== updated.id) {
    void router.push({ name: 'prompt-detail', params: { promptId: updated.id } })
  }
}

function formatDate(iso: string): string {
  if (!iso) return ''
  try {
    const d = new Date(iso)
    return d.toLocaleString('zh-CN', {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      hour12: false,
    })
  } catch {
    return iso
  }
}

async function copyTemplate(text: string) {
  try {
    await navigator.clipboard.writeText(text)
    copiedTemplate.value = true
    setTimeout(() => (copiedTemplate.value = false), 2000)
  } catch {
    // Ignore clipboard errors
  }
}
</script>

<template>
  <div class="prompts-view h-full overflow-y-auto bg-[var(--color-bg)]" aria-label="提示词详情">
    <!-- Active Prompt Studio Layout -->
    <div v-if="activePrompt" class="prompt-detail-container p-6 md:p-8 space-y-6 max-w-5xl mx-auto">
      <!-- 1. Header Summary Panel -->
      <header class="detail-header p-6 rounded-[var(--radius,6px)] bg-[var(--color-surface)] border border-[var(--color-border)] flex items-start justify-between gap-6 flex-wrap">
        <div class="header-main flex-1 min-w-[280px] space-y-3">
          <div class="header-title-row flex items-center gap-3 flex-wrap">
            <h1 class="text-xl md:text-2xl font-semibold text-[var(--color-text)] tracking-tight">
              {{ activePrompt.name }}
            </h1>
            <span
              v-if="hasUserPromptPlaceholder(activePrompt.content)"
              class="placeholder-badge inline-flex items-center gap-1 font-mono text-2xs px-2 py-0.5 rounded bg-[var(--color-surface-muted)] text-[var(--color-primary)] border border-[var(--color-border)]"
            >
              <span>包含 {user_prompt} 占位符</span>
            </span>
          </div>

          <p v-if="activePrompt.description" class="header-desc text-xs md:text-sm text-[var(--color-text-muted)] leading-relaxed">
            {{ activePrompt.description }}
          </p>

          <div class="header-meta flex items-center gap-5 text-xs text-[var(--color-text-muted)] pt-1 flex-wrap font-mono tabular-nums">
            <span>创建时间: {{ formatDate(activePrompt.createdAt) }}</span>
            <span v-if="activePrompt.updatedAt">更新时间: {{ formatDate(activePrompt.updatedAt) }}</span>
            <span>字符数: {{ activePrompt.content.length }}</span>
            <span>行数: {{ activePrompt.content.split('\n').length }}</span>
          </div>
        </div>

        <!-- Action buttons -->
        <div class="header-actions flex items-center gap-2.5 shrink-0">
          <UiButton
            variant="ghost"
            class="action-btn copy-btn text-xs"
            title="复制模版原始内容"
            data-test="copy-template-btn"
            @click="copyTemplate(activePrompt.content)"
          >
            <svg viewBox="0 0 16 16" width="12" height="12" fill="none" stroke="currentColor" stroke-width="1.6" class="mr-1 inline-block">
              <rect x="5" y="5" width="8" height="8" rx="1" />
              <path d="M3 11V3h8" />
            </svg>
            <span>{{ copiedTemplate ? '已复制' : '复制模版' }}</span>
          </UiButton>
          <UiButton
            variant="secondary"
            class="action-btn edit-btn text-xs px-3 py-1.5"
            data-test="edit-prompt-btn"
            @click="showEditDialog = true"
          >
            编辑
          </UiButton>
          <UiButton
            variant="danger"
            class="action-btn delete-btn text-xs px-3 py-1.5"
            data-test="delete-prompt-btn"
            @click="deleteCurrentPrompt"
          >
            删除
          </UiButton>
        </div>
      </header>

      <!-- 2. Single Full-width Template Content Section -->
      <section class="section-card template-section p-6 rounded-[var(--radius,6px)] bg-[var(--color-surface)] border border-[var(--color-border)] flex flex-col gap-4">
        <div class="section-head flex items-center justify-between pb-3 border-b border-[var(--color-border)]">
          <div class="flex items-center gap-2">
            <svg class="text-[var(--color-text-muted)]" viewBox="0 0 16 16" width="14" height="14" fill="none" stroke="currentColor" stroke-width="1.6" aria-hidden="true">
              <rect x="2.5" y="2.5" width="11" height="11" rx="1.5" />
              <line x1="5.5" y1="6" x2="10.5" y2="6" />
              <line x1="5.5" y1="9" x2="8.5" y2="9" />
            </svg>
            <h2 class="text-sm font-semibold text-[var(--color-text)]">提示词模版正文</h2>
          </div>
          <span class="text-xs text-[var(--color-text-muted)] font-mono tabular-nums">
            {{ activePrompt.content.length }} 字符
          </span>
        </div>

        <p class="text-xs text-[var(--color-text-muted)] leading-relaxed">
          用于向 Agent 注入标准化系统上下文或任务规则。若包含 <code>{user_prompt}</code>，在新建会话或任务调度时将自动替换为具体输入。
        </p>

        <div class="template-content-box rounded-[var(--radius,6px)] bg-[var(--color-surface-muted)] border border-[var(--color-border)] overflow-hidden">
          <pre class="template-pre p-5 text-xs font-mono text-[var(--color-text)] whitespace-pre-wrap leading-relaxed overflow-x-auto select-text">{{ activePrompt.content }}</pre>
        </div>
      </section>

      <!-- Edit Dialog -->
      <Teleport to="body">
        <PromptManageDialog
          v-if="showEditDialog"
          :prompt="activePrompt"
          @close="showEditDialog = false"
          @saved="onSaved"
        />
      </Teleport>
    </div>

    <!-- Empty Workspace State -->
    <div v-else class="empty-view flex items-center justify-center h-full p-6 md:p-12 overflow-y-auto">
      <div class="max-w-xl w-full bg-[var(--color-surface)] border border-[var(--color-border)] rounded-[var(--radius,6px)] p-8 md:p-10 flex flex-col gap-6 text-center items-center">
        <div class="w-12 h-12 rounded-full bg-[var(--color-surface-muted)] border border-[var(--color-border)] flex items-center justify-center text-[var(--color-text-muted)]">
          <svg viewBox="0 0 24 24" width="22" height="22" fill="none" stroke="currentColor" stroke-width="1.6" aria-hidden="true">
            <path d="M12 20h9" />
            <path d="M16.5 3.5a2.121 2.121 0 0 1 3 3L7 19l-4 1 1-4L16.5 3.5z" />
          </svg>
        </div>
        <div class="space-y-2">
          <h2 class="text-xl font-semibold text-[var(--color-text)] tracking-tight">未选择提示词</h2>
          <p class="text-xs md:text-sm text-[var(--color-text-muted)] leading-relaxed max-w-md">
            从左侧列表中选择一个提示词查看模版详情，或新建提示词模版以提升会话与自动化任务的复用效率。
          </p>
        </div>
      </div>
    </div>
  </div>
</template>
