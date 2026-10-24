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
    :title="isEditing ? '编辑提示词' : '新建提示词'"
    width-class="w-full max-w-xl"
    @close="emit('close')"
  >
    <form class="prompt-dialog-form space-y-4" @submit.prevent="submit">
      <UiAlert v-if="errorMessage" variant="error" class="dialog-error">
        {{ errorMessage }}
      </UiAlert>

      <div class="space-y-1.5">
        <label for="prompt-name" class="text-xs font-semibold text-slate-700 dark:text-slate-300">
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

      <div class="space-y-1.5">
        <label for="prompt-desc" class="text-xs font-semibold text-slate-700 dark:text-slate-300">
          描述 / 用途说明
        </label>
        <UiInput
          id="prompt-desc"
          v-model="description"
          placeholder="简要说明该提示词的用途"
          data-test="prompt-desc-input"
          block
        />
      </div>

      <div class="space-y-1.5">
        <div class="content-header flex items-center justify-between gap-2">
          <label for="prompt-content" class="text-xs font-semibold text-slate-700 dark:text-slate-300">
            提示词模版内容 <span class="text-red-500">*</span>
          </label>
          <button
            type="button"
            class="btn-insert-placeholder text-xs px-2 py-0.5 rounded border border-dashed border-blue-500 text-blue-600 dark:text-blue-400 bg-blue-50/60 dark:bg-blue-950/30 hover:bg-blue-100 transition-colors"
            title="在光标处插入占位符"
            data-test="btn-insert-placeholder"
            @click="insertPlaceholder"
          >
            + 插入 {user_prompt} 占位符
          </button>
        </div>
        <p class="field-hint text-xs text-slate-400">
          可以使用 <code class="px-1 py-0.5 bg-slate-100 dark:bg-slate-800 rounded font-mono">{user_prompt}</code> 作为占位符。在创建会话时，用户输入的消息将自动替换该占位符。
        </p>
        <textarea
          id="prompt-content"
          ref="contentTextareaRef"
          v-model="content"
          rows="8"
          class="w-full rounded bg-white dark:bg-slate-900 border border-slate-300 dark:border-slate-700 p-2.5 text-xs font-mono text-slate-800 dark:text-slate-100 placeholder-slate-400 outline-none focus:border-blue-500 focus:ring-2 focus:ring-blue-500/20 leading-relaxed resize-y"
          placeholder="输入提示词模版，例如：&#10;请对以下代码进行审查，并指出潜在缺陷与优化建议：&#10;&#10;{user_prompt}"
          required
          data-test="prompt-content-input"
        ></textarea>
      </div>

      <div class="dialog-actions flex items-center justify-end gap-2.5 pt-3 border-t border-slate-100 dark:border-slate-800">
        <UiButton type="button" variant="secondary" class="btn-cancel" @click="emit('close')">
          取消
        </UiButton>
        <UiButton
          type="submit"
          variant="primary"
          class="btn-save"
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
