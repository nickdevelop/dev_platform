<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import type { Prompt } from '../../api/prompts'
import { usePromptsStore } from '../../stores/prompts'
import PromptManageDialog from './PromptManageDialog.vue'
import { applyUserPromptTemplate, hasUserPromptPlaceholder } from './promptTemplate'
import { UiButton } from '../../shared/ui'

const route = useRoute()
const router = useRouter()
const promptsStore = usePromptsStore()

const showEditDialog = ref(false)
const testInput = ref('')
const copiedTemplate = ref(false)
const copiedMerged = ref(false)

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

const previewMergedText = computed(() => {
  if (!activePrompt.value) return ''
  return applyUserPromptTemplate(activePrompt.value.content, testInput.value)
})

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

async function copyText(text: string, type: 'template' | 'merged') {
  try {
    await navigator.clipboard.writeText(text)
    if (type === 'template') {
      copiedTemplate.value = true
      setTimeout(() => (copiedTemplate.value = false), 2000)
    } else {
      copiedMerged.value = true
      setTimeout(() => (copiedMerged.value = false), 2000)
    }
  } catch {
    // Ignore clipboard errors
  }
}

function fillExampleCode() {
  testInput.value = 'function calculateTotal(items) {\n  return items.reduce((sum, item) => sum + item.price, 0);\n}'
}

function clearTestInput() {
  testInput.value = ''
}
</script>

