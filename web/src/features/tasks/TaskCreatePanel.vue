<script setup lang="ts">
// Task Creation Panel: structured with clean session template fields,
// task type selector and execution mode configuration.
import { computed, onMounted, ref, watch } from 'vue'
import type { BackendId } from '../../api/agents'
import type { ScheduleType, TaskType, FlowRoleInput } from '../../api/tasks'
import { createTaskRole } from '../../api/tasks'
import ModelSelect from '../chat/composer/ModelSelect.vue'
import { useAgentsStore } from '../../stores/agents'
import { useProjectsStore } from '../../stores/projects'
import { usePromptsStore } from '../../stores/prompts'
import { useTasksStore } from '../../stores/tasks'
import { useDraftsStore } from '../../stores/drafts'
import { applyUserPromptTemplate, hasUserPromptPlaceholder } from '../prompts/promptTemplate'
import { UiButton, UiInput, UiSelect, UiTextarea, UiAlert } from '../../shared/ui'
import CronBuilder from '../../shared/components/CronBuilder.vue'

const props = defineProps<{
  initialProjectId?: number
}>()

const emit = defineEmits<{
  created: [taskId: number]
  cancel: []
}>()

const agentsStore = useAgentsStore()
const projectsStore = useProjectsStore()
const promptsStore = usePromptsStore()
const tasksStore = useTasksStore()
const draftsStore = useDraftsStore()

const draft = draftsStore.taskCreateDraft

// Task Type: Standard (Session Template) is active; Flow is a visible extension point.
const selectedTaskType = ref<TaskType>(draft.taskType ?? 'standard')

// Basic Task Info
const title = ref(draft.title ?? '')
const description = ref(draft.description ?? '')

// Template / Session Info
const selectedProjectId = ref<number | null>(
  props.initialProjectId ?? (draft.projectId !== null && draft.projectId !== undefined ? Number(draft.projectId) : null),
)
const selectedBackend = ref<BackendId | ''>(draft.backend ?? '')
const selectedModel = ref(draft.model ?? '')
const modelOk = ref(false)
const promptText = ref(draft.promptText ?? '')
const selectedPromptId = ref<number | ''>(draft.promptId ?? '')

// Collaborative Flow Configuration
const orchestratorModel = ref(draft.model || 'gpt-4o')
const flowRoles = ref<FlowRoleInput[]>([
  {
    name: '架构师',
    description: '负责系统架构方案设计、模块拆解与接口规约',
    backend: 'llm',
    model: 'gpt-4o',
    systemPrompt: '你是一名资深系统架构师，负责分析需求并制定清晰架构方案。',
    color: '#8b5cf6',
  },
  {
    name: '开发专家',
    description: '负责按照设计方案编写与修改工作区代码',
    backend: 'opencode',
    model: 'gpt-4o',
    systemPrompt: '你是一名经验丰富的全栈工程师，擅长编写健壮、规范的代码并进行自测。',
    color: '#3b82f6',
  },
])

function addFlowRole() {
  const colors = ['#3b82f6', '#8b5cf6', '#10b981', '#f59e0b', '#ec4899', '#06b6d4']
  flowRoles.value.push({
    name: `专家${flowRoles.value.length + 1}`,
    description: '专业子任务执行',
    backend: 'llm',
    model: orchestratorModel.value || 'gpt-4o',
    systemPrompt: '请高质量完成指派的子任务并反馈结果。',
    color: colors[flowRoles.value.length % colors.length],
  })
}

function removeFlowRole(idx: number) {
  flowRoles.value.splice(idx, 1)
}

// Execution Mode / Schedule Info
const scheduleType = ref<ScheduleType>(draft.scheduleType ?? 'manual')
const scheduledAtDate = ref(draft.scheduledAtDate ?? '')
const cronShortcut = ref(draft.cronShortcut ?? 'custom')
const customCronExpr = ref(draft.customCronExpr ?? '0 9 * * *')

watch(
  [
    selectedTaskType,
    title,
    description,
    selectedProjectId,
    selectedBackend,
    selectedModel,
    promptText,
    selectedPromptId,
    scheduleType,
    scheduledAtDate,
    cronShortcut,
    customCronExpr,
  ],
  () => {
    draftsStore.setTaskCreateDraft({
      taskType: selectedTaskType.value,
      title: title.value,
      description: description.value,
      projectId: selectedProjectId.value,
      backend: selectedBackend.value,
      model: selectedModel.value,
      promptText: promptText.value,
      promptId: selectedPromptId.value,
      scheduleType: scheduleType.value,
      scheduledAtDate: scheduledAtDate.value,
      cronShortcut: cronShortcut.value,
      customCronExpr: customCronExpr.value,
    })
  },
)

