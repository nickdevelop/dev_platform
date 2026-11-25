<script setup lang="ts">
// TeamEditDialog: create or edit one AI team (a group).
//
// Was a full-page workbench panel; now a form inside the shared
// `EntityFormDialog` shell over the list.
//
// The staffing step is the reason this page exists: experts are chosen in a
// second-level picker (`el-checkbox-group` laid out as a compact card grid, with
// its own search) and the selection is echoed back into the form as removable
// member chips, so the roster is always visible while you edit the rest of the
// team.
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { ElMessage, type FormInstance, type FormRules } from 'element-plus'
import { Plus, Search } from '@element-plus/icons-vue'
import type { GroupDetail, Group, CreateGroupPayload, UpdateGroupPayload } from '../../../api/groups'
import type { AgentRole } from '../../../api/roles'
import { useRolesStore } from '../../../stores/roles'
import { useModelsStore } from '../../../stores/models'
import { useGroupsStore } from '../../../stores/groups'
import BackendTag from '../../../shared/components/BackendTag.vue'
import EntityFormDialog from '../../../shared/components/EntityFormDialog.vue'

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

const formRef = ref<FormInstance>()
const saving = ref(false)

const form = reactive({
  name: '',
  description: '',
  orchestratorModel: '',
  orchestratorPrompt: '',
  roleIds: [] as number[],
})

// Second-level staffing picker.
const pickerVisible = ref(false)
const pickerKeyword = ref('')
const pickedIds = ref<number[]>([])

const isEdit = computed(() => !!props.team)

const defaultOrchestratorPrompt = `你是一个软件开发团队的中控调度大脑。
职责：
1. 深入分析用户的目标需求，制定模块化执行方案。
2. 自主指派团队中的专业角色（如架构师、后端、前端、测试）分工完成代码编写与验证。
3. 协调解决角色之间的接口对齐与冲突，确保整体代码质量。`

const rules = computed<FormRules>(() => ({
  name: [{ required: true, message: '请输入团队名称', trigger: 'blur' }],
  orchestratorModel: [
    { required: true, message: '请指定中控调度大脑模型', trigger: 'change' },
  ],
  roleIds: [
    {
      validator: (_rule, _value, callback) => {
        if (form.roleIds.length === 0) {
          callback(new Error('请至少为该团队编制一位专家员工成员'))
          return
        }
        callback()
      },
      trigger: 'change',
    },
  ],
}))

const chatModelOptions = computed(() => {
  const items: { value: string; label: string; service: string }[] = []
  for (const svc of modelsStore.services || []) {
    if (!svc.enabled) continue
    for (const m of svc.models || []) {
      if (m.usable && m.supportsText) {
        items.push({
          value: m.modelId,
          label: m.name ? `${m.name} (${m.modelId})` : m.modelId,
          service: svc.name,
        })
      }
    }
  }
  return items
})

const selectedRoles = computed<AgentRole[]>(() =>
  form.roleIds.map((id) => rolesStore.byId(id)).filter((r): r is AgentRole => !!r),
)

const pickerCandidates = computed(() => {
  const q = pickerKeyword.value.trim().toLowerCase()
  const list = rolesStore.roles || []
  if (!q) return list
  return list.filter(
    (r) =>
      r.name.toLowerCase().includes(q) ||
      (r.description && r.description.toLowerCase().includes(q)) ||
      r.model.toLowerCase().includes(q),
  )
})

onMounted(async () => {
  await Promise.all([
    rolesStore.roles.length === 0 ? rolesStore.loadRoles() : Promise.resolve(),
    modelsStore.services.length === 0 ? modelsStore.load() : Promise.resolve(),
  ])
  initForm()
})

watch(
  () => props.team,
  () => initForm(),
  { deep: true },
)

watch(chatModelOptions, (opts) => {
  if (!form.orchestratorModel && opts.length > 0) form.orchestratorModel = opts[0].value
})

