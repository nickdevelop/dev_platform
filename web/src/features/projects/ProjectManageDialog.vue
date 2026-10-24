<script setup lang="ts">
// Project management dialog for one action: register, create, move, rename (directory),
// rename-name (display name) or unregister. Unregister removes the project from the platform
// (stopping its live agent instances and dropping their sessions) and never touches the
// directory on disk. Move and rename directory require the typed current project name.
// The parent owns visibility: it receives `submitted` and `close`.
import { computed, onMounted, ref } from 'vue'
import { describeApiError, UnauthorizedError } from '../../api/client'
import {
  createProject,
  moveProject,
  registerProject,
  renameProject,
  updateProjectName,
  unregisterProject,
  type Project,
} from '../../api/projects'
import { UiDialog, UiButton, UiInput, UiAlert } from '../../shared/ui'

type DialogKind = 'register' | 'create' | 'move' | 'rename' | 'rename-name' | 'unregister'

const props = defineProps<{
  kind: DialogKind
  project: Project | null
}>()

const emit = defineEmits<{
  close: []
  submitted: [kind: DialogKind]
}>()

const dialogError = ref('')
const submitting = ref(false)
const pathInput = ref('')
const nameInput = ref('')
const confirmNameInput = ref('')

onMounted(() => {
  if (props.kind === 'rename-name' && props.project) {
    nameInput.value = props.project.name
  }
})

const dialogTitle = computed(() => {
  const name = props.project?.name
  switch (props.kind) {
    case 'register':
      return '打开已有项目目录'
    case 'create':
      return '新建项目目录'
    case 'move':
      return `移动项目 ${name}`
    case 'rename':
      return `重命名项目目录 ${name}`
    case 'rename-name':
      return `修改项目名称 ${name}`
    case 'unregister':
      return '解除项目登记（保留目录）'
  }
})

const confirmLabel = computed(() => {
  switch (props.kind) {
    case 'register':
      return '打开'
    case 'create':
      return '创建'
    case 'move':
      return '移动'
    case 'rename':
      return '重命名目录'
    case 'rename-name':
      return '保存'
    case 'unregister':
      return '解除登记'
  }
})

const needsTypedConfirmation = computed(() => props.kind === 'move' || props.kind === 'rename')

const typedNameMatches = computed(
  () => !!props.project && confirmNameInput.value === props.project.name,
)

const canSubmit = computed(() => {
  if (submitting.value) return false
  if (props.kind === 'rename-name') {
    return nameInput.value.trim() !== ''
  }
  if (needsTypedConfirmation.value && !typedNameMatches.value) return false
  return true
})

