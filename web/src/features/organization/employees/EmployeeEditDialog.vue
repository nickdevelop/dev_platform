<script setup lang="ts">
// EmployeeEditDialog: create or edit one AI employee (an agent role).
//
// Was a full-page workbench panel; now a form inside the shared
// `EntityFormDialog` shell, over the list. The engine choice drives the model
// field — a governed picker for the LLM engine, a free-text CLI model name for
// the others — which is the one piece of real logic here.
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { ElMessage, type FormInstance, type FormRules } from 'element-plus'
import type { AgentRole, CreateRolePayload, UpdateRolePayload } from '../../../api/roles'
import { useRolesStore } from '../../../stores/roles'
import { usePromptsStore } from '../../../stores/prompts'
import { useModelsStore } from '../../../stores/models'
import { useAgentsStore } from '../../../stores/agents'
import BackendIcon from '../../../shared/components/BackendIcon.vue'
import EntityFormDialog from '../../../shared/components/EntityFormDialog.vue'

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

const formRef = ref<FormInstance>()
const submitting = ref(false)

const form = reactive({
  name: '',
  description: '',
  backend: 'llm',
  model: '',
  color: defaultColors[0],
  promptSource: 'prompt' as 'prompt' | 'custom',
  promptId: 0 as number | 0,
  customPrompt: '',
})

const isEdit = computed(() => !!props.employee)

const rules = computed<FormRules>(() => ({
  name: [{ required: true, message: '请输入员工姓名或岗位名称', trigger: 'blur' }],
  model: [
    {
      validator: (_rule, _value, callback) => {
        if (!form.model.trim()) callback(new Error('请选择或指定运行模型'))
        else callback()
      },
      trigger: 'change',
    },
  ],
  promptId: [
    {
      validator: (_rule, _value, callback) => {
        if (form.promptSource === 'prompt' && !form.promptId) {
          callback(new Error('请选择引用的角色提示词，或切换至自定义人设'))
          return
        }
        callback()
      },
      trigger: 'change',
    },
  ],
}))

const rolePrompts = computed(() => (promptsStore.prompts || []).filter((p) => p.type === 'role'))

const selectableBackends = computed(() => {
  const list = [{ id: 'llm', label: '通用大模型 (LLM)' }]
  for (const b of agentsStore.backends || []) {
    if (b.enabled !== false) list.push({ id: b.backend, label: b.label || b.backend })
  }
  return list
})

const llmModelOptions = computed(() => {
  const items: { value: string; label: string; service: string }[] = []
  for (const svc of modelsStore.services || []) {
    if (!svc.enabled) continue
    for (const m of svc.models || []) {
      if (m.usable && m.supportsText) {
        items.push({
          value: m.modelId,
          label: m.name ? `${m.name} (${m.modelId})` : m.modelId,
          service: svc.name,
        })
      }
    }
  }
  return items
})

const promptSourceOptions = [
  { label: '引用提示词库', value: 'prompt' },
  { label: '自定义人设', value: 'custom' },
]

const selectedPromptPreview = computed(() => {
  if (!form.promptId) return ''
  const p = promptsStore.byId(form.promptId)
  return p ? p.content : ''
})

onMounted(async () => {
  await Promise.all([
    promptsStore.prompts.length === 0 ? promptsStore.loadPrompts() : Promise.resolve(),
    modelsStore.services.length === 0 ? modelsStore.load() : Promise.resolve(),
    agentsStore.backends.length === 0 ? agentsStore.loadBackends() : Promise.resolve(),
  ])
  initForm()
})

watch(
  () => props.employee,
  () => initForm(),
  { deep: true },
)

function selectBackend(backend: string) {
  if (form.backend === backend) return
  form.backend = backend
  if (backend === 'llm') {
    const opts = llmModelOptions.value
    form.model = opts.length > 0 ? opts[0].value : ''
  } else {
    form.model = ''
  }
  formRef.value?.clearValidate(['model'])
}

