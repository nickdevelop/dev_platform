<script setup lang="ts">
// Project management dialog for one action: register, create, move, rename,
// unregister or delete. Unregister (keeps the directory) and delete (removes
// the directory) are separate, explicitly labeled flows; move, rename and
// delete require the typed current project name, and delete additionally needs
// an explicit permanent-delete confirmation when the server reports the OS trash
// unavailable. The parent owns visibility: it receives `submitted` and `close`.
import { computed, ref } from 'vue'
import { ApiError, describeApiError, UnauthorizedError } from '../api/client'
import {
  createProject,
  deleteProject,
  moveProject,
  registerProject,
  renameProject,
  unregisterProject,
  type Project,
} from '../api/projects'
import { useAuthStore } from '../stores/auth'
import ProjectDialog from './ProjectDialog.vue'

type DialogKind = 'register' | 'create' | 'move' | 'rename' | 'unregister' | 'delete'

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
const permanent = ref(false)
const trashUnavailable = ref(false)

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
      return `重命名项目 ${name}`
    case 'unregister':
      return '移除平台记录（保留目录）'
    case 'delete':
      return '删除项目目录'
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
      return '重命名'
    case 'unregister':
      return '移除记录'
    case 'delete':
      return '删除目录'
  }
})

const needsTypedConfirmation = computed(
  () => props.kind === 'move' || props.kind === 'rename' || props.kind === 'delete',
)

const typedNameMatches = computed(
  () => !!props.project && confirmNameInput.value === props.project.name,
)

const canSubmit = computed(() => {
  if (submitting.value) return false
  if (needsTypedConfirmation.value && !typedNameMatches.value) return false
  if (props.kind === 'delete' && trashUnavailable.value && !permanent.value) return false
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
      case 'unregister':
        await unregisterProject(project!.id, auth.csrfToken)
        break
      case 'delete':
        await deleteProject(project!.id, confirmNameInput.value, permanent.value, auth.csrfToken)
        break
    }
    emit('submitted', kind)
    emit('close')
  } catch (error) {
    if (error instanceof UnauthorizedError) return // global handler went to login
    if (error instanceof ApiError && error.code === 'trash_unavailable' && kind === 'delete') {
      // The server refused to delete without the trash; nothing was removed.
      // Offer the explicit permanent-delete confirmation.
      trashUnavailable.value = true
      dialogError.value = '系统回收站不可用，目录未被删除。如确认放弃该目录，请勾选永久删除后重试。'
    } else {
      dialogError.value = describeApiError(error)
    }
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <ProjectDialog :title="dialogTitle" :danger="kind === 'delete'" @close="emit('close')">
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

      <template v-else-if="kind === 'rename'">
        <label for="dialog-name">新目录名</label>
        <input id="dialog-name" v-model="nameInput" type="text" autocomplete="off" spellcheck="false" />
        <label for="dialog-confirm-name">输入当前项目名“{{ project?.name }}”以确认重命名</label>
        <input id="dialog-confirm-name" v-model="confirmNameInput" class="confirm-name" type="text" autocomplete="off" />
      </template>

      <template v-else-if="kind === 'unregister'">
        <p>将从平台移除项目 {{ project?.name }} 的记录。</p>
        <p class="safe-note">磁盘上的目录 {{ project?.path }} 会被完整保留，不受影响。</p>
      </template>

      <template v-else-if="kind === 'delete'">
        <p class="danger-note">项目 {{ project?.name }} 的目录将被删除！</p>
        <p>平台会优先把目录 {{ project?.path }} 移入系统回收站。</p>
        <p v-if="trashUnavailable" class="danger-note">
          系统回收站不可用。若继续，只能永久删除该目录，且无法恢复。
        </p>
        <label v-if="trashUnavailable" class="permanent-option">
          <input v-model="permanent" type="checkbox" />
          我确认永久删除该目录（不进回收站，无法恢复）
        </label>
        <label for="dialog-confirm-name">输入项目名“{{ project?.name }}”以确认删除</label>
        <input id="dialog-confirm-name" v-model="confirmNameInput" class="confirm-name" type="text" autocomplete="off" />
      </template>

      <p v-if="dialogError" class="dialog-error" role="alert">{{ dialogError }}</p>

      <div class="dialog-actions">
        <button type="button" class="cancel" @click="emit('close')">取消</button>
        <button type="button" class="confirm" :disabled="!canSubmit" @click="submit">
          {{ confirmLabel }}
        </button>
      </div>
    </form>
  </ProjectDialog>
</template>

<style scoped>
.dialog-actions button {
  padding: 0.35rem 0.7rem;
  border: 1px solid var(--color-border-strong);
  border-radius: 6px;
  background: var(--color-surface);
  cursor: pointer;
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
  padding: 0.4rem 0.6rem;
  border: 1px solid var(--color-border-strong);
  border-radius: 6px;
}

.dialog-form .hint {
  color: var(--color-text-muted);
  font-size: 0.85rem;
  margin: 0;
}

.dialog-form .safe-note {
  color: var(--color-success);
}

.dialog-form .danger-note {
  color: var(--color-danger);
}

.dialog-form .permanent-option {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  font-weight: 400;
}

.dialog-error {
  color: var(--color-danger);
  margin: 0;
}

.dialog-actions {
  display: flex;
  justify-content: flex-end;
  gap: 0.5rem;
  margin-top: 0.75rem;
}

.dialog-actions .confirm {
  background: var(--color-primary);
  border-color: var(--color-primary);
  color: var(--color-surface);
}

.dialog-actions .confirm:disabled {
  background: var(--color-info-border);
  border-color: var(--color-info-border);
  cursor: default;
}

:deep(.dialog.danger) .dialog-actions .confirm {
  background: var(--color-danger);
  border-color: var(--color-danger);
}

:deep(.dialog.danger) .dialog-actions .confirm:disabled {
  background: var(--color-danger-border);
  border-color: var(--color-danger-border);
}
</style>
