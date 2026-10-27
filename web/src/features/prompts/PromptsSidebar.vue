<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import type { Prompt } from '../../api/prompts'
import { usePromptsStore } from '../../stores/prompts'
import PromptManageDialog from './PromptManageDialog.vue'
import { hasUserPromptPlaceholder } from './promptTemplate'

const route = useRoute()
const router = useRouter()
const promptsStore = usePromptsStore()

const searchKeyword = ref('')
const showManageDialog = ref(false)

onMounted(async () => {
  await promptsStore.loadPrompts()
})

const activePromptId = computed<number | null>(() =>
  route.params.promptId !== undefined ? Number(route.params.promptId) : null,
)

const filteredPrompts = computed(() => {
  const q = searchKeyword.value.trim().toLowerCase()
  if (!q) return promptsStore.prompts
  return promptsStore.prompts.filter(
    (p) =>
      p.name.toLowerCase().includes(q) ||
      (p.description && p.description.toLowerCase().includes(q)) ||
      p.content.toLowerCase().includes(q),
  )
})

function selectPrompt(id: number) {
  void router.push({ name: 'prompt-detail', params: { promptId: id } })
}

function openCreateDialog() {
  showManageDialog.value = true
}

function onSaved(prompt: Prompt) {
  showManageDialog.value = false
  selectPrompt(prompt.id)
}
</script>

<template>
  <div class="prompts-sidebar" aria-label="提示词列表">
    <div class="sidebar-actions">
      <button
        type="button"
        class="action-btn new-prompt-btn"
        data-test="new-prompt-btn"
        @click="openCreateDialog"
      >
        + 新建提示词
      </button>
    </div>

    <input
      v-model="searchKeyword"
      type="search"
      class="search"
      placeholder="搜索提示词名称或描述"
      aria-label="搜索提示词名称或描述"
      data-test="prompt-search-input"
    />

    <div class="prompts-list" role="list">
      <div
        v-if="promptsStore.loading && promptsStore.prompts.length === 0"
        class="empty-state"
      >
        加载中…
      </div>
      <div
        v-else-if="filteredPrompts.length === 0"
        class="empty-state"
      >
        {{ searchKeyword.trim() ? '未找到匹配的提示词' : '暂无提示词，点击上方新建' }}
      </div>
      <div
        v-for="prompt in filteredPrompts"
        :key="prompt.id"
        role="listitem"
        class="prompt-card"
        :class="{ active: activePromptId === prompt.id }"
        :data-test="`prompt-card-${prompt.id}`"
        @click="selectPrompt(prompt.id)"
      >
        <div class="prompt-card-top">
          <span class="prompt-card-name" :title="prompt.name">{{ prompt.name }}</span>
          <span
            v-if="hasUserPromptPlaceholder(prompt.content)"
            class="placeholder-tag"
            title="支持填充用户输入 {user_prompt}"
          >
            占位符
          </span>
        </div>
        <p v-if="prompt.description" class="prompt-card-desc">
          {{ prompt.description }}
        </p>
        <p v-else class="prompt-card-preview">
          {{ prompt.content }}
        </p>
      </div>
    </div>

    <PromptManageDialog
      v-if="showManageDialog"
      @close="showManageDialog = false"
      @saved="onSaved"
    />
  </div>
</template>

<style scoped>
.prompts-sidebar {
  display: flex;
  flex-direction: column;
  min-height: 0;
  flex: 1;
  overflow: hidden;
}

.sidebar-actions {
  display: flex;
  gap: calc(var(--space-unit) * 1.0);
  padding: 0 calc(var(--space-unit) * 2.0) calc(var(--space-unit) * 1.333);
}

.action-btn,
.new-prompt-btn {
  flex: 1;
  width: 100%;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border: 1px solid var(--color-border-strong);
  background: var(--color-surface);
  color: var(--color-text);
  border-radius: var(--radius);
  padding: calc(var(--space-unit) * 0.667) calc(var(--space-unit) * 1.333);
  font: inherit;
  font-size: 0.857rem;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.15s ease;
  white-space: nowrap;
}

.action-btn:hover,
.new-prompt-btn:hover {
  background: var(--color-surface-muted);
  border-color: var(--color-primary);
  color: var(--color-primary);
  opacity: 1;
}

.search {
  margin: 0 calc(var(--space-unit) * 2.0) calc(var(--space-unit) * 1.333);
  padding: calc(var(--space-unit) * 1.0) calc(var(--space-unit) * 1.667);
  border: 1px solid var(--color-border-strong);
  border-radius: var(--radius);
  background: var(--color-surface);
  color: var(--color-text);
  font: inherit;
  font-size: 0.929rem;
  box-sizing: border-box;
  outline: none;
  transition: border-color 0.15s ease, box-shadow 0.15s ease;
}

.search:focus {
  border-color: var(--color-primary);
  box-shadow: 0 0 0 2px var(--color-info-soft);
}

.search::placeholder {
  color: var(--color-text-muted);
}

.prompts-list {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 0 calc(var(--space-unit) * 1.5) calc(var(--space-unit) * 2.0);
  display: flex;
  flex-direction: column;
  gap: calc(var(--space-unit) * 0.3);
}

.empty-state {
  margin: calc(var(--space-unit) * 1.5) calc(var(--space-unit) * 1.0);
  text-align: center;
  color: var(--color-text-muted);
  font-size: 0.857rem;
}

.prompt-card {
  padding: calc(var(--space-unit) * 0.4) calc(var(--space-unit) * 0.8);
  border: 1px solid transparent;
  border-radius: var(--radius);
  background: transparent;
  color: var(--color-text);
  cursor: pointer;
  outline: none;
  transition: all 0.15s ease;
  display: flex;
  flex-direction: column;
  gap: calc(var(--space-unit) * 0.15);
  user-select: none;
}

.prompt-card:hover {
  border-color: var(--color-border);
  background: var(--color-surface-muted);
}

.prompt-card.active {
  border-color: var(--color-primary);
  background: var(--color-surface-muted);
  box-shadow: 0 0 6px var(--color-primary-soft, rgba(59, 130, 246, 0.2));
  font-weight: 500;
}

.prompt-card-top {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 6px;
  min-width: 0;
}

.prompt-card-name {
  font-weight: 500;
  font-size: 0.857rem;
  color: var(--color-text);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  flex: 1;
}

.placeholder-tag {
  font-size: 0.7rem;
  font-weight: 500;
  padding: 0.05rem 0.35rem;
  border-radius: 3px;
  background: rgba(59, 130, 246, 0.12);
  color: #3b82f6;
  flex-shrink: 0;
}

.prompt-card-desc,
.prompt-card-preview {
  margin: 0;
  font-size: 0.75rem;
  color: var(--color-text-muted);
  line-height: 1.35;
  display: -webkit-box;
  -webkit-line-clamp: 1;
  -webkit-box-orient: vertical;
  overflow: hidden;
  text-overflow: ellipsis;
}
</style>
