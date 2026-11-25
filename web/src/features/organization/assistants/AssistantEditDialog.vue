<script setup lang="ts">
// AssistantEditDialog: create or edit one AI assistant.
//
// This used to be a full-page workbench panel that replaced the list, hid the URL
// and re-implemented Esc / Ctrl+S by hand. It is now a form inside the shared
// `EntityFormDialog` shell: the route stays stable, the shell owns the header,
// the save strip and the shortcuts, and the four numbered sections survive as
// sections. Validation is `el-form` + `rules` (inline, under the field that
// failed); the save result is reported by `ElMessage`.
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { ElMessage, type FormInstance, type FormRules } from 'element-plus'
import type { ChatGroup, ChatGroupInput } from '../../../api/chats'
import { useChatsStore } from '../../../stores/chats'
import { useModelsStore } from '../../../stores/models'
import { usePromptsStore } from '../../../stores/prompts'
import EntityFormDialog from '../../../shared/components/EntityFormDialog.vue'

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

const formRef = ref<FormInstance>()
const submitting = ref(false)

const form = reactive({
  name: '',
  defaultModelId: '' as number | '',
  promptSource: 'prompt' as 'prompt' | 'custom',
  promptId: 0 as number | 0,
  systemPrompt: '',
  icon: '',
  showInFloat: false,
  enablePlatformTools: false,
  maxToolSteps: 10,
})

// Assistant avatars are user-chosen glyphs, not interface icons, so they keep the
// platform's existing preset set.
const PRESET_ICONS = [
  '🤖', '💡', '💻', '📝', '🎨', '🔍', '🚀', '⚙️', // taste-skill-ignore
  '🛠️', '📊', '🌐', '📚', '💬', '⚡', '🎯', '🔮', // taste-skill-ignore
]

const isEdit = computed(() => !!props.assistant)

const rules = computed<FormRules>(() => ({
  name: [
    { required: true, message: '请输入助手名称', trigger: 'blur' },
    { max: 60, message: '助手名称不能超过 60 个字符', trigger: 'blur' },
  ],
  promptId: [
    {
      validator: (_rule, _value, callback) => {
        if (form.promptSource === 'prompt' && !form.promptId) {
          callback(new Error('请选择绑定的角色提示词，或切换为自定义人设'))
          return
        }
        callback()
      },
      trigger: 'change',
    },
  ],
}))

onMounted(async () => {
  await Promise.all([
    modelsStore.services.length === 0 ? modelsStore.load() : Promise.resolve(),
    promptsStore.prompts.length === 0 ? promptsStore.loadPrompts() : Promise.resolve(),
  ])
  initForm()
})

watch(
  () => props.assistant,
  () => initForm(),
  { deep: true },
)

function initForm() {
  formRef.value?.clearValidate()
  if (props.assistant) {
    form.name = props.assistant.name || ''
    form.defaultModelId = props.assistant.defaultModelId || ''
    form.icon = props.assistant.icon || ''
    form.showInFloat = !!props.assistant.showInFloat
    form.enablePlatformTools = !!props.assistant.enablePlatformTools
    form.maxToolSteps = props.assistant.maxToolSteps || 10
    if (props.assistant.promptId && props.assistant.promptId > 0) {
      form.promptSource = 'prompt'
      form.promptId = props.assistant.promptId
      form.systemPrompt = ''
    } else {
      form.promptSource = props.assistant.systemPrompt ? 'custom' : 'prompt'
      form.promptId = 0
      form.systemPrompt = props.assistant.systemPrompt || ''
    }
    return
  }
  form.name = ''
  form.defaultModelId = ''
  form.icon = '🤖' // taste-skill-ignore
  form.showInFloat = false
  form.enablePlatformTools = false
  form.maxToolSteps = 10
  form.systemPrompt = ''
  const roles = (promptsStore.prompts || []).filter((p) => p.type === 'role')
  if (roles.length > 0) {
    form.promptSource = 'prompt'
    form.promptId = roles[0].id
  } else {
    form.promptSource = 'custom'
    form.promptId = 0
  }
}

const rolePrompts = computed(() => (promptsStore.prompts || []).filter((p) => p.type === 'role'))

const promptSourceOptions = [
  { label: '引用提示词库', value: 'prompt' },
  { label: '自定义人设', value: 'custom' },
]

const usableModels = computed(
  () =>
    (modelsStore.services ?? []).flatMap((service) =>
      (service.models ?? [])
        .filter((model) => model.usable && model.supportsText)
        .map((model) => ({ id: model.id, name: `${model.name || model.modelId} (${service.name})` })),
    ),
)

const selectedPromptPreview = computed(() => {
  if (!form.promptId) return ''
  const p = promptsStore.byId(form.promptId)
  return p ? p.content : ''
})

const currentAvatar = computed(() => {
  if (form.icon.trim()) return form.icon.trim()
  const trimmed = form.name.trim()
  return trimmed ? Array.from(trimmed)[0].toUpperCase() : '助'
})