const creating = ref(false)
const errorMessage = ref('')

const selectableBackends = computed(() =>
  agentsStore.backends.filter((entry) => entry.enabled !== false),
)

const selectedCapabilities = computed(() =>
  selectedBackend.value === '' ? null : agentsStore.capabilitiesFor(selectedBackend.value),
)

const selectedPrompt = computed(() =>
  selectedPromptId.value !== '' ? promptsStore.byId(Number(selectedPromptId.value)) ?? null : null,
)

// Synchronize cron shortcut with customCronExpr
watch(cronShortcut, (val) => {
  if (val === 'hourly') {
    customCronExpr.value = '0 * * * *'
  } else if (val === 'daily_9am') {
    customCronExpr.value = '0 9 * * *'
  } else if (val === 'weekly_mon') {
    customCronExpr.value = '0 9 * * 1'
  }
})

onMounted(async () => {
  await Promise.all([
    projectsStore.load(),
    agentsStore.loadBackends(),
    promptsStore.loadPrompts(),
  ])

  if (!selectedProjectId.value && projectsStore.projects.length > 0) {
    selectedProjectId.value = projectsStore.projects[0].id
  }
  if (selectedBackend.value === '' && selectableBackends.value.length > 0) {
    selectedBackend.value = selectableBackends.value[0].backend
  }
})

function selectBackend(backend: BackendId) {
  if (!agentsStore.backendEnabledFor(backend)) return
  if (selectedBackend.value === backend) return
  selectedBackend.value = backend
  selectedModel.value = ''
  modelOk.value = false
}

function onPromptTemplateChange() {
  if (selectedPrompt.value) {
    // If user prompt hasn't been edited, fill with template content
    if (!promptText.value.trim()) {
      promptText.value = selectedPrompt.value.content
    }
  }
}

async function handleCreateTask() {
  if (!title.value.trim()) {
    errorMessage.value = '请输入任务名称'
    return
  }
  if (!selectedProjectId.value) {
    errorMessage.value = '请选择关联项目'
    return
  }

  // Validate schedule settings
  let scheduledAt: string | undefined
  let cronExpr: string | undefined
  let scheduleEnabled = false

  if (scheduleType.value === 'once') {
    if (!scheduledAtDate.value) {
      errorMessage.value = '请选择定时执行的具体时间'
      return
    }
    const targetDate = new Date(scheduledAtDate.value)
    if (isNaN(targetDate.getTime()) || targetDate.getTime() <= Date.now()) {
      errorMessage.value = '定时执行时间必须为未来时间'
      return
    }
    scheduledAt = targetDate.toISOString()
    scheduleEnabled = true
  } else if (scheduleType.value === 'cron') {
    if (!customCronExpr.value.trim()) {
      errorMessage.value = '请输入有效的 Cron 表达式'
      return
    }
    cronExpr = customCronExpr.value.trim()
    scheduleEnabled = true
  }

  // 1. Collaborative Flow Task Creation
  if (selectedTaskType.value === 'flow') {
    if (!orchestratorModel.value.trim()) {
      errorMessage.value = '请输入或选择中控编排模型'
      return
    }
    if (!promptText.value.trim()) {
      errorMessage.value = '请填写多 Agent 协同流程的整体目标'
      return
    }
    if (flowRoles.value.length === 0) {
      errorMessage.value = '协同流程必须至少包含一个 Agent 角色'
      return
    }

    creating.value = true
    errorMessage.value = ''
    try {
      const defaultBackend = (selectableBackends.value[0]?.backend as BackendId) || 'opencode'
      const created = await tasksStore.createTask({
        title: title.value.trim(),
        description: description.value.trim(),
        taskType: 'flow',
        projectId: selectedProjectId.value,
        backend: defaultBackend,
        model: orchestratorModel.value.trim(),
        orchestratorModel: orchestratorModel.value.trim(),
        promptTemplate: promptText.value.trim(),
        scheduleType: scheduleType.value,
        scheduledAt,
        cronExpr,
        scheduleEnabled,
      })

      // Batch create initial configured roles
      for (const r of flowRoles.value) {
        await createTaskRole(created.id, r)
      }

      emit('created', created.id)
      draftsStore.clearTaskCreateDraft()
    } catch (err: any) {
      errorMessage.value = err.message || '创建协同流程任务失败'
    } finally {
      creating.value = false
    }
    return
  }

  // 2. Standard Session Template Task Creation
  if (!selectedBackend.value) {
    errorMessage.value = '请选择 Agent 后端'
    return
  }
  if (selectedCapabilities.value?.modelSelection && !selectedModel.value) {
    errorMessage.value = '请选择使用的模型'
    return
  }
  if (!promptText.value.trim() && !selectedPrompt.value) {
    errorMessage.value = '请填写执行提示词模板'
    return
  }

  const finalPrompt = selectedPrompt.value
    ? hasUserPromptPlaceholder(selectedPrompt.value.content)
      ? applyUserPromptTemplate(selectedPrompt.value.content, promptText.value)
      : promptText.value || selectedPrompt.value.content
    : promptText.value.trim()

  creating.value = true
  errorMessage.value = ''

  try {
    const created = await tasksStore.createTask({
      title: title.value.trim(),
      description: description.value.trim(),
      taskType: 'standard',
      projectId: selectedProjectId.value,
      backend: selectedBackend.value,
      model: selectedModel.value || 'default',
      promptTemplate: finalPrompt,
      scheduleType: scheduleType.value,
      scheduledAt,
      cronExpr,
      scheduleEnabled,
    })

    emit('created', created.id)
    draftsStore.clearTaskCreateDraft()
  } catch (err: any) {
    errorMessage.value = err.message || '创建任务失败'
  } finally {
    creating.value = false
  }
}
</script>

