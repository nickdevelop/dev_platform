<script setup lang="ts">
import { computed, nextTick, ref } from 'vue'
import { describeApiError } from '../../api/client'
import type { Prompt } from '../../api/prompts'
import { usePromptsStore } from '../../stores/prompts'
import { UiDialog, UiButton, UiInput, UiAlert } from '../../shared/ui'
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
  <UiDialog
    :title="isEditing ? '编辑提示词模版' : '新建提示词模版'"
    width-class="w-full max-w-xl"
    @close="emit('close')"
  >
    <form class="prompt-dialog-form space-y-4" @submit.prevent="submit">
      <UiAlert v-if="errorMessage" variant="error" class="dialog-error">
        {{ errorMessage }}
      </UiAlert>

      <!-- 1. Name -->
      <div class="space-y-1.5">
        <label for="prompt-name" class="text-xs font-medium text-[var(--color-text)]">
          提示词名称 <span class="text-red-500">*</span>
        </label>
        <UiInput
          id="prompt-name"
          v-model="name"
          placeholder="例如：代码审查助手、Git 提交信息生成"
          required
          autofocus
          data-test="prompt-name-input"
          block
        />
      </div>

      <!-- 2. Description -->
      <div class="space-y-1.5">
        <label for="prompt-desc" class="text-xs font-medium text-[var(--color-text)]">
          描述 / 用途说明
        </label>
        <UiInput
          id="prompt-desc"
          v-model="description"
          placeholder="简要说明该提示词的应用场景与期望产出"
          data-test="prompt-desc-input"
          block
        />
      </div>

      <!-- 3. Content Textarea with Toolbar -->
      <div class="space-y-1.5">
        <div class="content-header flex items-center justify-between gap-2">
          <label for="prompt-content" class="text-xs font-medium text-[var(--color-text)]">
            模版内容 <span class="text-red-500">*</span>
          </label>
          <button
            type="button"
            class="btn-insert-placeholder inline-flex items-center gap-1 text-[11px] font-mono px-2 py-0.5 rounded bg-[var(--color-surface-muted)] text-[var(--color-primary)] border border-[var(--color-border)] hover:bg-[var(--color-surface)] transition-colors"
            title="在光标处插入占位符"
            data-test="btn-insert-placeholder"
            @click="insertPlaceholder"
          >
            <span>+ 插入 {user_prompt} 占位符</span>
          </button>
        </div>
        <p class="field-hint text-[11px] text-[var(--color-text-muted)] leading-relaxed">
          可使用 <code>{user_prompt}</code> 作为变量槽。在实际创建会话或触发任务时，用户的输入消息将自动填充该位置。
        </p>
        <textarea
          id="prompt-content"
          ref="contentTextareaRef"
          v-model="content"
          rows="8"
          class="w-full rounded-[var(--radius,6px)] bg-[var(--color-surface-muted)] border border-[var(--color-border)] p-3 text-xs font-mono text-[var(--color-text)] placeholder-[var(--color-text-muted)] outline-none focus:border-[var(--color-primary)] transition-colors leading-relaxed resize-y"
          placeholder="输入提示词模版，例如：&#10;请对以下代码进行审查，并指出潜在缺陷与优化建议：&#10;&#10;{user_prompt}"
          required
          data-test="prompt-content-input"
        ></textarea>
      </div>

      <!-- 4. Dialog Actions -->
      <div class="dialog-actions flex items-center justify-end gap-2.5 pt-3 border-t border-[var(--color-border)]">
        <UiButton type="button" variant="secondary" class="btn-cancel text-xs" @click="emit('close')">
          取消
        </UiButton>
        <UiButton
          type="submit"
          variant="primary"
          class="btn-save text-xs font-medium px-4 py-2"
          :disabled="saving"
          :loading="saving"
          data-test="prompt-save-btn"
        >
          {{ saving ? '保存中…' : isEditing ? '保存修改' : '立即创建' }}
        </UiButton>
      </div>
    </form>
  </UiDialog>
</template>
