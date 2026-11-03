<script setup lang="ts">
// FlowRoleManager: Manage agent roles for a collaborative flow task.
// Refactored to integrate with Platform Role Management & Prompt Library.
import { computed, onMounted, ref } from 'vue'
import type { FlowRole, FlowRoleInput } from '../../../api/tasks'
import { useTasksStore } from '../../../stores/tasks'
import { useRolesStore } from '../../../stores/roles'
import { usePromptsStore } from '../../../stores/prompts'
import {
  UiButton,
  UiInput,
  UiTextarea,
  UiDialog,
  UiAlert,
  UiSelectMenu,
  type SelectMenuItem,
} from '../../../shared/ui'
import FlowRoleBadge from './FlowRoleBadge.vue'

const props = defineProps<{
  taskId: number
  roles: FlowRole[]
  readOnly?: boolean
}>()

const emit = defineEmits<{
  updated: []
}>()

const tasksStore = useTasksStore()
const rolesStore = useRolesStore()
const promptsStore = usePromptsStore()

const showEditDialog = ref(false)
const showImportDialog = ref(false)
const selectedImportRoleId = ref<number | 0>(0)
const editingRoleId = ref<number | null>(null)
const submitting = ref(false)
const formError = ref('')

const defaultColors = ['#3b82f6', '#8b5cf6', '#10b981', '#f59e0b', '#ec4899', '#06b6d4']

const promptSource = ref<'prompt' | 'custom'>('prompt')
const promptIdInput = ref<number | 0>(0)

const formData = ref<FlowRoleInput>({
  name: '',
  description: '',
  backend: 'llm',
  model: '',
  promptId: 0,
  systemPrompt: '',
  color: defaultColors[0],
})

onMounted(async () => {
  if (rolesStore.roles.length === 0) {
    void rolesStore.loadRoles()
  }
  if (promptsStore.prompts.length === 0) {
    void promptsStore.loadPrompts()
  }
})

const rolePrompts = computed(() => promptsStore.prompts.filter((p) => p.type === 'role'))

