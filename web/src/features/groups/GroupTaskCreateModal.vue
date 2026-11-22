<script setup lang="ts">
// GroupTaskCreateModal: Create a task for a team group, with project and git worktree selection.
import { computed, ref, watch } from 'vue'
import type { CreateGroupTaskPayload, GroupTask } from '../../api/groups'
import { useProjectsStore } from '../../stores/projects'
import { useWorktreesStore } from '../../stores/worktrees'
import { useGroupsStore } from '../../stores/groups'
import {
  UiButton,
  UiDialog,
  UiInput,
  UiTextarea,
  UiSelectMenu,
  UiAlert,
  type SelectMenuItem,
} from '../../shared/ui'
import CronBuilder from '../../shared/components/CronBuilder.vue'

const props = defineProps<{
  open: boolean
  groupId: number
  task?: GroupTask | null
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
  created: [task: GroupTask]
}>()

const projectsStore = useProjectsStore()
const worktreesStore = useWorktreesStore()
const groupsStore = useGroupsStore()

const title = ref('')
const description = ref('')
const selectedProjectId = ref<number | null>(null)
const selectedWorktreeId = ref<number>(0)
const prompt = ref('')

// Scheduling
const scheduleType = ref<'manual' | 'once' | 'cron'>('manual')
const scheduledAtDate = ref('')
const customCronExpr = ref('0 9 * * *')

const saving = ref(false)
const errorMessage = ref('')

const isEdit = computed(() => !!props.task)

const projectOptions = computed<SelectMenuItem<number>[]>(() => {
  return (projectsStore.projects || []).map((p) => ({
    value: p.id,
    label: p.name,
    sublabel: p.path,
  }))
})

const selectedProject = computed(() => {
  if (selectedProjectId.value === null) return undefined
  return projectsStore.byId(Number(selectedProjectId.value))
})

const projectIsGit = computed(() => selectedProject.value?.isGit === true)

const projectWorktrees = computed(() => {
  if (selectedProjectId.value === null) return []
  return worktreesStore.forProject(Number(selectedProjectId.value))
})

const worktreeOptions = computed<SelectMenuItem<number>[]>(() => {
  const p = selectedProject.value
  const options: SelectMenuItem<number>[] = [
    {
      value: 0,
      label: '主工作区 (默认)',
      sublabel: p?.path,
    },
  ]
  for (const wt of projectWorktrees.value) {
    options.push({
      value: wt.id,
      label: `工作树: ${wt.branch}`,
      sublabel: wt.available ? wt.path : `${wt.path}（不可用）`,
      disabled: !wt.available,
    })
  }
  return options
})

// Load worktrees whenever selected project changes
watch(selectedProjectId, async (newId) => {
  if (newId) {
    await worktreesStore.load(Number(newId))
    const exists = projectWorktrees.value.some((wt) => wt.id === selectedWorktreeId.value && wt.available)
    if (!exists) {
      selectedWorktreeId.value = 0
    }
  } else {
    selectedWorktreeId.value = 0
  }
})

watch(
  () => props.open,
  async (isOpen) => {
    if (!isOpen) return
    errorMessage.value = ''
    await projectsStore.load()

    if (props.task) {
      title.value = props.task.title
      description.value = props.task.description || ''
      selectedProjectId.value = props.task.projectId
      selectedWorktreeId.value = props.task.worktreeId || 0
      prompt.value = props.task.prompt
      scheduleType.value = props.task.scheduleType || 'manual'
      scheduledAtDate.value = props.task.scheduledAt ? props.task.scheduledAt.slice(0, 16) : ''
      customCronExpr.value = props.task.cronExpr || '0 9 * * *'
    } else {
      title.value = ''
      description.value = ''
      prompt.value = ''
      scheduleType.value = 'manual'
      scheduledAtDate.value = ''
      customCronExpr.value = '0 9 * * *'
      if (projectsStore.projects.length > 0 && selectedProjectId.value === null) {
        selectedProjectId.value = projectsStore.projects[0].id
      }
      selectedWorktreeId.value = 0
    }
  },
  { immediate: true },
)

