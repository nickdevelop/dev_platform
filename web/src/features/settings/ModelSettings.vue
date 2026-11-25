<script setup lang="ts">
// Model management: model services (OpenAI-compatible endpoints) and their
// models — manual add, list-from-service, per-model edit/disable, connectivity
// test.
//
// Element Plus owns the surface. The shape stays "one card per service, a model
// table inside it", because a service is the unit the user reasons about (the
// endpoint, its key, its model roster, its 获取模型 action) and AgentSettingsView
// already established that a card may embed a small `el-table`. What changed:
// the two inline forms (service, model) became `EntityFormDialog`s — they used
// to swap the page body for a form, and a validation failure only appeared after
// submit because there was nothing to validate against; the hand-built `<table>`
// became `el-table`; `UiConfirmDialog` became `ElMessageBox`; and the per-model
// test feedback is a single line under the table rather than one row per model
// with `v-show` on it.
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { storeToRefs } from 'pinia'
import { ElMessage, ElMessageBox, type FormInstance, type FormRules } from 'element-plus'
import { Box, Edit, Plus, Search, Delete } from '@element-plus/icons-vue'
import { useModelsStore } from '../../stores/models'
import type { ModelDto, ModelInput, RemoteModel, ServiceDto } from '../../api/models'
import EntityFormDialog from '../../shared/components/EntityFormDialog.vue'
import ListEmptyState from '../../shared/components/ListEmptyState.vue'
import { formatTokens } from '../../shared/format'

const models = useModelsStore()
const { services, error: modelsError } = storeToRefs(models)

// A failed load / save / delete leaves the last good data on screen, so the
// store's `error` is the page's only failure channel and it reports once.
watch(modelsError, (message) => {
  if (message) ElMessage.error(message)
})

// ------------------------------------------------------- service form -------
const serviceVisible = ref(false)
const serviceFormRef = ref<FormInstance>()
const savingService = ref(false)
const editingServiceId = ref<number | null>(null)
const serviceForm = reactive({ name: '', baseUrl: '', apiKey: '', enabled: true })

const serviceIsEditing = computed(() => editingServiceId.value !== null)

