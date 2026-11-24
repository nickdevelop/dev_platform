<script setup lang="ts">
// GroupEditPanel: Main workbench panel for creating or editing a team group.
// Runs inside AppShell's router-view (pane) without modal backdrops, keeping
// the category rail and middle sidebar 100% visible and interactive.
// Adopts the "Add / Remove" pattern for team members instead of checkbox toggles.
import { computed, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import type { GroupDetail, Group, CreateGroupPayload, UpdateGroupPayload } from '../../api/groups'
import type { AgentRole } from '../../api/roles'
import { useRolesStore } from '../../stores/roles'
import { useModelsStore } from '../../stores/models'
import { useGroupsStore } from '../../stores/groups'
import {
  UiButton,
  UiInput,
  UiTextarea,
  UiSelectMenu,
  UiAlert,
  type SelectMenuItem,
} from '../../shared/ui'
import BackendTag from '../../shared/components/BackendTag.vue'

const props = defineProps<{
  group?: GroupDetail | Group | null
}>()

const emit = defineEmits<{
  saved: [groupId: number]
  cancel: []
}>()

const router = useRouter()
const rolesStore = useRolesStore()
const modelsStore = useModelsStore()
const groupsStore = useGroupsStore()

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

const defaultOrchestratorPrompt = `你是一个软件开发团队的中控调度大脑。
职责：
1. 深入分析用户的目标需求，制定模块化执行方案。
2. 自主指派团队中的专业角色（如架构师、后端、前端、测试）分工完成代码编写与验证。
3. 协调解决角色之间的接口对齐与冲突，确保整体代码质量。`

onMounted(async () => {
  await Promise.all([rolesStore.loadRoles(), modelsStore.load()])
  initForm()
})

watch(
  () => props.group,
  () => {
    initForm()
  },
  { deep: true },
)

// Auto-select first chat model if none is selected
watch(chatModelOptions, (opts) => {
  if (!orchestratorModel.value && opts.length > 0) {
    orchestratorModel.value = opts[0].value
  }
})

function initForm() {
  errorMessage.value = ''
  if (props.group) {
    name.value = props.group.name
    description.value = props.group.description || ''
    orchestratorModel.value = props.group.orchestratorModel
    orchestratorPrompt.value = props.group.orchestratorPrompt || ''
    selectedRoleIds.value = ('members' in props.group && props.group.members ? props.group.members : []).map((m) => m.roleId)
  } else {
    name.value = ''
    description.value = ''
    orchestratorModel.value = chatModelOptions.value[0]?.value || ''
    orchestratorPrompt.value = defaultOrchestratorPrompt
    // Default to first two roles if available
    selectedRoleIds.value = (rolesStore.roles || []).slice(0, 2).map((r) => r.id)
  }
}

// ----------------------------------------------------
// Team Members (Add / Remove pattern)
// ----------------------------------------------------
const selectedRoles = computed<AgentRole[]>(() => {
  return selectedRoleIds.value
    .map((id) => rolesStore.byId(id))
    .filter((r): r is AgentRole => !!r)
})

// Unselected roles available to be added via dropdown or picker
const availableRoleOptions = computed<SelectMenuItem<number | 0>[]>(() => {
  const items: SelectMenuItem<number | 0>[] = [
    { value: 0, label: '+ 选择专家角色加入团队…' },
  ]
  for (const r of rolesStore.roles || []) {
    if (!selectedRoleIds.value.includes(r.id)) {
      items.push({
        value: r.id,
        label: `${r.name} (${r.backend} / ${r.model})`,
        sublabel: r.description || undefined,
      })
    }
  }
  return items
})

const unselectedRoles = computed<AgentRole[]>(() => {
  return (rolesStore.roles || []).filter((r) => !selectedRoleIds.value.includes(r.id))
})

function addRole(roleId: number) {
  if (roleId && !selectedRoleIds.value.includes(roleId)) {
    selectedRoleIds.value.push(roleId)
  }
}

function removeRole(roleId: number) {
  const idx = selectedRoleIds.value.indexOf(roleId)
  if (idx !== -1) {
    selectedRoleIds.value.splice(idx, 1)
  }
}

// ----------------------------------------------------
// Save & Cancel
// ----------------------------------------------------
async function handleSave() {
  errorMessage.value = ''
  const trimmedName = name.value.trim()
  if (!trimmedName) {
    errorMessage.value = '团队名称不能为空'
    return
  }
  if (!orchestratorModel.value) {
    errorMessage.value = '请指定中控调度大脑模型'
    return
  }

  saving.value = true
  try {
    if (props.group) {
      const updatePayload: UpdateGroupPayload = {
        name: trimmedName,
        description: description.value.trim(),
        orchestratorModel: orchestratorModel.value,
        orchestratorPrompt: orchestratorPrompt.value,
      }
      const updated = await groupsStore.updateGroup(props.group.id, updatePayload)
      await groupsStore.updateMembers(props.group.id, selectedRoleIds.value)
      emit('saved', updated.id)
    } else {
      const createPayload: CreateGroupPayload = {
        name: trimmedName,
        description: description.value.trim(),
        orchestratorModel: orchestratorModel.value,
        orchestratorPrompt: orchestratorPrompt.value,
        roleIds: selectedRoleIds.value,
      }
      const created = await groupsStore.createGroup(createPayload)
      emit('saved', created.id)
    }
  } catch (err: any) {
    errorMessage.value = err?.message || '保存团队群组失败'
  } finally {
    saving.value = false
  }
}

function handleCancel() {
  emit('cancel')
}
</script>

<template>
  <div class="group-edit-container flex justify-center items-start p-6 md:p-8 overflow-y-auto h-full bg-[var(--color-bg)]" data-test="group-edit-panel">
    <div class="edit-card w-full max-w-3xl bg-[var(--color-surface)] border border-[var(--color-border)] rounded-[var(--radius,6px)] p-6 md:p-8 space-y-6">
      <!-- Header -->
      <header class="flex items-start justify-between pb-4 border-b border-[var(--color-border)]">
        <div class="space-y-1">
          <h1 class="text-xl font-semibold text-[var(--color-text)] tracking-tight">
            {{ isEdit ? '编辑团队群组' : '新建团队群组' }}
          </h1>
          <p class="text-xs text-[var(--color-text-muted)]">
            配置协作团队人设、中控调度大脑模型、专属调度提示词规范及纳管的专家角色成员
          </p>
        </div>
        <button
          type="button"
          class="text-[var(--color-text-muted)] hover:text-[var(--color-text)] p-1.5 rounded transition-colors inline-flex items-center justify-center"
          title="关闭"
          aria-label="关闭面板"
          @click="handleCancel"
        >
          <svg viewBox="0 0 16 16" width="14" height="14" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true">
            <line x1="3" y1="3" x2="13" y2="13" />
            <line x1="13" y1="3" x2="3" y2="13" />
          </svg>
        </button>
      </header>

      <!-- Alert -->
      <UiAlert v-if="errorMessage" variant="error">
        {{ errorMessage }}
      </UiAlert>

      <form class="space-y-6" @submit.prevent="handleSave">
        <!-- 1. Basic Info -->
        <div class="space-y-3.5">
          <label class="text-xs font-semibold text-[var(--color-text-muted)] uppercase tracking-wider">
            基本信息
          </label>
          <div class="grid grid-cols-1 gap-3">
            <div class="flex flex-col gap-1.5">
              <label class="text-xs font-medium text-[var(--color-text)]">
                团队名称 <span class="text-red-500">*</span>
              </label>
              <UiInput
                v-model="name"
                placeholder="例如：系统重构攻坚组、全栈开发协作团队、前端体验研发组"
                maxlength="60"
                required
                block
              />
            </div>
            <div class="flex flex-col gap-1.5">
              <label class="text-xs font-medium text-[var(--color-text)]">
                团队职责描述 (可选)
              </label>
              <UiInput
                v-model="description"
                placeholder="简述该团队的核心定位、业务场景与工作范围"
                block
              />
            </div>
          </div>
        </div>

        <!-- 2. Orchestrator Brain Config -->
        <div class="p-4 rounded-[var(--radius,6px)] border border-[var(--color-border)] bg-[var(--color-surface-muted)] space-y-4">
          <div class="flex items-center gap-2 font-medium text-xs text-[var(--color-text)]">
            <svg viewBox="0 0 16 16" width="14" height="14" fill="none" stroke="currentColor" stroke-width="1.8" class="text-accent">
              <circle cx="8" cy="8" r="3" />
              <path d="M8 1v2M8 13v2M1 8h2M13 8h2" />
            </svg>
            <span class="font-semibold">中控调度大脑设定</span>
          </div>

          <div class="flex flex-col gap-1.5">
            <div class="flex items-center justify-between">
              <label class="text-xs font-medium text-[var(--color-text)]">
                调度大脑模型 <span class="text-red-500">*</span>
                <span class="text-[var(--color-text-muted)] font-normal text-2xs">（中枢将基于该模型流式分析总目标并自主调用 Function Calls 派发专家角色）</span>
              </label>
              <button
                type="button"
                class="text-2xs text-accent hover:underline"
                @click="router.push({ name: 'settings-models' })"
              >
                配置大模型服务 →
              </button>
            </div>

            <!-- Model Selection Dropdown -->
            <div v-if="chatModelOptions.length > 0">
              <UiSelectMenu
                v-model="orchestratorModel"
                :options="chatModelOptions"
                placeholder="请选择中枢调度大脑模型…"
                block
              />
            </div>
            <div
              v-else
              class="p-3 text-xs text-amber-600 bg-amber-500/10 border border-amber-500/30 rounded flex items-center justify-between"
            >
              <span>尚未配置可用的 Chat 文本模型，请先前往平台设置添加模型服务。</span>
              <UiButton
                type="button"
                variant="secondary"
                size="sm"
                class="text-xs"
                @click="router.push({ name: 'settings-models' })"
              >
                前往配置
              </UiButton>
            </div>
          </div>

          <div class="flex flex-col gap-1.5">
            <label class="text-xs font-medium text-[var(--color-text)]">
              中控角色提示词与团队规范
              <span class="text-[var(--color-text-muted)] font-normal text-2xs">（作为中控大脑系统上下文隐式注入，统领团队成员协作规范）</span>
            </label>
            <UiTextarea
              v-model="orchestratorPrompt"
              :rows="5"
              placeholder="输入中控大脑在调度时应遵循的角色设定、代码规范、架构约束与推进准则…"
              class="w-full font-mono text-2xs"
            />
          </div>
        </div>

        <!-- 3. Team Member Roles (Add / Remove Pattern) -->
        <div class="space-y-3">
          <div class="flex items-center justify-between">
            <div class="flex items-center gap-2">
              <label class="text-xs font-semibold text-[var(--color-text-muted)] tracking-wide">
                团队专家成员
              </label>
              <span class="text-2xs px-1.5 py-0.5 rounded bg-surface-muted text-foreground font-mono border border-border">
                已加入 {{ selectedRoles.length }} 位专家
              </span>
            </div>
            <button
              type="button"
              class="text-2xs text-accent hover:underline"
              @click="router.push({ name: 'settings-roles' })"
            >
              管理全局专家角色库 →
            </button>
          </div>

          <!-- Add Member Selector Dropdown -->
          <div v-if="availableRoleOptions.length > 1" class="max-w-md">
            <UiSelectMenu
              :model-value="0"
              :options="availableRoleOptions"
              placeholder="+ 从角色管理中选择专家加入团队…"
              size="sm"
              block
              @update:model-value="(val) => { if (val) addRole(Number(val)) }"
            />
          </div>
          <div
            v-else-if="rolesStore.roles && rolesStore.roles.length > 0 && selectedRoles.length === rolesStore.roles.length"
            class="text-2xs text-[var(--color-text-muted)]"
          >
            ✓ 角色库中的所有可用专家角色已全部加入本团队
          </div>

          <!-- Active Team Members Card List (With Remove action) -->
          <div v-if="selectedRoles.length > 0" class="grid grid-cols-1 gap-2.5">
            <div
              v-for="r in selectedRoles"
              :key="r.id"
              class="p-3 rounded-[var(--radius,6px)] bg-[var(--color-surface-muted)] border border-[var(--color-border)] flex items-start justify-between gap-3 transition-colors hover:border-[var(--color-border-strong)]"
            >
              <div class="flex items-start gap-2.5 min-w-0 flex-1">
                <div
                  class="w-6 h-6 rounded flex items-center justify-center text-xs font-semibold shrink-0 mt-0.5"
                  :style="{
                    backgroundColor: r.color ? `${r.color}20` : 'rgba(59, 130, 246, 0.15)',
                    color: r.color || '#3b82f6',
                  }"
                >
                  {{ r.name.slice(0, 1) }}
                </div>
                <div class="space-y-1 min-w-0 flex-1">
                  <div class="flex items-center gap-2 flex-wrap">
                    <span class="text-xs font-semibold text-[var(--color-text)]">
                      {{ r.name }}
                    </span>
                    <BackendTag :backend="r.backend" />
                    <span class="text-2xs px-1.5 py-0.5 rounded bg-[var(--color-surface)] text-[var(--color-text-muted)] font-mono border border-[var(--color-border)]">
                      模型: {{ r.model || '跟随默认' }}
                    </span>
                  </div>
                  <p v-if="r.description" class="text-2xs text-[var(--color-text-muted)] line-clamp-1 leading-relaxed">
                    {{ r.description }}
                  </p>
                </div>
              </div>

              <button
                type="button"
                class="text-2xs text-red-500 hover:text-red-600 px-2 py-1 rounded shrink-0 transition-colors border border-transparent hover:border-red-500/20 hover:bg-red-500/10"
                title="从团队移除该专家"
                @click="removeRole(r.id)"
              >
                移除
              </button>
            </div>
          </div>

          <!-- Empty State: No members added yet -->
          <div
            v-else
            class="p-6 text-center rounded-[var(--radius,6px)] border border-dashed border-[var(--color-border)] bg-[var(--color-surface)]/50 space-y-2"
          >
            <div class="text-xs font-medium text-[var(--color-text)]">团队暂未添加任何专家成员</div>
            <p class="text-2xs text-[var(--color-text-muted)] max-w-sm mx-auto">
              团队协同需要专家分工协作。请使用上方下拉菜单添加需要参与该团队的专家角色。
            </p>
            <div v-if="unselectedRoles.length > 0" class="flex items-center justify-center gap-2 flex-wrap pt-2">
              <span class="text-2xs text-[var(--color-text-muted)]">快捷添加候选：</span>
              <button
                v-for="cand in unselectedRoles.slice(0, 4)"
                :key="cand.id"
                type="button"
                class="px-2 py-1 text-2xs rounded bg-[var(--color-surface-muted)] border border-[var(--color-border)] hover:border-[var(--color-primary)] text-[var(--color-text)] transition-colors inline-flex items-center gap-1"
                @click="addRole(cand.id)"
              >
                <span>+</span>
                <span>{{ cand.name }}</span>
              </button>
            </div>
          </div>
        </div>

        <!-- Footer Actions -->
        <div class="flex items-center justify-end gap-3 pt-4 border-t border-[var(--color-border)]">
          <UiButton
            type="button"
            variant="secondary"
            class="text-xs px-4 py-2"
            @click="handleCancel"
          >
            取消
          </UiButton>
          <UiButton
            type="submit"
            variant="primary"
            :loading="saving"
            class="text-xs px-5 py-2 font-medium"
          >
            {{ isEdit ? '保存团队群组' : '创建团队群组' }}
          </UiButton>
        </div>
      </form>
    </div>
  </div>
</template>
