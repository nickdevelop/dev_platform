<script setup lang="ts">
// SettingsPromptsView: Global Prompt Management in Platform Settings.
// Conforms to design-taste-frontend (App Screens) guidelines:
// flat surfaces, quiet borders, zero resting shadows, data-first typography.
import { computed, onMounted, ref } from 'vue'
import { usePromptsStore } from '../../stores/prompts'
import type { Prompt, PromptType } from '../../api/prompts'
import PromptManageDialog from '../prompts/PromptManageDialog.vue'
import { hasUserPromptPlaceholder } from '../prompts/promptTemplate'
import { UiButton, UiInput } from '../../shared/ui'

const promptsStore = usePromptsStore()

const keyword = ref('')
const activeTab = ref<'all' | 'role' | 'flow'>('all')
const showEditDialog = ref(false)
const editingPrompt = ref<Prompt | null>(null)
const copiedId = ref<number | null>(null)

onMounted(async () => {
  if (promptsStore.prompts.length === 0) {
    await promptsStore.loadPrompts()
  }
})

const roleCount = computed(() => (promptsStore.prompts || []).filter((p) => p.type === 'role').length)
const flowCount = computed(() => (promptsStore.prompts || []).filter((p) => p.type === 'flow').length)

const filteredPrompts = computed(() => {
  let list = promptsStore.prompts || []
  if (activeTab.value !== 'all') {
    list = list.filter((p) => p.type === activeTab.value)
  }
  const kw = keyword.value.trim().toLowerCase()
  if (!kw) return list
  return list.filter(
    (p) =>
      p.name.toLowerCase().includes(kw) ||
      (p.description && p.description.toLowerCase().includes(kw)) ||
      p.content.toLowerCase().includes(kw),
  )
})

function openCreateDialog(defaultType: PromptType = 'role') {
  editingPrompt.value = null
  showEditDialog.value = true
}

function openEditDialog(prompt: Prompt) {
  editingPrompt.value = prompt
  showEditDialog.value = true
}

async function handleDeletePrompt(prompt: Prompt) {
  if (!confirm(`确定要删除提示词「${prompt.name}」吗？`)) return
  try {
    await promptsStore.deletePrompt(prompt.id)
  } catch (err: any) {
    alert(err.message || '删除提示词失败')
  }
}