const serviceRules: FormRules = {
  name: [{ required: true, message: '请输入服务名称', trigger: 'blur' }],
  baseUrl: [
    { required: true, message: '请输入 API 地址', trigger: 'blur' },
    {
      validator: (_rule, value: string, callback) => {
        if (!value || /^https?:\/\//.test(value.trim())) callback()
        else callback(new Error('地址需以 http:// 或 https:// 开头'))
      },
      trigger: 'blur',
    },
  ],
}

function openCreate() {
  editingServiceId.value = null
  Object.assign(serviceForm, { name: '', baseUrl: '', apiKey: '', enabled: true })
  serviceVisible.value = true
}

function openEdit(service: ServiceDto) {
  editingServiceId.value = service.id
  Object.assign(serviceForm, {
    name: service.name,
    baseUrl: service.baseUrl,
    apiKey: '',
    enabled: service.enabled,
  })
  serviceVisible.value = true
}

async function submitService() {
  if (savingService.value) return
  const valid = await serviceFormRef.value?.validate().catch(() => false)
  if (!valid) return

  savingService.value = true
  const id = editingServiceId.value
  // The store keeps the last good state and sets `error` on failure (surfaced by
  // the watcher above), so the dialog only closes on a real success.
  const ok = await models.saveService({ ...serviceForm }, id === null ? undefined : id)
  savingService.value = false
  if (ok) {
    serviceVisible.value = false
    ElMessage.success(id === null ? '已新增模型服务' : '已保存服务修改')
  }
}

async function removeService(service: ServiceDto) {
  const modelCount = service.models?.length || 0
  try {
    await ElMessageBox.confirm(
      `该服务下的 ${modelCount} 个模型配置将一并删除，且无法恢复。`,
      `确定要删除服务「${service.name}」吗？`,
      { type: 'warning', confirmButtonText: '删除', cancelButtonText: '取消', confirmButtonType: 'danger' },
    )
  } catch {
    return
  }
  const ok = await models.removeService(service.id)
  if (ok) ElMessage.success(`已删除服务「${service.name}」`)
}

// ------------------------------------------------- remote candidate picking --
const candidates = reactive<Record<number, RemoteModel[]>>({})
const selected = reactive<Record<number, string[]>>({})
// One service's 获取模型 may be in flight while another's add is running, so the
// busy flags are keyed by service id rather than a single boolean.
const fetching = reactive<Record<number, boolean>>({})
const adding = reactive<Record<number, boolean>>({})

async function fetchCandidates(service: ServiceDto) {
  if (fetching[service.id]) return
  fetching[service.id] = true
  try {
    candidates[service.id] = await models.fetchRemote({
      baseUrl: service.baseUrl,
      secretRef: service.hasKey ? service.secretRef : undefined,
    })
    selected[service.id] = []
    if (candidates[service.id].length === 0) ElMessage.info('该服务没有返回可添加的模型')
  } finally {
    fetching[service.id] = false
  }
}

async function addSelected(service: ServiceDto) {
  const picks = (selected[service.id] ?? []).map(
    (id) => (candidates[service.id] ?? []).find((c) => c.id === id),
  )
  if (picks.length === 0 || adding[service.id]) return

  adding[service.id] = true
  let added = 0
  for (const candidate of picks) {
    if (!candidate) continue
    const ok = await models.addModelTo(service.id, {
      modelId: candidate.id,
      name: candidate.name && candidate.name !== '' ? candidate.name : candidate.id,
      supportsText: true,
      supportsImage: false,
      supportsTools: false,
      contextTokens: 0,
      outputTokens: 0,
      enabled: true,
    })
    if (ok) added += 1
  }
  adding[service.id] = false
  candidates[service.id] = []
  selected[service.id] = []
  if (added > 0) ElMessage.success(`已添加 ${added} 个模型`)
}

// ---------------------------------------------------------- model form ------
const modelVisible = ref(false)
const modelFormRef = ref<FormInstance>()
const savingModel = ref(false)
const editingModelId = ref<number | null>(null)
const modelForm = reactive<ModelInput & { modelId: string }>({
  modelId: '',
  name: '',
  supportsText: true,
  supportsImage: false,
  supportsTools: false,
  contextTokens: 0,
  outputTokens: 0,
  enabled: true,
})

const modelRules: FormRules = {
  modelId: [{ required: true, message: '请输入模型 ID', trigger: 'blur' }],
  name: [{ required: true, message: '请输入友好名称', trigger: 'blur' }],
}

/** The three capability flags as one checkbox-group value, so the form has a
    single model to validate and render instead of three loose booleans. */
const capabilityValues = computed({
  get: () =>
    (['supportsText', 'supportsImage', 'supportsTools'] as const)
      .filter((key) => modelForm[key])
      .map((key) => key),
  set: (next: string[]) => {
    modelForm.supportsText = next.includes('supportsText')
    modelForm.supportsImage = next.includes('supportsImage')
    modelForm.supportsTools = next.includes('supportsTools')
  },
})

const CAPABILITIES = [
  { key: 'supportsText', label: '文本输入' },
  { key: 'supportsImage', label: '图片输入' },
  { key: 'supportsTools', label: '工具调用（Function Call）' },
] as const

function openModelEdit(model: ModelDto) {
  editingModelId.value = model.id
  Object.assign(modelForm, {
    modelId: model.modelId,
    name: model.name,
    supportsText: model.supportsText,
    supportsImage: model.supportsImage,
    supportsTools: model.supportsTools,
    contextTokens: model.contextTokens,
    outputTokens: model.outputTokens,
    enabled: model.enabled,
  })
  modelVisible.value = true
}

async function submitModel() {
  if (editingModelId.value === null || savingModel.value) return
  const valid = await modelFormRef.value?.validate().catch(() => false)
  if (!valid) return

  savingModel.value = true
  const ok = await models.saveModel(editingModelId.value, { ...modelForm })
  savingModel.value = false
  if (ok) {
    modelVisible.value = false
    editingModelId.value = null
    ElMessage.success(`已保存模型「${modelForm.name}」`)
  }
}

async function removeModel(model: ModelDto) {
  const ok = await models.removeModel(model.id)
  if (ok) ElMessage.success(`已删除模型「${model.name}」`)
}

// ------------------------------------------------------------ test feedback -
// One verdict line per service — the last test run — rather than one row per
// model with `v-show` on it: the old markup put `models.length` paragraphs in
// the DOM to show one.
const lastTest = reactive<Record<number, { text: string; ok: boolean } | null>>({})
const testing = reactive<Record<number, boolean>>({})

async function runTest(service: ServiceDto, model: ModelDto) {
  if (testing[model.id]) return
  testing[model.id] = true
  try {
    const result = await models.test({
      baseUrl: service.baseUrl,
      secretRef: service.hasKey ? service.secretRef : undefined,
      modelId: model.modelId,
    })
    lastTest[service.id] = result
      ? { ok: true, text: `${model.name}：调用正常（${result.latencyMs}ms）` }
      : { ok: false, text: `${model.name}：${modelsError.value || '测试失败'}` }
  } finally {
    testing[model.id] = false
  }
}

function modelCount(service: ServiceDto): number {
  return service.models?.length || 0
}

onMounted(() => {
  void models.load()
})
</script>

<template>
  <section class="models-settings space-y-4" aria-label="模型设置">
    <header class="settings-page-head settings-page-head-split">
      <div>
        <h1 class="settings-page-title">模型</h1>
        <p class="settings-page-desc">
          配置 OpenAI 兼容端点服务，按服务发现与管理供平台 Chat 与 Agent 调用的模型
        </p>
      </div>
      <!-- 页面级动作只画在页头右侧：模型页是多分区卡，没有一张工具条能兼带所有服务，
           而它又必须一直看得见——空列表不能把入口吃掉。 -->
      <el-button type="primary" :icon="Plus" data-test="btn-create-service" @click="openCreate">
        新增服务
      </el-button>
    </header>

    <!-- No service yet: the canvas has no table to hang an #empty slot on, so the
         shared empty state stands on its own inside a panel. -->
    <section v-if="services.length === 0" class="settings-panel">
      <ListEmptyState
        state="empty"
        :icon="Box"
        title="暂未配置模型服务"
        hint="接入 OpenAI、DeepSeek、Ollama 等模型 API 后，即可一键获取远端模型列表并供 Chat 与 Agent 选用。"
      />
    </section>

    <!-- One panel per service: the endpoint is the unit the user acts on. -->
    <section
      v-for="service in services"
      :key="service.id"
      class="settings-panel"
      :data-service="service.id"
    >
      <header class="settings-panel-head settings-panel-head-split">
        <div class="min-w-0">
          <div class="flex items-center gap-2 flex-wrap">
            <h2 class="settings-panel-title">{{ service.name }}</h2>
            <el-tag v-if="!service.enabled" type="info" size="small" disable-transitions>已停用</el-tag>
          </div>
          <p class="settings-panel-desc">
            <code class="path-code">{{ service.baseUrl }}</code>
            <span class="ml-2 tabular-nums">{{ modelCount(service) }} 个模型</span>
          </p>
        </div>
        <div class="flex items-center gap-2 shrink-0">
          <el-button
            :loading="fetching[service.id]"
            :icon="Search"
            :data-fetch-models="service.id"
            @click="fetchCandidates(service)"
          >
            获取模型
          </el-button>
          <el-button :icon="Edit" :data-test="`btn-edit-service-${service.id}`" @click="openEdit(service)">
            编辑
          </el-button>
          <el-button
            type="danger"
            plain
            :icon="Delete"
            :data-test="`btn-delete-service-${service.id}`"
            @click="removeService(service)"
          >
            删除
          </el-button>
        </div>
      </header>

      <!-- Remote candidates: a checkbox group of model ids, kept in the panel so
           the picks land next to the service they belong to. -->
      <div v-if="candidates[service.id]?.length" class="candidates" data-test="candidate-panel">
        <div class="flex items-center justify-between gap-2 flex-wrap">
          <span class="candidates-title">勾选要添加至本服务的模型</span>
          <span class="tabular-nums text-platform-text-muted">
            {{ candidates[service.id].length }} 个远端模型 · 已选 {{ (selected[service.id] ?? []).length }}
          </span>
        </div>
        <el-checkbox-group v-model="selected[service.id]" class="candidate-grid">
          <el-checkbox
            v-for="candidate in candidates[service.id]"
            :key="candidate.id"
            :value="candidate.id"
            class="candidate-card"
            :data-candidate="candidate.id"
          >
            <code class="path-code truncate max-w-[220px]">{{ candidate.name || candidate.id }}</code>
          </el-checkbox>
        </el-checkbox-group>
        <div class="flex justify-end">
          <el-button
            type="primary"
            :loading="adding[service.id]"
            :data-add-selected="service.id"
            @click="addSelected(service)"
          >
            添加选中
          </el-button>
        </div>
      </div>

      <el-table :data="service.models" row-key="id" data-test="model-table">
        <el-table-column label="模型 ID" min-width="150" show-overflow-tooltip>
          <template #default="{ row }">
            <code class="path-code">{{ row.modelId }}</code>
          </template>
        </el-table-column>

        <el-table-column label="名称" min-width="130" show-overflow-tooltip>
          <template #default="{ row }">
            <span class="font-medium">{{ row.name }}</span>
          </template>
        </el-table-column>

        <el-table-column label="能力" min-width="140">
          <template #default="{ row }">
            <div class="inline-flex items-center gap-1 flex-wrap">
              <el-tag
                v-if="row.supportsText"
                class="entity-chip entity-chip--cap-text"
                size="small"
                disable-transitions
              >
                文本
              </el-tag>
              <el-tag
                v-if="row.supportsImage"
                class="entity-chip entity-chip--cap-image"
                size="small"
                disable-transitions
              >
                图片
              </el-tag>
              <el-tag
                v-if="row.supportsTools"
                class="entity-chip entity-chip--cap-tools"
                size="small"
                disable-transitions
              >
                工具
              </el-tag>
              <span v-if="!row.supportsText && !row.supportsImage && !row.supportsTools" class="text-platform-text-muted">
                未标注
              </span>
            </div>
          </template>
        </el-table-column>

        <el-table-column label="上下文" width="92" align="right">
          <template #default="{ row }">
            <span class="font-mono">{{ row.contextTokens ? formatTokens(row.contextTokens) : '-' }}</span>
          </template>
        </el-table-column>

        <el-table-column label="输出" width="92" align="right">
          <template #default="{ row }">
            <span class="font-mono">{{ row.outputTokens ? formatTokens(row.outputTokens) : '-' }}</span>
          </template>
        </el-table-column>

        <el-table-column label="状态" width="88">
          <template #default="{ row }">
            <el-tag :type="row.usable ? 'success' : 'info'" size="small" disable-transitions>
              {{ row.usable ? '可用' : '不可用' }}
            </el-tag>
          </template>
        </el-table-column>

        <el-table-column label="操作" width="148" align="right" fixed="right">
          <template #default="{ row }">
            <div class="inline-flex items-center gap-1">
              <el-button
                link
                type="primary"
                size="small"
                :loading="testing[row.id]"
                :data-test-model="row.id"
                @click="runTest(service, row as ModelDto)"
              >
                测试
              </el-button>
              <el-button link type="primary" size="small" @click="openModelEdit(row as ModelDto)">编辑</el-button>
              <el-button
                link
                type="danger"
                size="small"
                :data-test="`btn-delete-model-${row.id}`"
                @click="removeModel(row as ModelDto)"
              >
                删除
              </el-button>
            </div>
          </template>
        </el-table-column>

        <template #empty>
          <ListEmptyState
            state="empty"
            :icon="Box"
            title="该服务下还没有模型"
            hint="点击「获取模型」从端点拉取模型清单，勾选后添加。"
          />
        </template>
      </el-table>

      <!-- Last connectivity verdict for this service's table. -->
      <p
        v-if="lastTest[service.id]"
        class="test-line"
        :class="{ 'is-failed': lastTest[service.id]?.ok === false }"
        data-test="model-test-line"
      >
        <span>{{ lastTest[service.id]?.text }}</span>
        <el-button link size="small" @click="lastTest[service.id] = null">忽略</el-button>
      </p>
    </section>

    <!-- Service create / edit -->
    <EntityFormDialog
      v-if="serviceVisible"
      :title="serviceIsEditing ? '编辑服务' : '新增服务'"
      subtitle="支持接入任何兼容 OpenAI Chat Completion 协议的模型接口"
      :identifier="serviceIsEditing ? editingServiceId : null"
      :saving="savingService"
      :width="620"
      save-test="btn-save-service"
      data-test="service-edit-dialog"
      @save="submitService"
      @cancel="serviceVisible = false"
    >
      <el-form
        ref="serviceFormRef"
        :model="serviceForm"
        :rules="serviceRules"
        label-position="top"
        @submit.prevent="submitService"
      >
        <section class="entity-form-section">
          <el-form-item label="服务名称" prop="name">
            <el-input
              v-model="serviceForm.name"
              maxlength="40"
              placeholder="例如：DeepSeek 官方 / Ollama 本地 / SiliconFlow"
              data-test="input-service-name"
            />
          </el-form-item>

          <el-form-item label="API 地址" prop="baseUrl">
            <el-input
              v-model="serviceForm.baseUrl"
              placeholder="API 地址（如 https://api.openai.com/v1）"
              data-test="input-service-base-url"
            />
          </el-form-item>

          <el-form-item label="API Key">
            <div class="w-full space-y-1">
              <el-input
                v-model="serviceForm.apiKey"
                type="password"
                show-password
                placeholder="留空保留已保存的 Key，填写新值则替换"
                data-test="input-service-api-key"
              />
              <p class="entity-field-hint">敏感凭据写优先存储，已保存的密钥不会在界面明文回显。</p>
            </div>
          </el-form-item>

          <el-form-item>
            <div class="entity-switch-row">
              <div class="min-w-0">
                <div class="entity-switch-title">启用此服务</div>
                <p class="entity-switch-desc">停用后该服务下的模型不会出现在 Chat 与 Agent 的可选清单里。</p>
              </div>
              <el-switch v-model="serviceForm.enabled" data-test="switch-service-enabled" />
            </div>
          </el-form-item>
        </section>
      </el-form>
    </EntityFormDialog>

    <!-- Model edit -->
    <EntityFormDialog
      v-if="modelVisible"
      title="编辑模型参数"
      subtitle="调整模型标识、友好名称与输入输出 Token 限制"
      :identifier="editingModelId"
      :saving="savingModel"
      :width="680"
      save-test="btn-save-model"
      data-test="model-edit-dialog"
      @save="submitModel"
      @cancel="modelVisible = false"
    >
      <el-form
        ref="modelFormRef"
        :model="modelForm"
        :rules="modelRules"
        label-position="top"
        @submit.prevent="submitModel"
      >
        <section class="entity-form-section">
          <div class="entity-grid-2">
            <el-form-item label="模型 ID" prop="modelId">
              <el-input
                v-model="modelForm.modelId"
                class="entity-mono-input"
                placeholder="模型 ID（API 实际入参）"
                data-test="input-model-id"
              />
            </el-form-item>
            <el-form-item label="友好名称" prop="name">
              <el-input
                v-model="modelForm.name"
                maxlength="60"
                placeholder="界面展示名称"
                data-test="input-model-name"
              />
            </el-form-item>
          </div>

          <el-form-item label="模型能力支持">
            <el-checkbox-group v-model="capabilityValues" data-test="check-model-capabilities">
              <el-checkbox v-for="cap in CAPABILITIES" :key="cap.key" :value="cap.key">
                {{ cap.label }}
              </el-checkbox>
            </el-checkbox-group>
          </el-form-item>

          <div class="entity-grid-2">
            <el-form-item label="上下文 Token 限制">
              <el-input-number
                v-model="modelForm.contextTokens"
                :min="0"
                :step="1024"
                :controls-position="'right'"
                class="w-full"
                data-test="input-model-context-tokens"
              />
            </el-form-item>
            <el-form-item label="最大输出 Token 限制">
              <el-input-number
                v-model="modelForm.outputTokens"
                :min="0"
                :step="256"
                :controls-position="'right'"
                class="w-full"
                data-test="input-model-output-tokens"
              />
            </el-form-item>
          </div>

          <el-form-item>
            <div class="entity-switch-row">
              <div class="min-w-0">
                <div class="entity-switch-title">启用此模型</div>
                <p class="entity-switch-desc">停用后模型保留配置，但不可被选用。</p>
              </div>
              <el-switch v-model="modelForm.enabled" data-test="switch-model-enabled" />
            </div>
          </el-form-item>
        </section>
      </el-form>
    </EntityFormDialog>
  </section>
</template>

<style scoped>
.path-code {
  padding: 1px 5px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  background: var(--color-surface);
  font-family: var(--font-mono);
  font-size: var(--text-size-11);
  color: var(--color-text);
}

/* 远端候选区：坐在分区卡内部，用一层更浅的面与模型表分开。 */
.candidates {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-bottom: 12px;
  padding: 10px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  background: var(--color-surface);
}

.candidates-title {
  font-size: var(--text-size-12);
  font-weight: 500;
}

.candidate-grid {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  max-height: 190px;
  overflow-y: auto;
}

/* Each candidate is a chip-sized checkbox: the library draws the box and the
   checked state, this only gives the row its hairline edge. */
.candidate-card {
  margin-right: 0;
  padding: 2px 8px 2px 4px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  height: auto;
}

.test-line {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin: 8px 0 0;
  padding: 5px 10px;
  border: 1px solid var(--color-border);
  border-left: 2px solid var(--color-success);
  border-radius: var(--radius);
  background: var(--color-surface);
  font-size: var(--text-size-12);
}

.test-line.is-failed {
  border-left-color: var(--color-danger);
}
</style>
