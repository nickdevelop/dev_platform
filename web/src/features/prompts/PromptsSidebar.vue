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
const editingPrompt = ref<Prompt | null>(null)

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
  editingPrompt.value = null
  showManageDialog.value = true
}

function openEditDialog(prompt: Prompt) {
  editingPrompt.value = prompt
  showManageDialog.value = true
}

function onSaved(prompt: Prompt) {
  showManageDialog.value = false
  editingPrompt.value = null
  selectPrompt(prompt.id)
}
</script>

<template>
  <div class="prompts-sidebar sidebar-panel" aria-label="提示词列表">
    <!-- Zone A: Top action row -->
    <div class="sidebar-actions">
      <button
        type="button"
        class="action-btn new-prompt-btn"
        data-test="new-prompt-btn"
        title="新建提示词模板"
        @click="openCreateDialog"
      >
        <svg viewBox="0 0 16 16" width="12" height="12" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
          <line x1="8" y1="3" x2="8" y2="13" />
          <line x1="3" y1="8" x2="13" y2="8" />
        </svg>
        <span>新建提示词</span>
      </button>
    </div>

    <!-- Zone B: Search input with inset magnifier -->
    <div class="sidebar-search-bar">
      <svg class="sidebar-search-icon" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true">
        <circle cx="7" cy="7" r="4.5" />
        <line x1="10.5" y1="10.5" x2="14" y2="14" stroke-linecap="round" />
      </svg>
      <input
        v-model="searchKeyword"
        type="search"
        class="search sidebar-search-input"
        placeholder="搜索提示词名称或内容"
        aria-label="搜索提示词名称或描述"
        data-test="prompt-search-input"
      />
      <button
        v-if="searchKeyword"
        type="button"
        class="sidebar-search-clear"
        title="清空搜索"
        aria-label="清空搜索"
        @click="searchKeyword = ''"
      >
        <svg viewBox="0 0 16 16" width="10" height="10" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
          <line x1="4" y1="4" x2="12" y2="12" />
          <line x1="12" y1="4" x2="4" y2="12" />
        </svg>
      </button>
    </div>

    <!-- Zone C: Prompts list -->
    <div class="prompts-list sidebar-list-container" role="list">
      <div
        v-if="promptsStore.loading && promptsStore.prompts.length === 0"
        class="empty-state sidebar-empty"
      >
        加载中…
      </div>
      <div
        v-else-if="filteredPrompts.length === 0"
        class="empty-state sidebar-empty"
      >
        {{ searchKeyword.trim() ? '未找到匹配的提示词' : '暂无提示词，点击上方新建' }}
      </div>

      <div
        v-for="prompt in filteredPrompts"
        :key="prompt.id"
        role="listitem"
        class="prompt-card sidebar-card group"
        :class="{ active: activePromptId === prompt.id }"
        :data-test="`prompt-card-${prompt.id}`"
        tabindex="0"
        @click="selectPrompt(prompt.id)"
        @keydown.enter.prevent="selectPrompt(prompt.id)"
        @keydown.space.prevent="selectPrompt(prompt.id)"
      >
        <!-- Row 1: Name + Placeholder Pill -->
        <div class="prompt-card-top sidebar-card-row-top">
          <span class="prompt-card-name sidebar-card-title" :title="prompt.name">{{ prompt.name }}</span>
          <span
            v-if="hasUserPromptPlaceholder(prompt.content)"
            class="placeholder-tag sidebar-card-badge"
            title="包含 {user_prompt} 动态插值"
          >
            {user_prompt}
          </span>
        </div>

        <!-- Row 2: Description or content preview + Hover Action -->
        <div class="prompt-card-bottom sidebar-card-row-bottom">
          <p v-if="prompt.description" class="prompt-card-desc sidebar-card-meta truncate" :title="prompt.description">
            {{ prompt.description }}
          </p>
          <p v-else class="prompt-card-preview sidebar-card-meta truncate" :title="prompt.content">
            {{ prompt.content }}
          </p>
          <div class="prompt-card-actions sidebar-card-actions" @click.stop>
            <button
              type="button"
              class="icon-btn sidebar-icon-btn"
              title="编辑提示词"
              :aria-label="`编辑提示词「${prompt.name}」`"
              @click.stop="openEditDialog(prompt)"
            >
              <svg viewBox="0 0 16 16" width="11" height="11" fill="none" stroke="currentColor" stroke-width="1.8">
                <path d="M11 2.5l2.5 2.5L5 13.5H2.5V11L11 2.5z" />
              </svg>
            </button>
          </div>
        </div>
      </div>
    </div>

    <PromptManageDialog
      v-if="showManageDialog"
      :prompt="editingPrompt"
      @close="showManageDialog = false; editingPrompt = null"
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

.placeholder-tag {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 10px;
  color: var(--color-accent, #4a8ad6);
  background: var(--color-surface-muted, #f1f5f9);
  padding: 1px 4px;
  border-radius: 3px;
  border: 1px solid var(--color-border);
}

.prompt-card-desc,
.prompt-card-preview {
  margin: 0;
  line-height: 1.25;
}
</style>
