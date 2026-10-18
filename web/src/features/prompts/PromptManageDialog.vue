<script setup lang="ts">
import { computed, nextTick, ref } from 'vue'
import { describeApiError } from '../../api/client'
import type { Prompt } from '../../api/prompts'
import { usePromptsStore } from '../../stores/prompts'
import AppDialog from '../../shared/components/AppDialog.vue'
import { USER_PROMPT_PLACEHOLDER } from './promptTemplate'

const props = defineProps<{
  prompt?: Prompt | null
}>()

const emit = defineEmits<{
  close: []
  saved: [prompt: Prompt]
}>()

const promptsStore = usePromptsStore()

const name = ref(props.prompt?.name ?? '')
const description = ref(props.prompt?.description ?? '')
const content = ref(props.prompt?.content ?? '')
const contentTextareaRef = ref<HTMLTextAreaElement | null>(null)

const saving = ref(false)
const errorMessage = ref('')

const isEditing = computed(() => !!props.prompt)

function insertPlaceholder() {
  const textarea = contentTextareaRef.value
  if (!textarea) {
    content.value += USER_PROMPT_PLACEHOLDER
    return
  }

  const start = textarea.selectionStart ?? content.value.length
  const end = textarea.selectionEnd ?? content.value.length
  const before = content.value.substring(0, start)
  const after = content.value.substring(end)
  content.value = before + USER_PROMPT_PLACEHOLDER + after

  const newCursor = start + USER_PROMPT_PLACEHOLDER.length
  void nextTick(() => {
    textarea.focus()
    textarea.setSelectionRange(newCursor, newCursor)
  })
}

async function submit() {
  if (!name.value.trim()) {
    errorMessage.value = '请输入提示词名称'
    return
  }
  if (!content.value.trim()) {
    errorMessage.value = '请输入提示词内容'
    return
  }

  saving.value = true
  errorMessage.value = ''

  try {
    let saved: Prompt
    if (isEditing.value && props.prompt) {
      saved = await promptsStore.updatePrompt(props.prompt.id, {
        name: name.value.trim(),
        description: description.value.trim(),
        content: content.value,
      })
    } else {
      saved = await promptsStore.createPrompt({
        name: name.value.trim(),
        description: description.value.trim(),
        content: content.value,
      })
    }
    emit('saved', saved)
    emit('close')
  } catch (err) {
    errorMessage.value = describeApiError(err)
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <AppDialog
    :title="isEditing ? '编辑提示词' : '新建提示词'"
    @close="emit('close')"
  >
    <form class="prompt-dialog-form" @submit.prevent="submit">
      <p v-if="errorMessage" class="dialog-error" role="alert">{{ errorMessage }}</p>

      <label for="prompt-name">提示词名称 <span class="required">*</span></label>
      <input
        id="prompt-name"
        v-model="name"
        type="text"
        placeholder="例如：代码审查助手、Git 提交信息生成"
        required
        autofocus
        data-test="prompt-name-input"
      />

      <label for="prompt-desc">描述 / 用途说明</label>
      <input
        id="prompt-desc"
        v-model="description"
        type="text"
        placeholder="简要说明该提示词的用途"
        data-test="prompt-desc-input"
      />

      <div class="content-header">
        <label for="prompt-content">提示词模版内容 <span class="required">*</span></label>
        <button
          type="button"
          class="btn-insert-placeholder"
          title="在光标处插入占位符"
          data-test="btn-insert-placeholder"
          @click="insertPlaceholder"
        >
          + 插入 {user_prompt} 占位符
        </button>
      </div>
      <p class="field-hint">
        可以使用 <code>{user_prompt}</code> 作为占位符。在创建会话时，用户输入的消息将自动替换该占位符。
      </p>
      <textarea
        id="prompt-content"
        ref="contentTextareaRef"
        v-model="content"
        rows="8"
        placeholder="输入提示词模版，例如：&#10;请对以下代码进行审查，并指出潜在缺陷与优化建议：&#10;&#10;{user_prompt}"
        required
        data-test="prompt-content-input"
      ></textarea>

      <div class="dialog-actions">
        <button type="button" class="btn-cancel" @click="emit('close')">取消</button>
        <button type="submit" class="btn-save" :disabled="saving" data-test="prompt-save-btn">
          {{ saving ? '保存中…' : isEditing ? '保存修改' : '立即创建' }}
        </button>
      </div>
    </form>
  </AppDialog>
</template>

<style scoped>
.prompt-dialog-form {
  display: flex;
  flex-direction: column;
  gap: calc(var(--space-unit) * 1.5);
}

.dialog-error {
  margin: 0;
  padding: calc(var(--space-unit) * 1.25);
  background: var(--color-danger-soft);
  color: var(--color-danger);
  border-radius: 6px;
  font-size: 0.875rem;
}

label {
  font-size: 0.875rem;
  font-weight: 500;
  color: var(--color-text);
}

.required {
  color: var(--color-danger);
}

input[type='text'],
textarea {
  width: 100%;
  padding: calc(var(--space-unit) * 1) calc(var(--space-unit) * 1.25);
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: 6px;
  color: var(--color-text);
  font-size: 0.875rem;
  font-family: inherit;
  box-sizing: border-box;
}

textarea {
  font-family: var(--font-mono);
  line-height: 1.5;
  resize: vertical;
}

input:focus,
textarea:focus {
  outline: none;
  border-color: var(--color-primary);
  box-shadow: 0 0 0 2px var(--color-primary-soft);
}

.content-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: calc(var(--space-unit) * 1);
}

.btn-insert-placeholder {
  font-size: 0.775rem;
  padding: 3px 8px;
  border-radius: 4px;
  border: 1px dashed var(--color-primary);
  color: var(--color-primary);
  background: var(--color-primary-soft);
  cursor: pointer;
  transition: all 0.15s ease;
}

.btn-insert-placeholder:hover {
  background: var(--color-primary);
  color: #fff;
}

.field-hint {
  margin: calc(var(--space-unit) * -0.75) 0 0 0;
  font-size: 0.75rem;
  color: var(--color-text-muted);
}

.field-hint code {
  padding: 1px 4px;
  background: var(--color-surface-muted);
  border-radius: 3px;
  font-family: var(--font-mono);
  color: var(--color-primary);
}

.dialog-actions {
  display: flex;
  justify-content: flex-end;
  gap: calc(var(--space-unit) * 1.25);
  margin-top: calc(var(--space-unit) * 1);
}

.btn-cancel {
  padding: calc(var(--space-unit) * 1) calc(var(--space-unit) * 2);
  background: transparent;
  border: 1px solid var(--color-border);
  border-radius: 6px;
  color: var(--color-text);
  cursor: pointer;
  font-size: 0.875rem;
}

.btn-save {
  padding: calc(var(--space-unit) * 1) calc(var(--space-unit) * 2);
  background: var(--color-primary);
  border: none;
  border-radius: 6px;
  color: #fff;
  cursor: pointer;
  font-size: 0.875rem;
  font-weight: 500;
}

.btn-save:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}
</style>