<template>
  <div class="task-create-container flex justify-center items-start p-6 md:p-8 overflow-y-auto h-full bg-[var(--color-bg)]" data-test="task-create-panel">
    <div class="create-card w-full max-w-3xl bg-[var(--color-surface)] border border-[var(--color-border)] rounded-[var(--radius,6px)] p-6 md:p-8 space-y-6">
      <!-- Header -->
      <header class="create-header flex items-start justify-between pb-4 border-b border-[var(--color-border)]">
        <div class="header-titles space-y-1">
          <h1 class="text-xl font-semibold text-[var(--color-text)] tracking-tight">新建任务</h1>
          <p class="subtitle text-xs text-[var(--color-text-muted)]">配置自动化会话模板与排期调度策略</p>
        </div>
        <button
          type="button"
          class="btn-close text-[var(--color-text-muted)] hover:text-[var(--color-text)] p-1.5 rounded transition-colors inline-flex items-center justify-center"
          title="取消"
          aria-label="关闭面板"
          @click="emit('cancel')"
        >
          <svg viewBox="0 0 16 16" width="14" height="14" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true">
            <line x1="3" y1="3" x2="13" y2="13" />
            <line x1="13" y1="3" x2="3" y2="13" />
          </svg>
        </button>
      </header>

      <form class="create-form space-y-6" @submit.prevent="handleCreateTask">
        <!-- 1. Task Type Selection -->
        <div class="form-section space-y-2">
          <label class="form-section-title text-xs font-semibold text-[var(--color-text-muted)]">任务类型</label>
          <div class="task-type-selector grid grid-cols-1 sm:grid-cols-2 gap-3">
            <div
              class="type-card flex items-start gap-3 p-3.5 rounded-[var(--radius,6px)] border cursor-pointer select-none transition-colors"
              :class="[
                selectedTaskType === 'standard'
                  ? 'active border-[var(--color-primary)] bg-[var(--color-surface-muted)]'
                  : 'border-[var(--color-border)] hover:bg-[var(--color-surface-muted)]',
              ]"
              @click="selectedTaskType = 'standard'"
            >
              <div class="type-icon text-[var(--color-primary)] shrink-0 mt-0.5">
                <svg viewBox="0 0 16 16" width="16" height="16" fill="none" stroke="currentColor" stroke-width="1.8">
                  <polygon points="9 2 2 10 8 10 7 15 14 7 8 7 9 2" />
                </svg>
              </div>
              <div class="type-info flex-1">
                <div class="flex items-center gap-2">
                  <span class="type-title text-xs font-semibold text-[var(--color-text)]">普通任务</span>
                  <span class="type-pill standard">推荐</span>
                </div>
                <p class="type-desc text-xs text-[var(--color-text-muted)] mt-1 leading-relaxed">基于会话模板运行，支持手动执行、定时执行与周期自动化调度。</p>
              </div>
            </div>

            <div
              class="type-card flex items-start gap-3 p-3.5 rounded-[var(--radius,6px)] border cursor-pointer select-none transition-colors"
              :class="[
                selectedTaskType === 'flow'
                  ? 'active border-[var(--color-primary)] bg-[var(--color-surface-muted)]'
                  : 'border-[var(--color-border)] hover:bg-[var(--color-surface-muted)]',
              ]"
              @click="selectedTaskType = 'flow'"
            >
              <div class="type-icon text-[var(--color-primary)] shrink-0 mt-0.5">
                <svg viewBox="0 0 16 16" width="16" height="16" fill="none" stroke="currentColor" stroke-width="1.8">
                  <circle cx="8" cy="8" r="3" />
                  <path d="M8 1v2M8 13v2M1 8h2M13 8h2" />
                </svg>
              </div>
              <div class="type-info flex-1">
                <div class="flex items-center gap-2">
                  <span class="type-title text-xs font-semibold text-[var(--color-text)]">多 Agent 协同流程任务</span>
                  <span class="type-pill flow font-semibold">团队协同</span>
                </div>
                <p class="type-desc text-xs text-[var(--color-text-muted)] mt-1 leading-relaxed">平台中控自主调度与派发多 Agent 角色，以 IM 群聊形式闭环协同达成研发目标。</p>
              </div>
            </div>
          </div>
        </div>

        <!-- 2. Basic Info -->
        <div class="form-section space-y-3.5">
          <label class="form-section-title text-xs font-semibold text-[var(--color-text-muted)]">基本信息</label>
          <div class="form-group flex flex-col gap-1.5">
            <label for="task-title" class="text-xs font-medium text-[var(--color-text)]">
              任务名称 <span class="text-red-500">*</span>
            </label>
            <UiInput
              id="task-title"
              v-model="title"
              placeholder="例如：每日生产服务健康巡检与错误汇总"
              required
              block
            />
          </div>
          <div class="form-group flex flex-col gap-1.5">
            <label for="task-desc" class="text-xs font-medium text-[var(--color-text)]">
              任务描述 (可选)
            </label>
            <UiInput
              id="task-desc"
              v-model="description"
              placeholder="简要说明此任务的目的、触发背景或交付物"
              block
            />
          </div>
        </div>

        <!-- 3. Session Template Configuration / Flow Orchestrator Configuration -->
        <!-- 3A. Flow Task Specific Configuration -->
        <div v-if="selectedTaskType === 'flow'" class="form-section space-y-4">
          <label class="form-section-title text-xs font-semibold text-[var(--color-text-muted)] flex items-center gap-1.5">
            <svg viewBox="0 0 16 16" width="13" height="13" fill="none" stroke="currentColor" stroke-width="1.8" class="text-blue-500">
              <circle cx="8" cy="8" r="3" />
              <path d="M8 1v2M8 13v2M1 8h2M13 8h2" />
            </svg>
            <span>协同中控与目标配置</span>
          </label>

          <!-- Project Selector -->
          <div class="form-group flex flex-col gap-1.5">
            <label for="flow-project-select" class="text-xs font-medium text-[var(--color-text)]">
              工作区项目 <span class="text-red-500">*</span>
            </label>
            <UiSelect
              id="flow-project-select"
              v-model="selectedProjectId"
              required
              block
            >
              <option :value="null" disabled>请选择项目工作区…</option>
              <option
                v-for="p in projectsStore.projects ?? []"
                :key="p.id"
                :value="p.id"
              >
                {{ p.name }} ({{ p.path }})
              </option>
            </UiSelect>
          </div>

          <!-- Orchestrator Model Input -->
          <div class="form-group flex flex-col gap-1.5">
            <label for="orchestrator-model" class="text-xs font-medium text-[var(--color-text)]">
              中控调度大脑模型 <span class="text-red-500">*</span>
              <span class="label-hint text-[var(--color-text-muted)] font-normal">（用于拆解任务、决策分工与调度各角色的决策大模型）</span>
            </label>
            <UiInput
              id="orchestrator-model"
              v-model="orchestratorModel"
              placeholder="例如：gpt-4o、claude-3-5-sonnet、deepseek-chat"
              required
              block
            />
          </div>

          <!-- Goal / Global Prompt Textarea -->
          <div class="form-group flex flex-col gap-1.5">
            <label for="flow-goal" class="text-xs font-medium text-[var(--color-text)]">
              协同总目标与研发要求 <span class="text-red-500">*</span>
              <span class="label-hint text-[var(--color-text-muted)] font-normal">（输入要达成的总目标，中控将在群聊中据此拆解并指派角色）</span>
            </label>
            <UiTextarea
              id="flow-goal"
              v-model="promptText"
              :rows="4"
              placeholder="例如：请对当前项目的工作区代码进行架构梳理，由架构师输出模块设计方案，再由开发专家编写对应代码并进行自测验证…"
              required
            />
          </div>

          <!-- Configured Agent Roles List -->
          <div class="roles-config-section pt-2 space-y-3">
            <div class="flex items-center justify-between gap-3">
              <label class="text-xs font-semibold text-[var(--color-text)] flex items-center gap-2">
                <span>团队协同角色分工</span>
                <span class="text-[10px] px-1.5 py-0.2 rounded bg-blue-500/10 text-blue-500 font-mono border border-blue-500/20">
                  {{ flowRoles.length }} 个角色
                </span>
              </label>
              <button
                type="button"
                class="text-xs text-[var(--color-primary)] hover:underline inline-flex items-center gap-1 font-medium"
                @click="addFlowRole"
              >
                + 添加角色
              </button>
            </div>

            <!-- Role Cards Grid -->
            <div class="grid grid-cols-1 gap-2.5">
              <div
                v-for="(r, idx) in flowRoles"
                :key="idx"
                class="p-3 rounded-[var(--radius,6px)] bg-[var(--color-surface-muted)] border border-[var(--color-border)] space-y-2"
              >
                <div class="flex items-center justify-between gap-2">
                  <div class="flex items-center gap-2">
                    <span
                      class="w-2.5 h-2.5 rounded-full shrink-0"
                      :style="{ backgroundColor: r.color || '#3b82f6' }"
                    />
                    <input
                      v-model="r.name"
                      class="text-xs font-semibold bg-transparent border-b border-[var(--color-border)] focus:border-[var(--color-primary)] focus:outline-none text-[var(--color-text)] px-1 py-0.5"
                      placeholder="角色名称"
                      required
                    />
                    <select
                      v-model="r.backend"
                      class="text-[11px] h-6 px-1.5 rounded bg-[var(--color-surface)] border border-[var(--color-border)] text-[var(--color-text)] focus:outline-none font-mono"
                    >
                      <option value="llm">llm (纯大模型)</option>
                      <option value="opencode">opencode</option>
                      <option value="pi">pi</option>
                      <option value="deepseek">deepseek</option>
                    </select>
                    <input
                      v-model="r.model"
                      class="text-[11px] h-6 px-2 rounded bg-[var(--color-surface)] border border-[var(--color-border)] text-[var(--color-text)] focus:outline-none font-mono w-32"
                      placeholder="分配模型"
                      required
                    />
                  </div>

                  <button
                    v-if="flowRoles.length > 1"
                    type="button"
                    class="text-[11px] text-red-500 hover:text-red-600 p-1"
                    title="移除该角色"
                    @click="removeFlowRole(idx)"
                  >
                    移除
                  </button>
                </div>

                <div class="grid grid-cols-1 md:grid-cols-2 gap-2 text-xs">
                  <input
                    v-model="r.description"
                    class="p-1.5 rounded bg-[var(--color-surface)] border border-[var(--color-border)] text-[var(--color-text)] text-[11px] focus:outline-none focus:border-[var(--color-primary)]"
                    placeholder="角色职责描述（供中控识别）"
                  />
                  <input
                    v-model="r.systemPrompt"
                    class="p-1.5 rounded bg-[var(--color-surface)] border border-[var(--color-border)] text-[var(--color-text)] text-[11px] focus:outline-none focus:border-[var(--color-primary)] font-mono"
                    placeholder="角色专属系统提示词 (System Prompt)"
                  />
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- 3B. Standard Session Template Configuration -->
        <div v-else class="form-section space-y-3.5">
          <label class="form-section-title text-xs font-semibold text-[var(--color-text-muted)]">会话执行模板</label>

          <!-- Project Selector -->
          <div class="form-group flex flex-col gap-1.5">
            <label for="project-select" class="text-xs font-medium text-[var(--color-text)]">
              关联项目 <span class="text-red-500">*</span>
            </label>
            <UiSelect
              id="project-select"
              v-model="selectedProjectId"
              required
              block
            >
              <option :value="null" disabled>请选择项目…</option>
              <option
                v-for="p in projectsStore.projects ?? []"
                :key="p.id"
                :value="p.id"
              >
                {{ p.name }} ({{ p.path }})
              </option>
            </UiSelect>
          </div>

          <!-- Agent Backend -->
          <div class="form-group flex flex-col gap-1.5">
            <label class="text-xs font-medium text-[var(--color-text)]">
              Agent 后端 <span class="text-red-500">*</span>
            </label>
            <div class="backend-switch flex flex-wrap gap-2" role="radiogroup">
              <button
                v-for="b in selectableBackends"
                :key="b.backend"
                type="button"
                role="radio"
                class="backend-option px-3 py-1.5 rounded-[var(--radius,6px)] border text-xs font-medium transition-colors select-none"
                :class="[
                  selectedBackend === b.backend
                    ? 'border-[var(--color-primary)] bg-[var(--color-surface-muted)] text-[var(--color-primary)]'
                    : 'border-[var(--color-border)] text-[var(--color-text)] hover:bg-[var(--color-surface-muted)]',
                ]"
                :aria-checked="selectedBackend === b.backend"
                @click="selectBackend(b.backend)"
              >
                <span class="backend-name">{{ b.label || b.backend }}</span>
              </button>
            </div>
          </div>

          <!-- Model Select -->
          <div
            v-if="selectedCapabilities?.modelSelection && selectedBackend !== ''"
            class="form-group flex flex-col gap-1.5"
          >
            <label class="text-xs font-medium text-[var(--color-text)]">
              运行模型 <span class="text-red-500">*</span>
            </label>
            <div class="model-select-wrapper">
              <ModelSelect
                v-model="selectedModel"
                :key="selectedBackend"
                :backend="selectedBackend"
                :model-selection="true"
                @status="modelOk = $event"
              />
            </div>
          </div>

          <!-- Prompt Preset -->
          <div class="form-group flex flex-col gap-1.5">
            <label for="prompt-select" class="text-xs font-medium text-[var(--color-text)]">
              预设提示词模板 (可选)
            </label>
            <UiSelect
              id="prompt-select"
              v-model="selectedPromptId"
              block
              @change="onPromptTemplateChange"
            >
              <option value="">不使用预设模板（直接在下方输入）</option>
              <option
                v-for="p in promptsStore.prompts"
                :key="p.id"
                :value="p.id"
              >
                {{ p.name }}{{ p.description ? ` (${p.description})` : '' }}
              </option>
            </UiSelect>
          </div>

          <!-- Prompt Template Textarea -->
          <div class="form-group flex flex-col gap-1.5">
            <label for="task-prompt" class="text-xs font-medium text-[var(--color-text)]">
              执行提示词模板 <span class="text-red-500">*</span>
              <span class="label-hint text-[var(--color-text-muted)] font-normal">（每次执行此任务时，Agent 将使用该提示词开启新会话）</span>
            </label>
            <UiTextarea
              id="task-prompt"
              v-model="promptText"
              :rows="5"
              placeholder="详细描述任务要求、执行步骤与产出规范…"
              required
            />
          </div>
        </div>

        <!-- 4. Execution Mode Options -->
        <div class="form-section space-y-3.5">
          <label class="form-section-title text-xs font-semibold text-[var(--color-text-muted)]">执行方式选项</label>
          <div class="schedule-mode-grid grid grid-cols-1 sm:grid-cols-3 gap-3">
            <div
              class="mode-card p-3 rounded-[var(--radius,6px)] border cursor-pointer select-none transition-colors"
              :class="[
                scheduleType === 'manual'
                  ? 'border-[var(--color-primary)] bg-[var(--color-surface-muted)]'
                  : 'border-[var(--color-border)] hover:bg-[var(--color-surface-muted)]',
              ]"
              @click="scheduleType = 'manual'"
            >
              <input
                v-model="scheduleType"
                type="radio"
                value="manual"
                class="sr-only"
              />
              <div class="mode-header flex items-center gap-1.5 text-xs font-semibold text-[var(--color-text)] mb-1">
                <svg viewBox="0 0 16 16" width="13" height="13" fill="none" stroke="currentColor" stroke-width="1.8">
                  <polygon points="5 3 13 8 5 13 5 3" />
                </svg>
                <span>手动执行</span>
              </div>
              <p class="mode-desc text-[11px] text-[var(--color-text-muted)] leading-relaxed">仅在任务创建完成后，由人工在任务详情页点击按钮立即触发。</p>
            </div>

            <div
              class="mode-card p-3 rounded-[var(--radius,6px)] border cursor-pointer select-none transition-colors"
              :class="[
                scheduleType === 'once'
                  ? 'border-[var(--color-primary)] bg-[var(--color-surface-muted)]'
                  : 'border-[var(--color-border)] hover:bg-[var(--color-surface-muted)]',
              ]"
              @click="scheduleType = 'once'"
            >
              <input
                v-model="scheduleType"
                type="radio"
                value="once"
                class="sr-only"
              />
              <div class="mode-header flex items-center gap-1.5 text-xs font-semibold text-[var(--color-text)] mb-1">
                <svg viewBox="0 0 16 16" width="13" height="13" fill="none" stroke="currentColor" stroke-width="1.8">
                  <circle cx="8" cy="8" r="6" />
                  <polyline points="8 4.5 8 8 10.5 9.5" />
                </svg>
                <span>定时执行 (单次)</span>
              </div>
              <p class="mode-desc text-[11px] text-[var(--color-text-muted)] leading-relaxed">在设定的未来时间点自动启动并执行一次任务会话。</p>
            </div>

            <div
              class="mode-card p-3 rounded-[var(--radius,6px)] border cursor-pointer select-none transition-colors"
              :class="[
                scheduleType === 'cron'
                  ? 'border-[var(--color-primary)] bg-[var(--color-surface-muted)]'
                  : 'border-[var(--color-border)] hover:bg-[var(--color-surface-muted)]',
              ]"
              @click="scheduleType = 'cron'"
            >
              <input
                v-model="scheduleType"
                type="radio"
                value="cron"
                class="sr-only"
              />
              <div class="mode-header flex items-center gap-1.5 text-xs font-semibold text-[var(--color-text)] mb-1">
                <svg viewBox="0 0 16 16" width="13" height="13" fill="none" stroke="currentColor" stroke-width="1.8">
                  <path d="M2.5 8a5.5 5.5 0 0 1 9.4-3.9L13.5 6" />
                  <polyline points="13.5 2.5 13.5 6 10 6" />
                  <path d="M13.5 8a5.5 5.5 0 0 1-9.4 3.9L2.5 10" />
                  <polyline points="2.5 13.5 2.5 10 6 10" />
                </svg>
                <span>周期执行 (Cron)</span>
              </div>
              <p class="mode-desc text-[11px] text-[var(--color-text-muted)] leading-relaxed">按照预设周期或 Cron 表达式自动循环执行任务并沉淀历史。</p>
            </div>
          </div>

          <!-- Once config -->
          <div v-if="scheduleType === 'once'" class="schedule-config-box p-3.5 rounded-[var(--radius,6px)] bg-[var(--color-surface-muted)] border border-[var(--color-border)] space-y-1.5">
            <label for="schedule-datetime" class="text-xs font-medium text-[var(--color-text)]">
              定时触发时间 <span class="text-red-500">*</span>
            </label>
            <UiInput
              id="schedule-datetime"
              v-model="scheduledAtDate"
              type="datetime-local"
              required
              block
            />
          </div>

          <!-- Cron config -->
          <div v-if="scheduleType === 'cron'" class="schedule-config-box">
            <CronBuilder id="cron-expr" v-model="customCronExpr" />
          </div>
        </div>

        <UiAlert v-if="errorMessage" variant="error" class="error-banner">
          {{ errorMessage }}
        </UiAlert>

        <!-- Actions -->
        <div class="form-actions flex items-center justify-end gap-3 pt-4 border-t border-[var(--color-border)]">
          <UiButton type="button" variant="secondary" class="btn-cancel text-xs" @click="emit('cancel')">取消</UiButton>
          <UiButton
            type="submit"
            variant="primary"
            class="btn-submit text-xs font-medium px-4 py-2"
            :disabled="creating"
            :loading="creating"
          >
            {{ creating ? '创建中…' : (selectedTaskType === 'flow' ? '创建协同流程' : '创建任务') }}
          </UiButton>
        </div>
      </form>
    </div>
  </div>
</template>

<style scoped>
@import './tasks-view.css';
</style>
