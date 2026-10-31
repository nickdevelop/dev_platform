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
    <!-- Top action row: consistent with project and task sidebars -->
    <div class="sidebar-actions">
      <button
        type="button"
        class="action-btn new-prompt-btn"
        data-test="new-prompt-btn"
        @click="openCreateDialog"
      >
        <svg viewBox="0 0 16 16" width="12" height="12" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
          <line x1="8" y1="3" x2="8" y2="13" />
          <line x1="3" y1="8" x2="13" y2="8" />
        </svg>
        <span>新建提示词</span>
      </button>
    </div>

    <!-- Search input -->
    <input
      v-model="searchKeyword"
      type="search"
      class="search"
      placeholder="搜索提示词名称或内容"
      aria-label="搜索提示词名称或描述"
      data-test="prompt-search-input"
    />

    <!-- Prompts list -->
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
        tabindex="0"
        @click="selectPrompt(prompt.id)"
        @keydown.enter.prevent="selectPrompt(prompt.id)"
        @keydown.space.prevent="selectPrompt(prompt.id)"
      >
        <!-- Row 1: Name + Placeholder Pill -->
        <div class="prompt-card-top">
          <span class="prompt-card-name" :title="prompt.name">{{ prompt.name }}</span>
          <span
            v-if="hasUserPromptPlaceholder(prompt.content)"
            class="placeholder-tag"
            title="包含 {user_prompt} 动态插值"
          >
            {user_prompt}
          </span>
        </div>

        <!-- Row 2: Description or content preview -->
        <div class="prompt-card-bottom">
          <p v-if="prompt.description" class="prompt-card-desc truncate">
            {{ prompt.description }}
          </p>
          <p v-else class="prompt-card-preview truncate">
            {{ prompt.content }}
          </p>
        </div>
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
  background: var(--color-surface, #ffffff);
}

.sidebar-actions {
  display: flex;
  gap: 6px;
  padding: 0 12px 8px 12px;
}

.action-btn,
.new-prompt-btn {
  flex: 1;
  width: 100%;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 4px;
  height: 28px;
  border: 1px solid var(--color-border, #e2e8f0);
  background: var(--color-surface, #ffffff);
  color: var(--color-text, #1e293b);
  border-radius: var(--radius, 6px);
  padding: 0 10px;
  font: inherit;
  font-size: 12px;
  font-weight: 500;
  cursor: pointer;
  transition: background-color 0.12s ease-out, border-color 0.12s ease-out, color 0.12s ease-out;
  white-space: nowrap;
}

.action-btn:hover,
.new-prompt-btn:hover {
  background: var(--color-surface-muted, #f1f5f9);
  border-color: var(--color-border-strong, #cbd5e1);
}

.search {
  margin: 0 12px 8px 12px;
  padding: 4px 10px;
  height: 28px;
  border: 1px solid var(--color-border, #e2e8f0);
  border-radius: var(--radius, 6px);
  background: var(--color-surface, #ffffff);
  color: var(--color-text, #1e293b);
  font: inherit;
  font-size: 12px;
  box-sizing: border-box;
  outline: none;
  transition: border-color 0.12s ease-out;
}

.search:focus {
  border-color: var(--color-primary, #1b6c80);
}

.search::placeholder {
  color: var(--color-text-muted, #94a3b8);
}

.prompts-list {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 0 10px 16px 10px;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.empty-state {
  margin: 24px 8px;
  text-align: center;
  color: var(--color-text-muted, #64748b);
  font-size: 12px;
  line-height: 1.5;
}

.prompt-card {
  padding: 8px 10px;
  border-radius: var(--radius, 6px);
  border: 1px solid var(--color-border, #e2e8f0);
  border-left-width: 3px;
  border-left-color: var(--color-border, #cbd5e1);
  background: var(--color-surface, #ffffff);
  color: var(--color-text, #1e293b);
  cursor: pointer;
  outline: none;
  transition: background-color 0.12s ease-out, border-color 0.12s ease-out;
  display: flex;
  flex-direction: column;
  gap: 4px;
  user-select: none;
}

.prompt-card:hover {
  background: var(--color-surface-muted, #f8fafc);
  border-color: var(--color-border-strong, #cbd5e1);
}

.prompt-card.active {
  background: var(--color-surface-muted, #f1f5f9);
  border-color: var(--color-border-strong, #cbd5e1);
  border-left-color: var(--color-primary, #1b6c80);
}

.prompt-card-top {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 6px;
  min-width: 0;
}

.prompt-card-name {
  font-size: 13px;
  font-weight: 500;
  color: var(--color-text, #1e293b);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  flex: 1;
}

.placeholder-tag {
  font-family: var(--font-mono, monospace);
  font-size: 10px;
  padding: 1px 5px;
  border-radius: 3px;
  background: var(--color-surface-muted, #f1f5f9);
  color: var(--color-text-muted, #475569);
  border: 1px solid var(--color-border, #e2e8f0);
  white-space: nowrap;
  flex-shrink: 0;
}

.prompt-card-bottom {
  display: flex;
  align-items: center;
  font-size: 11px;
  color: var(--color-text-muted, #64748b);
  line-height: 1.3;
}

.prompt-card-desc,
.prompt-card-preview {
  margin: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
