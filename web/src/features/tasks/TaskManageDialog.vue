<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { listBackendModels, type BackendId, type ModelInfo } from '../../api/agents'
import { describeApiError } from '../../api/client'
import type { FlowType, Task } from '../../api/tasks'
import { useAgentsStore } from '../../stores/agents'
import { useProjectsStore } from '../../stores/projects'
import { useTasksStore } from '../../stores/tasks'
import AppDialog from '../../shared/components/AppDialog.vue'

const props = defineProps<{
  task?: Task | null
}>()

const emit = defineEmits<{
  close: []
  saved: [task: Task]
}>()

const projectsStore = useProjectsStore()
const agentsStore = useAgentsStore()
const tasksStore = useTasksStore()

const title = ref(props.task?.title ?? '')
const description = ref(props.task?.description ?? '')
const selectedProjectId = ref<number>(props.task?.projectId ?? 0)
const selectedBackend = ref<BackendId | ''>(props.task?.backend ?? '')
const selectedModel = ref(props.task?.model ?? '')
const selectedFlowType = ref<FlowType>(props.task?.flowType ?? 'transactional')

const models = ref<ModelInfo[]>([])
const loadingModels = ref(false)
const saving = ref(false)
const errorMessage = ref('')

const isEditing = computed(() => !!props.task)

const selectableBackends = computed(() =>
  agentsStore.backends.filter((b) => b.enabled !== false),
)

onMounted(async () => {
  await Promise.all([
    projectsStore.load(),
    agentsStore.loadBackends(),
    tasksStore.loadFlowDefs(),
  ])

  if (!isEditing.value) {
    if (selectedProjectId.value === 0 && projectsStore.projects.length > 0) {
      selectedProjectId.value = projectsStore.projects[0].id
    }
    if (selectedBackend.value === '' && selectableBackends.value.length > 0) {
      selectedBackend.value = selectableBackends.value[0].backend
    }
  }

  void loadModels()
})

async function loadModels() {
  if (selectedProjectId.value === 0 || selectedBackend.value === '') {
    models.value = []
    return
  }
  loadingModels.value = true
  try {
    const list = await listBackendModels(selectedProjectId.value, selectedBackend.value)
    models.value = list.models
    if (
      models.value.length > 0 &&
      (!selectedModel.value || !models.value.some((m) => m.id === selectedModel.value))
    ) {
      selectedModel.value = models.value[0].id
    }
  } catch (err) {
    console.error('Failed to load models for backend', err)
  } finally {
    loadingModels.value = false
  }
}

watch([selectedProjectId, selectedBackend], () => {
  void loadModels()
})