<template>
  <div class="prompts-view h-full overflow-y-auto bg-[var(--color-bg)]" aria-label="提示词详情">
    <!-- Active Prompt Studio Layout -->
    <div v-if="activePrompt" class="prompt-detail-container p-6 md:p-8 space-y-6 max-w-7xl mx-auto">
      <!-- 1. Header Summary Panel -->
      <header class="detail-header p-6 rounded-[var(--radius,6px)] bg-[var(--color-surface)] border border-[var(--color-border)] flex items-start justify-between gap-6 flex-wrap">
        <div class="header-main flex-1 min-w-[280px] space-y-3">
          <div class="header-title-row flex items-center gap-3 flex-wrap">
            <h1 class="text-xl md:text-2xl font-semibold text-[var(--color-text)] tracking-tight">
              {{ activePrompt.name }}
            </h1>
            <span
              v-if="hasUserPromptPlaceholder(activePrompt.content)"
              class="placeholder-badge inline-flex items-center gap-1 font-mono text-[11px] px-2 py-0.5 rounded bg-[var(--color-surface-muted)] text-[var(--color-primary)] border border-[var(--color-border)]"
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
          </div>
        </div>

        <!-- Action buttons -->
        <div class="header-actions flex items-center gap-2.5 shrink-0">
          <UiButton
            variant="ghost"
            class="action-btn copy-btn text-xs"
            title="复制模版原始内容"
            @click="copyText(activePrompt.content, 'template')"
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

      <!-- 2. Dual Studio: Split Columns (1:1 on desktop, stacked on mobile) -->
      <div class="detail-body grid grid-cols-1 lg:grid-cols-2 gap-6 items-start">
        <!-- Left: Prompt Template Source Card -->
        <section class="section-card template-section p-6 rounded-[var(--radius,6px)] bg-[var(--color-surface)] border border-[var(--color-border)] flex flex-col gap-4">
          <div class="section-head flex items-center justify-between pb-3 border-b border-[var(--color-border)]">
            <div class="flex items-center gap-2">
              <svg class="text-[var(--color-text-muted)]" viewBox="0 0 16 16" width="14" height="14" fill="none" stroke="currentColor" stroke-width="1.6" aria-hidden="true">
                <rect x="2.5" y="2.5" width="11" height="11" rx="1.5" />
                <line x1="5.5" y1="6" x2="10.5" y2="6" />
                <line x1="5.5" y1="9" x2="8.5" y2="9" />
              </svg>
              <h2 class="text-sm font-semibold text-[var(--color-text)]">提示词模版</h2>
            </div>
            <span class="text-xs text-[var(--color-text-muted)] font-mono tabular-nums">
              {{ activePrompt.content.split('\n').length }} 行
            </span>
          </div>

          <p class="text-xs text-[var(--color-text-muted)] leading-relaxed">
            用于向 Agent 注入标准化系统上下文或任务规则。若包含 <code>{user_prompt}</code>，在新建会话或任务调度时将自动替换为具体输入。
          </p>

          <div class="template-content-box rounded-[var(--radius,6px)] bg-[var(--color-surface-muted)] border border-[var(--color-border)] overflow-hidden">
            <pre class="template-pre p-4 text-xs font-mono text-[var(--color-text)] whitespace-pre-wrap leading-relaxed overflow-x-auto select-text">{{ activePrompt.content }}</pre>
          </div>
        </section>

        <!-- Right: Real-time Live Sandbox & Preview -->
        <section class="section-card preview-section p-6 rounded-[var(--radius,6px)] bg-[var(--color-surface)] border border-[var(--color-border)] flex flex-col gap-4">
          <div class="section-head flex items-center justify-between pb-3 border-b border-[var(--color-border)]">
            <div class="flex items-center gap-2">
              <svg class="text-[var(--color-primary)]" viewBox="0 0 16 16" width="14" height="14" fill="none" stroke="currentColor" stroke-width="1.6" aria-hidden="true">
                <polygon points="8 2 13 5 13 11 8 14 3 11 3 5 8 2" />
                <line x1="8" y1="2" x2="8" y2="14" />
              </svg>
              <h2 class="text-sm font-semibold text-[var(--color-text)]">填充效果模拟测试</h2>
            </div>
            <span class="text-xs text-[var(--color-text-muted)] font-medium">实时合成预览</span>
          </div>

          <div class="preview-grid space-y-4">
            <!-- Simulated Input -->
            <div class="preview-input-box space-y-2">
              <div class="flex items-center justify-between text-xs">
                <label for="test-user-input" class="font-medium text-[var(--color-text)]">
                  模拟用户输入：
                </label>
                <div class="flex items-center gap-2">
                  <button
                    type="button"
                    class="text-[11px] text-[var(--color-primary)] hover:underline"
                    @click="fillExampleCode"
                  >
                    填入示例
                  </button>
                  <span class="text-slate-300 dark:text-slate-700">|</span>
                  <button
                    v-if="testInput"
                    type="button"
                    class="text-[11px] text-[var(--color-text-muted)] hover:text-[var(--color-text)]"
                    @click="clearTestInput"
                  >
                    清空
                  </button>
                </div>
              </div>
              <textarea
                id="test-user-input"
                v-model="testInput"
                rows="4"
                class="w-full rounded-[var(--radius,6px)] bg-[var(--color-surface-muted)] border border-[var(--color-border)] p-3 text-xs font-mono text-[var(--color-text)] placeholder-[var(--color-text-muted)] outline-none focus:border-[var(--color-primary)] transition-colors leading-relaxed resize-y"
                placeholder="在此输入测试文本（如：用户发送的问题或代码）"
                data-test="prompt-test-input"
              ></textarea>
            </div>

            <!-- Merged Output -->
            <div class="preview-output-box space-y-2">
              <div class="flex items-center justify-between text-xs">
                <span class="font-medium text-[var(--color-text)]">
                  合成后发送给 Agent 的内容：
                </span>
                <button
                  type="button"
                  class="text-[11px] text-[var(--color-text-muted)] hover:text-[var(--color-text)] inline-flex items-center gap-1 transition-colors"
                  title="复制合成后的完整文本"
                  @click="copyText(previewMergedText, 'merged')"
                >
                  <svg viewBox="0 0 16 16" width="11" height="11" fill="none" stroke="currentColor" stroke-width="1.6">
                    <rect x="5" y="5" width="8" height="8" rx="1" />
                    <path d="M3 11V3h8" />
                  </svg>
                  <span>{{ copiedMerged ? '已复制' : '复制合成结果' }}</span>
                </button>
              </div>
              <div class="rounded-[var(--radius,6px)] bg-[var(--color-surface-muted)] border border-[var(--color-border)] overflow-hidden">
                <pre class="preview-output-pre p-4 text-xs font-mono text-[var(--color-text)] whitespace-pre-wrap leading-relaxed overflow-x-auto select-text min-h-[120px]" data-test="prompt-preview-output">{{ previewMergedText }}</pre>
              </div>
            </div>
          </div>
        </section>
      </div>

      <!-- Edit Dialog -->
      <PromptManageDialog
        v-if="showEditDialog"
        :prompt="activePrompt"
        @close="showEditDialog = false"
        @saved="onSaved"
      />
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
        <UiButton
          variant="primary"
          class="btn-primary-create text-xs px-4 py-2 font-medium"
          @click="showEditDialog = true"
        >
          + 新建提示词
        </UiButton>
      </div>

      <PromptManageDialog
        v-if="showEditDialog"
        @close="showEditDialog = false"
        @saved="onSaved"
      />
    </div>
  </div>
</template>

<style scoped>
.prompts-view {
  box-sizing: border-box;
}
</style>