function initForm() {
  formRef.value?.clearValidate()
  if (props.team) {
    form.name = props.team.name || ''
    form.description = props.team.description || ''
    form.orchestratorModel = props.team.orchestratorModel || ''
    form.orchestratorPrompt = props.team.orchestratorPrompt || defaultOrchestratorPrompt
    form.roleIds =
      'members' in props.team && props.team.members ? props.team.members.map((m) => m.roleId) : []
    return
  }
  form.name = ''
  form.description = ''
  form.orchestratorModel = chatModelOptions.value[0]?.value || ''
  form.orchestratorPrompt = defaultOrchestratorPrompt
  form.roleIds = (rolesStore.roles || []).slice(0, 2).map((r) => r.id)
}

function openPicker() {
  pickedIds.value = [...form.roleIds]
  pickerKeyword.value = ''
  pickerVisible.value = true
}

function confirmPicker() {
  // Order follows the roster, not the click order, so the member list stays
  // stable between edits.
  const chosen = new Set(pickedIds.value)
  form.roleIds = (rolesStore.roles || []).filter((r) => chosen.has(r.id)).map((r) => r.id)
  pickerVisible.value = false
  void formRef.value?.validateField('roleIds').catch(() => undefined)
}

function removeRole(roleId: number) {
  form.roleIds = form.roleIds.filter((id) => id !== roleId)
  void formRef.value?.validateField('roleIds').catch(() => undefined)
}

function roleById(id: number): AgentRole | undefined {
  return rolesStore.byId(id)
}

