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
    <div class="sidebar-head">
      <h2>提示词</h2>
    </div>

    <div class="sidebar-actions">
      <button
        type="button"
        class="new-prompt-btn"
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
  height: 100%;
  overflow: hidden;
  background: var(--color-surface);
}

.sidebar-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  min-height: 28px;
  padding: calc(var(--space-unit) * 1.5) calc(var(--space-unit) * 2);
}

.sidebar-head h2 {
  font-size: 1rem;
  font-weight: 600;
  margin: 0;
  color: var(--color-text);
}

.sidebar-actions {
  display: flex;
  padding: calc(var(--space-unit) * 1) calc(var(--space-unit) * 1.5) 0;
}

.new-prompt-btn {
  width: 100%;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 6px 12px;
  background: var(--color-primary);
  border: none;
  border-radius: var(--radius, 4px);
  color: #fff;
  font-size: 0.8125rem;
  font-weight: 500;
  cursor: pointer;
  transition: opacity 0.15s ease;
}

.new-prompt-btn:hover {
  opacity: 0.9;
}

.search {
  margin: calc(var(--space-unit) * 1.5);
  padding: calc(var(--space-unit) * 1) calc(var(--space-unit) * 1.25);
  border: 1px solid var(--color-border);
  border-radius: 6px;
  background: var(--color-bg);
  color: var(--color-text);
  font-size: 0.875rem;
}

.search:focus {
  outline: none;
  border-color: var(--color-primary);
}

.prompts-list {
  flex: 1;
  overflow-y: auto;
  padding: 0 calc(var(--space-unit) * 1.5) calc(var(--space-unit) * 1.5);
  display: flex;
  flex-direction: column;
  gap: calc(var(--space-unit) * 1);
}

.empty-state {
  padding: calc(var(--space-unit) * 4) 0;
  text-align: center;
  color: var(--color-text-muted);
  font-size: 0.875rem;
}

.prompt-card {
  padding: calc(var(--space-unit) * 1.25);
  border: 1px solid var(--color-border);
  border-radius: 6px;
  background: var(--color-surface);
  cursor: pointer;
  transition: all 0.15s ease;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.prompt-card:hover {
  border-color: var(--color-border-strong);
  background: var(--color-surface-muted);
}

.prompt-card.active {
  border-color: var(--color-primary);
  background: var(--color-primary-soft);
}

.prompt-card-top {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 6px;
}

.prompt-card-name {
  font-weight: 500;
  font-size: 0.875rem;
  color: var(--color-text);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.placeholder-tag {
  font-size: 0.6875rem;
  padding: 1px 4px;
  border-radius: 3px;
  background: var(--color-primary-soft);
  color: var(--color-primary);
  flex-shrink: 0;
}

.prompt-card-desc,
.prompt-card-preview {
  margin: 0;
  font-size: 0.775rem;
  color: var(--color-text-muted);
  overflow: hidden;
  text-overflow: ellipsis;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  line-height: 1.35;
}
</style>