function selectIcon(icon: string) {
  form.icon = form.icon === icon ? '' : icon
}

async function handleSubmit() {
  if (submitting.value) return
  const valid = await formRef.value?.validate().catch(() => false)
  if (!valid) return

  submitting.value = true
  const payload: ChatGroupInput = {
    name: form.name.trim(),
    icon: form.icon.trim() || undefined,
    defaultModelId: form.defaultModelId === '' ? undefined : Number(form.defaultModelId),
    promptId: form.promptSource === 'prompt' && form.promptId ? Number(form.promptId) : undefined,
    systemPrompt: form.promptSource === 'custom' ? form.systemPrompt.trim() || undefined : undefined,
    showInFloat: form.showInFloat,
    enablePlatformTools: form.enablePlatformTools,
    maxToolSteps: form.enablePlatformTools ? Number(form.maxToolSteps) || 10 : undefined,
  }

  try {
    let saved: ChatGroup | null = null
    if (isEdit.value && props.assistant) {
      const ok = await chatsStore.updateGroup(props.assistant.id, payload)
      if (!ok) {
        ElMessage.error(chatsStore.actionError || '更新 AI 助手失败')
        return
      }
      saved = chatsStore.groupById(props.assistant.id) || props.assistant
    } else {
      saved = await chatsStore.createGroup(payload)
      if (!saved) {
        ElMessage.error(chatsStore.actionError || '创建 AI 助手失败')
        return
      }
    }
    ElMessage.success(isEdit.value ? `已保存「${saved.name}」` : `已创建助手「${saved.name}」`)
    emit('saved', saved)
  } catch (err: any) {
    ElMessage.error(err?.message || '保存 AI 助手失败，请稍后重试。')
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <EntityFormDialog
    :title="isEdit ? `编辑 AI 助手 · ${props.assistant?.name}` : '新建 AI 助手'"
    subtitle="日常大模型人设与平台自治工具治理，支持常驻桌面快捷助手随时呼出。"
    :identifier="isEdit ? props.assistant?.id : null"
    :saving="submitting"
    save-test="btn-save-assistant"
    width="920"
    :data-test="`assistant-edit-dialog-${isEdit ? 'edit' : 'create'}`"
    @save="handleSubmit"
    @cancel="emit('cancel')"
  >
    <el-form
      ref="formRef"
      :model="form"
      :rules="rules"
      label-position="top"
      @submit.prevent="handleSubmit"
    >
      <section class="entity-form-section">
        <header class="entity-section-head">
          <h3 class="entity-section-title"><span class="entity-section-num font-mono">01</span>基础档案与视觉标识</h3>
          <p class="entity-section-desc">定义该助手的直观名称、代表性图标及人设职责定位。</p>
        </header>

        <div class="entity-grid-3">
          <div class="span-2">
            <div class="entity-grid-2">
              <el-form-item label="助手名称" prop="name">
                <el-input
                  v-model="form.name"
                  placeholder="例如：开发助手、架构设计顾问、文档管家…"
                  maxlength="60"
                  data-test="input-assistant-name"
                />
              </el-form-item>

              <el-form-item label="自定义图标 / 字符">
                <div class="flex items-center gap-2 w-full">
                  <el-input v-model="form.icon" placeholder="单个汉字 / 英文标识" maxlength="10" />
                  <el-button v-if="form.icon" size="small" @click="form.icon = ''">重置</el-button>
                </div>
              </el-form-item>
            </div>
          </div>

          <div class="space-y-2">
            <label class="text-xs font-medium text-foreground">头像预览与预设推荐</label>
            <div class="avatar-preview">
              <div class="avatar-glyph select-none">{{ currentAvatar }}</div>
              <div class="min-w-0">
                <div class="text-xs font-medium text-foreground truncate">{{ form.name || '助手名称' }}</div>
                <div class="text-3xs text-platform-text-muted truncate mt-0.5">视觉代表徽章</div>
              </div>
            </div>
            <div class="flex flex-wrap gap-1">
              <button
                v-for="icon in PRESET_ICONS"
                :key="icon"
                type="button"
                class="preset-chip"
                :class="form.icon === icon ? 'is-picked' : ''"
                @click="selectIcon(icon)"
              >
                {{ icon }}
              </button>
            </div>
          </div>
        </div>
      </section>

      <section class="entity-form-section">
        <header class="entity-section-head">
          <h3 class="entity-section-title"><span class="entity-section-num font-mono">02</span>运行模型与推理配置</h3>
          <p class="entity-section-desc">指定该助手发起对话时默认使用的大语言模型，或跟随全局默认设定。</p>
        </header>

        <el-form-item label="默认运行大模型">
          <div class="w-full max-w-md space-y-1">
            <el-select v-model="form.defaultModelId" placeholder="跟随默认 / 未指定" data-test="select-assistant-model">
              <el-option label="跟随默认 / 未指定" value="" />
              <el-option v-for="m in usableModels" :key="m.id" :label="m.name" :value="m.id" />
            </el-select>
            <p class="entity-field-hint">若选择「跟随默认」，助手将自动使用系统平台服务配置中设定的主力对话模型。</p>
          </div>
        </el-form-item>
      </section>

      <section class="entity-form-section">
        <header class="entity-section-head entity-section-head-split">
          <div>
            <h3 class="entity-section-title"><span class="entity-section-num font-mono">03</span>人设规范与系统提示词</h3>
            <p class="entity-section-desc">赋予助手专业的思考框架、角色定位、回复风格与工程准则。</p>
          </div>
          <el-segmented v-model="form.promptSource" :options="promptSourceOptions" size="small" />
        </header>

        <el-form-item v-if="form.promptSource === 'prompt'" label="选择角色类提示词" prop="promptId">
          <div class="w-full space-y-2">
            <el-select
              v-model="form.promptId"
              filterable
              placeholder="从提示词库选择角色规范…"
              class="w-full max-w-md"
              data-test="select-assistant-prompt"
            >
              <el-option v-for="p in rolePrompts" :key="p.id" :label="p.name" :value="p.id">
                <span>{{ p.name }}</span>
                <span v-if="p.description" class="entity-option-sub">{{ p.description }}</span>
              </el-option>
            </el-select>

            <div v-if="selectedPromptPreview" class="entity-prompt-preview font-mono select-text">
              {{ selectedPromptPreview }}
            </div>
            <p v-else class="entity-field-hint">提示：未选择提示词时，将无法加载专属预设人设。</p>
          </div>
        </el-form-item>

        <el-form-item v-else label="专属系统提示词（System Prompt）">
          <div class="w-full space-y-1">
            <el-input
              v-model="form.systemPrompt"
              type="textarea"
              :rows="7"
              resize="vertical"
              class="entity-mono-input"
              placeholder="输入该 AI 助手的系统人设与指导指令，例如：你是一个经验丰富的前端架构专家，专注于 Vue 3、TypeScript 与响应式设计…"
              data-test="textarea-assistant-prompt"
            />
            <p class="entity-field-hint text-right font-mono">{{ form.systemPrompt.length }} 字</p>
          </div>
        </el-form-item>
      </section>

      <section class="entity-form-section">
        <header class="entity-section-head">
          <h3 class="entity-section-title"><span class="entity-section-num font-mono">04</span>交互形态与自治能力</h3>
          <p class="entity-section-desc">控制助手在全局桌面环境中的驻留方式，以及是否具备平台级自治执行工具权限。</p>
        </header>

        <div class="space-y-2">
          <div class="entity-switch-row">
            <div>
              <div class="entity-switch-title">常驻桌面快捷助手</div>
              <div class="entity-switch-desc">
                开启后，该助手将出现在页面右下角的常驻快捷助手呼出栏中，随时点击即刻发起新对话。
              </div>
            </div>
            <el-switch v-model="form.showInFloat" class="mt-0.5" />
          </div>

          <div class="entity-switch-row">
            <div>
              <div class="entity-switch-title">开启平台自治工具能力</div>
              <div class="entity-switch-desc">
                允许该助手在对话中自主分析并调用平台工具链（如项目文件检索、环境诊断等）。
              </div>
            </div>
            <el-switch v-model="form.enablePlatformTools" class="mt-0.5" />
          </div>

          <el-form-item
            v-if="form.enablePlatformTools"
            label="自治思考步数上限（Max Steps）"
            class="stepper"
          >
            <div class="space-y-1">
              <el-input-number v-model="form.maxToolSteps" :min="1" :max="50" controls-position="right" />
              <p class="entity-field-hint">防止大模型陷入长时死循环，单轮对话最大自治工具调用步数。</p>
            </div>
          </el-form-item>
        </div>
      </section>
    </el-form>
  </EntityFormDialog>
</template>

<style scoped>
/* Only the assistant's own chrome: the avatar preview and its preset chips. The
   dialog shell, section rhythm and hints come from the shared editor styles. */
.span-2 {
  grid-column: span 2;
}

.avatar-preview {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 12px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  background: var(--color-surface-muted);
}

.avatar-glyph {
  width: 44px;
  height: 44px;
  flex: none;
  display: flex;
  align-items: center;
  justify-content: center;
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  background: var(--color-surface);
  font-size: var(--text-size-20);
  color: var(--color-text);
}

.preset-chip {
  width: 26px;
  height: 26px;
  display: flex;
  align-items: center;
  justify-content: center;
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  background: var(--color-surface);
  font-size: var(--text-size-13);
  transition:
    border-color 150ms cubic-bezier(0.2, 0, 0, 1),
    background-color 150ms cubic-bezier(0.2, 0, 0, 1);
}

.preset-chip:hover {
  background: var(--color-surface-muted);
}

.preset-chip.is-picked {
  border-color: var(--color-primary);
  background: color-mix(in srgb, var(--color-primary) 8%, var(--color-surface));
}

.stepper {
  margin-top: 4px;
  padding-left: 12px;
  border-left: 2px solid color-mix(in srgb, var(--color-primary) 40%, transparent);
}
</style>
