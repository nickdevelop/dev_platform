<script setup lang="ts">
// GroupManageDialog: Create or edit a team group, configure the orchestrator brain model,
// orchestrator system prompt, and select member roles from the role library.
import { computed, onMounted, ref, watch } from 'vue'
import type { GroupDetail, CreateGroupPayload, UpdateGroupPayload } from '../../api/groups'
import type { AgentRole } from '../../api/roles'
import { useRolesStore } from '../../stores/roles'
import { useModelsStore } from '../../stores/models'
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
import BackendTag from '../../shared/components/BackendTag.vue'

const props = defineProps<{
  open: boolean
  group?: GroupDetail | null
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
  saved: [groupId: number]
}>()

const rolesStore = useRolesStore()
const modelsStore = useModelsStore()

const name = ref('')
const description = ref('')
const orchestratorModel = ref('')
const orchestratorPrompt = ref('')
const selectedRoleIds = ref<number[]>([])

const saving = ref(false)
const errorMessage = ref('')

const isEdit = computed(() => !!props.group)

// Available chat models from configured model services
const chatModelOptions = computed<SelectMenuItem<string>[]>(() => {
  const items: SelectMenuItem<string>[] = []
  for (const svc of modelsStore.services || []) {
    if (!svc.enabled) continue
    for (const m of svc.models || []) {
      if (m.usable && m.supportsText) {
        items.push({
          value: m.modelId,
          label: m.name ? `${m.name} (${m.modelId})` : m.modelId,
          sublabel: svc.name,
        })
      }
    }
  }
  return items
})

// Roles selected from global role library
const selectedRoles = computed<AgentRole[]>(() => {
  return selectedRoleIds.value
    .map((id) => rolesStore.byId(id))
    .filter((r): r is AgentRole => !!r)
})

const unselectedRoles = computed<AgentRole[]>(() => {
  return (rolesStore.roles || []).filter((r) => !selectedRoleIds.value.includes(r.id))
})

watch(
  () => props.open,
  async (isOpen) => {
    if (!isOpen) return
    errorMessage.value = ''
    await Promise.all([rolesStore.loadRoles(), modelsStore.load()])

    if (props.group) {
      name.value = props.group.name
      description.value = props.group.description || ''
      orchestratorModel.value = props.group.orchestratorModel
      orchestratorPrompt.value = props.group.orchestratorPrompt || ''
      selectedRoleIds.value = (props.group.members || []).map((m) => m.roleId)
    } else {
      name.value = ''
      description.value = ''
      orchestratorModel.value = chatModelOptions.value[0]?.value || ''
      orchestratorPrompt.value = `你是一个软件开发团队的中控调度大脑。
职责：
1. 深入分析用户的目标需求，制定模块化执行方案。
2. 自主指派团队中的专业角色（如架构师、后端、前端、测试）分工完成代码编写与验证。
3. 协调解决角色之间的接口对齐与冲突，确保整体代码质量。`
      // Preselect first two roles if available
      selectedRoleIds.value = (rolesStore.roles || []).slice(0, 2).map((r) => r.id)
    }
  },
  { immediate: true },
)

function toggleRole(roleId: number) {
  const idx = selectedRoleIds.value.indexOf(roleId)
  if (idx === -1) {
    selectedRoleIds.value.push(roleId)
  } else {
    selectedRoleIds.value.splice(idx, 1)
  }
}

