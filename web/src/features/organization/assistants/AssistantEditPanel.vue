<script setup lang="ts">
// AssistantEditPanel: Full-page in-place workbench panel for creating or editing an AI Assistant.
// Runs inside the main workbench view without modal dialogs or backdrop overlays.
// Conforms to design-taste-frontend standards:
//   - Clean header with back button, breadcrumb title, and sticky primary actions
//   - Centered max-w-4xl structured form with clear card sections
//   - Real-time preview of avatar, prompt, and tool governance
//   - Keyboard shortcut support (Ctrl+S to save, Esc to cancel)
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import type { ChatGroup, ChatGroupInput } from '../../../api/chats'
import { useChatsStore } from '../../../stores/chats'
import { useModelsStore } from '../../../stores/models'
import { usePromptsStore } from '../../../stores/prompts'
import {
  UiButton,
  UiInput,
  UiTextarea,
  UiSelectMenu,
  UiAlert,
  type SelectMenuItem,
} from '../../../shared/ui'

const props = defineProps<{
  assistant?: ChatGroup | null
}>()

const emit = defineEmits<{
  saved: [assistant: ChatGroup]
  cancel: []
}>()

const chatsStore = useChatsStore()
const modelsStore = useModelsStore()
const promptsStore = usePromptsStore()

const nameInput = ref('')
const defaultModelIdInput = ref<number | ''>('')
const promptSource = ref<'prompt' | 'custom'>('prompt')
const promptIdInput = ref<number | 0>(0)
const systemPromptInput = ref('')
const iconInput = ref('')
const showInFloatInput = ref(false)
const enablePlatformToolsInput = ref(false)
const maxToolStepsInput = ref(10)

const error = ref('')
const submitting = ref(false)

const PRESET_ICONS = [
  '🤖', '💡', '💻', '📝', '🎨', '🔍', '🚀', '⚙️', // taste-skill-ignore
  '🛠️', '📊', '🌐', '📚', '💬', '⚡', '🎯', '🔮', // taste-skill-ignore
]

const isEdit = computed(() => !!props.assistant)

onMounted(async () => {
  await Promise.all([
    modelsStore.services.length === 0 ? modelsStore.load() : Promise.resolve(),
    promptsStore.prompts.length === 0 ? promptsStore.loadPrompts() : Promise.resolve(),
  ])
  initForm()
  window.addEventListener('keydown', onKeyDown)
})

onUnmounted(() => {
  window.removeEventListener('keydown', onKeyDown)
})

function onKeyDown(e: KeyboardEvent) {
  if ((e.ctrlKey || e.metaKey) && e.key === 's') {
    e.preventDefault()
    void handleSubmit()
  } else if (e.key === 'Escape') {
    e.preventDefault()
    handleCancel()
  }
}

watch(
  () => props.assistant,
  () => {
    initForm()
  },
  { deep: true },
)

function initForm() {
  error.value = ''
  if (props.assistant) {
    nameInput.value = props.assistant.name || ''
    defaultModelIdInput.value = props.assistant.defaultModelId || ''
    if (props.assistant.promptId && props.assistant.promptId > 0) {
      promptSource.value = 'prompt'
      promptIdInput.value = props.assistant.promptId
      systemPromptInput.value = ''
    } else {
      promptSource.value = props.assistant.systemPrompt ? 'custom' : 'prompt'
      promptIdInput.value = 0
      systemPromptInput.value = props.assistant.systemPrompt || ''
    }
    iconInput.value = props.assistant.icon || ''
    showInFloatInput.value = !!props.assistant.showInFloat
    enablePlatformToolsInput.value = !!props.assistant.enablePlatformTools
    maxToolStepsInput.value = props.assistant.maxToolSteps || 10
  } else {
    nameInput.value = ''
    defaultModelIdInput.value = ''
    const roles = (promptsStore.prompts || []).filter((p) => p.type === 'role')
    if (roles.length > 0) {
      promptSource.value = 'prompt'
      promptIdInput.value = roles[0].id
    } else {
      promptSource.value = 'custom'
      promptIdInput.value = 0
    }
    systemPromptInput.value = ''
    iconInput.value = '🤖'
    showInFloatInput.value = false
    enablePlatformToolsInput.value = false
    maxToolStepsInput.value = 10
  }
}

