<script setup lang="ts">
// EmployeeEditPanel: Full-page in-place workbench panel for creating or editing an AI Employee (Agent Role).
// Runs inside the main workbench view without modal dialogs or backdrop overlays.
// Conforms to design-taste-frontend standards:
//   - Clean header with back button, breadcrumb title, and sticky primary actions
//   - Centered max-w-4xl structured form with clear card sections
//   - Real-time preview of avatar color, engine integration, and prompt specifications
//   - Keyboard shortcut support (Ctrl+S to save, Esc to cancel)
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import type { AgentRole, CreateRolePayload, UpdateRolePayload } from '../../../api/roles'
import { useRolesStore } from '../../../stores/roles'
import { usePromptsStore } from '../../../stores/prompts'
import { useModelsStore } from '../../../stores/models'
import { useAgentsStore } from '../../../stores/agents'
import BackendIcon from '../../../shared/components/BackendIcon.vue'
import {
  UiButton,
  UiInput,
  UiTextarea,
  UiAlert,
  UiSelectMenu,
  type SelectMenuItem,
} from '../../../shared/ui'

const props = defineProps<{
  employee?: AgentRole | null
}>()

const emit = defineEmits<{
  saved: [employeeId: number]
  cancel: []
}>()

const rolesStore = useRolesStore()
const promptsStore = usePromptsStore()
const modelsStore = useModelsStore()
const agentsStore = useAgentsStore()

const defaultColors = ['#3b82f6', '#8b5cf6', '#10b981', '#f59e0b', '#ec4899', '#06b6d4']

const formName = ref('')
const formDescription = ref('')
const formBackend = ref('llm')
const formModel = ref('')
const formColor = ref(defaultColors[0])
const formPromptSource = ref<'prompt' | 'custom'>('prompt')
const formPromptId = ref<number | 0>(0)
const formCustomPrompt = ref('')

const error = ref('')
const submitting = ref(false)

const isEdit = computed(() => !!props.employee)

