<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { listAgentBackendModels, type BackendId, type ModelInfo } from '../../api/agents'
import { describeApiError } from '../../api/client'
import type { ScheduleType, Task } from '../../api/tasks'
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
const promptTemplate = ref(props.task?.promptTemplate ?? '')

// Scheduling
const scheduleType = ref<ScheduleType>(props.task?.scheduleType ?? 'manual')
const scheduledAtDate = ref(props.task?.scheduledAt ? props.task.scheduledAt.slice(0, 16) : '')
const cronExpr = ref(props.task?.cronExpr ?? '0 9 * * *')

const models = ref<ModelInfo[]>([])
const loadingModels = ref(false)
const saving = ref(false)
const errorMessage = ref('')

const isEditing = computed(() => !!props.task)

const selectableBackends = computed(() =>
  agentsStore.backends.filter((b) => b.enabled !== false),
)

onMounted(async () => {
  await Promise.all([projectsStore.load(), agentsStore.loadBackends()])

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
  if (selectedBackend.value === '') {
    models.value = []
    return
  }
  loadingModels.value = true
  try {
    const list = await listAgentBackendModels(selectedBackend.value)
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

// The catalog is platform-wide, so only a backend change needs a reload.
watch(selectedBackend, () => {
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
  if (!promptTemplate.value.trim()) {
    errorMessage.value = '请填写执行提示词模板'
    return
  }

  let scheduledAt: string | undefined
  let scheduleEnabled = false
  let cron: string | undefined

  if (scheduleType.value === 'once') {
    if (!scheduledAtDate.value) {
      errorMessage.value = '请设置定时执行时间'
      return
    }
    const targetDate = new Date(scheduledAtDate.value)
    scheduledAt = targetDate.toISOString()
    scheduleEnabled = true
  } else if (scheduleType.value === 'cron') {
    if (!cronExpr.value.trim()) {
      errorMessage.value = '请输入有效的 Cron 表达式'
      return
    }
    cron = cronExpr.value.trim()
    scheduleEnabled = true
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
        promptTemplate: promptTemplate.value.trim(),
        scheduleType: scheduleType.value,
        scheduledAt,
        cronExpr: cron,
        scheduleEnabled,
      })
    } else {
      savedTask = await tasksStore.createTask({
        title: title.value.trim(),
        description: description.value.trim(),
        taskType: 'standard',
        projectId: selectedProjectId.value,
        backend: selectedBackend.value,
        model: selectedModel.value.trim(),
        promptTemplate: promptTemplate.value.trim(),
        scheduleType: scheduleType.value,
        scheduledAt,
        cronExpr: cron,
        scheduleEnabled,
      })
    }
    emit('saved', savedTask)
  } catch (err: any) {
    errorMessage.value = describeApiError(err) || '保存失败'
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <AppDialog
    :title="isEditing ? '编辑任务模板' : '新建任务模板'"
    width="38rem"
    @close="emit('close')"
  >
    <form class="manage-form" @submit.prevent="submit">
      <div class="field">
        <label for="dlg-task-title">任务名称 <span class="required">*</span></label>
        <input
          id="dlg-task-title"
          v-model="title"
          type="text"
          class="input"
          placeholder="例如：每日生产服务健康巡检"
          required
        />
      </div>

      <div class="field">
        <label for="dlg-task-desc">任务描述 (可选)</label>
        <input
          id="dlg-task-desc"
          v-model="description"
          type="text"
          class="input"
          placeholder="任务目标与简要说明"
        />
      </div>

      <div class="field-row">
        <div class="field flex-1">
          <label for="dlg-task-project">关联项目 <span class="required">*</span></label>
          <select id="dlg-task-project" v-model="selectedProjectId" class="select" required>
            <option :value="0" disabled>选择项目…</option>
            <option v-for="p in projectsStore.projects" :key="p.id" :value="p.id">
              {{ p.name }}
            </option>
          </select>
        </div>

        <div class="field flex-1">
          <label for="dlg-task-backend">Agent 后端 <span class="required">*</span></label>
          <select id="dlg-task-backend" v-model="selectedBackend" class="select" required>
            <option value="" disabled>选择 Agent…</option>
            <option
              v-for="b in selectableBackends"
              :key="b.backend"
              :value="b.backend"
            >
              {{ b.label || b.backend }}
            </option>
          </select>
        </div>
      </div>

      <div class="field">
        <label for="dlg-task-model">模型 <span class="required">*</span></label>
        <div class="model-row">
          <select
            v-if="models.length > 0"
            id="dlg-task-model"
            v-model="selectedModel"
            class="select"
            :disabled="loadingModels"
          >
            <option v-for="m in models" :key="m.id" :value="m.id">
              {{ m.name || m.id }}
            </option>
          </select>
          <input
            v-else
            id="dlg-task-model"
            v-model="selectedModel"
            type="text"
            class="input"
            placeholder="输入模型标识符，如 claude-3-5-sonnet"
            :disabled="loadingModels"
            required
          />
          <span v-if="loadingModels" class="loading-hint">加载模型中…</span>
        </div>
      </div>

      <div class="field">
        <label for="dlg-task-prompt">执行提示词模板 <span class="required">*</span></label>
        <textarea
          id="dlg-task-prompt"
          v-model="promptTemplate"
          rows="4"
          class="textarea"
          placeholder="每次启动执行会话时发送给 Agent 的任务提示词模板…"
          required
        ></textarea>
      </div>

      <div class="field">
        <label>执行方式选项</label>
        <div class="schedule-radios">
          <label class="radio-label">
            <input v-model="scheduleType" type="radio" value="manual" />
            手动执行
          </label>
          <label class="radio-label">
            <input v-model="scheduleType" type="radio" value="once" />
            定时执行
          </label>
          <label class="radio-label">
            <input v-model="scheduleType" type="radio" value="cron" />
            周期执行 (Cron)
          </label>
        </div>
      </div>

      <div v-if="scheduleType === 'once'" class="schedule-subfield">
        <label for="dlg-task-time">触发时间点 <span class="required">*</span></label>
        <input
          id="dlg-task-time"
          v-model="scheduledAtDate"
          type="datetime-local"
          class="input"
          required
        />
      </div>

      <div v-if="scheduleType === 'cron'" class="schedule-subfield">
        <label for="dlg-task-cron">Cron 表达式 (分 时 日 月 周) <span class="required">*</span></label>
        <input
          id="dlg-task-cron"
          v-model="cronExpr"
          type="text"
          class="input font-mono"
          placeholder="0 9 * * *"
          required
        />
      </div>

      <p v-if="errorMessage" class="error-msg" role="alert">{{ errorMessage }}</p>

      <div class="actions">
        <button type="button" class="btn-cancel" @click="emit('close')">取消</button>
        <button type="submit" class="btn-submit" :disabled="saving">
          {{ saving ? '保存中…' : '保存' }}
        </button>
      </div>
    </form>
  </AppDialog>
</template>

<style scoped>
.manage-form {
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.field {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
}

.field label {
  font-size: 0.85rem;
  font-weight: 500;
  color: var(--color-text);
}

.required {
  color: #ef4444;
}

.field-row {
  display: flex;
  gap: 1rem;
}

.flex-1 {
  flex: 1;
}

.input,
.select,
.textarea {
  padding: 0.5rem 0.75rem;
  border: 1px solid var(--color-border);
  border-radius: 6px;
  background: var(--color-surface);
  color: var(--color-text);
  font-size: 0.88rem;
}

.input:focus,
.select:focus,
.textarea:focus {
  outline: none;
  border-color: var(--color-primary, #3b82f6);
}

.font-mono {
  font-family: monospace;
}

.model-row {
  display: flex;
  align-items: center;
  gap: 0.75rem;
}

.loading-hint {
  font-size: 0.8rem;
  color: var(--color-text-muted);
}

.schedule-radios {
  display: flex;
  gap: 1.25rem;
  padding: 0.35rem 0;
}

.radio-label {
  display: inline-flex;
  align-items: center;
  gap: 0.35rem;
  font-size: 0.88rem;
  cursor: pointer;
}

.schedule-subfield {
  padding: 0.75rem;
  background: var(--color-surface-muted);
  border-radius: 6px;
  border: 1px solid var(--color-border);
}

.error-msg {
  font-size: 0.85rem;
  color: #ef4444;
  margin: 0;
}

.actions {
  display: flex;
  justify-content: flex-end;
  gap: 0.75rem;
  margin-top: 0.5rem;
}

.btn-cancel {
  padding: 0.45rem 1rem;
  border: 1px solid var(--color-border);
  border-radius: 6px;
  background: var(--color-surface);
  color: var(--color-text);
  cursor: pointer;
  font-size: 0.88rem;
}

.btn-submit {
  padding: 0.45rem 1.25rem;
  border: none;
  border-radius: 6px;
  background: var(--color-primary, #3b82f6);
  color: #fff;
  cursor: pointer;
  font-weight: 600;
  font-size: 0.88rem;
}

.btn-submit:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
</style>