async function submit() {
  if (!title.value.trim()) {
    errorMessage.value = '请输入任务名称'
    return
  }
  if (!selectedProjectId.value) {
    errorMessage.value = '请选择关联项目'
    return
  }
  if (!selectedBackend.value) {
    errorMessage.value = '请选择 Agent 类型'
    return
  }
  if (!selectedModel.value.trim()) {
    errorMessage.value = '请选择或输入模型'
    return
  }

  saving.value = true
  errorMessage.value = ''

  try {
    let savedTask: Task
    if (isEditing.value && props.task) {
      savedTask = await tasksStore.updateTask(props.task.id, {
        title: title.value.trim(),
        description: description.value.trim(),
        projectId: selectedProjectId.value,
        backend: selectedBackend.value,
        model: selectedModel.value.trim(),
      })
    } else {
      savedTask = await tasksStore.createTask({
        title: title.value.trim(),
        description: description.value.trim(),
        projectId: selectedProjectId.value,
        backend: selectedBackend.value,
        model: selectedModel.value.trim(),
        flowType: selectedFlowType.value,
      })
    }
    emit('saved', savedTask)
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
    :title="isEditing ? '编辑任务' : '新建任务'"
    @close="emit('close')"
  >
    <form class="dialog-form" @submit.prevent="submit">
      <p v-if="errorMessage" class="dialog-error" role="alert">{{ errorMessage }}</p>

      <label for="task-title">任务名称 <span class="required">*</span></label>
      <input
        id="task-title"
        v-model="title"
        type="text"
        placeholder="简要概括任务目标"
        required
        autofocus
      />

      <label for="task-desc">任务描述 / 需求说明</label>
      <textarea
        id="task-desc"
        v-model="description"
        rows="3"
        placeholder="详细描述具体需求、业务背景或约束条件..."
      ></textarea>

      <div class="form-row">
        <div class="form-field">
          <label for="task-project">关联项目 <span class="required">*</span></label>
          <select id="task-project" v-model="selectedProjectId" required>
            <option v-for="p in projectsStore.projects" :key="p.id" :value="p.id">
              {{ p.name }}
            </option>
          </select>
        </div>

        <div class="form-field">
          <label for="task-backend">Agent 类型 <span class="required">*</span></label>
          <select id="task-backend" v-model="selectedBackend" required>
            <option v-for="b in selectableBackends" :key="b.backend" :value="b.backend">
              {{ b.label || b.backend }}
            </option>
          </select>
        </div>
      </div>

      <label for="task-model">执行模型 <span class="required">*</span></label>
      <select v-if="models.length > 0" id="task-model" v-model="selectedModel" :disabled="loadingModels" required>
        <option v-for="m in models" :key="m.id" :value="m.id">{{ m.name || m.id }}</option>
      </select>
      <input
        v-else
        id="task-model"
        v-model="selectedModel"
        type="text"
        placeholder="请输入模型名称（如 openai/gpt-4o）"
        :disabled="loadingModels"
        required
      />
      <p v-if="loadingModels" class="hint">加载模型列表中...</p>

      <template v-if="!isEditing">
        <span class="field-label">编排流程类型 <span class="required">*</span></span>
        <div class="flow-options">
          <label class="flow-card" :class="{ selected: selectedFlowType === 'transactional' }">
            <input
              type="radio"
              v-model="selectedFlowType"
              value="transactional"
              name="flowType"
            />
            <span class="flow-info">
              <span class="flow-name">事务型（2 步）</span>
              <span class="flow-desc">1. 任务分析 ➔ 2. 任务执行。Agent 分析任务、消除歧义与不确定点，确认后每次新起独立会话执行。</span>
            </span>
          </label>
          <label class="flow-card" :class="{ selected: selectedFlowType === 'development' }">
            <input
              type="radio"
              v-model="selectedFlowType"
              value="development"
              name="flowType"
            />
            <span class="flow-info">
              <span class="flow-name">开发类（4 步）</span>
              <span class="flow-desc">1. 需求分析 ➔ 2. 需求设计 ➔ 3. 制定计划 ➔ 4. 执行。制定计划时完成全项确认。</span>
            </span>
          </label>
        </div>
      </template>

      <div class="dialog-actions">
        <button type="button" class="cancel" @click="emit('close')">取消</button>
        <button type="submit" class="confirm" :disabled="saving">
          {{ saving ? '保存中...' : isEditing ? '保存修改' : '立即创建' }}
        </button>
      </div>
    </form>
  </AppDialog>
</template>

<style scoped>
/* The dialog body follows the project dialogs' conventions: labels at
   --space-unit rhythm, controls at --control-height and --radius, and the same
   cancel/confirm footer. AppDialog owns the frame. */
.dialog-form {
  display: flex;
  flex-direction: column;
  gap: calc(var(--space-unit) * 0.75);
}

.dialog-form label,
.field-label {
  font-weight: 600;
  margin-top: calc(var(--space-unit) * 0.5);
}

.field-label {
  display: inline-block;
}

.dialog-form input[type='text'],
.dialog-form textarea,
.dialog-form select {
  padding: calc(var(--space-unit) * 0.667) calc(var(--space-unit) * 1);
  border: 1px solid var(--color-border-strong);
  border-radius: var(--radius);
  background: var(--color-surface);
  color: var(--color-text);
  font: inherit;
}

.dialog-form input[type='text'],
.dialog-form select {
  height: var(--control-height);
}

.dialog-form textarea {
  resize: vertical;
}

.dialog-form .required {
  color: var(--color-danger);
}

.hint {
  color: var(--color-text-muted);
  font-size: 0.85rem;
  margin: 0;
}

.form-row {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: calc(var(--space-unit) * 1.5);
}

.form-field {
  display: flex;
  flex-direction: column;
  gap: calc(var(--space-unit) * 0.5);
}

.flow-options {
  display: flex;
  flex-direction: column;
  gap: calc(var(--space-unit) * 1);
}

.flow-card {
  display: flex;
  align-items: flex-start;
  gap: calc(var(--space-unit) * 1.333);
  padding: calc(var(--space-unit) * 1.333) calc(var(--space-unit) * 1.667);
  border: 1px solid var(--color-border-strong);
  border-radius: var(--radius);
  background: var(--color-surface);
  cursor: pointer;
}

.flow-card.selected {
  border-color: var(--color-primary);
  background: var(--color-primary-soft);
}

.flow-card input[type='radio'] {
  margin-top: calc(var(--space-unit) * 0.5);
}

.flow-info {
  display: flex;
  flex-direction: column;
  gap: calc(var(--space-unit) * 0.25);
}

.flow-name {
  font-weight: 600;
  font-size: 0.9rem;
}

.flow-desc {
  font-size: 0.8rem;
  color: var(--color-text-muted);
}

.dialog-error {
  color: var(--color-danger);
  margin: 0;
}

.dialog-actions {
  display: flex;
  justify-content: flex-end;
  gap: calc(var(--space-unit) * 0.75);
  margin-top: calc(var(--space-unit) * 1.5);
}

.dialog-actions button {
  height: var(--control-height);
  padding: 0 calc(var(--space-unit) * 1.5);
  border: 1px solid var(--color-border-strong);
  border-radius: var(--radius);
  background: var(--color-surface);
  color: var(--color-text);
  cursor: pointer;
  font: inherit;
}

.dialog-actions .confirm {
  background: var(--color-primary);
  border-color: var(--color-primary);
  color: var(--color-on-primary);
}

.dialog-actions .confirm:disabled {
  background: var(--color-info-border);
  border-color: var(--color-info-border);
  cursor: default;
}
</style>
