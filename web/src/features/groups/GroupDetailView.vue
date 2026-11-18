<script setup lang="ts">
// GroupDetailView: Displays team group info, orchestrator brain configuration,
// member experts from role library, and the list of group tasks with worktree badges.
import { computed, onMounted, ref, watch } from 'vue'
import type { GroupDetail, GroupTask } from '../../api/groups'
import { useGroupsStore } from '../../stores/groups'
import { useProjectsStore } from '../../stores/projects'
import { useWorktreesStore } from '../../stores/worktrees'
import BackendTag from '../../shared/components/BackendTag.vue'
import GroupTaskCreateModal from './GroupTaskCreateModal.vue'
import GroupFlowChatDialog from './GroupFlowChatDialog.vue'
import { UiButton, UiBadge, UiAlert, UiConfirmDialog } from '../../shared/ui'

const props = defineProps<{
  groupId: number
}>()

const emit = defineEmits<{
  edit: []
  deleted: []
}>()

const groupsStore = useGroupsStore()
const projectsStore = useProjectsStore()
const worktreesStore = useWorktreesStore()

const showCreateTaskModal = ref(false)
const editingTask = ref<GroupTask | null>(null)
const showFlowChatDialog = ref(false)
const activeChatTask = ref<GroupTask | null>(null)

const showPromptExpanded = ref(false)
const pendingDeleteGroup = ref<boolean>(false)
const pendingDeleteTask = ref<GroupTask | null>(null)
const actionError = ref('')

const group = computed(() => groupsStore.activeGroupDetail)
const tasks = computed(() => groupsStore.groupTasks)

watch(
  () => props.groupId,
  async (newId) => {
    if (!newId) return
    actionError.value = ''
    await Promise.all([
      groupsStore.loadGroup(newId),
      groupsStore.loadGroupTasks(newId),
      projectsStore.load(),
    ])
    // Preload worktrees for relevant projects
    for (const t of groupsStore.groupTasks) {
      if (t.projectId) {
        void worktreesStore.load(t.projectId)
      }
    }
  },
  { immediate: true },
)

function getProjectName(projectId: number) {
  return projectsStore.byId(projectId)?.name || `项目 #${projectId}`
}

function getWorktreeBranch(projectId: number, worktreeId: number) {
  if (!worktreeId || worktreeId === 0) return '主工作区 (main)'
  const wt = worktreesStore.byId(projectId, worktreeId)
  return wt ? wt.branch : `工作树 #${worktreeId}`
}

function getStatusBadge(status?: string) {
  switch (status) {
    case 'running':
      return { variant: 'warning' as const, label: '正在执行' }
    case 'success':
      return { variant: 'success' as const, label: '执行成功' }
    case 'failed':
      return { variant: 'danger' as const, label: '执行失败' }
    default:
      return { variant: 'neutral' as const, label: '空闲就绪' }
  }
}

async function handleTriggerTask(task: GroupTask) {
  actionError.value = ''
  try {
    activeChatTask.value = task
    showFlowChatDialog.value = true
    await groupsStore.triggerTask(props.groupId, task.id)
  } catch (err: any) {
    actionError.value = err?.message || '触发任务失败'
  }
}

function openTaskChat(task: GroupTask) {
  activeChatTask.value = task
  showFlowChatDialog.value = true
}

function onEditTask(task: GroupTask) {
  editingTask.value = task
  showCreateTaskModal.value = true
}

function onNewTask() {
  editingTask.value = null
  showCreateTaskModal.value = true
}

async function confirmDeleteGroup() {
  actionError.value = ''
  try {
    await groupsStore.deleteGroup(props.groupId)
    pendingDeleteGroup.value = false
    emit('deleted')
  } catch (err: any) {
    actionError.value = err?.message || '删除群组失败'
  }
}

async function confirmDeleteTask() {
  if (!pendingDeleteTask.value) return
  actionError.value = ''
  try {
    await groupsStore.deleteGroupTask(props.groupId, pendingDeleteTask.value.id)
    pendingDeleteTask.value = null
  } catch (err: any) {
    actionError.value = err?.message || '删除任务失败'
  }
}
</script>

