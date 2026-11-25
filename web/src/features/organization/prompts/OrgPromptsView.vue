<script setup lang="ts">
// OrgPromptsView: AI Prompt Management in Organization Center.
// Hosts global role and workflow prompt templates referenced across Assistants,
// Employees, and Teams.
// Follows design-taste-frontend (Data Terminal / App Screens) compact standards:
//   - Aligned with Org header structure (Title + Count Badge + Primary Action)
//   - Compact search and segmented filter bar (All / Role / Flow)
//   - Clean responsive card grid with preview snippets and copy action
import { computed, onMounted, ref } from 'vue'
import { usePromptsStore } from '../../../stores/prompts'
import type { Prompt, PromptType } from '../../../api/prompts'
import { PromptManageDialog } from '../../prompts'
import { UiButton, UiInput, UiConfirmDialog } from '../../../shared/ui'

const promptsStore = usePromptsStore()

const keyword = ref('')
const activeTab = ref<'all' | 'role' | 'flow'>('all')
const showEditDialog = ref(false)
const editingPrompt = ref<Prompt | null>(null)
const pendingDelete = ref<Prompt | null>(null)
const deleting = ref(false)
const deleteError = ref('')
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

function handleDeletePrompt(prompt: Prompt) {
  deleteError.value = ''
  pendingDelete.value = prompt
}

async function confirmDeletePrompt() {
  if (!pendingDelete.value || deleting.value) return
  const target = pendingDelete.value
  deleting.value = true
  deleteError.value = ''
  try {
    await promptsStore.deletePrompt(target.id)
    pendingDelete.value = null
  } catch (err: any) {
    deleteError.value = promptsStore.error || err.message || '删除提示词失败'
  } finally {
    deleting.value = false
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
  <div class="org-prompts-view h-full flex flex-col min-h-0 bg-[var(--color-bg)] overflow-hidden" data-test="org-prompts-view" aria-label="AI提示词">
    <!-- Top Header: Title + Count + Action Button -->
    <header class="px-6 py-4 border-b border-border bg-surface shrink-0">
      <div class="flex items-center justify-between gap-4 flex-wrap">
        <div>
          <div class="flex items-center gap-2.5">
            <h1 class="text-lg font-semibold text-foreground tracking-tight">AI提示词</h1>
            <span class="px-2 py-0.5 rounded-full text-2xs font-mono font-medium bg-surface-muted text-platform-text-muted border border-border">
              {{ promptsStore.prompts.length }} 个提示词
            </span>
          </div>
          <p class="text-xs text-platform-text-muted mt-0.5">
            管理角色类人设与流程类协同提示词，供助手、数字员工与协同团队统一引用。
          </p>
        </div>

        <UiButton
          variant="primary"
          class="shrink-0 text-xs px-3.5 py-1.5"
          data-test="btn-create-prompt"
          @click="openCreateDialog(activeTab === 'flow' ? 'flow' : 'role')"
        >
          <svg viewBox="0 0 16 16" width="12" height="12" fill="none" stroke="currentColor" stroke-width="2" class="mr-1.5 inline-block" aria-hidden="true">
            <line x1="8" y1="3" x2="8" y2="13" />
            <line x1="3" y1="8" x2="13" y2="8" />
          </svg>
          新建提示词
        </UiButton>
      </div>

      <!-- Filter & Search Toolbar -->
      <div class="flex flex-col sm:flex-row items-stretch sm:items-center justify-between gap-3 mt-4 pt-3 border-t border-border/50">
        <!-- Tabs for All / Role / Flow -->
        <div class="flex items-center gap-1 p-0.5 rounded-[var(--radius,6px)] bg-[var(--color-surface-muted)] border border-[var(--color-border)] text-xs self-start sm:self-auto">
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
            <span class="w-1.5 h-1.5 rounded-full bg-accent" />
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
        <div class="flex items-center gap-2 w-full sm:w-72">
          <UiInput
            v-model="keyword"
            placeholder="搜索提示词名称、描述或正文…"
            class="text-xs"
            block
          />
        </div>
      </div>
    </header>

    <!-- Main Content Area with Scroll -->
    <div class="flex-1 min-h-0 overflow-y-auto p-6">
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
                  class="text-2xs px-1.5 py-0.5 rounded font-mono border"
                  :class="[
                    prompt.type === 'role'
                      ? 'bg-accent/10 text-accent border-accent/20'
                      : 'bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 border-emerald-500/20',
                  ]"
                >
                  {{ prompt.type === 'role' ? '角色类' : '流程类' }}
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
          <div class="pt-2 border-t border-[var(--color-border)] flex items-center justify-between text-2xs text-[var(--color-text-muted)] font-mono">
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
          {{ activeTab === 'all' ? '尚未创建任何提示词' : activeTab === 'role' ? '暂无角色类提示词' : '暂无流程类提示词' }}
        </div>
        <p class="text-xs text-[var(--color-text-muted)] max-w-md mx-auto">
          {{ activeTab === 'role' ? '角色类提示词用于设定 Agent 人设、专业背景与思考准则，可在助手与数字员工中统一引用。' : '流程类提示词偏向工作流程与协同任务规范，可在多 Agent 协同团队中统一引用。' }}
        </p>
        <UiButton
          variant="primary"
          class="text-xs px-3.5 py-1.5 mt-2"
          @click="openCreateDialog(activeTab === 'flow' ? 'flow' : 'role')"
        >
          新建提示词
        </UiButton>
      </div>
    </div>

    <!-- Create / Edit Dialog -->
    <Teleport to="body">
      <PromptManageDialog
        v-if="showEditDialog"
        :prompt="editingPrompt"
        :default-type="activeTab === 'flow' ? 'flow' : 'role'"
        @close="showEditDialog = false"
        @saved="showEditDialog = false"
      />
    </Teleport>

    <UiConfirmDialog
      v-if="pendingDelete"
      title="删除提示词"
      :message="`确定要删除提示词「${pendingDelete.name}」吗？`"
      detail="引用该提示词的角色、助手或会话将无法继续读取其内容。"
      confirm-label="删除"
      :loading="deleting"
      :error="deleteError"
      @close="pendingDelete = null"
      @confirm="confirmDeletePrompt"
    />
  </div>
</template>

<style scoped>
.prompt-card {
  min-height: 150px;
}
</style>
