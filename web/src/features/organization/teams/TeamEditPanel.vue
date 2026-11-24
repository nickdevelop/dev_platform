<script setup lang="ts">
// TeamEditPanel: Full-page in-place workbench panel for creating or editing an AI Team (Group).
// Runs inside the main workbench view without modal dialogs or backdrop overlays.
// Conforms to design-taste-frontend standards:
//   - Clean header with back button, breadcrumb title, and sticky primary actions
//   - Centered max-w-4xl structured form with clear card sections
//   - Real-time management of orchestrator brain models and member staffing
//   - Keyboard shortcut support (Ctrl+S to save, Esc to cancel)
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import type { GroupDetail, Group, CreateGroupPayload, UpdateGroupPayload } from '../../../api/groups'
import type { AgentRole } from '../../../api/roles'
import { useRolesStore } from '../../../stores/roles'
import { useModelsStore } from '../../../stores/models'
import { useGroupsStore } from '../../../stores/groups'
import BackendTag from '../../../shared/components/BackendTag.vue'
import {
  UiButton,
  UiInput,
  UiTextarea,
  UiSelectMenu,
  UiAlert,
  type SelectMenuItem,
} from '../../../shared/ui'

const props = defineProps<{
  team?: GroupDetail | Group | null
}>()

const emit = defineEmits<{
  saved: [teamId: number]
  cancel: []
}>()

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

const isEdit = computed(() => !!props.team)

const defaultOrchestratorPrompt = `你是一个软件开发团队的中控调度大脑。
职责：
1. 深入分析用户的目标需求，制定模块化执行方案。
2. 自主指派团队中的专业角色（如架构师、后端、前端、测试）分工完成代码编写与验证。
3. 协调解决角色之间的接口对齐与冲突，确保整体代码质量。`

onMounted(async () => {
  await Promise.all([
    rolesStore.roles.length === 0 ? rolesStore.loadRoles() : Promise.resolve(),
    modelsStore.services.length === 0 ? modelsStore.load() : Promise.resolve(),
  ])
  initForm()
  window.addEventListener('keydown', onKeyDown)
})

onUnmounted(() => {
  window.removeEventListener('keydown', onKeyDown)
})

function onKeyDown(e: KeyboardEvent) {
  if ((e.ctrlKey || e.metaKey) && e.key === 's') {
    e.preventDefault()
    void handleSave()
  } else if (e.key === 'Escape') {
    e.preventDefault()
    handleCancel()
  }
}

watch(
  () => props.team,
  () => {
    initForm()
  },
  { deep: true },
)

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

watch(chatModelOptions, (opts) => {
  if (!orchestratorModel.value && opts.length > 0) {
    orchestratorModel.value = opts[0].value
  }
})

function initForm() {
  errorMessage.value = ''
  if (props.team) {
    name.value = props.team.name || ''
    description.value = props.team.description || ''
    orchestratorModel.value = props.team.orchestratorModel || ''
    orchestratorPrompt.value = props.team.orchestratorPrompt || defaultOrchestratorPrompt
    selectedRoleIds.value = ('members' in props.team && props.team.members ? props.team.members : []).map((m) => m.roleId)
  } else {
    name.value = ''
    description.value = ''
    orchestratorModel.value = chatModelOptions.value[0]?.value || ''
    orchestratorPrompt.value = defaultOrchestratorPrompt
    selectedRoleIds.value = (rolesStore.roles || []).slice(0, 2).map((r) => r.id)
  }
}

const selectedRoles = computed<AgentRole[]>(() => {
  return selectedRoleIds.value
    .map((id) => rolesStore.byId(id))
    .filter((r): r is AgentRole => !!r)
})

const unselectedRoles = computed<AgentRole[]>(() => {
  return (rolesStore.roles || []).filter((r) => !selectedRoleIds.value.includes(r.id))
})