const promptSelectOptions = computed<SelectMenuItem<number | 0>[]>(() => {
  const items: SelectMenuItem<number | 0>[] = [
    { value: 0, label: '选择提示词模板…' },
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

const importRoleOptions = computed<SelectMenuItem<number | 0>[]>(() => {
  const items: SelectMenuItem<number | 0>[] = [
    { value: 0, label: '选择要导入的全局角色…' },
  ]
  for (const r of rolesStore.roles) {
    items.push({
      value: r.id,
      label: `${r.name} (${r.backend} / ${r.model})`,
    })
  }
  return items
})

function getPromptName(id?: number): string {
  if (!id) return ''
  const p = promptsStore.byId(id)
  return p ? p.name : `提示词 #${id}`
}

function openCreateDialog() {
  editingRoleId.value = null
  promptSource.value = rolePrompts.value.length > 0 ? 'prompt' : 'custom'
  promptIdInput.value = rolePrompts.value.length > 0 ? rolePrompts.value[0].id : 0
  formData.value = {
    name: '',
    description: '',
    backend: 'llm',
    model: '',
    promptId: promptIdInput.value,
    systemPrompt: '',
    color: defaultColors[props.roles.length % defaultColors.length],
  }
  formError.value = ''
  showEditDialog.value = true
}

function openEditDialog(role: FlowRole) {
  editingRoleId.value = role.id
  if (role.promptId && role.promptId > 0) {
    promptSource.value = 'prompt'
    promptIdInput.value = role.promptId
  } else {
    promptSource.value = 'custom'
    promptIdInput.value = 0
  }
  formData.value = {
    name: role.name,
    description: role.description || '',
    backend: role.backend || 'llm',
    model: role.model || '',
    promptId: role.promptId || 0,
    systemPrompt: role.systemPrompt || '',
    color: role.color || defaultColors[0],
  }
  formError.value = ''
  showEditDialog.value = true
}

async function handleSaveRole() {
  if (!formData.value.name.trim()) {
    formError.value = '请输入角色名称（如：架构师、前端专家）'
    return
  }
  if (!formData.value.model.trim()) {
    formError.value = '请输入或选择模型名称（如：gpt-4o、claude-3-5-sonnet）'
    return
  }

  const pId = promptSource.value === 'prompt' ? Number(promptIdInput.value) || 0 : 0
  const payload: FlowRoleInput = {
    ...formData.value,
    promptId: pId,
    systemPrompt: promptSource.value === 'custom' ? formData.value.systemPrompt : '',
  }

  submitting.value = true
  formError.value = ''
  try {
    if (editingRoleId.value) {
      await tasksStore.updateRole(props.taskId, editingRoleId.value, payload)
    } else {
      await tasksStore.createRole(props.taskId, payload)
    }
    showEditDialog.value = false
    emit('updated')
  } catch (err: any) {
    formError.value = err.message || '保存角色失败'
  } finally {
    submitting.value = false
  }
}

async function handleImportRole() {
  if (!selectedImportRoleId.value) return
  const role = rolesStore.byId(selectedImportRoleId.value)
  if (!role) return

  submitting.value = true
  try {
    await tasksStore.createRole(props.taskId, {
      name: role.name,
      description: role.description,
      backend: role.backend,
      model: role.model,
      promptId: role.promptId,
      systemPrompt: role.systemPrompt,
      color: role.color,
    })
    showImportDialog.value = false
    emit('updated')
  } catch (err: any) {
    alert(err.message || '导入角色失败')
  } finally {
    submitting.value = false
  }
}

async function handleDeleteRole(role: FlowRole) {
  if (props.readOnly) return
  if (!confirm(`确定要移除角色「${role.name}」吗？`)) return
  try {
    await tasksStore.deleteRole(props.taskId, role.id)
    emit('updated')
  } catch (err: any) {
    alert(err.message || '删除角色失败')
  }
}
</script>

<template>
  <div class="flow-role-manager space-y-4" data-test="flow-role-manager">
    <!-- Header with Actions -->
    <div class="flex items-center justify-between gap-4 flex-wrap">
      <div class="space-y-0.5">
        <h3 class="text-sm font-semibold text-[var(--color-text)] flex items-center gap-2">
          <span>Agent 协同角色分工</span>
          <span class="text-xs px-1.5 py-0.5 rounded bg-[var(--color-surface-muted)] text-[var(--color-text-muted)] font-mono border border-[var(--color-border)]">
            {{ roles.length }} 个角色
          </span>
        </h3>
        <p class="text-xs text-[var(--color-text-muted)]">
          配置中控派发任务时可选用的专家角色。支持统一引用平台角色库与提示词模板。
        </p>
      </div>

      <div v-if="!readOnly" class="flex items-center gap-2">
        <UiButton
          v-if="rolesStore.roles.length > 0"
          variant="secondary"
          class="text-xs px-2.5 py-1"
          data-test="btn-import-role"
          @click="showImportDialog = true; selectedImportRoleId = 0"
        >
          从角色库添加
        </UiButton>
        <UiButton
          variant="secondary"
          class="text-xs px-2.5 py-1"
          data-test="btn-add-role"
          @click="openCreateDialog"
        >
          <svg viewBox="0 0 16 16" width="12" height="12" fill="none" stroke="currentColor" stroke-width="2" class="mr-1 inline-block">
            <line x1="8" y1="3" x2="8" y2="13" />
            <line x1="3" y1="8" x2="13" y2="8" />
          </svg>
          新建角色
        </UiButton>
      </div>
    </div>

    <!-- Role List Grid -->
    <div v-if="roles.length > 0" class="grid grid-cols-1 md:grid-cols-2 gap-3">
      <div
        v-for="role in roles"
        :key="role.id"
        class="role-card p-3 rounded-[var(--radius,6px)] bg-[var(--color-surface)] border border-[var(--color-border)] flex flex-col justify-between gap-2.5 hover:border-[var(--color-primary)] transition-colors"
        :data-test="`role-card-${role.id}`"
      >
        <div class="space-y-1.5">
          <div class="flex items-center justify-between gap-2">
            <FlowRoleBadge :role="role" />
            <div v-if="!readOnly" class="flex items-center gap-1">
              <button
                type="button"
                class="text-[11px] text-[var(--color-text-muted)] hover:text-[var(--color-text)] p-1 rounded transition-colors"
                title="编辑角色"
                @click="openEditDialog(role)"
              >
                编辑
              </button>
              <button
                type="button"
                class="text-[11px] text-red-500 hover:text-red-600 p-1 rounded transition-colors"
                title="移除角色"
                @click="handleDeleteRole(role)"
              >
                删除
              </button>
            </div>
          </div>

          <p v-if="role.description" class="text-xs text-[var(--color-text-muted)] line-clamp-2 leading-relaxed">
            {{ role.description }}
          </p>
        </div>

        <!-- Prompt preview snippet with source info -->
        <div class="space-y-1 pt-1 border-t border-[var(--color-border)]">
          <div class="text-[10px] text-[var(--color-text-muted)] flex items-center gap-1">
            <span class="font-medium text-[var(--color-text)]">提示词:</span>
            <span v-if="role.promptId" class="text-[var(--color-primary)]">
              [引用] {{ getPromptName(role.promptId) }}
            </span>
            <span v-else>自定义设定</span>
          </div>
          <div v-if="role.systemPrompt" class="text-[11px] text-[var(--color-text-muted)] font-mono bg-[var(--color-surface-muted)] p-2 rounded border border-[var(--color-border)] truncate">
            {{ role.systemPrompt }}
          </div>
        </div>
      </div>
    </div>

    <!-- Empty state -->
    <div
      v-else
      class="p-6 text-center rounded-[var(--radius,6px)] border border-dashed border-[var(--color-border)] bg-[var(--color-surface)]/50 space-y-2"
    >
      <div class="text-xs text-[var(--color-text-muted)]">
        尚未配置任何 Agent 角色。协同流程需要至少一个角色才能执行。
      </div>
      <div v-if="!readOnly" class="flex items-center justify-center gap-2 pt-1">
        <UiButton
          v-if="rolesStore.roles.length > 0"
          variant="secondary"
          class="text-xs px-3 py-1.5"
          @click="showImportDialog = true; selectedImportRoleId = 0"
        >
          从角色库快速添加
        </UiButton>
        <UiButton
          variant="primary"
          class="text-xs px-3 py-1.5"
          @click="openCreateDialog"
        >
          新建专属角色
        </UiButton>
      </div>
    </div>

    <!-- Import From Role Library Dialog -->
    <UiDialog
      v-if="showImportDialog"
      title="从角色管理库导入专家角色"
      width-class="w-full max-w-md"
      @close="showImportDialog = false"
    >
      <div class="space-y-4">
        <p class="text-xs text-[var(--color-text-muted)]">
          选择已在平台设置中配置的专家角色加入当前协同任务。
        </p>

        <UiSelectMenu
          v-model="selectedImportRoleId"
          :options="importRoleOptions"
          placeholder="请选择专家角色…"
          block
        />

        <div class="flex items-center justify-end gap-2.5 pt-3 border-t border-[var(--color-border)]">
          <UiButton
            type="button"
            variant="secondary"
            class="text-xs px-3 py-1.5"
            @click="showImportDialog = false"
          >
            取消
          </UiButton>
          <UiButton
            type="button"
            variant="primary"
            class="text-xs px-4 py-1.5"
            :disabled="!selectedImportRoleId || submitting"
            :loading="submitting"
            @click="handleImportRole"
          >
            确认加入
          </UiButton>
        </div>
      </div>
    </UiDialog>

    <!-- Create / Edit Dialog -->
    <UiDialog
      v-if="showEditDialog"
      :title="editingRoleId ? '编辑 Agent 角色' : '新建 Agent 角色'"
      width-class="w-full max-w-xl"
      @close="showEditDialog = false"
    >
      <form class="space-y-4" @submit.prevent="handleSaveRole">
        <UiAlert v-if="formError" variant="error">
          {{ formError }}
        </UiAlert>

        <!-- Role Name & Color -->
        <div class="grid grid-cols-3 gap-3">
          <div class="col-span-2 space-y-1">
            <label class="text-xs font-medium text-[var(--color-text)]">角色名称 *</label>
            <UiInput
              v-model="formData.name"
              placeholder="例如：架构师、前端开发、代码审查员"
              required
              autofocus
            />
          </div>
          <div class="space-y-1">
            <label class="text-xs font-medium text-[var(--color-text)]">标识色彩</label>
            <div class="flex items-center gap-1.5 pt-1.5">
              <button
                v-for="color in defaultColors"
                :key="color"
                type="button"
                class="w-5 h-5 rounded-full border transition-transform"
                :class="[formData.color === color ? 'scale-110 border-white ring-2 ring-blue-500/50' : 'border-transparent opacity-70 hover:opacity-100']"
                :style="{ backgroundColor: color }"
                @click="formData.color = color"
              />
            </div>
          </div>
        </div>

        <!-- Description -->
        <div class="space-y-1">
          <label class="text-xs font-medium text-[var(--color-text)]">角色职责描述</label>
          <UiInput
            v-model="formData.description"
            placeholder="供中控大脑在派活时识别该角色的专长与任务边界"
          />
        </div>

        <!-- Backend & Model -->
        <div class="grid grid-cols-2 gap-3">
          <div class="space-y-1">
            <label class="text-xs font-medium text-[var(--color-text)]">后端类型</label>
            <select
              v-model="formData.backend"
              class="w-full text-xs h-8 px-2 rounded bg-[var(--color-surface)] border border-[var(--color-border)] text-[var(--color-text)] focus:outline-none focus:border-[var(--color-primary)] font-mono"
            >
              <option value="llm">llm (纯大模型)</option>
              <option value="opencode">opencode (工作区 Coding Agent)</option>
              <option value="pi">pi (极简 Coding Agent)</option>
              <option value="deepseek">deepseek (DSH Agent)</option>
            </select>
          </div>
          <div class="space-y-1">
            <label class="text-xs font-medium text-[var(--color-text)]">分配模型 *</label>
            <UiInput
              v-model="formData.model"
              placeholder="例如：gpt-4o、claude-3-5-sonnet"
              required
            />
          </div>
        </div>

        <!-- Prompt Source Configuration (Reference vs Custom) -->
        <div class="space-y-2 pt-2 border-t border-[var(--color-border)]">
          <div class="flex items-center justify-between">
            <label class="text-xs font-medium text-[var(--color-text)]">角色专属提示词</label>
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
                <span>自定义提示词</span>
              </label>
            </div>
          </div>

          <!-- Reference Prompt Menu -->
          <div v-if="promptSource === 'prompt'" class="space-y-2">
            <UiSelectMenu
              v-model="promptIdInput"
              :options="promptSelectOptions"
              placeholder="选择角色类提示词…"
              block
            />
            <div v-if="selectedPromptPreview" class="space-y-1">
              <span class="text-[11px] text-[var(--color-text-muted)]">提示词预览:</span>
              <div class="text-xs font-mono text-[var(--color-text-muted)] bg-[var(--color-surface-muted)] p-2 rounded border border-[var(--color-border)] max-h-32 overflow-y-auto whitespace-pre-wrap leading-relaxed">
                {{ selectedPromptPreview }}
              </div>
            </div>
          </div>

          <!-- Custom Textarea -->
          <div v-else class="space-y-1">
            <UiTextarea
              v-model="formData.systemPrompt"
              placeholder="设定该角色的专业技能、规范准则与输出要求。当派发该角色时自动作为角色人设注入。"
              :rows="4"
            />
          </div>
        </div>

        <!-- Dialog Footer Actions -->
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
            {{ editingRoleId ? '更新角色' : '创建角色' }}
          </UiButton>
        </div>
      </form>
    </UiDialog>
  </div>
</template>
