<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { listAgentBackendModels, type BackendId, type ModelInfo } from '../../api/agents'
import { describeApiError } from '../../api/client'
import type { ScheduleType, Task } from '../../api/tasks'
import { useAgentsStore } from '../../stores/agents'
import { useProjectsStore } from '../../stores/projects'
import { useTasksStore } from '../../stores/tasks'
import { UiFormDialog, UiInput, UiSelect, UiTextarea } from '../../shared/ui'

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
  <UiFormDialog
    :title="isEditing ? '编辑任务模板' : '新建任务模板'"
    width-class="w-full max-w-xl"
    :confirm-label="saving ? '保存中…' : '保存'"
    :confirm-loading="saving"
    :error="errorMessage"
    @close="emit('close')"
    @submit="submit"
  >
    <div class="field flex flex-col gap-1.5">
      <label for="dlg-task-title" class="text-xs font-medium text-slate-700 dark:text-slate-300">
        任务名称 <span class="text-red-500">*</span>
      </label>
      <UiInput
        id="dlg-task-title"
        v-model="title"
        placeholder="例如：每日生产服务健康巡检"
        required
        block
      />
    </div>

    <div class="field flex flex-col gap-1.5">
      <label for="dlg-task-desc" class="text-xs font-medium text-slate-700 dark:text-slate-300">
        任务描述 (可选)
      </label>
      <UiInput
        id="dlg-task-desc"
        v-model="description"
        placeholder="任务目标与简要说明"
        block
      />
    </div>

    <div class="field-row grid grid-cols-1 sm:grid-cols-2 gap-3">
      <div class="field flex flex-col gap-1.5">
        <label for="dlg-task-project" class="text-xs font-medium text-slate-700 dark:text-slate-300">
          关联项目 <span class="text-red-500">*</span>
        </label>
        <UiSelect id="dlg-task-project" v-model="selectedProjectId" block required>
          <option :value="0" disabled>选择项目…</option>
          <option v-for="p in projectsStore.projects" :key="p.id" :value="p.id">
            {{ p.name }}
          </option>
        </UiSelect>
      </div>

      <div class="field flex flex-col gap-1.5">
        <label for="dlg-task-backend" class="text-xs font-medium text-slate-700 dark:text-slate-300">
          Agent 后端 <span class="text-red-500">*</span>
        </label>
        <UiSelect id="dlg-task-backend" v-model="selectedBackend" block required>
          <option value="" disabled>选择 Agent…</option>
          <option
            v-for="b in selectableBackends"
            :key="b.backend"
            :value="b.backend"
          >
            {{ b.label || b.backend }}
          </option>
        </UiSelect>
      </div>
    </div>

    <div class="field flex flex-col gap-1.5">
      <label for="dlg-task-model" class="text-xs font-medium text-slate-700 dark:text-slate-300">
        模型 <span class="text-red-500">*</span>
      </label>
      <div class="model-row space-y-1">
        <UiSelect
          v-if="models.length > 0"
          id="dlg-task-model"
          v-model="selectedModel"
          :disabled="loadingModels"
          block
        >
          <option v-for="m in models" :key="m.id" :value="m.id">
            {{ m.name || m.id }}
          </option>
        </UiSelect>
        <UiInput
          v-else
          id="dlg-task-model"
          v-model="selectedModel"
          placeholder="输入模型标识符，如 claude-3-5-sonnet"
          :disabled="loadingModels"
          required
          block
        />
        <span v-if="loadingModels" class="text-[11px] text-slate-400">加载模型中…</span>
      </div>
    </div>

    <div class="field flex flex-col gap-1.5">
      <label for="dlg-task-prompt" class="text-xs font-medium text-slate-700 dark:text-slate-300">
        执行提示词模板 <span class="text-red-500">*</span>
      </label>
      <UiTextarea
        id="dlg-task-prompt"
        v-model="promptTemplate"
        :rows="4"
        placeholder="每次启动执行会话时发送给 Agent 的任务提示词模板…"
        required
      />
    </div>

    <div class="field flex flex-col gap-2">
      <label class="text-xs font-medium text-slate-700 dark:text-slate-300">执行方式选项</label>
      <div class="schedule-radios flex items-center gap-4 text-xs text-slate-700 dark:text-slate-300">
        <label class="inline-flex items-center gap-1.5 cursor-pointer">
          <input v-model="scheduleType" type="radio" value="manual" class="text-blue-600 accent-blue-600" />
          <span>手动执行</span>
        </label>
        <label class="inline-flex items-center gap-1.5 cursor-pointer">
          <input v-model="scheduleType" type="radio" value="once" class="text-blue-600 accent-blue-600" />
          <span>定时执行</span>
        </label>
        <label class="inline-flex items-center gap-1.5 cursor-pointer">
          <input v-model="scheduleType" type="radio" value="cron" class="text-blue-600 accent-blue-600" />
          <span>周期执行 (Cron)</span>
        </label>
      </div>
    </div>

    <div v-if="scheduleType === 'once'" class="schedule-subfield flex flex-col gap-1.5">
      <label for="dlg-task-time" class="text-xs font-medium text-slate-700 dark:text-slate-300">
        触发时间点 <span class="text-red-500">*</span>
      </label>
      <UiInput
        id="dlg-task-time"
        v-model="scheduledAtDate"
        type="datetime-local"
        required
        block
      />
    </div>

    <div v-if="scheduleType === 'cron'" class="schedule-subfield flex flex-col gap-1.5">
      <label for="dlg-task-cron" class="text-xs font-medium text-slate-700 dark:text-slate-300">
        Cron 表达式 (分 时 日 月 周) <span class="text-red-500">*</span>
      </label>
      <UiInput
        id="dlg-task-cron"
        v-model="cronExpr"
        placeholder="0 9 * * *"
        class="font-mono"
        required
        block
      />
    </div>
  </UiFormDialog>
</template>
