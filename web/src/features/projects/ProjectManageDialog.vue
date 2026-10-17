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
import { useAuthStore } from '../../stores/auth'
import AppDialog from '../../shared/components/AppDialog.vue'

type DialogKind = 'register' | 'create' | 'move' | 'rename' | 'rename-name' | 'unregister'

const props = defineProps<{
  kind: DialogKind
  project: Project | null
}>()

const emit = defineEmits<{
  close: []
  submitted: [kind: DialogKind]
}>()

const auth = useAuthStore()

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
      return '注册已有项目目录'
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
      return '注册'
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
        await registerProject(pathInput.value.trim(), auth.csrfToken)
        break
      case 'create':
        await createProject(pathInput.value.trim(), nameInput.value.trim(), auth.csrfToken)
        break
      case 'move':
        await moveProject(project!.id, pathInput.value.trim(), confirmNameInput.value, auth.csrfToken)
        break
      case 'rename':
        await renameProject(project!.id, nameInput.value.trim(), confirmNameInput.value, auth.csrfToken)
        break
      case 'rename-name':
        await updateProjectName(project!.id, nameInput.value.trim(), auth.csrfToken)
        break
      case 'unregister':
        await unregisterProject(project!.id, auth.csrfToken)
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
  <AppDialog :title="dialogTitle" @close="emit('close')">
    <form class="dialog-form" @submit.prevent="submit">
      <template v-if="kind === 'register'">
        <label for="dialog-path">目录的绝对路径</label>
        <input id="dialog-path" v-model="pathInput" type="text" autocomplete="off" spellcheck="false" />
        <p class="hint">目录必须位于已授权的工作区内；平台只记录路径，不会修改目录内容。</p>
      </template>

      <template v-else-if="kind === 'create'">
        <label for="dialog-path">父目录的绝对路径</label>
        <input id="dialog-path" v-model="pathInput" type="text" autocomplete="off" spellcheck="false" />
        <label for="dialog-name">项目目录名</label>
        <input id="dialog-name" v-model="nameInput" type="text" autocomplete="off" spellcheck="false" />
        <p class="hint">将在父目录下创建同名新目录；已存在的目标不会被覆盖。</p>
      </template>

      <template v-else-if="kind === 'move'">
        <p>把项目 {{ project?.name }}（{{ project?.path }}）移动到新位置。</p>
        <label for="dialog-path">目标完整路径</label>
        <input id="dialog-path" v-model="pathInput" type="text" autocomplete="off" spellcheck="false" />
        <label for="dialog-confirm-name">输入当前项目名“{{ project?.name }}”以确认移动</label>
        <input id="dialog-confirm-name" v-model="confirmNameInput" class="confirm-name" type="text" autocomplete="off" />
      </template>

      <template v-else-if="kind === 'rename-name'">
        <label for="dialog-display-name">项目显示名称</label>
        <input id="dialog-display-name" v-model="nameInput" type="text" autocomplete="off" spellcheck="false" />
        <p class="hint">仅修改在平台中展示的项目名称，不会修改磁盘上的物理目录。</p>
      </template>

      <template v-else-if="kind === 'rename'">
        <label for="dialog-name">新目录名</label>
        <input id="dialog-name" v-model="nameInput" type="text" autocomplete="off" spellcheck="false" />
        <label for="dialog-confirm-name">输入当前项目名“{{ project?.name }}”以确认重命名目录</label>
        <input id="dialog-confirm-name" v-model="confirmNameInput" class="confirm-name" type="text" autocomplete="off" />
        <p class="hint">将重命名磁盘上的项目文件夹，同时需要暂停关联的 Agent 实例。</p>
      </template>

      <template v-else-if="kind === 'unregister'">
        <p>将从平台移除项目 {{ project?.name }}，同时停止它正在运行的 Agent 实例，并清除其会话索引。</p>
        <p class="safe-note">磁盘上的目录 {{ project?.path }} 会被完整保留，不受影响。</p>
        <p class="hint">如有实例无法停止，移除会被中止，项目保持登记。</p>
      </template>

      <p v-if="dialogError" class="dialog-error" role="alert">{{ dialogError }}</p>

      <div class="dialog-actions">
        <button type="button" class="cancel" @click="emit('close')">取消</button>
        <button type="button" class="confirm" :disabled="!canSubmit" @click="submit">
          {{ confirmLabel }}
        </button>
      </div>
    </form>
  </AppDialog>
</template>

<style scoped>
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

.dialog-form {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.dialog-form label {
  font-weight: 600;
  margin-top: 0.4rem;
}

.dialog-form input[type='text'] {
  height: var(--control-height);
  padding: 0 calc(var(--space-unit) * 0.75);
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  background: var(--color-surface);
  color: var(--color-text);
  font: inherit;
}

.dialog-form input[type='text']:focus {
  outline: none;
  border-color: var(--color-primary);
  box-shadow: 0 0 0 2px var(--color-primary-soft);
}

.hint {
  margin: 0;
  font-size: 0.857rem;
  color: var(--color-text-muted);
}

.safe-note {
  margin: 0;
  font-size: 0.857rem;
  color: var(--color-success, #16a34a);
}

.dialog-error {
  margin: 0.25rem 0 0 0;
  color: var(--color-danger);
  font-size: 0.857rem;
}

.dialog-actions {
  display: flex;
  justify-content: flex-end;
  gap: calc(var(--space-unit) * 0.75);
  margin-top: calc(var(--space-unit) * 1.25);
}

.dialog-actions .confirm {
  background: var(--color-primary);
  border-color: var(--color-primary);
  color: #fff;
}

.dialog-actions .confirm:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
</style>