onMounted(async () => {
  await Promise.all([
    promptsStore.prompts.length === 0 ? promptsStore.loadPrompts() : Promise.resolve(),
    modelsStore.services.length === 0 ? modelsStore.load() : Promise.resolve(),
    agentsStore.backends.length === 0 ? agentsStore.loadBackends() : Promise.resolve(),
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
  () => props.employee,
  () => {
    initForm()
  },
  { deep: true },
)

function initForm() {
  error.value = ''
  if (props.employee) {
    formName.value = props.employee.name || ''
    formDescription.value = props.employee.description || ''
    formBackend.value = props.employee.backend || 'llm'
    formModel.value = props.employee.model || ''
    formColor.value = props.employee.color || defaultColors[0]
    if (props.employee.promptId && props.employee.promptId > 0) {
      formPromptSource.value = 'prompt'
      formPromptId.value = props.employee.promptId
      formCustomPrompt.value = ''
    } else {
      formPromptSource.value = props.employee.systemPrompt ? 'custom' : 'prompt'
      formPromptId.value = 0
      formCustomPrompt.value = props.employee.systemPrompt || ''
    }
  } else {
    formName.value = ''
    formDescription.value = ''
    formBackend.value = 'llm'
    const defaultOpts = llmModelSelectOptions.value
    formModel.value = defaultOpts.length > 0 ? defaultOpts[0].value : ''
    formColor.value = defaultColors[(rolesStore.roles || []).length % defaultColors.length]
    formPromptSource.value = rolePrompts.value.length > 0 ? 'prompt' : 'custom'
    formPromptId.value = rolePrompts.value.length > 0 ? rolePrompts.value[0].id : 0
    formCustomPrompt.value = ''
  }
}

interface BackendChoice {
  id: string
  label: string
}

const selectableBackends = computed<BackendChoice[]>(() => {
  const list: BackendChoice[] = [
    { id: 'llm', label: '通用大模型 (LLM)' },
  ]
  for (const b of (agentsStore.backends || [])) {
    if (b.enabled !== false) {
      list.push({
        id: b.backend,
        label: b.label || b.backend,
      })
    }
  }
  return list
})

function selectBackend(backend: string) {
  if (formBackend.value === backend) return
  formBackend.value = backend
  if (backend === 'llm') {
    const opts = llmModelSelectOptions.value
    formModel.value = opts.length > 0 ? opts[0].value : ''
  } else {
    formModel.value = ''
  }
}

const rolePrompts = computed(() => {
  return (promptsStore.prompts || []).filter((p) => p.type === 'role')
})

const promptSelectOptions = computed<SelectMenuItem<number | 0>[]>(() => {
  const items: SelectMenuItem<number | 0>[] = [
    { value: 0, label: '选择提示词…' },
  ]
  for (const p of rolePrompts.value) {
    items.push({
      value: p.id,
      label: p.name,
      sublabel: p.description
        ? (p.description.length > 50 ? p.description.slice(0, 47) + '…' : p.description)
        : undefined,
    })
  }
  return items
})

const llmModelSelectOptions = computed<SelectMenuItem<string>[]>(() => {
  const items: SelectMenuItem<string>[] = []
  for (const svc of (modelsStore.services || [])) {
    if (!svc.enabled) continue
    for (const m of (svc.models || [])) {
      if (m.usable && m.supportsText) {
        items.push({
          value: m.modelId,
          label: m.name ? `${m.name} (${m.modelId})` : m.modelId,
          sublabel: svc.name,
        })
      }
    }
  }
  return items
})

const selectedPromptPreview = computed(() => {
  if (formPromptId.value === 0) return ''
  const p = promptsStore.byId(formPromptId.value)
  return p ? p.content : ''
})

async function handleSubmit() {
  if (!formName.value.trim()) {
    error.value = '请输入员工姓名或岗位名称。'
    return
  }
  if (!formModel.value.trim()) {
    error.value = '请选择或指定运行模型。'
    return
  }
  if (formPromptSource.value === 'prompt' && (!formPromptId.value || formPromptId.value === 0)) {
    error.value = '请选择引用的角色提示词，或切换至自定义人设。'
    return
  }

  submitting.value = true
  error.value = ''

  try {
    const payload: CreateRolePayload | UpdateRolePayload = {
      name: formName.value.trim(),
      description: formDescription.value.trim(),
      backend: formBackend.value.trim(),
      model: formModel.value.trim(),
      color: formColor.value,
      promptId: formPromptSource.value === 'prompt' ? Number(formPromptId.value) : 0,
      systemPrompt: formPromptSource.value === 'custom' ? formCustomPrompt.value.trim() : undefined,
    }

    let resultId: number
    if (isEdit.value && props.employee) {
      const updated = await rolesStore.updateRole(props.employee.id, payload)
      if (!updated) {
        error.value = rolesStore.error || '更新 AI 员工失败'
        return
      }
      resultId = updated.id
    } else {
      const created = await rolesStore.createRole(payload as CreateRolePayload)
      if (!created) {
        error.value = rolesStore.error || '创建 AI 员工失败'
        return
      }
      resultId = created.id
    }
    emit('saved', resultId)
  } catch (err: any) {
    error.value = err?.message || '保存 AI 员工失败，请稍后重试。'
  } finally {
    submitting.value = false
  }
}

function handleCancel() {
  emit('cancel')
}
</script>

<template>
  <div class="h-full flex flex-col min-h-0 bg-[var(--color-bg)] overflow-hidden" data-test="employee-edit-panel">
    <!-- Top Action Header: Consistent with Assistant & Team Edit Views -->
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
                {{ isEdit ? `编辑 AI 员工 · ${props.employee?.name}` : '新建 AI 员工' }}
              </h1>
              <span v-if="isEdit" class="px-1.5 py-0.5 rounded text-3xs font-mono bg-surface-muted text-platform-text-muted border border-border">
                #{{ props.employee?.id }}
              </span>
            </div>
            <p class="text-2xs text-platform-text-muted truncate mt-0.5">
              专职数字员工与执行引擎分工，作为 AI 团队的专业协同专家编制。
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
            data-test="btn-save-employee"
            @click="handleSubmit"
          >
            <svg v-if="submitting" class="animate-spin -ml-0.5 mr-1.5 h-3.5 w-3.5" viewBox="0 0 24 24" fill="none">
              <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
              <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z"></path>
            </svg>
            <span>{{ submitting ? '保存中...' : (isEdit ? '保存修改' : '创建员工') }}</span>
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

        <!-- Card 1: Basic Profile & Visual Identity -->
        <div class="border border-border rounded-lg bg-surface p-5 space-y-4">
          <div class="border-b border-border/60 pb-3">
            <h2 class="text-xs font-semibold text-foreground tracking-wide uppercase">01 基础档案与视觉标识</h2>
            <p class="text-2xs text-platform-text-muted mt-0.5">设定专职数字员工的姓名、职位称谓、视觉色彩与主要工作职责。</p>
          </div>

          <div class="grid grid-cols-1 md:grid-cols-3 gap-5">
            <!-- Name & Description -->
            <div class="md:col-span-2 space-y-3">
              <div class="space-y-1.5">
                <label class="text-xs font-medium text-foreground">
                  员工姓名 / 岗位名称 <span class="text-red-500">*</span>
                </label>
                <UiInput
                  v-model="formName"
                  placeholder="例如：高级架构师、代码审查专家、前端效能工程师…"
                  data-test="input-employee-name"
                />
              </div>

              <div class="space-y-1.5">
                <label class="text-xs font-medium text-foreground">职责与专业定位</label>
                <UiInput
                  v-model="formDescription"
                  placeholder="简要阐述该专职员工的核心职责、技术领域与交付要求"
                />
              </div>
            </div>

            <!-- Avatar Preview & Color Palette -->
            <div class="space-y-2">
              <label class="text-xs font-medium text-foreground">标识色彩与头像</label>
              <div class="flex items-center gap-3 p-3 rounded-lg bg-surface-muted/60 border border-border">
                <span
                  class="w-12 h-12 rounded-lg flex items-center justify-center text-white text-base font-bold shadow-2xs shrink-0 transition-colors"
                  :style="{ backgroundColor: formColor }"
                >
                  {{ (formName || '工').slice(0, 1) }}
                </span>
                <div class="min-w-0">
                  <div class="text-xs font-medium text-foreground truncate">{{ formName || '员工姓名' }}</div>
                  <div class="text-3xs text-platform-text-muted truncate mt-0.5">组织协同角色</div>
                </div>
              </div>

              <!-- Color dots -->
              <div class="flex items-center gap-2 pt-1">
                <button
                  v-for="c in defaultColors"
                  :key="c"
                  type="button"
                  class="w-6 h-6 rounded-full transition-transform hover:scale-115 flex items-center justify-center"
                  :class="{ 'ring-2 ring-accent ring-offset-2 ring-offset-surface scale-110': formColor === c }"
                  :style="{ backgroundColor: c }"
                  :title="`色彩：${c}`"
                  @click="formColor = c"
                >
                  <svg v-if="formColor === c" viewBox="0 0 16 16" width="10" height="10" fill="none" stroke="#fff" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
                    <polyline points="3 8 6 11 13 4" />
                  </svg>
                </button>
              </div>
            </div>
          </div>
        </div>

        <!-- Card 2: Engine & Model Configuration -->
        <div class="border border-border rounded-lg bg-surface p-5 space-y-4">
          <div class="border-b border-border/60 pb-3">
            <h2 class="text-xs font-semibold text-foreground tracking-wide uppercase">02 执行引擎与运行模型</h2>
            <p class="text-2xs text-platform-text-muted mt-0.5">选择员工底层的执行引擎（通用大模型或异构 Coding Agent），并绑定具体模型参数。</p>
          </div>

          <!-- Backend Selection -->
          <div class="space-y-2">
            <label class="text-xs font-medium text-foreground">
              执行引擎 (Backend) <span class="text-red-500">*</span>
            </label>
            <div class="grid grid-cols-2 sm:grid-cols-4 gap-2.5">
              <button
                v-for="b in selectableBackends"
                :key="b.id"
                type="button"
                class="p-2.5 rounded-lg border text-left flex items-center gap-2.5 transition-all text-xs"
                :class="formBackend === b.id
                  ? 'border-accent bg-accent/5 font-semibold text-foreground shadow-2xs'
                  : 'border-border bg-surface hover:bg-surface-muted/60 text-platform-text-muted hover:text-foreground'"
                @click="selectBackend(b.id)"
              >
                <BackendIcon :backend="b.id" class="w-4 h-4 shrink-0" />
                <span class="truncate">{{ b.label }}</span>
              </button>
            </div>
          </div>

          <!-- Model Selection -->
          <div class="space-y-1.5 max-w-lg pt-1">
            <label class="text-xs font-medium text-foreground">
              运行模型 (Model) <span class="text-red-500">*</span>
            </label>
            <template v-if="formBackend === 'llm'">
              <UiSelectMenu
                v-model="formModel"
                :options="llmModelSelectOptions"
                placeholder="请选择通用大模型"
                data-test="select-employee-model"
              />
              <p class="text-3xs text-platform-text-muted mt-1">从模型服务配置中选择支持文本推理的大模型。</p>
            </template>
            <template v-else>
              <UiInput
                v-model="formModel"
                placeholder="请输入该 Agent 后端支持的模型标识，例如 default 或 claude-3-7-sonnet"
                data-test="input-employee-model"
              />
              <p class="text-3xs text-platform-text-muted mt-1">异构引擎支持自定义模型别名或填入 default。</p>
            </template>
          </div>
        </div>

        <!-- Card 3: Persona & System Prompts -->
        <div class="border border-border rounded-lg bg-surface p-5 space-y-4">
          <div class="flex items-center justify-between border-b border-border/60 pb-3">
            <div>
              <h2 class="text-xs font-semibold text-foreground tracking-wide uppercase">03 人设规范与系统提示词</h2>
              <p class="text-2xs text-platform-text-muted mt-0.5">决定该员工的专业技能树、编码准则、代码审查规范与交互思考模式。</p>
            </div>

            <!-- Radio Segment -->
            <div class="flex items-center p-0.5 rounded bg-surface-muted border border-border text-2xs font-medium">
              <button
                type="button"
                class="px-2.5 py-1 rounded transition-colors"
                :class="formPromptSource === 'prompt' ? 'bg-surface text-foreground shadow-2xs font-semibold' : 'text-platform-text-muted hover:text-foreground'"
                @click="formPromptSource = 'prompt'"
              >
                引用提示词库
              </button>
              <button
                type="button"
                class="px-2.5 py-1 rounded transition-colors"
                :class="formPromptSource === 'custom' ? 'bg-surface text-foreground shadow-2xs font-semibold' : 'text-platform-text-muted hover:text-foreground'"
                @click="formPromptSource = 'custom'"
              >
                自定义人设
              </button>
            </div>
          </div>

          <!-- Prompt Library Reference Mode -->
          <div v-if="formPromptSource === 'prompt'" class="space-y-3">
            <div class="space-y-1.5 max-w-lg">
              <label class="text-xs font-medium text-foreground">
                选择角色提示词 <span class="text-red-500">*</span>
              </label>
              <UiSelectMenu
                v-model="formPromptId"
                :options="promptSelectOptions"
                placeholder="选择角色类提示词"
                data-test="select-employee-prompt"
              />
            </div>

            <!-- Preview Box -->
            <div v-if="selectedPromptPreview" class="space-y-1.5">
              <label class="text-3xs font-medium text-platform-text-muted uppercase">提示词内容预览</label>
              <div class="bg-surface-muted/80 p-3.5 rounded-md border border-border text-xs text-foreground font-mono leading-relaxed max-h-48 overflow-y-auto whitespace-pre-wrap select-text">
                {{ selectedPromptPreview }}
              </div>
            </div>
            <div v-else class="text-2xs text-platform-text-muted py-2">
              提示：请先选择角色提示词以加载人设。
            </div>
          </div>

          <!-- Custom Inline Mode -->
          <div v-else class="space-y-1.5">
            <div class="flex items-center justify-between">
              <label class="text-xs font-medium text-foreground">
                专属人设定义与系统提示词 (System Prompt)
              </label>
              <span class="text-3xs text-platform-text-muted font-mono">{{ formCustomPrompt.length }} 字</span>
            </div>
            <UiTextarea
              v-model="formCustomPrompt"
              placeholder="输入该专职员工的专业职责、技术规范与输出要求，例如：你是一个严谨的架构师，主导微服务架构评审与模块边界划分…"
              :rows="6"
              class="font-mono text-xs leading-relaxed"
              data-test="textarea-employee-prompt"
            />
          </div>
        </div>

        <!-- Bottom Action Bar -->
        <div class="flex items-center justify-between pt-2 pb-6 border-t border-border">
          <span class="text-2xs text-platform-text-muted">
            已就绪，保存后可直接在 AI 团队中编制该员工。
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
              {{ submitting ? '保存中...' : (isEdit ? '保存修改' : '创建员工') }}
            </UiButton>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