async function handleSave() {
  if (!name.value.trim()) {
    errorMessage.value = '请输入群组/团队名称'
    return
  }
  if (!orchestratorModel.value.trim()) {
    errorMessage.value = '请选择中控调度大脑模型'
    return
  }
  if (selectedRoleIds.value.length === 0) {
    errorMessage.value = '请至少从角色库中选择一位团队成员'
    return
  }

  saving.value = true
  errorMessage.value = ''

  try {
    const groupsStore = useGroupsStore()

    if (isEdit.value && props.group) {
      const updatePayload: UpdateGroupPayload = {
        name: name.value.trim(),
        description: description.value.trim(),
        orchestratorModel: orchestratorModel.value.trim(),
        orchestratorPrompt: orchestratorPrompt.value.trim(),
      }
      await groupsStore.updateGroup(props.group.id, updatePayload)
      await groupsStore.updateMembers(props.group.id, selectedRoleIds.value)
      emit('saved', props.group.id)
    } else {
      const createPayload: CreateGroupPayload = {
        name: name.value.trim(),
        description: description.value.trim(),
        orchestratorModel: orchestratorModel.value.trim(),
        orchestratorPrompt: orchestratorPrompt.value.trim(),
        roleIds: selectedRoleIds.value,
      }
      const created = await groupsStore.createGroup(createPayload)
      emit('saved', created.id)
    }
    emit('update:open', false)
  } catch (err: any) {
    errorMessage.value = err?.message || '保存团队群组失败'
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <UiDialog
    v-if="open"
    :title="isEdit ? '编辑团队群组' : '创建团队群组'"
    width-class="max-w-2xl w-full"
    @close="emit('update:open', false)"
  >
    <div class="space-y-5 text-xs text-[var(--color-text)]">
      <UiAlert v-if="errorMessage" variant="error">
        {{ errorMessage }}
      </UiAlert>

      <!-- Basic Info -->
      <div class="grid grid-cols-1 gap-4">
        <div>
          <label class="block font-medium mb-1 text-[var(--color-text-muted)]">团队名称 *</label>
          <UiInput
            v-model="name"
            placeholder="例如：系统重构攻坚组、全栈开发协作团队"
            class="w-full"
            maxlength="60"
          />
        </div>
        <div>
          <label class="block font-medium mb-1 text-[var(--color-text-muted)]">团队职责描述</label>
          <UiInput
            v-model="description"
            placeholder="简述该团队的核心定位与业务场景"
            class="w-full"
          />
        </div>
      </div>

      <!-- Orchestrator Brain Config -->
      <div class="p-3.5 rounded border border-[var(--color-border)] bg-[var(--color-surface-muted)] space-y-3">
        <div class="flex items-center gap-2 font-medium text-[var(--color-text)]">
          <svg viewBox="0 0 16 16" width="14" height="14" fill="none" stroke="currentColor" stroke-width="1.8" class="text-accent">
            <circle cx="8" cy="8" r="3" />
            <path d="M8 1v2M8 13v2M1 8h2M13 8h2" />
          </svg>
          <span>中控调度大脑设定</span>
        </div>

        <div>
          <label class="block font-medium mb-1 text-[var(--color-text-muted)]">调度大脑模型 *</label>
          <UiSelectMenu
            v-model="orchestratorModel"
            :options="chatModelOptions"
            placeholder="请选择中枢大脑大模型…"
            class="w-full"
          />
        </div>

        <div>
          <label class="block font-medium mb-1 text-[var(--color-text-muted)]">
            中控大脑角色提示词（群组相关信息与规范）
          </label>
          <UiTextarea
            v-model="orchestratorPrompt"
            placeholder="定义中控调度大脑的行为规则、调度偏好及团队规约..."
            :rows="4"
            class="w-full font-mono text-2xs"
          />
        </div>
      </div>

      <!-- Member Selection from Role Library -->
      <div class="space-y-2">
        <div class="flex items-center justify-between">
          <label class="block font-medium text-[var(--color-text-muted)]">
            团队成员（从角色库勾选专家加入）*
          </label>
          <span class="text-2xs text-[var(--color-text-muted)]">已选 {{ selectedRoleIds.length }} 位专家</span>
        </div>

        <div class="grid grid-cols-1 sm:grid-cols-2 gap-2 max-h-56 overflow-y-auto p-1 border border-[var(--color-border)] rounded bg-[var(--color-surface)]">
          <div
            v-for="role in (rolesStore.roles || [])"
            :key="role.id"
            class="p-2.5 rounded border cursor-pointer select-none transition-colors flex items-start gap-2.5"
            :class="[
              selectedRoleIds.includes(role.id)
                ? 'border-[var(--color-primary)] bg-[var(--color-surface-muted)]'
                : 'border-[var(--color-border)] hover:bg-[var(--color-surface-muted)]',
            ]"
            @click="toggleRole(role.id)"
          >
            <input
              type="checkbox"
              :checked="selectedRoleIds.includes(role.id)"
              class="mt-0.5 rounded text-[var(--color-primary)]"
              @click.stop="toggleRole(role.id)"
            />
            <div class="min-w-0 flex-1">
              <div class="flex items-center justify-between gap-1">
                <span class="font-medium truncate text-[var(--color-text)]">{{ role.name }}</span>
                <BackendTag :backend="role.backend" />
              </div>
              <p class="text-2xs text-[var(--color-text-muted)] truncate mt-0.5">
                {{ role.description || role.systemPrompt || '暂无描述' }}
              </p>
              <div class="text-2xs text-[var(--color-text-muted)] mt-1 font-mono">
                {{ role.model }}
              </div>
            </div>
          </div>

          <div
            v-if="(rolesStore.roles || []).length === 0"
            class="col-span-2 py-6 text-center text-[var(--color-text-muted)] text-2xs"
          >
            角色库中暂无专家角色，请先前往「设置 - 角色管理」创建角色。
          </div>
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
          {{ isEdit ? '保存修改' : '创建团队' }}
        </UiButton>
      </div>
    </template>
  </UiDialog>
</template>
