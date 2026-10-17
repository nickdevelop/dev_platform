<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { listBackendModels, type BackendId, type ModelInfo } from '../../api/agents'
import { describeApiError } from '../../api/client'
import type { FlowType, Task } from '../../api/tasks'
import { useAgentsStore } from '../../stores/agents'
import { useProjectsStore } from '../../stores/projects'
import { useTasksStore } from '../../stores/tasks'

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
  <div class="dialog-backdrop" @click.self="emit('close')">
    <div class="task-dialog" role="dialog" :aria-label="isEditing ? '编辑任务' : '新建任务'">
      <div class="dialog-header">
        <h2>{{ isEditing ? '编辑任务' : '新建任务' }}</h2>
        <button type="button" class="close-btn" @click="emit('close')" aria-label="关闭">×</button>
      </div>

      <form @submit.prevent="submit" class="dialog-body">
        <p v-if="errorMessage" class="error-banner" role="alert">{{ errorMessage }}</p>

        <div class="form-group">
          <label for="task-title">任务名称 <span class="required">*</span></label>
          <input
            id="task-title"
            v-model="title"
            type="text"
            placeholder="简要概括任务目标"
            required
            autofocus
          />
        </div>

        <div class="form-group">
          <label for="task-desc">任务描述 / 需求说明</label>
          <textarea
            id="task-desc"
            v-model="description"
            rows="3"
            placeholder="详细描述具体需求、业务背景或约束条件..."
          ></textarea>
        </div>

        <div class="form-row">
          <div class="form-group">
            <label for="task-project">关联项目 <span class="required">*</span></label>
            <select id="task-project" v-model="selectedProjectId" required>
              <option v-for="p in projectsStore.projects" :key="p.id" :value="p.id">
                {{ p.name }}
              </option>
            </select>
          </div>

          <div class="form-group">
            <label for="task-backend">Agent 类型 <span class="required">*</span></label>
            <select id="task-backend" v-model="selectedBackend" required>
              <option v-for="b in selectableBackends" :key="b.backend" :value="b.backend">
                {{ b.label || b.backend }}
              </option>
            </select>
          </div>
        </div>

        <div class="form-group">
          <label for="task-model">执行模型 <span class="required">*</span></label>
          <div class="model-select-wrapper">
            <select
              v-if="models.length > 0"
              id="task-model"
              v-model="selectedModel"
              :disabled="loadingModels"
              required
            >
              <option v-for="m in models" :key="m.id" :value="m.id">
                {{ m.name || m.id }}
              </option>
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
          </div>
          <span v-if="loadingModels" class="field-hint">加载模型列表中...</span>
        </div>

        <div v-if="!isEditing" class="form-group">
          <label>编排流程类型 <span class="required">*</span></label>
          <div class="flow-options">
            <label class="flow-card" :class="{ selected: selectedFlowType === 'transactional' }">
              <input
                type="radio"
                v-model="selectedFlowType"
                value="transactional"
                name="flowType"
              />
              <div class="flow-info">
                <span class="flow-name">事务型（2 步）</span>
                <span class="flow-desc">1. 任务确认 ➔ 2. 任务执行。提前确认所有事项，执行不打断。</span>
              </div>
            </label>
            <label class="flow-card" :class="{ selected: selectedFlowType === 'development' }">
              <input
                type="radio"
                v-model="selectedFlowType"
                value="development"
                name="flowType"
              />
              <div class="flow-info">
                <span class="flow-name">开发类（4 步）</span>
                <span class="flow-desc">1. 需求分析 ➔ 2. 需求设计 ➔ 3. 制定计划 ➔ 4. 执行。制定计划时完成全项确认。</span>
              </div>
            </label>
          </div>
        </div>

        <div class="dialog-actions">
          <button type="button" class="btn-cancel" @click="emit('close')">取消</button>
          <button type="submit" class="btn-primary" :disabled="saving">
            {{ saving ? '保存中...' : isEditing ? '保存修改' : '立即创建' }}
          </button>
        </div>
      </form>
    </div>
  </div>
</template>

<style scoped>
.dialog-backdrop {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.5);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
  backdrop-filter: blur(2px);
}

.task-dialog {
  background: var(--color-bg, #1e1e24);
  border: 1px solid var(--color-border, #333);
  border-radius: 8px;
  width: 540px;
  max-width: 90vw;
  box-shadow: 0 12px 32px rgba(0, 0, 0, 0.4);
  overflow: hidden;
  display: flex;
  flex-direction: column;
}

.dialog-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 16px 20px;
  border-bottom: 1px solid var(--color-border, #333);
}

.dialog-header h2 {
  font-size: 1.15rem;
  margin: 0;
  font-weight: 600;
}

.close-btn {
  background: transparent;
  border: none;
  font-size: 1.5rem;
  line-height: 1;
  color: var(--color-text-muted, #888);
  cursor: pointer;
}

.dialog-body {
  padding: 20px;
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.error-banner {
  background: rgba(220, 53, 69, 0.15);
  border: 1px solid rgba(220, 53, 69, 0.4);
  color: #ff6b6b;
  padding: 8px 12px;
  border-radius: 4px;
  font-size: 0.9rem;
  margin: 0;
}

.form-group {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.form-group label {
  font-size: 0.88rem;
  font-weight: 500;
  color: var(--color-text, #e0e0e0);
}

.required {
  color: #ff6b6b;
}

.form-row {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
}

input[type='text'],
textarea,
select {
  background: var(--color-input-bg, #2a2a32);
  border: 1px solid var(--color-border, #444);
  color: var(--color-text, #fff);
  padding: 8px 12px;
  border-radius: 6px;
  font-size: 0.9rem;
  font-family: inherit;
  outline: none;
}

input[type='text']:focus,
textarea:focus,
select:focus {
  border-color: var(--color-primary, #3b82f6);
}

.field-hint {
  font-size: 0.8rem;
  color: var(--color-text-muted, #888);
}

.flow-options {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.flow-card {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  padding: 10px 12px;
  border: 1px solid var(--color-border, #444);
  border-radius: 6px;
  background: var(--color-card-bg, #25252c);
  cursor: pointer;
  transition: border-color 0.2s, background 0.2s;
}

.flow-card.selected {
  border-color: var(--color-primary, #3b82f6);
  background: rgba(59, 130, 246, 0.08);
}

.flow-card input[type='radio'] {
  margin-top: 3px;
}

.flow-info {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.flow-name {
  font-weight: 600;
  font-size: 0.9rem;
}

.flow-desc {
  font-size: 0.8rem;
  color: var(--color-text-muted, #999);
}

.dialog-actions {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  margin-top: 8px;
}

.btn-cancel {
  background: transparent;
  border: 1px solid var(--color-border, #555);
  color: var(--color-text, #eee);
  padding: 8px 16px;
  border-radius: 6px;
  cursor: pointer;
}

.btn-primary {
  background: var(--color-primary, #3b82f6);
  color: #fff;
  border: none;
  padding: 8px 18px;
  border-radius: 6px;
  font-weight: 500;
  cursor: pointer;
}

.btn-primary:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}
</style>
