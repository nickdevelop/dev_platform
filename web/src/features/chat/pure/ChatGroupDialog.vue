<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import type { ChatGroup } from '../../../api/chats'
import { useChatsStore } from '../../../stores/chats'
import { useModelsStore } from '../../../stores/models'
import { usePromptsStore } from '../../../stores/prompts'
import { UiButton, UiDialog, UiSelectMenu, type SelectMenuItem } from '../../../shared/ui'

const props = defineProps<{
  mode: 'create' | 'rename'
  group?: ChatGroup | null
}>()

const emit = defineEmits<{
  close: []
  submitted: [group: ChatGroup]
}>()

const chats = useChatsStore()
const models = useModelsStore()
const promptsStore = usePromptsStore()

const nameInput = ref('')
const defaultModelIdInput = ref<number | ''>('')
const promptSource = ref<'prompt' | 'custom'>('prompt')
const promptIdInput = ref<number | 0>(0)
const systemPromptInput = ref('')
const iconInput = ref('')
const showInFloatInput = ref(false)
const enablePlatformToolsInput = ref(false)
const maxToolStepsInput = ref(5)

const error = ref('')
const submitting = ref(false)

const PRESET_ICONS = [
  '🤖', '💡', '💻', '📝', '🎨', '🔍', '🚀', '⚙️', // taste-skill-ignore
  '🛠️', '📊', '🌐', '📚', '💬', '⚡', '🎯', '🔮', // taste-skill-ignore
]

onMounted(async () => {
  if (models.services.length === 0) {
    void models.load()
  }
  if (promptsStore.prompts.length === 0) {
    await promptsStore.loadPrompts()
  }
  if (props.mode === 'rename' && props.group) {
    nameInput.value = props.group.name
    defaultModelIdInput.value = props.group.defaultModelId ? props.group.defaultModelId : ''
    if (props.group.promptId && props.group.promptId > 0) {
      promptSource.value = 'prompt'
      promptIdInput.value = props.group.promptId
      systemPromptInput.value = ''
    } else {
      promptSource.value = props.group.systemPrompt ? 'custom' : 'prompt'
      promptIdInput.value = 0
      systemPromptInput.value = props.group.systemPrompt ?? ''
    }
    iconInput.value = props.group.icon ?? ''
    showInFloatInput.value = props.group.showInFloat ?? false
    enablePlatformToolsInput.value = props.group.enablePlatformTools ?? false
    maxToolStepsInput.value = props.group.maxToolSteps ?? 5
  } else {
    const roles = (promptsStore.prompts || []).filter((p) => p.type === 'role')
    if (roles.length > 0) {
      promptSource.value = 'prompt'
      promptIdInput.value = roles[0].id
    }
  }
})

const rolePrompts = computed(() => (promptsStore.prompts || []).filter((p) => p.type === 'role'))

const promptSelectOptions = computed<SelectMenuItem<number | 0>[]>(() => {
  const items: SelectMenuItem<number | 0>[] = [
    { value: 0, label: '请选择角色类提示词…' },
  ]
  for (const p of rolePrompts.value) {
    items.push({
      value: p.id,
      label: `${p.name} (${p.description || '无描述'})`,
    })
  }
  return items
})

const selectedPromptPreview = computed(() => {
  if (!promptIdInput.value) return ''
  const p = promptsStore.byId(promptIdInput.value)
  return p ? p.content : ''
})

const usableModels = computed(() =>
  (models.services ?? []).flatMap((service) =>
    (service.models ?? [])
      .filter((model) => model.usable && model.supportsText)
      .map((model) => ({
        id: model.id,
        name: `${model.name || model.modelId} (${service.name})`,
      })),
  ),
)

const modelOptions = computed<SelectMenuItem<number | ''>[]>(() => {
  const items: SelectMenuItem<number | ''>[] = [
    { value: '', label: '跟随默认 / 未指定' },
  ]
  for (const m of usableModels.value) {
    items.push({ value: m.id, label: m.name })
  }
  return items
})

const title = computed(() => {
  return props.mode === 'create' ? '新建助手' : `编辑助手「${props.group?.name || ''}」`
})

const confirmLabel = computed(() => {
  return props.mode === 'create' ? '创建' : '保存'
})

const defaultAvatar = computed(() => {
  const trimmed = nameInput.value.trim()
  if (!trimmed) return '助'
  return Array.from(trimmed)[0].toUpperCase()
})

const currentAvatar = computed(() => {
  if (iconInput.value.trim()) return iconInput.value.trim()
  return defaultAvatar.value
})

function selectIcon(icon: string) {
  if (iconInput.value === icon) {
    iconInput.value = ''
  } else {
    iconInput.value = icon
  }
}