async function handleSave() {
  if (saving.value) return
  const valid = await formRef.value?.validate().catch(() => false)
  if (!valid) return

  saving.value = true
  const trimmedName = form.name.trim()
  try {
    if (isEdit.value && props.team) {
      const updatePayload: UpdateGroupPayload = {
        name: trimmedName,
        description: form.description.trim(),
        orchestratorModel: form.orchestratorModel.trim(),
        orchestratorPrompt: form.orchestratorPrompt.trim(),
      }
      const updated = await groupsStore.updateGroup(props.team.id, updatePayload)
      await groupsStore.updateMembers(props.team.id, form.roleIds)
      ElMessage.success(`已保存团队「${trimmedName}」`)
      emit('saved', updated.id)
    } else {
      const createPayload: CreateGroupPayload = {
        name: trimmedName,
        description: form.description.trim(),
        orchestratorModel: form.orchestratorModel.trim(),
        orchestratorPrompt: form.orchestratorPrompt.trim(),
        roleIds: form.roleIds,
      }
      const created = await groupsStore.createGroup(createPayload)
      ElMessage.success(`已组建团队「${trimmedName}」`)
      emit('saved', created.id)
    }
  } catch (err: any) {
    ElMessage.error(err?.message || '保存 AI 团队失败，请稍后重试。')
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <EntityFormDialog
    :title="isEdit ? `编辑 AI 团队 · ${props.team?.name}` : '新建 AI 团队'"
    subtitle="中控大脑调度与专职 AI 员工组成的多 Agent 协同作战单元。"
    :identifier="isEdit ? props.team?.id : null"
    :saving="saving"
    save-test="btn-save-team"
    width="920"
    :data-test="`team-edit-dialog-${isEdit ? 'edit' : 'create'}`"
    @save="handleSave"
    @cancel="emit('cancel')"
  >
    <el-form
      ref="formRef"
      :model="form"
      :rules="rules"
      label-position="top"
      @submit.prevent="handleSave"
    >
      <section class="entity-form-section">
        <header class="entity-section-head">
          <h3 class="entity-section-title"><span class="entity-section-num font-mono">01</span>基础信息与团队定位</h3>
          <p class="entity-section-desc">设定协作团队的名称、定位及专职攻坚方向。</p>
        </header>

        <el-form-item label="团队名称" prop="name">
          <el-input
            v-model="form.name"
            placeholder="例如：系统重构攻坚组、全栈协同研发团队、UI体验专项组…"
            maxlength="60"
            data-test="input-team-name"
          />
        </el-form-item>

        <el-form-item label="团队定位与职责描述">
          <el-input
            v-model="form.description"
            placeholder="简要阐述该团队的业务领域、协作场景与攻坚目标"
            data-test="input-team-description"
          />
        </el-form-item>
      </section>

      <section class="entity-form-section">
        <header class="entity-section-head">
          <h3 class="entity-section-title"><span class="entity-section-num font-mono">02</span>中控调度大脑（Orchestrator）</h3>
          <p class="entity-section-desc">负责统筹拆解任务、动态分派角色、解决接口冲突的核心中控 LLM。</p>
        </header>

        <el-form-item label="调度大脑模型" prop="orchestratorModel">
          <div class="w-full max-w-md space-y-1">
            <el-select
              v-model="form.orchestratorModel"
              filterable
              placeholder="请选择中控调度大脑模型"
              data-test="select-orchestrator-model"
            >
              <el-option v-for="m in chatModelOptions" :key="m.value" :label="m.label" :value="m.value">
                <span>{{ m.label }}</span>
                <span class="entity-option-sub">{{ m.service }}</span>
              </el-option>
            </el-select>
            <p class="entity-field-hint">推荐选择推理能力与上下文逻辑分析更强的高阶旗舰大模型。</p>
          </div>
        </el-form-item>

        <el-form-item prop="orchestratorPrompt">
          <template #label>
            <span class="flex items-center gap-3">
              中控调度规范与系统指令
              <el-button link type="primary" size="small" @click="form.orchestratorPrompt = defaultOrchestratorPrompt">
                重置为默认调度规范
              </el-button>
            </span>
          </template>
          <el-input
            v-model="form.orchestratorPrompt"
            type="textarea"
            :rows="6"
            resize="vertical"
            class="entity-mono-input"
            placeholder="输入中控大脑的调度准则、任务分发规则与跨角色协调指令…"
            data-test="textarea-orchestrator-prompt"
          />
        </el-form-item>
      </section>

      <section class="entity-form-section">
        <header class="entity-section-head entity-section-head-split">
          <div>
            <h3 class="entity-section-title"><span class="entity-section-num font-mono">03</span>编制专家员工（Staffing）</h3>
            <p class="entity-section-desc">将已注册的 AI 专职数字员工编入作战队伍，由中控大脑统筹调度。</p>
          </div>
          <span class="entity-id-chip font-mono">已编制 {{ form.roleIds.length }} 名</span>
        </header>

        <el-form-item prop="roleIds">
          <div class="w-full space-y-2">
            <div class="flex items-center gap-2">
              <el-button :icon="Plus" data-test="select-add-member" @click="openPicker">
                选择专家员工
              </el-button>
              <span v-if="selectedRoles.length" class="entity-field-hint">
                点击成员卡片右上角可移出团队。
              </span>
            </div>

            <div v-if="selectedRoles.length" class="member-grid" data-test="staffed-members-grid">
              <div v-for="role in selectedRoles" :key="role.id" class="member-card">
                <span class="member-monogram" :style="{ background: role.color || 'var(--color-primary)' }">
                  {{ role.name.slice(0, 1) }}
                </span>
                <div class="min-w-0 flex-1">
                  <div class="flex items-center gap-1.5">
                    <span class="text-xs font-medium text-foreground truncate">{{ role.name }}</span>
                    <BackendTag :backend="role.backend || 'llm'" />
                  </div>
                  <div class="font-mono text-2xs text-platform-text-muted truncate mt-0.5">{{ role.model }}</div>
                </div>
                <button
                  type="button"
                  class="member-remove"
                  title="移出团队"
                  :aria-label="`将 ${role.name} 移出团队`"
                  @click="removeRole(role.id)"
                >
                  <el-icon><Close /></el-icon>
                </button>
              </div>
            </div>

            <p v-else class="entity-field-hint">
              AI 团队至少需要 1 名专职 AI 员工以协同开展工作，请先从员工库选择专家。
            </p>
          </div>
        </el-form-item>
      </section>
    </el-form>

    <!-- Second-level staffing picker: a card grid of checkboxes over the form. -->
    <el-dialog
      v-model="pickerVisible"
      title="选择编制专家员工"
      width="760"
      append-to-body
      :close-on-click-modal="false"
      data-test="staff-picker"
    >
      <div class="picker-toolbar">
        <el-input
          v-model="pickerKeyword"
          :prefix-icon="Search"
          placeholder="搜索员工姓名、岗位或模型…"
          clearable
          class="flex-1"
        />
        <span class="entity-field-hint">已勾选 {{ pickedIds.length }} 名</span>
      </div>

      <el-checkbox-group v-model="pickedIds" class="picker-grid">
        <el-checkbox
          v-for="r in pickerCandidates"
          :key="r.id"
          :value="r.id"
          class="picker-card"
          :data-test="`picker-role-${r.id}`"
        >
          <div class="picker-card-body">
            <span class="member-monogram" :style="{ background: r.color || 'var(--color-primary)' }">
              {{ r.name.slice(0, 1) }}
            </span>
            <div class="min-w-0">
              <div class="flex items-center gap-1.5">
                <span class="text-xs font-medium text-foreground truncate">{{ r.name }}</span>
                <BackendTag :backend="r.backend || 'llm'" />
              </div>
              <div class="font-mono text-2xs text-platform-text-muted truncate">{{ r.model }}</div>
              <p v-if="r.description" class="picker-card-desc truncate">{{ r.description }}</p>
            </div>
          </div>
        </el-checkbox>
      </el-checkbox-group>

      <p v-if="pickerCandidates.length === 0" class="entity-field-hint py-6 text-center">
        没有匹配的员工，换个关键词试试。
      </p>

      <template #footer>
        <div class="entity-dialog-foot">
          <span class="entity-shortcut-hint">编制变更会在保存后生效</span>
          <div class="entity-dialog-actions">
            <el-button @click="pickerVisible = false">取消</el-button>
            <el-button type="primary" @click="confirmPicker">确认</el-button>
          </div>
        </div>
      </template>
    </el-dialog>
  </EntityFormDialog>
</template>

<style scoped>
/* Staffing chrome: the member roster echoed into the form, and the picker grid.
   Compact rows rather than big cards, and nothing reaches into the library. */
.member-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px;
}

.member-card {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 10px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  background: var(--color-surface-muted);
}

.member-monogram {
  width: 26px;
  height: 26px;
  flex: none;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: var(--radius);
  font-size: var(--text-size-12);
  font-weight: 600;
  color: #fff;
}

.member-remove {
  flex: none;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 20px;
  height: 20px;
  border-radius: var(--radius);
  color: var(--color-text-muted);
  transition:
    color 150ms cubic-bezier(0.2, 0, 0, 1),
    background-color 150ms cubic-bezier(0.2, 0, 0, 1);
}

.member-remove:hover {
  color: var(--color-danger, #dc2626);
  background: color-mix(in srgb, #dc2626 10%, transparent);
}

.picker-toolbar {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 12px;
}

.picker-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px;
  max-height: 380px;
  overflow-y: auto;
}

.picker-card {
  height: auto;
  margin-right: 0;
  padding: 8px 10px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  background: var(--color-surface);
  align-items: flex-start;
}

.picker-card-body {
  display: flex;
  align-items: flex-start;
  gap: 10px;
}

.picker-card-desc {
  margin: 0;
  font-size: var(--text-size-11);
  color: var(--color-text-muted);
}

@media (max-width: 900px) {
  .member-grid,
  .picker-grid {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
