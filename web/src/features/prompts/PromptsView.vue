<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import type { Prompt } from '../../api/prompts'
import { usePromptsStore } from '../../stores/prompts'
import PromptManageDialog from './PromptManageDialog.vue'
import { applyUserPromptTemplate, hasUserPromptPlaceholder, USER_PROMPT_PLACEHOLDER } from './promptTemplate'

const route = useRoute()
const router = useRouter()
const promptsStore = usePromptsStore()

const showEditDialog = ref(false)
const testInput = ref('')

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
    return d.toLocaleString('zh-CN', { hour12: false })
  } catch {
    return iso
  }
}
</script>

<template>
  <div class="prompts-view" aria-label="提示词详情">
    <div v-if="activePrompt" class="prompt-detail-container">
      <header class="detail-header">
        <div class="header-main">
          <div class="header-title-row">
            <h1>{{ activePrompt.name }}</h1>
            <span
              v-if="hasUserPromptPlaceholder(activePrompt.content)"
              class="placeholder-badge"
            >
              包含 {user_prompt} 占位符
            </span>
          </div>
          <p v-if="activePrompt.description" class="header-desc">
            {{ activePrompt.description }}
          </p>
          <div class="header-meta">
            <span>创建时间：{{ formatDate(activePrompt.createdAt) }}</span>
            <span v-if="activePrompt.updatedAt">更新时间：{{ formatDate(activePrompt.updatedAt) }}</span>
          </div>
        </div>

        <div class="header-actions">
          <button
            type="button"
            class="action-btn edit-btn"
            data-test="edit-prompt-btn"
            @click="showEditDialog = true"
          >
            编辑
          </button>
          <button
            type="button"
            class="action-btn delete-btn"
            data-test="delete-prompt-btn"
            @click="deleteCurrentPrompt"
          >
            删除
          </button>
        </div>
      </header>

      <div class="detail-body">
        <section class="section-card template-section">
          <h2>提示词模版</h2>
          <div class="template-content-box">
            <pre class="template-pre">{{ activePrompt.content }}</pre>
          </div>
        </section>

        <section class="section-card preview-section">
          <h2>填充效果模拟测试</h2>
          <p class="section-hint">
            输入测试消息，实时预览 <code>{user_prompt}</code> 占位符替换后的合成效果：
          </p>
          <div class="preview-grid">
            <div class="preview-input-box">
              <label for="test-user-input">模拟用户输入：</label>
              <textarea
                id="test-user-input"
                v-model="testInput"
                rows="4"
                placeholder="在此输入测试文本（如：用户发送的问题或代码）"
                data-test="prompt-test-input"
              ></textarea>
            </div>
            <div class="preview-output-box">
              <label>合成后发送给 Agent 的内容：</label>
              <pre class="preview-output-pre" data-test="prompt-preview-output">{{ previewMergedText }}</pre>
            </div>
          </div>
        </section>
      </div>

      <PromptManageDialog
        v-if="showEditDialog"
        :prompt="activePrompt"
        @close="showEditDialog = false"
        @saved="onSaved"
      />
    </div>

    <div v-else class="empty-view">
      <div class="empty-content">
        <svg class="empty-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
          <path d="M12 20h9" />
          <path d="M16.5 3.5a2.121 2.121 0 0 1 3 3L7 19l-4 1 1-4L16.5 3.5z" />
        </svg>
        <h3>未选择提示词</h3>
        <p>从左侧列表中选择一个提示词查看详情，或新建提示词。</p>
        <button
          type="button"
          class="btn-primary-create"
          @click="showEditDialog = true"
        >
          + 新建提示词
        </button>
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
  height: 100%;
  overflow-y: auto;
  background: var(--color-bg);
  box-sizing: border-box;
}

.prompt-detail-container {
  max-width: 960px;
  margin: 0 auto;
  padding: calc(var(--space-unit) * 3);
  display: flex;
  flex-direction: column;
  gap: calc(var(--space-unit) * 2.5);
}

.detail-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: calc(var(--space-unit) * 2);
  padding-bottom: calc(var(--space-unit) * 2);
  border-bottom: 1px solid var(--color-border);
}

.header-main {
  flex: 1;
}

.header-title-row {
  display: flex;
  align-items: center;
  gap: calc(var(--space-unit) * 1.5);
  margin-bottom: calc(var(--space-unit) * 0.75);
}