function initForm() {
  formRef.value?.clearValidate()
  if (props.employee) {
    form.name = props.employee.name || ''
    form.description = props.employee.description || ''
    form.backend = props.employee.backend || 'llm'
    form.model = props.employee.model || ''
    form.color = props.employee.color || defaultColors[0]
    if (props.employee.promptId && props.employee.promptId > 0) {
      form.promptSource = 'prompt'
      form.promptId = props.employee.promptId
      form.customPrompt = ''
    } else {
      form.promptSource = props.employee.systemPrompt ? 'custom' : 'prompt'
      form.promptId = 0
      form.customPrompt = props.employee.systemPrompt || ''
    }
    return
  }
  form.name = ''
  form.description = ''
  form.backend = 'llm'
  const opts = llmModelOptions.value
  form.model = opts.length > 0 ? opts[0].value : ''
  form.color = defaultColors[(rolesStore.roles || []).length % defaultColors.length]
  form.promptSource = rolePrompts.value.length > 0 ? 'prompt' : 'custom'
  form.promptId = rolePrompts.value.length > 0 ? rolePrompts.value[0].id : 0
  form.customPrompt = ''
}

async function handleSubmit() {
  if (submitting.value) return
  const valid = await formRef.value?.validate().catch(() => false)
  if (!valid) return

  submitting.value = true
  const payload: CreateRolePayload | UpdateRolePayload = {
    name: form.name.trim(),
    description: form.description.trim(),
    backend: form.backend.trim(),
    model: form.model.trim(),
    color: form.color,
    promptId: form.promptSource === 'prompt' ? Number(form.promptId) : 0,
    systemPrompt: form.promptSource === 'custom' ? form.customPrompt.trim() : undefined,
  }

  try {
    let resultId: number
    if (isEdit.value && props.employee) {
      const updated = await rolesStore.updateRole(props.employee.id, payload)
      if (!updated) {
        ElMessage.error(rolesStore.error || '更新 AI 员工失败')
        return
      }
      resultId = updated.id
    } else {
      const created = await rolesStore.createRole(payload as CreateRolePayload)
      if (!created) {
        ElMessage.error(rolesStore.error || '创建 AI 员工失败')
        return
      }
      resultId = created.id
    }
    ElMessage.success(isEdit.value ? `已保存「${form.name.trim()}」` : `已创建 AI 员工「${form.name.trim()}」`)
    emit('saved', resultId)
  } catch (err: any) {
    ElMessage.error(err?.message || '保存 AI 员工失败，请稍后重试')
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <EntityFormDialog
    :title="isEdit ? `编辑 AI 员工 · ${props.employee?.name}` : '新建 AI 员工'"
    subtitle="专职数字员工档案、执行引擎接入与人设规范，编制后可被 AI 团队招募为协同专家。"
    :identifier="isEdit ? props.employee?.id : null"
    :saving="submitting"
    :save-label="isEdit ? '保存修改' : '创建员工'"
    save-test="btn-save-employee"
    width="880"
    :data-test="`employee-edit-dialog-${isEdit ? 'edit' : 'create'}`"
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
          <h3 class="entity-section-title"><span class="entity-section-num font-mono">01</span>员工档案与岗位</h3>
          <p class="entity-section-desc">定义数字员工的直观名称、岗位职责与身份色彩标识。</p>
        </header>

        <div class="entity-grid-2">
          <el-form-item label="姓名 / 岗位名称" prop="name">
            <el-input
              v-model="form.name"
              placeholder="例如：前端专家、架构专家、质量保证专家…"
              maxlength="40"
              data-test="input-employee-name"
            />
          </el-form-item>

          <el-form-item label="岗位职责简介">
            <el-input
              v-model="form.description"
              placeholder="例如：负责 Web 前端组件开发与性能优化…"
              maxlength="120"
            />
          </el-form-item>
        </div>

        <el-form-item label="标识色">
          <div class="swatches">
            <button
              v-for="c in defaultColors"
              :key="c"
              type="button"
              class="swatch"
              :class="form.color === c ? 'is-picked' : ''"
              :style="{ background: c }"
              :aria-label="`使用标识色 ${c}`"
              :aria-pressed="form.color === c"
              @click="form.color = c"
            />
          </div>
        </el-form-item>
      </section>

      <section class="entity-form-section">
        <header class="entity-section-head">
          <h3 class="entity-section-title"><span class="entity-section-num font-mono">02</span>执行引擎与运行模型</h3>
          <p class="entity-section-desc">
            通用大模型引擎从服务配置中选取受治理模型；其他引擎按其 CLI 规范指定模型。
          </p>
        </header>

        <div class="entity-grid-2">
          <el-form-item label="执行引擎">
            <el-select v-model="form.backend" @change="selectBackend">
              <el-option v-for="b in selectableBackends" :key="b.id" :label="b.label" :value="b.id">
                <span class="flex items-center gap-2">
                  <BackendIcon :backend="b.id" class="engine-icon" />
                  <span>{{ b.label }}</span>
                </span>
              </el-option>
            </el-select>
          </el-form-item>

          <el-form-item v-if="form.backend === 'llm'" label="运行模型" prop="model">
            <el-select
              v-model="form.model"
              filterable
              placeholder="从服务配置选择模型"
              data-test="select-employee-model"
            >
              <el-option v-for="m in llmModelOptions" :key="m.value" :label="m.label" :value="m.value">
                <span>{{ m.label }}</span>
                <span class="entity-option-sub">{{ m.service }}</span>
              </el-option>
            </el-select>
          </el-form-item>

          <el-form-item v-else label="运行模型" prop="model">
            <div class="w-full space-y-1">
              <el-input v-model="form.model" placeholder="填写 CLI 使用的模型名（如 sonnet / gpt-5）" />
              <p class="entity-field-hint">{{ form.backend }} 引擎由其自带 CLI 执行，模型名以 CLI 规范为准。</p>
            </div>
          </el-form-item>
        </div>
      </section>

      <section class="entity-form-section">
        <header class="entity-section-head entity-section-head-split">
          <div>
            <h3 class="entity-section-title"><span class="entity-section-num font-mono">03</span>人设与提示词</h3>
            <p class="entity-section-desc">选择引用的角色提示词库，或为该员工编写专属系统人设。</p>
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
              data-test="select-employee-prompt"
            >
              <el-option v-for="p in rolePrompts" :key="p.id" :label="p.name" :value="p.id">
                <span>{{ p.name }}</span>
                <span v-if="p.description" class="entity-option-sub">{{ p.description }}</span>
              </el-option>
            </el-select>

            <div v-if="selectedPromptPreview" class="entity-prompt-preview font-mono select-text">
              {{ selectedPromptPreview }}
            </div>
            <p v-else class="entity-field-hint">提示：未选择提示词时，该员工仅使用基础模型能力。</p>
          </div>
        </el-form-item>

        <el-form-item v-else label="专属系统人设（System Prompt）">
          <div class="w-full space-y-1">
            <el-input
              v-model="form.customPrompt"
              type="textarea"
              :rows="7"
              resize="vertical"
              class="entity-mono-input"
              placeholder="输入该员工的系统人设，例如：你是一名资深前端工程师，擅长 Vue 3 与 TypeScript，遵循以下准则：…"
              data-test="textarea-employee-prompt"
            />
            <p class="entity-field-hint text-right font-mono">{{ form.customPrompt.length }} 字</p>
          </div>
        </el-form-item>
      </section>
    </el-form>
  </EntityFormDialog>
</template>

<style scoped>
/* Only the employee's own control: the identity colour swatches. */
.swatches {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.swatch {
  width: 22px;
  height: 22px;
  border-radius: var(--radius);
  border: 1px solid var(--color-border);
  outline: 2px solid transparent;
  outline-offset: 1px;
  transition: outline-color 150ms cubic-bezier(0.2, 0, 0, 1);
}

.swatch.is-picked {
  outline-color: var(--color-text);
}

.engine-icon {
  width: 14px;
  height: 14px;
  flex: none;
}
</style>