async function handleSave() {
  if (!title.value.trim()) {
    errorMessage.value = '请输入任务标题'
    return
  }
  if (!selectedProjectId.value) {
    errorMessage.value = '请选择目标关联项目'
    return
  }
  if (!prompt.value.trim()) {
    errorMessage.value = '请填写本任务需要团队达成的目标需求 Prompt'
    return
  }

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
      errorMessage.value = '定时时间必须在未来'
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

  saving.value = true
  errorMessage.value = ''

  try {
    if (isEdit.value && props.task) {
      const updated = await groupsStore.updateGroupTask(props.groupId, props.task.id, {
        title: title.value.trim(),
        description: description.value.trim(),
        projectId: Number(selectedProjectId.value),
        worktreeId: selectedWorktreeId.value,
        prompt: prompt.value.trim(),
        scheduleType: scheduleType.value,
        scheduledAt,
        cronExpr,
        scheduleEnabled,
      })
      emit('created', updated)
    } else {
      const payload: CreateGroupTaskPayload = {
        title: title.value.trim(),
        description: description.value.trim(),
        projectId: Number(selectedProjectId.value),
        worktreeId: selectedWorktreeId.value,
        prompt: prompt.value.trim(),
        scheduleType: scheduleType.value,
        scheduledAt,
        cronExpr,
        scheduleEnabled,
      }
      const created = await groupsStore.createGroupTask(props.groupId, payload)
      emit('created', created)
    }
    emit('update:open', false)
  } catch (err: any) {
    errorMessage.value = err?.message || '保存任务失败'
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <UiDialog
    v-if="open"
    :title="isEdit ? '编辑群组任务' : '创建群组任务'"
    width-class="max-w-2xl w-full"
    @close="emit('update:open', false)"
  >
    <div class="space-y-4 text-xs text-[var(--color-text)]">
      <UiAlert v-if="errorMessage" variant="error">
        {{ errorMessage }}
      </UiAlert>

      <!-- Task Title & Description -->
      <div class="grid grid-cols-1 gap-3">
        <div>
          <label class="block font-medium mb-1 text-[var(--color-text-muted)]">任务标题 *</label>
          <UiInput
            v-model="title"
            placeholder="例如：重构用户鉴权模块、升级数据库迁移链"
            class="w-full"
            maxlength="80"
          />
        </div>
        <div>
          <label class="block font-medium mb-1 text-[var(--color-text-muted)]">任务补充描述</label>
          <UiInput
            v-model="description"
            placeholder="简述该任务背景"
            class="w-full"
          />
        </div>
      </div>

      <!-- Project & Worktree Selection -->
      <div class="p-3 rounded border border-[var(--color-border)] bg-[var(--color-surface-muted)] space-y-3">
        <div class="flex items-center gap-2 font-medium text-[var(--color-text)]">
          <svg viewBox="0 0 16 16" width="13" height="13" fill="none" stroke="currentColor" stroke-width="1.8" class="text-indigo-500">
            <path d="M14 11V5a2 2 0 0 0-2-2H4a2 2 0 0 0-2 2v6" />
            <circle cx="8" cy="11" r="3" />
          </svg>
          <span>目标项目与工作树（执行隔离环境）</span>
        </div>

        <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
          <div>
            <label class="block font-medium mb-1 text-[var(--color-text-muted)]">关联项目 *</label>
            <UiSelectMenu
              v-model="selectedProjectId"
              :options="projectOptions"
              placeholder="请选择项目…"
              class="w-full"
            />
          </div>

          <div>
            <label class="block font-medium mb-1 text-[var(--color-text-muted)]">
              项目工作树 (Worktree) *
            </label>
            <UiSelectMenu
              v-model="selectedWorktreeId"
              :options="worktreeOptions"
              placeholder="选择工作树或默认主工作区…"
              class="w-full"
              :disabled="!selectedProjectId"
            />
          </div>
        </div>

        <div v-if="selectedProject && projectIsGit" class="text-2xs text-[var(--color-text-muted)] flex items-center gap-1.5">
          <svg viewBox="0 0 16 16" width="12" height="12" fill="none" stroke="currentColor" stroke-width="1.8" class="text-green-500">
            <circle cx="5" cy="5" r="2" />
            <circle cx="11" cy="11" r="2" />
            <path d="M5 7v4a2 2 0 0 0 2 2h2" />
          </svg>
          <span>
            已检测到 Git 仓库，团队中各 Agent 实例将精确隔离在所选工作树工作区下执行。
          </span>
        </div>
      </div>

      <!-- Task Target Prompt -->
      <div>
        <label class="block font-medium mb-1 text-[var(--color-text-muted)]">
          任务目标需求 Prompt (投递给中控调度大脑的目标总览) *
        </label>
        <UiTextarea
          v-model="prompt"
          placeholder="详细阐述需要该团队协作完成的任务目标、具体交付物、测试验收标准等..."
          :rows="5"
          class="w-full font-mono text-2xs"
        />
      </div>

      <!-- Schedule Options -->
      <div class="p-3 rounded border border-[var(--color-border)] bg-[var(--color-surface)] space-y-3">
        <label class="block font-medium text-[var(--color-text-muted)]">执行与调度策略</label>
        <div class="flex items-center gap-4">
          <label class="flex items-center gap-1.5 cursor-pointer text-xs select-none">
            <input type="radio" v-model="scheduleType" value="manual" class="text-[var(--color-primary)]" />
            <span>手动立即执行</span>
          </label>
          <label class="flex items-center gap-1.5 cursor-pointer text-xs select-none">
            <input type="radio" v-model="scheduleType" value="once" class="text-[var(--color-primary)]" />
            <span>单次定时执行</span>
          </label>
          <label class="flex items-center gap-1.5 cursor-pointer text-xs select-none">
            <input type="radio" v-model="scheduleType" value="cron" class="text-[var(--color-primary)]" />
            <span>Cron 周期循环</span>
          </label>
        </div>

        <div v-if="scheduleType === 'once'" class="pt-2">
          <label class="block text-2xs text-[var(--color-text-muted)] mb-1">执行时间点</label>
          <UiInput
            v-model="scheduledAtDate"
            type="datetime-local"
            class="w-full max-w-xs"
          />
        </div>

        <div v-if="scheduleType === 'cron'" class="pt-2">
          <CronBuilder v-model="customCronExpr" />
        </div>
      </div>
    </div>

    <template #footer>
      <div class="flex items-center justify-end gap-2 pt-4">
        <UiButton
          variant="secondary"
          @click="emit('update:open', false)"
        >
          取消
        </UiButton>
        <UiButton
          variant="primary"
          :loading="saving"
          :disabled="saving"
          @click="handleSave"
        >
          {{ isEdit ? '保存任务' : '创建群组任务' }}
        </UiButton>
      </div>
    </template>
  </UiDialog>
</template>