async function submit() {
  if (!canSubmit.value) return
  const kind = props.kind
  const project = props.project
  submitting.value = true
  dialogError.value = ''
  try {
    switch (kind) {
      case 'register':
        await registerProject(pathInput.value.trim())
        break
      case 'create':
        await createProject(pathInput.value.trim(), nameInput.value.trim())
        break
      case 'move':
        await moveProject(project!.id, pathInput.value.trim(), confirmNameInput.value)
        break
      case 'rename':
        await renameProject(project!.id, nameInput.value.trim(), confirmNameInput.value)
        break
      case 'rename-name':
        await updateProjectName(project!.id, nameInput.value.trim())
        break
      case 'unregister':
        await unregisterProject(project!.id)
        break
    }
    emit('submitted', kind)
    emit('close')
  } catch (error) {
    if (error instanceof UnauthorizedError) return // global handler went to login
    dialogError.value = describeApiError(error)
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <UiDialog :title="dialogTitle" width-class="w-full max-w-lg" @close="emit('close')">
    <form class="dialog-form space-y-4" @submit.prevent="submit">
      <template v-if="kind === 'register'">
        <div class="flex flex-col gap-1.5">
          <label for="dialog-path" class="text-xs font-semibold text-slate-700 dark:text-slate-300">目录的绝对路径</label>
          <UiInput id="dialog-path" v-model="pathInput" autocomplete="off" spellcheck="false" block />
          <p class="hint text-xs text-slate-400">目录必须位于已授权的工作区内；平台只记录路径，不会修改目录内容。</p>
        </div>
      </template>

      <template v-else-if="kind === 'create'">
        <div class="flex flex-col gap-1.5">
          <label for="dialog-path" class="text-xs font-semibold text-slate-700 dark:text-slate-300">父目录的绝对路径</label>
          <UiInput id="dialog-path" v-model="pathInput" autocomplete="off" spellcheck="false" block />
        </div>
        <div class="flex flex-col gap-1.5">
          <label for="dialog-name" class="text-xs font-semibold text-slate-700 dark:text-slate-300">项目目录名</label>
          <UiInput id="dialog-name" v-model="nameInput" autocomplete="off" spellcheck="false" block />
          <p class="hint text-xs text-slate-400">将在父目录下创建同名新目录；已存在的目标不会被覆盖。</p>
        </div>
      </template>

      <template v-else-if="kind === 'move'">
        <p class="text-sm text-slate-700 dark:text-slate-300">把项目 {{ project?.name }}（{{ project?.path }}）移动到新位置。</p>
        <div class="flex flex-col gap-1.5">
          <label for="dialog-path" class="text-xs font-semibold text-slate-700 dark:text-slate-300">目标完整路径</label>
          <UiInput id="dialog-path" v-model="pathInput" autocomplete="off" spellcheck="false" block />
        </div>
        <div class="flex flex-col gap-1.5">
          <label for="dialog-confirm-name" class="text-xs font-semibold text-slate-700 dark:text-slate-300">
            输入当前项目名“{{ project?.name }}”以确认移动
          </label>
          <UiInput id="dialog-confirm-name" v-model="confirmNameInput" class="confirm-name" autocomplete="off" block />
        </div>
      </template>

      <template v-else-if="kind === 'rename-name'">
        <div class="flex flex-col gap-1.5">
          <label for="dialog-display-name" class="text-xs font-semibold text-slate-700 dark:text-slate-300">项目显示名称</label>
          <UiInput id="dialog-display-name" v-model="nameInput" autocomplete="off" spellcheck="false" block />
          <p class="hint text-xs text-slate-400">仅修改在平台中展示的项目名称，不会修改磁盘上的物理目录。</p>
        </div>
      </template>

      <template v-else-if="kind === 'rename'">
        <div class="flex flex-col gap-1.5">
          <label for="dialog-name" class="text-xs font-semibold text-slate-700 dark:text-slate-300">新目录名</label>
          <UiInput id="dialog-name" v-model="nameInput" autocomplete="off" spellcheck="false" block />
        </div>
        <div class="flex flex-col gap-1.5">
          <label for="dialog-confirm-name" class="text-xs font-semibold text-slate-700 dark:text-slate-300">
            输入当前项目名“{{ project?.name }}”以确认重命名目录
          </label>
          <UiInput id="dialog-confirm-name" v-model="confirmNameInput" class="confirm-name" autocomplete="off" block />
          <p class="hint text-xs text-slate-400">将重命名磁盘上的项目文件夹，同时需要暂停关联的 Agent 实例。</p>
        </div>
      </template>

      <template v-else-if="kind === 'unregister'">
        <p class="text-sm text-slate-700 dark:text-slate-300">将从平台移除项目 {{ project?.name }}，同时停止它正在运行的 Agent 实例，并清除其会话索引。</p>
        <p class="safe-note text-xs text-emerald-600 dark:text-emerald-400 font-medium">磁盘上的目录 {{ project?.path }} 会被完整保留，不受影响。</p>
        <p class="hint text-xs text-slate-400">如有实例无法停止，移除会被中止，项目保持登记。</p>
      </template>

      <UiAlert v-if="dialogError" variant="error" class="dialog-error">
        {{ dialogError }}
      </UiAlert>

      <div class="dialog-actions flex items-center justify-end gap-2.5 pt-3 border-t border-slate-100 dark:border-slate-800">
        <UiButton
          type="button"
          variant="secondary"
          class="cancel"
          @click="emit('close')"
        >
          取消
        </UiButton>
        <UiButton
          type="button"
          :variant="kind === 'unregister' ? 'danger' : 'primary'"
          class="confirm"
          :disabled="!canSubmit"
          :loading="submitting"
          @click="submit"
        >
          {{ confirmLabel }}
        </UiButton>
      </div>
    </form>
  </UiDialog>
</template>
