<script setup lang="ts">
// Project creation dialog for one action: register an existing directory or
// create a new one. Display-name editing and git worktree management live on
// the project settings tab; unregister lives on the projects management page.
// The parent owns visibility: it receives `submitted` and `close`.
import { computed, ref, watch } from 'vue'
import { describeApiError, UnauthorizedError } from '../../api/client'
import {
  createProject,
  registerProject,
  updateProjectName,
  type Project,
} from '../../api/projects'
import { UiDialog, UiButton, UiInput, UiAlert } from '../../shared/ui'
import ProxyDirectoryPicker from '../proxies/ProxyDirectoryPicker.vue'

type DialogKind = 'register' | 'create'

const props = defineProps<{
  kind: DialogKind
}>()

const emit = defineEmits<{
  close: []
  submitted: [kind: DialogKind]
}>()

const dialogError = ref('')
const submitting = ref(false)
const pathInput = ref('')
const nameInput = ref('')
const subDirName = ref('')
const createNewDir = ref(true)
const showDirPicker = ref(false)

// When user types project name, default the sub directory name to it if user hasn't explicitly diverged
let subDirManuallyEdited = false
watch(nameInput, (val) => {
  if (!subDirManuallyEdited && props.kind === 'create') {
    subDirName.value = val.trim().replace(/[\/\\]/g, '-').toLowerCase()
  }
})

function onSubDirInput(val: string) {
  subDirName.value = val
  subDirManuallyEdited = true
}

function onDirectorySelected(path: string) {
  pathInput.value = path
  showDirPicker.value = false
}

const dialogTitle = computed(() => {
  switch (props.kind) {
    case 'register':
      return '打开已有项目目录'
    case 'create':
      return '新建项目'
  }
})

const confirmLabel = computed(() => {
  switch (props.kind) {
    case 'register':
      return '打开'
    case 'create':
      return '确定'
  }
})

const canSubmit = computed(() => {
  if (submitting.value) return false
  if (props.kind === 'register') return pathInput.value.trim() !== ''
  if (!nameInput.value.trim() || !pathInput.value.trim()) return false
  if (createNewDir.value && !subDirName.value.trim()) return false
  return true
})

async function submit() {
  if (!canSubmit.value) return
  const kind = props.kind
  submitting.value = true
  dialogError.value = ''
  try {
    switch (kind) {
      case 'register':
        await registerProject(pathInput.value.trim())
        break
      case 'create': {
        const trimmedPath = pathInput.value.trim()
        const trimmedName = nameInput.value.trim()
        let created: Project
        if (createNewDir.value) {
          const dir = subDirName.value.trim() || trimmedName
          created = await createProject(trimmedPath, dir)
        } else {
          created = await registerProject(trimmedPath)
        }
        if (trimmedName && created.name !== trimmedName) {
          // Creating with a friendly name uses the register/create endpoints
          // plus one display-name update; later edits live on the project
          // settings tab.
          await updateProjectName(created.id, trimmedName)
        }
        break
      }
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
          <label for="dialog-project-name" class="text-xs font-semibold text-slate-700 dark:text-slate-300">
            项目名称 <span class="text-red-500">*</span>
          </label>
          <UiInput
            id="dialog-project-name"
            v-model="nameInput"
            placeholder="例如：my-awesome-app"
            autocomplete="off"
            spellcheck="false"
            block
          />
          <p class="hint text-xs text-slate-400">在平台工作区和侧边栏中显示的项目友好名称。</p>
        </div>

        <div class="flex flex-col gap-1.5">
          <label for="dialog-path" class="text-xs font-semibold text-slate-700 dark:text-slate-300">
            项目路径 <span class="text-red-500">*</span>
          </label>
          <div class="flex items-center gap-2">
            <UiInput
              id="dialog-path"
              v-model="pathInput"
              class="flex-1"
              placeholder="例如：/data/projects"
              autocomplete="off"
              spellcheck="false"
              block
            />
            <UiButton
              type="button"
              variant="secondary"
              size="md"
              class="shrink-0 text-xs px-3"
              @click="showDirPicker = true"
            >
              浏览…
            </UiButton>
          </div>
          <p class="hint text-xs text-slate-400">
            {{ createNewDir ? '将在该父目录下创建项目子目录' : '将直接把此目录登记为项目根目录' }}
          </p>
        </div>

        <div class="pt-1 border-t border-slate-100 dark:border-slate-800">
          <label class="flex items-center gap-2 text-xs text-slate-700 dark:text-slate-300 cursor-pointer select-none">
            <input
              v-model="createNewDir"
              type="checkbox"
              class="rounded border-slate-300 dark:border-slate-700 text-blue-600 focus:ring-blue-500"
            />
            <span class="font-medium">在指定目录下新建项目目录</span>
          </label>
        </div>

        <div v-if="createNewDir" class="flex flex-col gap-1.5 pl-6">
          <label for="dialog-subdir" class="text-xs font-medium text-slate-600 dark:text-slate-400">新建目录名</label>
          <UiInput
            id="dialog-subdir"
            :model-value="subDirName"
            placeholder="项目目录名"
            autocomplete="off"
            spellcheck="false"
            block
            @update:model-value="onSubDirInput"
          />
          <p class="hint text-xs text-slate-400">
            完整路径将为：<code class="font-mono text-slate-600 dark:text-slate-300">{{ pathInput ? `${pathInput.replace(/\/+$/, '')}/${subDirName || '...'}` : `.../${subDirName || ''}` }}</code>
          </p>
        </div>
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
          variant="primary"
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

  <ProxyDirectoryPicker
    v-if="showDirPicker"
    :initial-path="pathInput"
    @close="showDirPicker = false"
    @select="onDirectorySelected"
  />
</template>