const availableRoleOptions = computed<SelectMenuItem<number | 0>[]>(() => {
  const items: SelectMenuItem<number | 0>[] = [
    { value: 0, label: '+ 选择专家角色加入团队…' },
  ]
  for (const r of unselectedRoles.value) {
    items.push({
      value: r.id,
      label: `${r.name} (${r.backend || 'LLM'} / ${r.model})`,
      sublabel: r.description || undefined,
    })
  }
  return items
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

async function handleSave() {
  errorMessage.value = ''
  const trimmedName = name.value.trim()
  if (!trimmedName) {
    errorMessage.value = '请输入团队名称。'
    return
  }
  if (!orchestratorModel.value.trim()) {
    errorMessage.value = '请指定中控调度大脑模型。'
    return
  }
  if (selectedRoleIds.value.length === 0) {
    errorMessage.value = '请至少为该团队编制一位专家员工成员。'
    return
  }

  saving.value = true
  try {
    if (isEdit.value && props.team) {
      const updatePayload: UpdateGroupPayload = {
        name: trimmedName,
        description: description.value.trim(),
        orchestratorModel: orchestratorModel.value.trim(),
        orchestratorPrompt: orchestratorPrompt.value.trim(),
      }
      const updated = await groupsStore.updateGroup(props.team.id, updatePayload)
      await groupsStore.updateMembers(props.team.id, selectedRoleIds.value)
      emit('saved', updated.id)
    } else {
      const createPayload: CreateGroupPayload = {
        name: trimmedName,
        description: description.value.trim(),
        orchestratorModel: orchestratorModel.value.trim(),
        orchestratorPrompt: orchestratorPrompt.value.trim(),
        roleIds: selectedRoleIds.value,
      }
      const created = await groupsStore.createGroup(createPayload)
      emit('saved', created.id)
    }
  } catch (err: any) {
    errorMessage.value = err?.message || '保存 AI 团队失败，请稍后重试。'
  } finally {
    saving.value = false
  }
}

function handleCancel() {
  emit('cancel')
}
</script>

<template>
  <div class="h-full flex flex-col min-h-0 bg-[var(--color-bg)] overflow-hidden" data-test="team-edit-panel">
    <!-- Top Action Header: Consistent with Assistant & Employee Edit Views -->
    <header class="px-6 py-4 border-b border-border bg-surface shrink-0">
      <div class="flex items-center justify-between gap-4">
        <div class="flex items-center gap-3 min-w-0">
          <button
            type="button"
            class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded text-xs font-medium text-platform-text-muted hover:text-foreground hover:bg-surface-muted transition-colors border border-border"
            title="返回列表 (Esc)"
            @click="handleCancel"
          >
            <svg viewBox="0 0 16 16" width="12" height="12" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <polyline points="10 12 6 8 10 4" />
            </svg>
            <span>返回列表</span>
          </button>

          <div class="h-4 w-px bg-border"></div>

          <div class="min-w-0">
            <div class="flex items-center gap-2">
              <h1 class="text-base font-semibold text-foreground tracking-tight truncate">
                {{ isEdit ? `编辑 AI 团队 · ${props.team?.name}` : '新建 AI 团队' }}
              </h1>
              <span v-if="isEdit" class="px-1.5 py-0.5 rounded text-3xs font-mono bg-surface-muted text-platform-text-muted border border-border">
                #{{ props.team?.id }}
              </span>
            </div>
            <p class="text-2xs text-platform-text-muted truncate mt-0.5">
              中控大脑调度与专职 AI 员工组成的多 Agent 协同作战单元。
            </p>
          </div>
        </div>

        <div class="flex items-center gap-2 shrink-0">
          <UiButton
            variant="secondary"
            class="text-xs"
            @click="handleCancel"
          >
            取消
          </UiButton>
          <UiButton
            variant="primary"
            class="text-xs"
            :disabled="saving"
            data-test="btn-save-team"
            @click="handleSave"
          >
            <svg v-if="saving" class="animate-spin -ml-0.5 mr-1.5 h-3.5 w-3.5" viewBox="0 0 24 24" fill="none">
              <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
              <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z"></path>
            </svg>
            <span>{{ saving ? '保存中...' : (isEdit ? '保存修改' : '组建团队') }}</span>
            <span class="ml-1.5 text-3xs opacity-60 font-mono hidden sm:inline">Ctrl+S</span>
          </UiButton>
        </div>
      </div>
    </header>

    <!-- Scrollable Form Body -->
    <div class="flex-1 min-h-0 overflow-auto p-6">
      <div class="max-w-4xl mx-auto space-y-6">
        <!-- Error Alert -->
        <UiAlert v-if="errorMessage" variant="error">
          {{ errorMessage }}
        </UiAlert>

        <!-- Card 1: Basic Information -->
        <div class="border border-border rounded-lg bg-surface p-5 space-y-4">
          <div class="border-b border-border/60 pb-3">
            <h2 class="text-xs font-semibold text-foreground tracking-wide uppercase">01 基础信息与团队定位</h2>
            <p class="text-2xs text-platform-text-muted mt-0.5">设定协作团队的名称、定位及专职攻坚方向。</p>
          </div>

          <div class="space-y-3 max-w-2xl">
            <div class="space-y-1.5">
              <label class="text-xs font-medium text-foreground">
                团队名称 <span class="text-red-500">*</span>
              </label>
              <UiInput
                v-model="name"
                placeholder="例如：系统重构攻坚组、全栈协同研发团队、UI体验专项组…"
                maxlength="60"
                data-test="input-team-name"
              />
            </div>

            <div class="space-y-1.5">
              <label class="text-xs font-medium text-foreground">团队定位与职责描述</label>
              <UiInput
                v-model="description"
                placeholder="简要阐述该团队的业务领域、协作场景与攻坚目标"
                data-test="input-team-description"
              />
            </div>
          </div>
        </div>

        <!-- Card 2: Orchestrator Brain Config -->
        <div class="border border-border rounded-lg bg-surface p-5 space-y-4">
          <div class="border-b border-border/60 pb-3">
            <h2 class="text-xs font-semibold text-foreground tracking-wide uppercase">02 中控调度大脑 (Orchestrator)</h2>
            <p class="text-2xs text-platform-text-muted mt-0.5">负责统筹拆解任务、动态分派角色、解决接口冲突的核心中控 LLM。</p>
          </div>

          <!-- Orchestrator Model -->
          <div class="space-y-1.5 max-w-lg">
            <label class="text-xs font-medium text-foreground">
              调度大脑模型 <span class="text-red-500">*</span>
            </label>
            <UiSelectMenu
              v-model="orchestratorModel"
              :options="chatModelOptions"
              placeholder="请选择中控调度大脑模型"
              data-test="select-orchestrator-model"
            />
            <p class="text-3xs text-platform-text-muted mt-1">推荐选择推理能力与上下文逻辑分析更强的高阶旗舰大模型。</p>
          </div>

          <!-- Orchestrator Prompt -->
          <div class="space-y-1.5 pt-1">
            <div class="flex items-center justify-between">
              <label class="text-xs font-medium text-foreground">
                中控调度规范与系统指令 <span class="text-red-500">*</span>
              </label>
              <button
                type="button"
                class="text-3xs text-accent hover:underline"
                @click="orchestratorPrompt = defaultOrchestratorPrompt"
              >
                重置为默认调度规范
              </button>
            </div>
            <UiTextarea
              v-model="orchestratorPrompt"
              placeholder="输入中控大脑的调度准则、任务分发规则与跨角色协调指令…"
              :rows="5"
              class="font-mono text-xs leading-relaxed"
              data-test="textarea-orchestrator-prompt"
            />
          </div>
        </div>

        <!-- Card 3: Team Staffing (Expert Employees) -->
        <div class="border border-border rounded-lg bg-surface p-5 space-y-4">
          <div class="flex items-center justify-between border-b border-border/60 pb-3">
            <div>
              <h2 class="text-xs font-semibold text-foreground tracking-wide uppercase">03 编制专家员工 (Staffing)</h2>
              <p class="text-2xs text-platform-text-muted mt-0.5">将已注册的 AI 专职数字员工编入作战队伍，由中控大脑统筹调度。</p>
            </div>
            <span class="px-2 py-0.5 rounded-full text-2xs font-mono font-medium bg-surface-muted text-platform-text-muted border border-border">
              已编制 {{ selectedRoles.length }} 名员工
            </span>
          </div>

          <!-- Add Member Selector -->
          <div v-if="unselectedRoles.length > 0" class="max-w-md space-y-1.5">
            <label class="text-xs font-medium text-foreground">添加专家员工进团队</label>
            <UiSelectMenu
              :model-value="0"
              :options="availableRoleOptions"
              placeholder="+ 从员工库选择专家加入…"
              data-test="select-add-member"
              @update:model-value="addRole(Number($event))"
            />
          </div>

          <!-- Staffed Members List -->
          <div class="space-y-2">
            <label class="text-xs font-medium text-foreground">已编制团队成员名单 <span class="text-red-500">*</span></label>

            <div v-if="selectedRoles.length > 0" class="grid grid-cols-1 md:grid-cols-2 gap-3" data-test="staffed-members-grid">
              <div
                v-for="role in selectedRoles"
                :key="role.id"
                class="flex items-center justify-between p-3 rounded-lg border border-border bg-surface-muted/40 hover:bg-surface-muted/70 transition-colors group"
              >
                <div class="flex items-center gap-3 min-w-0">
                  <span
                    class="w-9 h-9 rounded-md flex items-center justify-center text-white text-xs font-semibold shrink-0 shadow-2xs"
                    :style="{ backgroundColor: role.color || '#3b82f6' }"
                  >
                    {{ role.name.slice(0, 1) }}
                  </span>
                  <div class="min-w-0">
                    <div class="flex items-center gap-1.5">
                      <span class="text-xs font-medium text-foreground truncate">{{ role.name }}</span>
                      <BackendTag :backend="role.backend || 'llm'" />
                    </div>
                    <div class="text-2xs text-platform-text-muted truncate mt-0.5 font-mono">
                      {{ role.model }}
                    </div>
                  </div>
                </div>

                <!-- Remove Button -->
                <button
                  type="button"
                  class="p-1 rounded text-platform-text-muted hover:text-red-500 hover:bg-red-500/10 transition-colors shrink-0 ml-2"
                  title="移出团队"
                  @click="removeRole(role.id)"
                >
                  <svg viewBox="0 0 16 16" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
                    <line x1="4" y1="4" x2="12" y2="12" />
                    <line x1="12" y1="4" x2="4" y2="12" />
                  </svg>
                </button>
              </div>
            </div>

            <!-- Empty Staffing Notice -->
            <div v-else class="p-6 rounded-lg border border-dashed border-border bg-surface-muted/20 text-center space-y-2">
              <p class="text-xs font-medium text-foreground">暂无编制员工</p>
              <p class="text-2xs text-platform-text-muted">
                AI 团队至少需要 1 名专职 AI 员工以协同开展工作。请点击上方下拉框添加。
              </p>
            </div>
          </div>
        </div>

        <!-- Bottom Action Bar -->
        <div class="flex items-center justify-between pt-2 pb-6 border-t border-border">
          <span class="text-2xs text-platform-text-muted">
            已就绪，保存后可立即在任务中心发起团队协同会话。
          </span>
          <div class="flex items-center gap-2">
            <UiButton
              variant="secondary"
              class="text-xs"
              @click="handleCancel"
            >
              取消
            </UiButton>
            <UiButton
              variant="primary"
              class="text-xs"
              :disabled="saving"
              @click="handleSave"
            >
              {{ saving ? '保存中...' : (isEdit ? '保存修改' : '组建团队') }}
            </UiButton>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