async function onSubmit() {
  const name = nameInput.value.trim()
  if (!name) {
    error.value = '助手名称不能为空。'
    return
  }
  if (name.length > 60) {
    error.value = '助手名称不能超过 60 个字符。'
    return
  }

  submitting.value = true
  error.value = ''
  const promptId = promptSource.value === 'prompt' ? Number(promptIdInput.value) || 0 : 0
  const systemPrompt = promptSource.value === 'custom' ? systemPromptInput.value.trim() : ''
  const payload = {
    name,
    defaultModelId: typeof defaultModelIdInput.value === 'number' ? defaultModelIdInput.value : 0,
    promptId,
    systemPrompt,
    icon: iconInput.value.trim(),
    showInFloat: showInFloatInput.value,
    enablePlatformTools: enablePlatformToolsInput.value,
    maxToolSteps: Math.min(20, Math.max(1, Number(maxToolStepsInput.value) || 5)),
  }

  try {
    if (props.mode === 'create') {
      const created = await chats.createGroup(payload)
      if (created) {
        emit('submitted', created)
        emit('close')
      } else {
        error.value = chats.actionError || '创建助手失败。'
      }
    } else if (props.mode === 'rename' && props.group) {
      const success = await chats.updateGroup(props.group.id, payload)
      if (success) {
        emit('submitted', { ...props.group, ...payload })
        emit('close')
      } else {
        error.value = chats.actionError || '更新助手失败。'
      }
    }
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <UiDialog :title="title" width-class="w-full max-w-lg" @close="emit('close')">
    <form class="assistant-form" @submit.prevent="onSubmit">
      <!-- 助手头像与基本信息 -->
      <div class="header-section">
        <div class="avatar-preview" :title="`图标预览：${currentAvatar}`">
          <span class="avatar-char">{{ currentAvatar }}</span>
        </div>
        <div class="header-fields">
          <label class="field">
            <span class="label-text">助手名称 <span class="required">*</span></span>
            <input
              v-model="nameInput"
              type="text"
              class="text-input"
              placeholder="例如：代码架构师、翻译助手"
              maxlength="60"
              autofocus
              required
            />
          </label>
        </div>
      </div>

      <!-- 图标选择 -->
      <div class="field">
        <div class="label-row">
          <span class="label-text">助手图标</span>
          <span class="hint-text">默认使用助手名称首字符</span>
        </div>
        <div class="preset-icons">
          <button
            v-for="icon in PRESET_ICONS"
            :key="icon"
            type="button"
            class="icon-option"
            :class="{ active: iconInput === icon }"
            @click="selectIcon(icon)"
          >
            {{ icon }}
          </button>
          <button
            v-if="iconInput"
            type="button"
            class="clear-icon-btn"
            title="恢复默认首字符图标"
            @click="iconInput = ''"
          >
            重置
          </button>
        </div>
      </div>

      <!-- 默认模型配置 -->
      <div class="field">
        <span class="label-text">默认模型</span>
        <UiSelectMenu
          v-model="defaultModelIdInput"
          class="text-input-menu"
          :options="modelOptions"
          placeholder="跟随默认 / 未指定"
          clearable
          block
        />
        <span class="hint-text">在当前助手下新建对话时默认选中的模型</span>
      </div>

      <!-- 系统提示词配置 -->
      <div class="field prompt-config-field">
        <div class="flex items-center justify-between pb-1">
          <span class="label-text">系统提示词 (System Prompt)</span>
          <div class="flex items-center gap-3 text-xs">
            <label class="flex items-center gap-1.5 cursor-pointer">
              <input
                v-model="promptSource"
                type="radio"
                value="prompt"
                class="accent-[var(--color-primary)]"
              />
              <span>引用提示词管理 (推荐)</span>
            </label>
            <label class="flex items-center gap-1.5 cursor-pointer">
              <input
                v-model="promptSource"
                type="radio"
                value="custom"
                class="accent-[var(--color-primary)]"
              />
              <span>自定义输入</span>
            </label>
          </div>
        </div>

        <!-- 引用提示词模式 -->
        <div v-if="promptSource === 'prompt'" class="space-y-2 pt-1">
          <UiSelectMenu
            v-model="promptIdInput"
            class="text-input-menu"
            :options="promptSelectOptions"
            placeholder="请选择角色类提示词…"
            block
          />
          <div v-if="selectedPromptPreview" class="prompt-preview-box">
            <div class="text-2xs text-[var(--color-text-muted)] mb-1">提示词内容预览:</div>
            <div class="text-xs font-mono text-[var(--color-text-muted)] bg-[var(--color-surface-muted)] p-2.5 rounded border border-[var(--color-border)] max-h-32 overflow-y-auto whitespace-pre-wrap leading-relaxed">
              {{ selectedPromptPreview }}
            </div>
          </div>
          <span class="hint-text">统一引用平台提示词库。当提示词在平台设置中更新后，本助手将自动同步最新人设</span>
        </div>

        <!-- 手动输入模式 -->
        <div v-else class="space-y-1 pt-1">
          <textarea
            v-model="systemPromptInput"
            class="text-input textarea-input"
            rows="4"
            placeholder="例如：你是一个严谨的代码审查助手，请从代码规范和潜在漏洞角度给出分析意见…"
          ></textarea>
          <span class="hint-text">新建对话时将注入模型上下文，不在对话消息气泡中体现</span>
        </div>
      </div>

      <!-- 浮窗显示开关 -->
      <div class="field checkbox-field">
        <label class="checkbox-label">
          <input v-model="showInFloatInput" type="checkbox" class="checkbox-input" />
          <span class="checkbox-text">在悬浮快捷按钮上显示该助手</span>
        </label>
        <span class="hint-text">勾选后将在全局悬浮按钮中展示专属图标，点击可快速建立对话</span>
      </div>

      <!-- 平台管理配置 -->
      <div class="field checkbox-field">
        <label class="checkbox-label">
          <input v-model="enablePlatformToolsInput" type="checkbox" class="checkbox-input" />
          <span class="checkbox-text">开启平台管理权限（平台管家模式）</span>
        </label>
        <span class="hint-text">允许该助手通过工具调用操作平台自身资源（如查询项目、增删改查提示词等）</span>
      </div>

      <!-- 单轮最大工具调用次数 -->
      <div v-if="enablePlatformToolsInput" class="field">
        <div class="label-row">
          <span class="label-text">单轮最大工具调用次数</span>
          <span class="hint-text">范围 1~20，默认 5 步</span>
        </div>
        <input
          v-model.number="maxToolStepsInput"
          type="number"
          min="1"
          max="20"
          class="text-input"
          placeholder="5"
        />
        <span class="hint-text">单次对话中最多允许模型连续执行的工具步数，防止死循环</span>
      </div>

      <p v-if="error" class="error-msg" role="alert">{{ error }}</p>

      <div class="actions">
        <UiButton variant="secondary" type="button" @click="emit('close')">取消</UiButton>
        <UiButton variant="primary" type="submit" :disabled="submitting || !nameInput.trim()">
          {{ submitting ? '提交中…' : confirmLabel }}
        </UiButton>
      </div>
    </form>
  </UiDialog>
</template>

<style scoped>
.assistant-form {
  display: flex;
  flex-direction: column;
  gap: calc(var(--space-unit) * 1.5);
  min-width: 340px;
}

.header-section {
  display: flex;
  gap: calc(var(--space-unit) * 1.5);
  align-items: center;
}

.avatar-preview {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 54px;
  height: 54px;
  border-radius: 12px;
  background: var(--color-primary);
  color: #fff;
  font-size: 1.6rem;
  font-weight: 600;
  box-shadow: 0 4px 12px rgba(15, 23, 42, 0.15);
  flex-shrink: 0;
  user-select: none;
}

.header-fields {
  flex: 1;
}

.field {
  display: flex;
  flex-direction: column;
  gap: calc(var(--space-unit) * 0.5);
}

.label-row {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
}

.label-text {
  font-size: 0.857rem;
  font-weight: 500;
  color: var(--color-text-muted);
}

.required {
  color: var(--color-danger);
}

.hint-text {
  font-size: 0.786rem;
  color: var(--color-text-muted);
}

.preset-icons {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  align-items: center;
}

.icon-option {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  font-size: 1.1rem;
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  background: var(--color-surface);
  cursor: pointer;
  transition: all 0.15s ease;
}

.icon-option:hover {
  border-color: var(--color-primary);
  transform: scale(1.08);
}

.icon-option.active {
  border-color: var(--color-primary);
  background: var(--color-info-soft, rgba(59, 130, 246, 0.1));
}

.clear-icon-btn {
  font-size: 0.786rem;
  color: var(--color-primary);
  background: transparent;
  border: none;
  cursor: pointer;
  padding: 4px 8px;
  text-decoration: underline;
}

.text-input {
  padding: calc(var(--space-unit) * 0.8) calc(var(--space-unit) * 1.2);
  border: 1px solid var(--color-border-strong);
  border-radius: var(--radius);
  background: var(--color-surface);
  color: var(--color-text);
  font: inherit;
  font-size: 0.929rem;
  outline: none;
  transition: border-color 0.15s ease, box-shadow 0.15s ease;
}

.text-input:focus {
  border-color: var(--color-primary);
  box-shadow: 0 0 0 2px var(--color-info-soft);
}

.select-input {
  cursor: pointer;
}

.textarea-input {
  resize: vertical;
  min-height: 80px;
}

.checkbox-field {
  gap: calc(var(--space-unit) * 0.25);
}

.checkbox-label {
  display: inline-flex;
  align-items: center;
  gap: calc(var(--space-unit) * 0.75);
  cursor: pointer;
  user-select: none;
}

.checkbox-input {
  width: 16px;
  height: 16px;
  accent-color: var(--color-primary);
  cursor: pointer;
}

.checkbox-text {
  font-size: 0.875rem;
  font-weight: 500;
  color: var(--color-text);
}

.error-msg {
  margin: 0;
  color: var(--color-danger);
  font-size: 0.857rem;
}

.actions {
  display: flex;
  justify-content: flex-end;
  gap: calc(var(--space-unit) * 1);
  margin-top: calc(var(--space-unit) * 0.5);
}
</style>
