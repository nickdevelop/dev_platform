<script setup lang="ts">
// SettingsRolesView: Global Agent Role Management in Platform Settings.
// Conforms to design-taste-frontend (App Screens) guidelines:
// flat surfaces, zero resting shadows, data-first typography, quiet controls.
import { computed, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useRolesStore } from '../../stores/roles'
import { usePromptsStore } from '../../stores/prompts'
import { useModelsStore } from '../../stores/models'
import { useAgentsStore } from '../../stores/agents'
import ModelSelect from '../chat/composer/ModelSelect.vue'
import { listAgentBackendModels, type BackendId, type ModelInfo } from '../../api/agents'
import { cachedModels, rememberModels } from '../../shared/modelCache'
import type { AgentRole, CreateRolePayload, UpdateRolePayload } from '../../api/roles'
import {
  UiButton,
  UiInput,
  UiTextarea,
  UiDialog,
  UiAlert,
  UiConfirmDialog,
  UiSelectMenu,
  type SelectMenuItem,
} from '../../shared/ui'

const router = useRouter()
const rolesStore = useRolesStore()
const promptsStore = usePromptsStore()
const modelsStore = useModelsStore()
const agentsStore = useAgentsStore()

const keyword = ref('')
const showEditDialog = ref(false)
const editingRoleId = ref<number | null>(null)
const submitting = ref(false)
const dialogError = ref('')
const pendingDelete = ref<AgentRole | null>(null)
const deleting = ref(false)
const deleteError = ref('')

const defaultColors = ['#3b82f6', '#8b5cf6', '#10b981', '#f59e0b', '#ec4899', '#06b6d4']

// Form fields
const formName = ref('')
const formDescription = ref('')
const formBackend = ref('llm')
const formModel = ref('')
const isCustomModel = ref(false)
const loadingModels = ref(false)
const agentBackendModels = ref<Record<string, ModelInfo[]>>({})
const formColor = ref(defaultColors[0])
const formPromptSource = ref<'prompt' | 'custom'>('prompt')
const formPromptId = ref<number | 0>(0)
const formCustomPrompt = ref('')

onMounted(async () => {
  await Promise.all([
    rolesStore.loadRoles(),
    promptsStore.loadPrompts(),
    modelsStore.load(),
    agentsStore.loadBackends(),
  ])
})

const filteredRoles = computed(() => {
  const all = rolesStore.roles || []
  const kw = keyword.value.trim().toLowerCase()
  if (!kw) return all
  return all.filter(
    (r) =>
      r.name.toLowerCase().includes(kw) ||
      (r.description && r.description.toLowerCase().includes(kw)) ||
      r.model.toLowerCase().includes(kw),
  )
})

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

// Only role-type prompts are suggested for agent role personas
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

function getPromptName(promptId: number): string {
  if (!promptId) return ''
  const p = promptsStore.byId(promptId)
  return p ? p.name : `提示词 #${promptId}`
}

function openCreateDialog() {
  editingRoleId.value = null
  formName.value = ''
  formDescription.value = ''
  formBackend.value = 'llm'
  isCustomModel.value = false
  const defaultOpts = llmModelSelectOptions.value
  formModel.value = defaultOpts.length > 0 ? defaultOpts[0].value : ''
  formColor.value = defaultColors[(rolesStore.roles || []).length % defaultColors.length]
  formPromptSource.value = rolePrompts.value.length > 0 ? 'prompt' : 'custom'
  formPromptId.value = rolePrompts.value.length > 0 ? rolePrompts.value[0].id : 0
  formCustomPrompt.value = ''
  dialogError.value = ''
  showEditDialog.value = true
}

function openEditDialog(role: AgentRole) {
  editingRoleId.value = role.id
  formName.value = role.name
  formDescription.value = role.description || ''
  formBackend.value = role.backend || 'llm'
  formModel.value = role.model
  formColor.value = role.color || defaultColors[0]
  isCustomModel.value = false
  if (role.promptId && role.promptId > 0) {
    formPromptSource.value = 'prompt'
    formPromptId.value = role.promptId
    formCustomPrompt.value = ''
  } else {
    formPromptSource.value = 'custom'
    formPromptId.value = 0
    formCustomPrompt.value = role.systemPrompt || ''
  }
  dialogError.value = ''
  showEditDialog.value = true
}