const rolePrompts = computed(() => (promptsStore.prompts || []).filter((p) => p.type === 'role'))

const promptSelectOptions = computed<SelectMenuItem<number | 0>[]>(() => {
  const items: SelectMenuItem<number | 0>[] = [
    { value: 0, label: '请选择角色类提示词…' },
  ]
  for (const p of rolePrompts.value) {
    items.push({
      value: p.id,
      label: p.name,
      sublabel: p.description || undefined,
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
  (modelsStore.services ?? []).flatMap((service) =>
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

async function handleSubmit() {
  const name = nameInput.value.trim()
  if (!name) {
    error.value = '请输入助手名称。'
    return
  }
  if (name.length > 60) {
    error.value = '助手名称不能超过 60 个字符。'
    return
  }
  if (promptSource.value === 'prompt' && !promptIdInput.value) {
    error.value = '请选择绑定的角色提示词，或切换为直接输入人设。'
    return
  }

  error.value = ''
  submitting.value = true

  const payload: ChatGroupInput = {
    name,
    icon: iconInput.value.trim() || undefined,
    defaultModelId: defaultModelIdInput.value === '' ? undefined : Number(defaultModelIdInput.value),
    promptId: promptSource.value === 'prompt' && promptIdInput.value ? Number(promptIdInput.value) : undefined,
    systemPrompt: promptSource.value === 'custom' ? systemPromptInput.value.trim() || undefined : undefined,
    showInFloat: showInFloatInput.value,
    enablePlatformTools: enablePlatformToolsInput.value,
    maxToolSteps: enablePlatformToolsInput.value ? Number(maxToolStepsInput.value) || 10 : undefined,
  }

  try {
    let saved: ChatGroup | null = null
    if (isEdit.value && props.assistant) {
      const ok = await chatsStore.updateGroup(props.assistant.id, payload)
      if (!ok) {
        error.value = chatsStore.actionError || '更新 AI 助手失败'
        return
      }
      saved = chatsStore.groupById(props.assistant.id) || props.assistant
    } else {
      saved = await chatsStore.createGroup(payload)
      if (!saved) {
        error.value = chatsStore.actionError || '创建 AI 助手失败'
        return
      }
    }
    emit('saved', saved)
  } catch (err: any) {
    error.value = err?.message || '保存 AI 助手失败，请稍后重试。'
  } finally {
    submitting.value = false
  }
}

function handleCancel() {
  emit('cancel')
}
</script>

<template>
  <div class="h-full flex flex-col min-h-0 bg-[var(--color-bg)] overflow-hidden" data-test="assistant-edit-panel">
    <!-- Top Action Header: Consistent with Employee & Team Edit Views -->
    <header class="px-6 py-4 border-b border-border bg-surface shrink-0">
      <div class="flex items-center justify-between gap-4">
        <div class="flex items-center gap-3 min-w-0">
          <button
            type="button"
            class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded text-xs font-medium text-platform-text-muted hover:text-foreground hover:bg-surface-muted transition-colors border border-border"
            title="返回列表 (Esc)"
            @click="handleCancel"
          >
            <svg viewBox="0 0 16 16" width="12" height="12" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <polyline points="10 12 6 8 10 4" />
            </svg>
            <span>返回列表</span>
          </button>

          <div class="h-4 w-px bg-border"></div>

          <div class="min-w-0">
            <div class="flex items-center gap-2">
              <h1 class="text-base font-semibold text-foreground tracking-tight truncate">
                {{ isEdit ? `编辑 AI 助手 · ${props.assistant?.name}` : '新建 AI 助手' }}
              </h1>
              <span v-if="isEdit" class="px-1.5 py-0.5 rounded text-3xs font-mono bg-surface-muted text-platform-text-muted border border-border">
                #{{ props.assistant?.id }}
              </span>
            </div>
            <p class="text-2xs text-platform-text-muted truncate mt-0.5">
              日常大模型人设与平台自治工具治理，支持常驻桌面快捷助手随时呼出。
            </p>
          </div>
        </div>

        <div class="flex items-center gap-2 shrink-0">
          <UiButton
            variant="secondary"
            class="text-xs"
            @click="handleCancel"
          >
            取消
          </UiButton>
          <UiButton
            variant="primary"
            class="text-xs"
            :disabled="submitting"
            data-test="btn-save-assistant"
            @click="handleSubmit"
          >
            <svg v-if="submitting" class="animate-spin -ml-0.5 mr-1.5 h-3.5 w-3.5" viewBox="0 0 24 24" fill="none">
              <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
              <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z"></path>
            </svg>
            <span>{{ submitting ? '保存中...' : (isEdit ? '保存修改' : '创建助手') }}</span>
            <span class="ml-1.5 text-3xs opacity-60 font-mono hidden sm:inline">Ctrl+S</span>
          </UiButton>
        </div>
      </div>
    </header>

    <!-- Scrollable Form Body -->
    <div class="flex-1 min-h-0 overflow-auto p-6">
      <div class="max-w-4xl mx-auto space-y-6">
        <!-- Error Alert -->
        <UiAlert v-if="error" variant="error">
          {{ error }}
        </UiAlert>

        <!-- Card 1: Basic Profile & Visual Avatar -->
        <div class="border border-border rounded-lg bg-surface p-5 space-y-4">
          <div class="border-b border-border/60 pb-3">
            <h2 class="text-xs font-semibold text-foreground tracking-wide uppercase">01 基础档案与视觉标识</h2>
            <p class="text-2xs text-platform-text-muted mt-0.5">定义该助手的直观名称、代表性图标及人设职责定位。</p>
          </div>

          <div class="grid grid-cols-1 md:grid-cols-3 gap-5">
            <!-- Name & Description -->
            <div class="md:col-span-2 space-y-3">
              <div class="space-y-1.5">
                <label class="text-xs font-medium text-foreground">
                  助手名称 <span class="text-red-500">*</span>
                </label>
                <UiInput
                  v-model="nameInput"
                  placeholder="例如：开发助手、架构设计顾问、文档管家…"
                  maxlength="60"
                  data-test="input-assistant-name"
                />
              </div>

              <div class="space-y-1.5">
                <label class="text-xs font-medium text-foreground">
                  自定义图标 / 字符
                </label>
                <div class="flex items-center gap-2">
                  <UiInput
                    v-model="iconInput"
                    placeholder="输入 Emoji 或单个汉字/英文标识"
                    class="flex-1"
                    maxlength="10"
                  />
                  <button
                    v-if="iconInput"
                    type="button"
                    class="px-2 py-1 text-2xs text-platform-text-muted hover:text-foreground border border-border rounded"
                    @click="iconInput = ''"
                  >
                    重置
                  </button>
                </div>
              </div>
            </div>

            <!-- Avatar Preview & Preset Icons -->
            <div class="space-y-2">
              <label class="text-xs font-medium text-foreground">
                头像预览与预设推荐
              </label>
              <div class="flex items-center gap-3 p-3 rounded-lg bg-surface-muted/60 border border-border">
                <div class="w-12 h-12 rounded-lg bg-surface border border-border flex items-center justify-center text-2xl shadow-2xs shrink-0 select-none font-medium">
                  {{ currentAvatar }}
                </div>
                <div class="min-w-0">
                  <div class="text-xs font-medium text-foreground truncate">{{ nameInput || '助手名称' }}</div>
                  <div class="text-3xs text-platform-text-muted truncate mt-0.5">视觉代表徽章</div>
                </div>
              </div>

              <!-- Preset chips -->
              <div class="flex flex-wrap gap-1 pt-1">
                <button
                  v-for="icon in PRESET_ICONS"
                  :key="icon"
                  type="button"
                  class="w-7 h-7 rounded border text-sm flex items-center justify-center transition-transform hover:scale-110"
                  :class="iconInput === icon ? 'border-accent bg-accent/10' : 'border-border bg-surface hover:bg-surface-muted'"
                  @click="selectIcon(icon)"
                >
                  {{ icon }}
                </button>
              </div>
            </div>
          </div>
        </div>

        <!-- Card 2: Model & Reasoning Config -->
        <div class="border border-border rounded-lg bg-surface p-5 space-y-4">
          <div class="border-b border-border/60 pb-3">
            <h2 class="text-xs font-semibold text-foreground tracking-wide uppercase">02 运行模型与推理配置</h2>
            <p class="text-2xs text-platform-text-muted mt-0.5">指定该助手发起对话时默认使用的大语言模型，或跟随全局默认设定。</p>
          </div>

          <div class="space-y-1.5 max-w-lg">
            <label class="text-xs font-medium text-foreground">
              默认运行大模型
            </label>
            <UiSelectMenu
              v-model="defaultModelIdInput"
              :options="modelOptions"
              placeholder="选择运行大模型"
              data-test="select-assistant-model"
            />
            <p class="text-3xs text-platform-text-muted mt-1">
              若选择「跟随默认」，助手将自动使用系统平台服务配置中设定的主力对话模型。
            </p>
          </div>
        </div>

        <!-- Card 3: Persona & System Prompts -->
        <div class="border border-border rounded-lg bg-surface p-5 space-y-4">
          <div class="flex items-center justify-between border-b border-border/60 pb-3">
            <div>
              <h2 class="text-xs font-semibold text-foreground tracking-wide uppercase">03 人设规范与系统提示词</h2>
              <p class="text-2xs text-platform-text-muted mt-0.5">赋予助手专业的思考框架、角色定位、回复风格与工程准则。</p>
            </div>

            <!-- Radio Segment -->
            <div class="flex items-center p-0.5 rounded bg-surface-muted border border-border text-2xs font-medium">
              <button
                type="button"
                class="px-2.5 py-1 rounded transition-colors"
                :class="promptSource === 'prompt' ? 'bg-surface text-foreground shadow-2xs font-semibold' : 'text-platform-text-muted hover:text-foreground'"
                @click="promptSource = 'prompt'"
              >
                引用提示词库
              </button>
              <button
                type="button"
                class="px-2.5 py-1 rounded transition-colors"
                :class="promptSource === 'custom' ? 'bg-surface text-foreground shadow-2xs font-semibold' : 'text-platform-text-muted hover:text-foreground'"
                @click="promptSource = 'custom'"
              >
                自定义人设
              </button>
            </div>
          </div>

          <!-- Prompt Library Reference Mode -->
          <div v-if="promptSource === 'prompt'" class="space-y-3">
            <div class="space-y-1.5 max-w-lg">
              <label class="text-xs font-medium text-foreground">
                选择角色类提示词 <span class="text-red-500">*</span>
              </label>
              <UiSelectMenu
                v-model="promptIdInput"
                :options="promptSelectOptions"
                placeholder="从提示词库选择角色规范…"
                data-test="select-assistant-prompt"
              />
            </div>

            <!-- Prompt Preview Box -->
            <div v-if="selectedPromptPreview" class="space-y-1.5">
              <label class="text-3xs font-medium text-platform-text-muted uppercase">提示词内容预览</label>
              <div class="bg-surface-muted/80 p-3.5 rounded-md border border-border text-xs text-foreground font-mono leading-relaxed max-h-48 overflow-y-auto whitespace-pre-wrap select-text">
                {{ selectedPromptPreview }}
              </div>
            </div>
            <div v-else class="text-2xs text-platform-text-muted py-2">
              提示：未选择提示词时，将无法加载专属预设人设。
            </div>
          </div>

          <!-- Custom Inline System Prompt Mode -->
          <div v-else class="space-y-1.5">
            <div class="flex items-center justify-between">
              <label class="text-xs font-medium text-foreground">
                专属系统提示词 (System Prompt)
              </label>
              <span class="text-3xs text-platform-text-muted font-mono">{{ systemPromptInput.length }} 字</span>
            </div>
            <UiTextarea
              v-model="systemPromptInput"
              placeholder="输入该 AI 助手的系统人设与指导指令，例如：你是一个经验丰富的前端架构专家，专注于 Vue 3、TypeScript 与响应式设计…"
              :rows="6"
              class="font-mono text-xs leading-relaxed"
              data-test="textarea-assistant-prompt"
            />
          </div>
        </div>

        <!-- Card 4: Specialized Capabilities & Desktop Launcher -->
        <div class="border border-border rounded-lg bg-surface p-5 space-y-4">
          <div class="border-b border-border/60 pb-3">
            <h2 class="text-xs font-semibold text-foreground tracking-wide uppercase">04 交互形态与自治能力</h2>
            <p class="text-2xs text-platform-text-muted mt-0.5">控制助手在全局桌面环境中的驻留方式，以及是否具备平台级自治执行工具权限。</p>
          </div>

          <div class="space-y-4">
            <!-- Float Launcher Switch -->
            <div class="flex items-start justify-between gap-4 p-3.5 rounded-lg border border-border bg-surface-muted/30">
              <div class="space-y-0.5">
                <div class="text-xs font-medium text-foreground">常驻桌面快捷助手</div>
                <div class="text-2xs text-platform-text-muted">
                  开启后，该助手将出现在页面右下角的常驻快捷助手呼出栏中，随时点击即刻发起新对话。
                </div>
              </div>
              <label class="relative inline-flex items-center cursor-pointer shrink-0 mt-0.5">
                <input
                  v-model="showInFloatInput"
                  type="checkbox"
                  class="sr-only peer"
                />
                <div class="w-9 h-5 bg-surface-muted peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-border after:border after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:bg-accent border border-border"></div>
              </label>
            </div>

            <!-- Autonomous Tools Switch -->
            <div class="flex items-start justify-between gap-4 p-3.5 rounded-lg border border-border bg-surface-muted/30">
              <div class="space-y-0.5">
                <div class="text-xs font-medium text-foreground">开启平台自治工具能力</div>
                <div class="text-2xs text-platform-text-muted">
                  允许该助手在对话中自主分析并调用平台工具链（如项目文件检索、环境诊断等）。
                </div>
              </div>
              <label class="relative inline-flex items-center cursor-pointer shrink-0 mt-0.5">
                <input
                  v-model="enablePlatformToolsInput"
                  type="checkbox"
                  class="sr-only peer"
                />
                <div class="w-9 h-5 bg-surface-muted peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-border after:border after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:bg-accent border border-border"></div>
              </label>
            </div>

            <!-- Max steps if enabled -->
            <div v-if="enablePlatformToolsInput" class="pl-4 border-l-2 border-accent/40 space-y-1.5 max-w-xs">
              <label class="text-xs font-medium text-foreground">
                自治思考步数上限 (Max Steps)
              </label>
              <UiInput
                v-model.number="maxToolStepsInput"
                type="number"
                min="1"
                max="50"
                placeholder="默认 10 步"
              />
              <p class="text-3xs text-platform-text-muted">防止大模型陷入长时死循环，单轮对话最大自治工具调用步数。</p>
            </div>
          </div>
        </div>

        <!-- Bottom Action Bar -->
        <div class="flex items-center justify-between pt-2 pb-6 border-t border-border">
          <span class="text-2xs text-platform-text-muted">
            已就绪，点击保存立即生效。
          </span>
          <div class="flex items-center gap-2">
            <UiButton
              variant="secondary"
              class="text-xs"
              @click="handleCancel"
            >
              取消
            </UiButton>
            <UiButton
              variant="primary"
              class="text-xs"
              :disabled="submitting"
              @click="handleSubmit"
            >
              {{ submitting ? '保存中...' : (isEdit ? '保存修改' : '创建助手') }}
            </UiButton>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