<template>
  <div class="h-full flex flex-col min-h-0 overflow-y-auto bg-[var(--color-bg)] p-6 space-y-6">
    <UiAlert v-if="actionError" variant="danger">
      {{ actionError }}
    </UiAlert>

    <!-- 1. Header Bar: Group Info & Quick Actions -->
    <div class="bg-[var(--color-surface)] border border-[var(--color-border)] rounded-[var(--radius,6px)] p-5">
      <div class="flex flex-col md:flex-row md:items-center justify-between gap-4">
        <div class="space-y-1">
          <div class="flex items-center gap-2.5">
            <h1 class="text-lg font-semibold text-[var(--color-text)] tracking-tight">
              {{ group?.name || '团队群组' }}
            </h1>
            <span class="inline-flex items-center gap-1 px-2 py-0.5 rounded text-2xs font-mono font-medium bg-blue-500/10 text-blue-600 dark:text-blue-400 border border-blue-500/20">
              <svg viewBox="0 0 16 16" width="12" height="12" fill="none" stroke="currentColor" stroke-width="1.8">
                <circle cx="8" cy="8" r="3" />
                <path d="M8 1v2M8 13v2M1 8h2M13 8h2" />
              </svg>
              中控大脑: {{ group?.orchestratorModel }}
            </span>
          </div>
          <p class="text-xs text-[var(--color-text-muted)] leading-relaxed">
            {{ group?.description || '暂无团队描述' }}
          </p>
        </div>

        <div class="flex items-center gap-2">
          <UiButton
            variant="secondary"
            class="text-xs"
            @click="emit('edit')"
          >
            编辑团队设置
          </UiButton>
          <UiButton
            variant="secondary"
            class="text-xs text-red-500 hover:text-red-600"
            @click="pendingDeleteGroup = true"
          >
            删除团队
          </UiButton>
        </div>
      </div>

      <!-- Orchestrator System Prompt Preview / Collapsible -->
      <div v-if="group?.orchestratorPrompt" class="mt-4 pt-3 border-t border-[var(--color-border)]">
        <button
          type="button"
          class="flex items-center gap-1.5 text-2xs font-medium text-[var(--color-text-muted)] hover:text-[var(--color-text)] transition-colors"
          @click="showPromptExpanded = !showPromptExpanded"
        >
          <svg
            viewBox="0 0 16 16"
            width="12"
            height="12"
            fill="none"
            stroke="currentColor"
            stroke-width="1.8"
            class="transition-transform"
            :class="[showPromptExpanded ? 'rotate-90' : '']"
          >
            <polyline points="6 3 11 8 6 13" />
          </svg>
          <span>中控调度大脑专属角色提示词与团队规范</span>
        </button>
        <div v-if="showPromptExpanded" class="mt-2 p-3 rounded bg-[var(--color-surface-muted)] text-2xs font-mono whitespace-pre-wrap leading-relaxed text-[var(--color-text)] border border-[var(--color-border)]">
          {{ group.orchestratorPrompt }}
        </div>
      </div>
    </div>

    <!-- 2. Team Members Strip: Roles from Role Library -->
    <div class="bg-[var(--color-surface)] border border-[var(--color-border)] rounded-[var(--radius,6px)] p-5 space-y-3">
      <div class="flex items-center justify-between">
        <div class="flex items-center gap-2">
          <span class="text-xs font-semibold text-[var(--color-text)] uppercase tracking-wider">团队专家成员</span>
          <span class="text-2xs text-[var(--color-text-muted)] font-mono">({{ group?.members?.length || 0 }})</span>
        </div>
        <UiButton
          variant="secondary"
          class="text-xs h-7 px-2.5"
          @click="showEditGroupDialog = true"
        >
          调整成员
        </UiButton>
      </div>

      <div class="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-3">
        <div
          v-for="m in (group?.members || [])"
          :key="m.id"
          class="p-3 rounded border border-[var(--color-border)] bg-[var(--color-surface-muted)] flex flex-col justify-between space-y-2 hover:border-[var(--color-border-strong)] transition-colors"
        >
          <div>
            <div class="flex items-center justify-between gap-1">
              <span class="text-xs font-medium text-[var(--color-text)] truncate">{{ m.name }}</span>
              <BackendTag :backend="m.backend" />
            </div>
            <p class="text-2xs text-[var(--color-text-muted)] line-clamp-2 mt-1">
              {{ m.description || m.systemPrompt || '暂无设定' }}
            </p>
          </div>
          <div class="text-3xs text-[var(--color-text-muted)] font-mono truncate pt-1 border-t border-[var(--color-border)]">
            模型: {{ m.model }}
          </div>
        </div>

        <div
          v-if="!group?.members || group.members.length === 0"
          class="col-span-full py-6 text-center text-xs text-[var(--color-text-muted)]"
        >
          当前团队暂未添加任何专家成员，请点击右上角「调整成员」从角色库添加。
        </div>
      </div>
    </div>

    <!-- 3. Group Tasks List with Project & Worktree Badges -->
    <div class="bg-[var(--color-surface)] border border-[var(--color-border)] rounded-[var(--radius,6px)] p-5 space-y-4">
      <div class="flex items-center justify-between">
        <div>
          <h2 class="text-xs font-semibold text-[var(--color-text)] uppercase tracking-wider">群组协同任务</h2>
          <p class="text-2xs text-[var(--color-text-muted)] mt-0.5">给团队创建的任务将由中控大脑自主规划，在指定项目的工作树分支下协作执行。</p>
        </div>
        <UiButton
          variant="primary"
          class="text-xs h-8 px-3"
          @click="onNewTask"
        >
          + 创建群组任务
        </UiButton>
      </div>

      <!-- Task Table -->
      <div class="overflow-x-auto border border-[var(--color-border)] rounded">
        <table class="w-full text-left text-xs">
          <thead class="bg-[var(--color-surface-muted)] border-b border-[var(--color-border)] text-[var(--color-text-muted)] uppercase text-3xs">
            <tr>
              <th class="py-2.5 px-3">任务名称</th>
              <th class="py-2.5 px-3">关联项目</th>
              <th class="py-2.5 px-3">项目工作树 (Worktree)</th>
              <th class="py-2.5 px-3">调度方式</th>
              <th class="py-2.5 px-3">执行状态</th>
              <th class="py-2.5 px-3 text-right">操作</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-[var(--color-border)] text-[var(--color-text)]">
            <tr
              v-for="task in tasks"
              :key="task.id"
              class="hover:bg-[var(--color-surface-muted)] transition-colors"
            >
              <td class="py-3 px-3">
                <div class="font-medium text-[var(--color-text)]">{{ task.title }}</div>
                <div v-if="task.description" class="text-2xs text-[var(--color-text-muted)] truncate max-w-xs">
                  {{ task.description }}
                </div>
              </td>
              <td class="py-3 px-3">
                <span class="inline-flex items-center gap-1 font-medium">
                  <svg viewBox="0 0 16 16" width="12" height="12" fill="none" stroke="currentColor" stroke-width="1.8" class="text-[var(--color-text-muted)]">
                    <path d="M2 13V3a1 1 0 0 1 1-1h4l2 2h4a1 1 0 0 1 1 1v8a1 1 0 0 1-1 1H3a1 1 0 0 1-1-1z" />
                  </svg>
                  {{ getProjectName(task.projectId) }}
                </span>
              </td>
              <td class="py-3 px-3">
                <span
                  class="inline-flex items-center gap-1 px-2 py-0.5 rounded text-2xs font-mono border"
                  :class="[
                    task.worktreeId === 0
                      ? 'bg-gray-500/10 text-gray-700 dark:text-gray-300 border-gray-500/20'
                      : 'bg-emerald-500/10 text-emerald-700 dark:text-emerald-300 border-emerald-500/30',
                  ]"
                >
                  <svg viewBox="0 0 16 16" width="11" height="11" fill="none" stroke="currentColor" stroke-width="1.8">
                    <circle cx="5" cy="5" r="2" />
                    <circle cx="11" cy="11" r="2" />
                    <path d="M5 7v4a2 2 0 0 0 2 2h2" />
                  </svg>
                  {{ getWorktreeBranch(task.projectId, task.worktreeId) }}
                </span>
              </td>
              <td class="py-3 px-3">
                <span class="capitalize text-2xs text-[var(--color-text-muted)]">
                  {{ task.scheduleType === 'cron' ? `Cron (${task.cronExpr})` : task.scheduleType === 'once' ? '定时单次' : '手动触发' }}
                </span>
              </td>
              <td class="py-3 px-3">
                <UiBadge :variant="getStatusBadge(task.lastRunStatus).variant">
                  {{ getStatusBadge(task.lastRunStatus).label }}
                </UiBadge>
              </td>
              <td class="py-3 px-3 text-right">
                <div class="flex items-center justify-end gap-1.5">
                  <UiButton
                    variant="primary"
                    class="text-2xs h-6 px-2"
                    @click="handleTriggerTask(task)"
                  >
                    立即执行
                  </UiButton>
                  <UiButton
                    variant="secondary"
                    class="text-2xs h-6 px-2"
                    @click="openTaskChat(task)"
                  >
                    群聊视窗
                  </UiButton>
                  <UiButton
                    variant="secondary"
                    class="text-2xs h-6 px-2"
                    @click="onEditTask(task)"
                  >
                    编辑
                  </UiButton>
                  <UiButton
                    variant="secondary"
                    class="text-2xs h-6 px-2 text-red-500 hover:text-red-600"
                    @click="pendingDeleteTask = task"
                  >
                    删除
                  </UiButton>
                </div>
              </td>
            </tr>

            <tr v-if="tasks.length === 0">
              <td colspan="6" class="py-8 text-center text-xs text-[var(--color-text-muted)]">
                当前团队暂未创建任何群组任务，请点击「+ 创建群组任务」开始协同。
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- Dialogs -->
    <GroupTaskCreateModal
      v-if="showCreateTaskModal"
      v-model:open="showCreateTaskModal"
      :group-id="groupId"
      :task="editingTask"
      @created="groupsStore.loadGroupTasks(groupId)"
    />

    <GroupFlowChatDialog
      v-if="showFlowChatDialog && activeChatTask"
      v-model:open="showFlowChatDialog"
      :group-id="groupId"
      :task="activeChatTask"
    />

    <!-- Delete Group Confirm -->
    <UiConfirmDialog
      v-if="pendingDeleteGroup"
      title="确认删除群组团队？"
      message="此操作将永久删除该群组团队及其关联的所有群组任务与协同执行记录，操作不可撤销。"
      confirm-label="确认删除"
      variant="danger"
      @confirm="confirmDeleteGroup"
      @close="pendingDeleteGroup = false"
    />

    <!-- Delete Task Confirm -->
    <UiConfirmDialog
      v-if="!!pendingDeleteTask"
      title="确认删除该群组任务？"
      message="此操作将删除该群组任务及其全部协同历史记录。"
      confirm-label="确认删除"
      variant="danger"
      @confirm="confirmDeleteTask"
      @close="pendingDeleteTask = null"
    />
  </div>
</template>