async function handleSaveRole() {
  if (!formName.value.trim()) {
    dialogError.value = '请输入角色名称'
    return
  }
  if (!formModel.value.trim()) {
    dialogError.value = '请输入或选择模型名称'
    return
  }

  const promptId = formPromptSource.value === 'prompt' ? Number(formPromptId.value) || 0 : 0
  const systemPrompt = formPromptSource.value === 'custom' ? formCustomPrompt.value.trim() : ''

  submitting.value = true
  dialogError.value = ''
  try {
    if (editingRoleId.value) {
      const payload: UpdateRolePayload = {
        name: formName.value.trim(),
        description: formDescription.value.trim(),
        backend: formBackend.value,
        model: formModel.value.trim(),
        color: formColor.value,
        promptId,
        systemPrompt,
      }
      await rolesStore.updateRole(editingRoleId.value, payload)
    } else {
      const payload: CreateRolePayload = {
        name: formName.value.trim(),
        description: formDescription.value.trim(),
        backend: formBackend.value,
        model: formModel.value.trim(),
        color: formColor.value,
        promptId,
        systemPrompt,
      }
      await rolesStore.createRole(payload)
    }
    showEditDialog.value = false
  } catch (err: any) {
    dialogError.value = err.message || '保存角色失败'
  } finally {
    submitting.value = false
  }
}

function handleDeleteRole(role: AgentRole) {
  deleteError.value = ''
  pendingDelete.value = role
}

async function confirmDeleteRole() {
  if (!pendingDelete.value || deleting.value) return
  const target = pendingDelete.value
  deleting.value = true
  deleteError.value = ''
  try {
    await rolesStore.deleteRole(target.id)
    pendingDelete.value = null
  } catch (err: any) {
    deleteError.value = rolesStore.error || err.message || '删除角色失败'
  } finally {
    deleting.value = false
  }
}

function goToPromptsSetting() {
  void router.push({ name: 'settings-prompts' })
}
</script>

