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
  height: 26px;
  border: 1px solid var(--color-border);
  background: var(--color-surface);
  color: var(--color-text);
  border-radius: var(--radius, 4px);
  padding: 0 8px;
  font: inherit;
  font-size: 12px;
  font-weight: 500;
  cursor: pointer;
  transition: color 0.12s ease-out, border-color 0.12s ease-out, background-color 0.12s ease-out;
  white-space: nowrap;
}

.action-btn:hover,
.new-prompt-btn:hover {
  background: var(--color-surface-muted);
  border-color: var(--color-border-strong);
  color: var(--color-text);
}

.search {
  margin: 0 12px 8px 12px;
  padding: 4px 10px;
  height: 28px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius, 4px);
  background: var(--color-surface);
  color: var(--color-text);
  font: inherit;
  font-size: 12px;
  box-sizing: border-box;
  outline: none;
  transition: border-color 0.12s ease-out;
}

.search:focus {
  border-color: var(--color-accent, #4a8ad6);
}

.search::placeholder {
  color: var(--color-text-muted);
}

.prompts-list {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 0 8px 16px 8px;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.empty-state {
  margin: 16px 8px;
  text-align: center;
  color: var(--color-text-muted);
  font-size: 12px;
}

.prompt-card {
  padding: 6px 8px;
  border-radius: var(--radius, 4px);
  border-left: 2px solid transparent;
  background: transparent;
  color: var(--color-text);
  cursor: pointer;
  outline: none;
  transition: background-color 0.12s ease-out, border-color 0.12s ease-out;
  display: flex;
  flex-direction: column;
  gap: 3px;
  user-select: none;
}

.prompt-card:hover {
  background: var(--color-surface-muted);
}

.prompt-card.active {
  border-left-color: var(--color-accent, #4a8ad6);
  background: var(--color-surface-muted);
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
  font-size: 12px;
  color: var(--color-text);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  flex: 1;
}

.placeholder-tag {
  font-size: 10px;
  font-weight: 500;
  padding: 0 4px;
  height: 16px;
  line-height: 16px;
  border-radius: 3px;
  background: var(--color-surface-muted);
  border: 1px solid var(--color-border);
  color: var(--color-accent, #4a8ad6);
  flex-shrink: 0;
}

.prompt-card-bottom {
  min-width: 0;
}

.prompt-card-desc,
.prompt-card-preview {
  margin: 0;
  font-size: 11px;
  color: var(--color-text-muted);
  line-height: 1.35;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
</style>