.header-title-row h1 {
  font-size: 1.35rem;
  font-weight: 600;
  margin: 0;
  color: var(--color-text);
}

.placeholder-badge {
  font-size: 0.75rem;
  padding: 2px 8px;
  border-radius: 4px;
  background: var(--color-primary-soft);
  color: var(--color-primary);
  font-weight: 500;
}

.header-desc {
  margin: 0 0 calc(var(--space-unit) * 1);
  color: var(--color-text-muted);
  font-size: 0.875rem;
  line-height: 1.5;
}

.header-meta {
  display: flex;
  gap: calc(var(--space-unit) * 2);
  font-size: 0.75rem;
  color: var(--color-text-muted);
}

.header-actions {
  display: flex;
  align-items: center;
  gap: calc(var(--space-unit) * 1);
}

.action-btn {
  padding: 6px 14px;
  font-size: 0.8125rem;
  border-radius: 6px;
  cursor: pointer;
  border: 1px solid var(--color-border);
  background: var(--color-surface);
  color: var(--color-text);
  transition: all 0.15s ease;
}

.action-btn:hover {
  background: var(--color-surface-muted);
}

.delete-btn {
  color: var(--color-danger);
  border-color: var(--color-danger-border);
}

.delete-btn:hover {
  background: var(--color-danger-soft);
}

.detail-body {
  display: flex;
  flex-direction: column;
  gap: calc(var(--space-unit) * 2.5);
}

.section-card {
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: 8px;
  padding: calc(var(--space-unit) * 2);
}

.section-card h2 {
  font-size: 1rem;
  font-weight: 600;
  margin: 0 0 calc(var(--space-unit) * 1);
  color: var(--color-text);
}

.section-hint {
  font-size: 0.8125rem;
  color: var(--color-text-muted);
  margin: 0 0 calc(var(--space-unit) * 1.5);
}

.section-hint code {
  padding: 1px 4px;
  background: var(--color-surface-muted);
  border-radius: 3px;
  color: var(--color-primary);
}

.template-content-box {
  background: var(--color-bg);
  border: 1px solid var(--color-border);
  border-radius: 6px;
  padding: calc(var(--space-unit) * 1.5);
}

.template-pre {
  margin: 0;
  white-space: pre-wrap;
  word-break: break-word;
  font-family: var(--font-mono);
  font-size: 0.875rem;
  line-height: 1.6;
  color: var(--color-text);
}

.preview-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: calc(var(--space-unit) * 2);
}

@media (max-width: 768px) {
  .preview-grid {
    grid-template-columns: 1fr;
  }
}

.preview-input-box,
.preview-output-box {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

label {
  font-size: 0.8125rem;
  font-weight: 500;
  color: var(--color-text-muted);
}

textarea {
  width: 100%;
  padding: calc(var(--space-unit) * 1);
  border: 1px solid var(--color-border);
  border-radius: 6px;
  background: var(--color-bg);
  color: var(--color-text);
  font-size: 0.875rem;
  font-family: inherit;
  box-sizing: border-box;
  resize: vertical;
}

textarea:focus {
  outline: none;
  border-color: var(--color-primary);
}

.preview-output-pre {
  margin: 0;
  min-height: 98px;
  padding: calc(var(--space-unit) * 1);
  background: var(--color-bg);
  border: 1px solid var(--color-border);
  border-radius: 6px;
  white-space: pre-wrap;
  word-break: break-word;
  font-family: var(--font-mono);
  font-size: 0.875rem;
  line-height: 1.5;
  color: var(--color-text);
  box-sizing: border-box;
}

.empty-view {
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
}

.empty-content {
  text-align: center;
  color: var(--color-text-muted);
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
}

.empty-icon {
  width: 48px;
  height: 48px;
  opacity: 0.5;
}

.empty-content h3 {
  margin: 0;
  font-size: 1.1rem;
  color: var(--color-text);
}

.empty-content p {
  margin: 0;
  font-size: 0.875rem;
}

.btn-primary-create {
  margin-top: 8px;
  padding: 8px 18px;
  background: var(--color-primary);
  color: #fff;
  border: none;
  border-radius: 6px;
  cursor: pointer;
  font-size: 0.875rem;
  font-weight: 500;
}
</style>