<template>
  <section class="settings-roles-view w-full space-y-6" aria-label="角色管理">
    <!-- Header Summary Panel -->
    <header class="settings-head pb-4 border-b border-[var(--color-border)] flex items-start justify-between gap-4 flex-wrap">
      <div>
        <h1 class="text-2xl font-semibold text-[var(--color-text)] tracking-tight">角色管理</h1>
        <p class="text-xs text-[var(--color-text-muted)] mt-1">
          配置平台通用的智能体专家角色，统一绑定提示词与模型，供多 Agent 协同流程与智能工作流按需选用。
        </p>
      </div>

      <div class="flex items-center gap-3">
        <UiButton
          variant="primary"
          class="text-xs px-3.5 py-1.5"
          data-test="btn-create-role"
          @click="openCreateDialog"
        >
          <svg viewBox="0 0 16 16" width="12" height="12" fill="none" stroke="currentColor" stroke-width="2" class="mr-1.5 inline-block">
            <line x1="8" y1="3" x2="8" y2="13" />
            <line x1="3" y1="8" x2="13" y2="8" />
          </svg>
          新建角色
        </UiButton>
      </div>
    </header>

    <!-- Filter & Statistics Bar -->
    <div class="flex items-center justify-between gap-4 flex-wrap">
      <div class="flex items-center gap-2 flex-1 max-w-sm">
        <UiInput
          v-model="keyword"
          placeholder="搜索角色名称、模型或描述…"
          class="text-xs"
          block
        />
      </div>

      <div class="text-xs text-[var(--color-text-muted)] font-mono tabular-nums">
        共 {{ (rolesStore.roles || []).length }} 个专家角色
      </div>
    </div>

    <!-- Role List Grid -->
    <div v-if="filteredRoles.length > 0" class="grid grid-cols-1 lg:grid-cols-2 gap-4">
      <div
        v-for="role in filteredRoles"
        :key="role.id"
        class="role-card p-4 rounded-[var(--radius,6px)] bg-[var(--color-surface)] border border-[var(--color-border)] flex flex-col justify-between gap-3 hover:border-[var(--color-primary)] transition-colors"
        :data-test="`role-card-${role.id}`"
      >
        <!-- Top row: Avatar color dot, Name, Backend & Model badges, Actions -->
        <div class="space-y-2">
          <div class="flex items-start justify-between gap-2">
            <div class="flex items-center gap-2.5 min-w-0">
              <span
                class="w-3.5 h-3.5 rounded-full flex-shrink-0"
                :style="{ backgroundColor: role.color || '#3b82f6' }"
              />
              <span class="text-sm font-semibold text-[var(--color-text)] truncate">
                {{ role.name }}
              </span>
              <span class="text-2xs px-1.5 py-0.5 rounded bg-[var(--color-surface-muted)] text-[var(--color-text-muted)] font-mono border border-[var(--color-border)]">
                {{ role.backend }}
              </span>
            </div>

            <div class="flex items-center gap-1.5">
              <button
                type="button"
                class="text-xs text-[var(--color-text-muted)] hover:text-[var(--color-text)] px-1.5 py-0.5 rounded transition-colors"
                title="编辑角色"
                @click="openEditDialog(role)"
              >
                编辑
              </button>
              <button
                type="button"
                class="text-xs text-red-500 hover:text-red-600 px-1.5 py-0.5 rounded transition-colors"
                title="删除角色"
                @click="handleDeleteRole(role)"
              >
                删除
              </button>
            </div>
          </div>

          <!-- Model info -->
          <div class="text-xs text-[var(--color-text-muted)] font-mono flex items-center gap-1.5">
            <span class="text-[var(--color-text)]">模型:</span>
            <span>{{ role.model }}</span>
          </div>

          <!-- Description -->
          <p v-if="role.description" class="text-xs text-[var(--color-text-muted)] line-clamp-2 leading-relaxed">
            {{ role.description }}
          </p>
        </div>

        <!-- Referenced Prompt Snippet -->
        <div class="pt-2 border-t border-[var(--color-border)] text-xs space-y-1">
          <div class="flex items-center justify-between text-2xs text-[var(--color-text-muted)]">
            <span class="flex items-center gap-1">
              <span class="font-medium text-[var(--color-text)]">提示词来源:</span>
              <span v-if="role.promptId" class="inline-flex items-center gap-1 text-[var(--color-primary)] font-medium">
                <span>[引用]</span> {{ getPromptName(role.promptId) }}
              </span>
              <span v-else class="text-[var(--color-text-muted)]">
                专属直接设定
              </span>
            </span>
          </div>

          <div
            v-if="role.systemPrompt"
            class="text-2xs font-mono text-[var(--color-text-muted)] bg-[var(--color-surface-muted)] p-2 rounded border border-[var(--color-border)] line-clamp-2"
          >
            {{ role.systemPrompt }}
          </div>
        </div>
      </div>
    </div>

    <!-- Empty State -->
    <div
      v-else
      class="p-12 text-center rounded-[var(--radius,6px)] border border-dashed border-[var(--color-border)] bg-[var(--color-surface)]/40 space-y-3"
    >
      <div class="text-sm font-medium text-[var(--color-text)]">尚未配置任何专家角色</div>
      <p class="text-xs text-[var(--color-text-muted)] max-w-md mx-auto">
        角色管理支持统一设置 Agent 的名称、模型、职责说明与关联提示词，可在协同流程中直接被编排器调用。
      </p>
      <UiButton
        variant="primary"
        class="text-xs px-3.5 py-1.5 mt-2"
        @click="openCreateDialog"
      >
        立即创建第一个角色
      </UiButton>
    </div>

    <!-- Create / Edit Dialog -->
    <Teleport to="body">
      <UiDialog
        v-if="showEditDialog"
        :title="editingRoleId ? '编辑专家角色' : '新建专家角色'"
        width-class="w-full max-w-xl"
        @close="showEditDialog = false"
      >
        <form class="space-y-4" @submit.prevent="handleSaveRole">
          <UiAlert v-if="dialogError" variant="error">
            {{ dialogError }}
          </UiAlert>

          <!-- 1. Name & Color -->
          <div class="grid grid-cols-3 gap-3">
            <div class="col-span-2 space-y-1.5">
              <label class="text-xs font-medium text-[var(--color-text)]">角色名称 <span class="text-red-500">*</span></label>
              <UiInput
                v-model="formName"
                placeholder="例如：系统架构师、前端专家、审查员"
                required
                autofocus
                block
              />
            </div>

            <div class="space-y-1.5">
              <label class="text-xs font-medium text-[var(--color-text)]">标识色彩</label>
              <div class="flex items-center gap-1.5 pt-2">
                <button
                  v-for="color in defaultColors"
                  :key="color"
                  type="button"
                  class="w-5 h-5 rounded-full border transition-transform"
                  :class="[formColor === color ? 'scale-110 border-white ring-2 ring-blue-500/50' : 'border-transparent opacity-70 hover:opacity-100']"
                  :style="{ backgroundColor: color }"
                  @click="formColor = color"
                />
              </div>
            </div>
          </div>

          <!-- 2. Description -->
          <div class="space-y-1.5">
            <label class="text-xs font-medium text-[var(--color-text)]">角色职责说明</label>
            <UiInput
              v-model="formDescription"
              placeholder="说明该角色的专业特长与工作边界，中控编排器派发任务时将以此为依据"
              block
            />
          </div>

          <!-- 3. Backend & Model Selection (Aligned with TaskCreatePanel) -->
          <div class="space-y-3">
            <!-- Agent Backend Selection (Capsule Radio Group) -->
            <div class="form-group flex flex-col gap-1.5">
              <label class="text-xs font-medium text-[var(--color-text)]">
                Agent 后端 <span class="text-red-500">*</span>
              </label>
              <div class="backend-switch flex flex-wrap gap-2" role="radiogroup">
                <button
                  v-for="b in selectableBackends"
                  :key="b.id"
                  type="button"
                  role="radio"
                  class="backend-option px-3 py-1.5 rounded-[var(--radius,6px)] border text-xs font-medium transition-colors select-none"
                  :class="[
                    formBackend === b.id
                      ? 'border-[var(--color-primary)] bg-[var(--color-surface-muted)] text-[var(--color-primary)] font-semibold'
                      : 'border-[var(--color-border)] text-[var(--color-text)] hover:bg-[var(--color-surface-muted)]',
                  ]"
                  :aria-checked="formBackend === b.id"
                  @click="selectBackend(b.id)"
                >
                  <span class="backend-name">{{ b.label }}</span>
                </button>
              </div>
            </div>

            <!-- Model Selection (ModelSelect for Agent backends, UiSelectMenu for LLM) -->
            <div class="form-group flex flex-col gap-1.5 min-w-0">
              <div class="flex items-center justify-between">
                <label class="text-xs font-medium text-[var(--color-text)]">
                  运行模型 <span class="text-red-500">*</span>
                </label>
                <button
                  type="button"
                  class="text-2xs text-[var(--color-primary)] hover:underline"
                  @click="isCustomModel = !isCustomModel"
                >
                  {{ isCustomModel ? '从候选列表选择' : '自定义输入' }}
                </button>
              </div>

              <!-- Custom text input mode -->
              <div v-if="isCustomModel">
                <UiInput
                  v-model="formModel"
                  placeholder="输入模型名称，例如：gpt-4o、claude-3-5-sonnet"
                  required
                  block
                />
              </div>
              <!-- When Agent backend: reuse platform standard ModelSelect -->
              <div v-else-if="formBackend !== 'llm'" class="model-select-wrapper">
                <ModelSelect
                  v-model="formModel"
                  :key="formBackend"
                  :backend="(formBackend as BackendId)"
                  :model-selection="true"
                />
              </div>
              <!-- When LLM backend: dropdown of platform chat models -->
              <div v-else-if="llmModelSelectOptions.length > 0">
                <UiSelectMenu
                  v-model="formModel"
                  :options="llmModelSelectOptions"
                  placeholder="请选择通用大模型…"
                  block
                />
              </div>
              <div v-else>
                <UiInput
                  v-model="formModel"
                  placeholder="例如：gpt-4o、claude-3-5-sonnet"
                  required
                  block
                />
              </div>
            </div>
          </div>

          <!-- 4. Prompt Source Selection (Reference Prompt vs Custom) -->
          <div class="space-y-2 pt-2 border-t border-[var(--color-border)]">
            <div class="flex items-center justify-between">
              <label class="text-xs font-medium text-[var(--color-text)]">提示词配置方式</label>
              <div class="flex items-center gap-3 text-xs">
                <label class="flex items-center gap-1.5 cursor-pointer">
                  <input
                    v-model="formPromptSource"
                    type="radio"
                    value="prompt"
                    class="text-[var(--color-primary)]"
                  />
                  <span>引用提示词管理 (推荐)</span>
                </label>
                <label class="flex items-center gap-1.5 cursor-pointer">
                  <input
                    v-model="formPromptSource"
                    type="radio"
                    value="custom"
                    class="text-[var(--color-primary)]"
                  />
                  <span>自定义提示词</span>
                </label>
              </div>
            </div>

            <!-- Prompt Reference Dropdown -->
            <div v-if="formPromptSource === 'prompt'" class="space-y-2">
              <div class="flex items-center gap-2">
                <div class="flex-1 min-w-0">
                  <UiSelectMenu
                    v-model="formPromptId"
                    :options="promptSelectOptions"
                    placeholder="选择角色类提示词…"
                    block
                  />
                </div>
                <UiButton
                  type="button"
                  variant="secondary"
                  class="text-xs px-2.5 py-1 whitespace-nowrap shrink-0"
                  @click="goToPromptsSetting"
                >
                  管理提示词
                </UiButton>
              </div>

              <!-- Preview of selected prompt content -->
              <div v-if="selectedPromptPreview" class="space-y-1">
                <span class="text-2xs text-[var(--color-text-muted)]">提示词实时预览:</span>
                <div class="text-xs font-mono text-[var(--color-text-muted)] bg-[var(--color-surface-muted)] p-2.5 rounded border border-[var(--color-border)] max-h-36 overflow-y-auto whitespace-pre-wrap leading-relaxed">
                  {{ selectedPromptPreview }}
                </div>
              </div>
              <p v-else class="text-xs text-[var(--color-text-muted)]">
                请选择一个提示词。引用的提示词在提示词管理中更新后，角色将自动同步生效。
              </p>
            </div>

            <!-- Custom Textarea -->
            <div v-else class="space-y-1">
              <UiTextarea
                v-model="formCustomPrompt"
                placeholder="设定该角色的系统人设、技能准则与输出要求…"
                :rows="5"
              />
            </div>
          </div>

          <!-- Actions -->
          <div class="flex items-center justify-end gap-2.5 pt-3 border-t border-[var(--color-border)]">
            <UiButton
              type="button"
              variant="secondary"
              class="text-xs px-3 py-1.5"
              @click="showEditDialog = false"
            >
              取消
            </UiButton>
            <UiButton
              type="submit"
              variant="primary"
              class="text-xs px-4 py-1.5"
              :loading="submitting"
              :disabled="submitting"
            >
              {{ editingRoleId ? '保存更新' : '立即创建' }}
            </UiButton>
          </div>
        </form>
      </UiDialog>
    </Teleport>

    <UiConfirmDialog
      v-if="pendingDelete"
      title="删除角色"
      :message="`确定要删除角色「${pendingDelete.name}」吗？`"
      detail="该角色将从角色库中移除，已配置到协同任务中的同名副本不受影响。"
      confirm-label="删除"
      :loading="deleting"
      :error="deleteError"
      @close="pendingDelete = null"
      @confirm="confirmDeleteRole"
    />
  </section>
</template>

<style scoped>
.role-card {
  min-height: 140px;
}
</style>