async function copyPromptContent(prompt: Prompt) {
  try {
    await navigator.clipboard.writeText(prompt.content)
    copiedId.value = prompt.id
    setTimeout(() => {
      if (copiedId.value === prompt.id) copiedId.value = null
    }, 2000)
  } catch {
    // Ignore clipboard error
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
</script>

<template>
  <section class="settings-prompts-view w-full space-y-6" aria-label="提示词管理">
    <!-- Header Summary Panel -->
    <header class="settings-head pb-4 border-b border-[var(--color-border)] flex items-start justify-between gap-4 flex-wrap">
      <div>
        <h1 class="text-[22px] font-semibold text-[var(--color-text)] tracking-tight">提示词管理</h1>
        <p class="text-xs text-[var(--color-text-muted)] mt-1">
          管理角色类人设与流程类协同模板，供角色管理、对话助手与多 Agent 协同流程统一引用。
        </p>
      </div>

      <div class="flex items-center gap-3">
        <UiButton
          variant="primary"
          class="text-xs px-3.5 py-1.5"
          data-test="btn-create-prompt"
          @click="openCreateDialog(activeTab === 'flow' ? 'flow' : 'role')"
        >
          <svg viewBox="0 0 16 16" width="12" height="12" fill="none" stroke="currentColor" stroke-width="2" class="mr-1.5 inline-block">
            <line x1="8" y1="3" x2="8" y2="13" />
            <line x1="3" y1="8" x2="13" y2="8" />
          </svg>
          新建提示词
        </UiButton>
      </div>
    </header>

    <!-- Filter & Segmented Tabs Bar -->
    <div class="flex items-center justify-between gap-4 flex-wrap">
      <!-- Tabs for All / Role / Flow -->
      <div class="flex items-center gap-1 p-0.5 rounded-[var(--radius,6px)] bg-[var(--color-surface-muted)] border border-[var(--color-border)] text-xs">
        <button
          type="button"
          class="px-3 py-1 rounded transition-colors font-medium"
          :class="[
            activeTab === 'all'
              ? 'bg-[var(--color-surface)] text-[var(--color-text)] shadow-sm'
              : 'text-[var(--color-text-muted)] hover:text-[var(--color-text)]',
          ]"
          @click="activeTab = 'all'"
        >
          全部 ({{ promptsStore.prompts.length }})
        </button>
        <button
          type="button"
          class="px-3 py-1 rounded transition-colors font-medium flex items-center gap-1.5"
          :class="[
            activeTab === 'role'
              ? 'bg-[var(--color-surface)] text-[var(--color-text)] shadow-sm'
              : 'text-[var(--color-text-muted)] hover:text-[var(--color-text)]',
          ]"
          @click="activeTab = 'role'"
        >
          <span class="w-1.5 h-1.5 rounded-full bg-blue-500" />
          <span>角色类 ({{ roleCount }})</span>
        </button>
        <button
          type="button"
          class="px-3 py-1 rounded transition-colors font-medium flex items-center gap-1.5"
          :class="[
            activeTab === 'flow'
              ? 'bg-[var(--color-surface)] text-[var(--color-text)] shadow-sm'
              : 'text-[var(--color-text-muted)] hover:text-[var(--color-text)]',
          ]"
          @click="activeTab = 'flow'"
        >
          <span class="w-1.5 h-1.5 rounded-full bg-emerald-500" />
          <span>流程类 ({{ flowCount }})</span>
        </button>
      </div>

      <!-- Search Input -->
      <div class="flex items-center gap-2 max-w-sm flex-1">
        <UiInput
          v-model="keyword"
          placeholder="搜索提示词名称、描述或正文…"
          class="text-xs"
          block
        />
      </div>
    </div>

    <!-- Prompts List Grid -->
    <div v-if="filteredPrompts.length > 0" class="grid grid-cols-1 lg:grid-cols-2 gap-4">
      <div
        v-for="prompt in filteredPrompts"
        :key="prompt.id"
        class="prompt-card p-4 rounded-[var(--radius,6px)] bg-[var(--color-surface)] border border-[var(--color-border)] flex flex-col justify-between gap-3 hover:border-[var(--color-primary)] transition-colors"
        :data-test="`prompt-card-${prompt.id}`"
      >
        <div class="space-y-2">
          <!-- Title Row & Badges -->
          <div class="flex items-start justify-between gap-2">
            <div class="flex items-center gap-2 min-w-0 flex-wrap">
              <span class="text-sm font-semibold text-[var(--color-text)] truncate">
                {{ prompt.name }}
              </span>
              <!-- Type Badge -->
              <span
                class="text-[11px] px-1.5 py-0.5 rounded font-mono border"
                :class="[
                  prompt.type === 'role'
                    ? 'bg-blue-500/10 text-blue-600 dark:text-blue-400 border-blue-500/20'
                    : 'bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 border-emerald-500/20',
                ]"
              >
                {{ prompt.type === 'role' ? '角色类' : '流程类' }}
              </span>
              <!-- Placeholder Badge -->
              <span
                v-if="hasUserPromptPlaceholder(prompt.content)"
                class="text-[10px] px-1.5 py-0.5 rounded bg-[var(--color-surface-muted)] text-[var(--color-text-muted)] font-mono border border-[var(--color-border)]"
                title="包含 {user_prompt} 占位符"
              >
                {user_prompt}
              </span>
            </div>

            <div class="flex items-center gap-1.5 flex-shrink-0">
              <button
                type="button"
                class="text-xs text-[var(--color-text-muted)] hover:text-[var(--color-text)] px-1.5 py-0.5 rounded transition-colors"
                title="编辑提示词"
                @click="openEditDialog(prompt)"
              >
                编辑
              </button>
              <button
                type="button"
                class="text-xs text-red-500 hover:text-red-600 px-1.5 py-0.5 rounded transition-colors"
                title="删除提示词"
                @click="handleDeletePrompt(prompt)"
              >
                删除
              </button>
            </div>
          </div>

          <!-- Description -->
          <p v-if="prompt.description" class="text-xs text-[var(--color-text-muted)] line-clamp-2 leading-relaxed">
            {{ prompt.description }}
          </p>

          <!-- Content Preview Snippet -->
          <div class="text-xs font-mono text-[var(--color-text-muted)] bg-[var(--color-surface-muted)] p-2.5 rounded border border-[var(--color-border)] line-clamp-3 whitespace-pre-wrap leading-relaxed select-text">
            {{ prompt.content }}
          </div>
        </div>

        <!-- Footer Meta & Copy -->
        <div class="pt-2 border-t border-[var(--color-border)] flex items-center justify-between text-[11px] text-[var(--color-text-muted)] font-mono">
          <span>更新: {{ formatDate(prompt.updatedAt || prompt.createdAt) }}</span>
          <button
            type="button"
            class="hover:text-[var(--color-text)] transition-colors flex items-center gap-1"
            @click="copyPromptContent(prompt)"
          >
            <span>{{ copiedId === prompt.id ? '已复制！' : '复制内容' }}</span>
          </button>
        </div>
      </div>
    </div>

    <!-- Empty State -->
    <div
      v-else
      class="p-12 text-center rounded-[var(--radius,6px)] border border-dashed border-[var(--color-border)] bg-[var(--color-surface)]/40 space-y-3"
    >
      <div class="text-sm font-medium text-[var(--color-text)]">
        {{ activeTab === 'all' ? '尚未创建任何提示词模版' : activeTab === 'role' ? '暂无角色类提示词' : '暂无流程类提示词' }}
      </div>
      <p class="text-xs text-[var(--color-text-muted)] max-w-md mx-auto">
        {{ activeTab === 'role' ? '角色类提示词用于设定 Agent 人设、专业背景与思考准则，可在角色管理与对话助手中引用。' : '流程类提示词偏向工作流程与协同任务规范，可在多 Agent 协同流程中引用。' }}
      </p>
      <UiButton
        variant="primary"
        class="text-xs px-3.5 py-1.5 mt-2"
        @click="openCreateDialog(activeTab === 'flow' ? 'flow' : 'role')"
      >
        新建提示词模版
      </UiButton>
    </div>

    <!-- Create / Edit Dialog -->
    <PromptManageDialog
      v-if="showEditDialog"
      :prompt="editingPrompt"
      :default-type="activeTab === 'flow' ? 'flow' : 'role'"
      @close="showEditDialog = false"
      @saved="showEditDialog = false"
    />
  </section>
</template>

<style scoped>
.prompt-card {
  min-height: 150px;
}
</style>
