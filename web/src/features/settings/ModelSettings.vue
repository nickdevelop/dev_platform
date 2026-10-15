<script setup lang="ts">
// Model management section: model services (OpenAI-compatible endpoints) and
// their models, with manual add, list-from-service, per-model edit/disable, and
// a connectivity test. API keys are write-only: the form shows whether a key is
// stored and clears it when the field is left empty on save.
import { onMounted, reactive, ref } from 'vue'
import { useModelsStore } from '../../stores/models'
import type { ModelDto, RemoteModel, ServiceDto } from '../../api/models'

const models = useModelsStore()

const showServiceForm = ref(false)
const serviceForm = reactive({ name: '', baseUrl: '', apiKey: '', enabled: true })
const editingServiceId = ref<number | null>(null)

const candidates = reactive<Record<number, RemoteModel[]>>({})
const selected = reactive<Record<number, Record<string, boolean>>>({})
const testMessage = reactive<Record<number, string>>({})

const editingModelId = ref<number | null>(null)
const modelForm = reactive({
  modelId: '',
  name: '',
  supportsText: true,
  supportsImage: false,
  supportsTools: false,
  contextTokens: 0,
  outputTokens: 0,
  enabled: true,
})

onMounted(() => {
  void models.load()
})

function openCreate() {
  editingServiceId.value = null
  Object.assign(serviceForm, { name: '', baseUrl: '', apiKey: '', enabled: true })
  showServiceForm.value = true
}

function openEdit(service: ServiceDto) {
  editingServiceId.value = service.id
  Object.assign(serviceForm, {
    name: service.name,
    baseUrl: service.baseUrl,
    apiKey: '',
    enabled: service.enabled,
  })
  showServiceForm.value = true
}

async function submitService() {
  const id = editingServiceId.value
  const ok = await models.saveService(
    { name: serviceForm.name, baseUrl: serviceForm.baseUrl, apiKey: serviceForm.apiKey, enabled: serviceForm.enabled },
    id === null ? undefined : id,
  )
  if (ok) showServiceForm.value = false
}

async function removeService(service: ServiceDto) {
  if (!window.confirm(`删除服务「${service.name}」及其全部模型？`)) return
  await models.removeService(service.id)
}

async function fetchCandidates(service: ServiceDto) {
  candidates[service.id] = await models.fetchRemote({
    baseUrl: service.baseUrl,
    secretRef: service.hasKey ? service.secretRef : undefined,
  })
  selected[service.id] = {}
}

function isSelected(serviceId: number, modelId: string): boolean {
  return selected[serviceId]?.[modelId] === true
}

async function addSelected(service: ServiceDto) {
  const picks = candidates[service.id] ?? []
  for (const candidate of picks) {
    if (!isSelected(service.id, candidate.id)) continue
    await models.addModelTo(service.id, {
      modelId: candidate.id,
      name: candidate.name && candidate.name !== '' ? candidate.name : candidate.id,
      supportsText: true,
      supportsImage: false,
      supportsTools: false,
      contextTokens: 0,
      outputTokens: 0,
      enabled: true,
    })
  }
  candidates[service.id] = []
  selected[service.id] = {}
}

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
}

async function submitModel() {
  if (editingModelId.value === null) return
  const ok = await models.saveModel(editingModelId.value, { ...modelForm })
  if (ok) editingModelId.value = null
}

async function removeModel(model: ModelDto) {
  await models.removeModel(model.id)
}

async function runTest(service: ServiceDto, model: ModelDto) {
  testMessage[model.id] = '测试中…'
  const result = await models.test({
    baseUrl: service.baseUrl,
    secretRef: service.hasKey ? service.secretRef : undefined,
    modelId: model.modelId,
  })
  testMessage[model.id] = result ? `调用正常（${result.latencyMs}ms）` : models.error || '测试失败'
}
</script>

<template>
  <section class="models" aria-label="模型设置">
    <div class="head">
      <h1>模型</h1>
      <button type="button" class="primary" @click="openCreate">新增服务</button>
    </div>

    <form v-if="showServiceForm" class="service-form" @submit.prevent="submitService">
      <input v-model="serviceForm.name" placeholder="服务名称" required />
      <input v-model="serviceForm.baseUrl" placeholder="API 地址（如 https://api.openai.com/v1）" required />
      <input
        v-model="serviceForm.apiKey"
        type="password"
        placeholder="API Key（留空将清除已保存的 Key）"
      />
      <label class="inline"><input v-model="serviceForm.enabled" type="checkbox" /> 启用</label>
      <div class="actions">
        <button type="submit" class="primary">保存</button>
        <button type="button" @click="showServiceForm = false">取消</button>
      </div>
    </form>

    <p v-if="models.error" class="error" role="alert">{{ models.error }}</p>

    <article v-for="service in models.services" :key="service.id" class="service" :data-service="service.id">
      <header>
        <div>
          <strong>{{ service.name }}</strong>
          <span class="muted">{{ service.baseUrl }}</span>
          <span v-if="!service.enabled" class="badge">已停用</span>
        </div>
        <div class="service-actions">
          <button type="button" :data-fetch-models="service.id" @click="fetchCandidates(service)">获取模型</button>
          <button type="button" @click="openEdit(service)">编辑</button>
          <button type="button" class="danger" @click="removeService(service)">删除</button>
        </div>
      </header>

      <div v-if="candidates[service.id]?.length" class="candidates">
        <label v-for="candidate in candidates[service.id]" :key="candidate.id" :data-candidate="candidate.id" class="inline">
          <input
            type="checkbox"
            :checked="isSelected(service.id, candidate.id)"
            @change="selected[service.id][candidate.id] = ($event.target as HTMLInputElement).checked"
          />
          {{ candidate.name || candidate.id }}
        </label>
        <button type="button" class="primary" :data-add-selected="service.id" @click="addSelected(service)">
          添加选中
        </button>
      </div>

      <table class="model-table">
        <thead>
          <tr>
            <th>模型 ID</th>
            <th>名称</th>
            <th>能力</th>
            <th>上下文</th>
            <th>输出</th>
            <th>状态</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="model in service.models" :key="model.id" :data-model="model.id">
            <td>{{ model.modelId }}</td>
            <td>{{ model.name }}</td>
            <td class="muted">
              <span v-if="model.supportsText">文本</span>
              <span v-if="model.supportsImage">图片</span>
              <span v-if="model.supportsTools">工具</span>
            </td>
            <td>{{ model.contextTokens }}</td>
            <td>{{ model.outputTokens }}</td>
            <td>
              <span v-if="model.usable">可用</span>
              <span v-else class="muted">不可用</span>
            </td>
            <td class="model-actions">
              <button type="button" :data-test-model="model.id" @click="runTest(service, model)">测试</button>
              <button type="button" @click="openModelEdit(model)">编辑</button>
              <button type="button" class="danger" @click="removeModel(model)">删除</button>
            </td>
          </tr>
        </tbody>
      </table>
      <p
        v-for="model in service.models"
        :key="`t-${model.id}`"
        v-show="testMessage[model.id]"
        class="test-message"
      >
        {{ testMessage[model.id] }}
      </p>
    </article>

    <form v-if="editingModelId !== null" class="model-form" @submit.prevent="submitModel">
      <h2>编辑模型</h2>
      <input v-model="modelForm.modelId" placeholder="模型 ID" required />
      <input v-model="modelForm.name" placeholder="模型名称" required />
      <label class="inline"><input v-model="modelForm.supportsText" type="checkbox" /> 文本输入</label>
      <label class="inline"><input v-model="modelForm.supportsImage" type="checkbox" /> 图片输入</label>
      <label class="inline"><input v-model="modelForm.supportsTools" type="checkbox" /> 工具调用</label>
      <label class="inline">上下文 <input v-model.number="modelForm.contextTokens" type="number" min="0" /></label>
      <label class="inline">输出 <input v-model.number="modelForm.outputTokens" type="number" min="0" /></label>
      <label class="inline"><input v-model="modelForm.enabled" type="checkbox" /> 启用</label>
      <div class="actions">
        <button type="submit" class="primary">保存</button>
        <button type="button" @click="editingModelId = null">取消</button>
      </div>
    </form>
  </section>
</template>

<style scoped>
.models h1 {
  font-size: 1.25rem;
  margin: 0;
}

.head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: calc(var(--space-unit) * 2);
}

.service {
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  background: var(--color-surface);
  padding: calc(var(--space-unit) * 2);
  margin-bottom: calc(var(--space-unit) * 2);
}

.service header {
  display: flex;
  justify-content: space-between;
  gap: calc(var(--space-unit) * 2);
}

.muted {
  color: var(--color-text-muted);
  margin-left: calc(var(--space-unit) * 1.5);
  font-size: 0.857rem;
}

.badge {
  margin-left: calc(var(--space-unit) * 1.5);
  padding: 0 var(--space-unit);
  border: 1px solid var(--color-border-strong);
  border-radius: var(--radius);
  font-size: 0.857rem;
}

.service-actions,
.model-actions,
.actions {
  display: flex;
  gap: var(--space-unit);
}

button {
  padding: calc(var(--space-unit) * 0.6) calc(var(--space-unit) * 1.5);
  border: 1px solid var(--color-border-strong);
  border-radius: var(--radius);
  background: var(--color-surface);
  color: var(--color-text);
  cursor: pointer;
  font: inherit;
}

button.primary {
  background: var(--color-primary);
  border-color: var(--color-primary);
  color: var(--color-on-primary);
}

button.danger {
  color: var(--color-danger);
  border-color: var(--color-danger-border);
}

.service-form,
.model-form {
  display: flex;
  flex-direction: column;
  gap: var(--space-unit);
  padding: calc(var(--space-unit) * 2);
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  margin-bottom: calc(var(--space-unit) * 2);
  background: var(--color-surface-muted);
}

.service-form input,
.model-form input {
  height: var(--control-height);
  padding: 0 calc(var(--space-unit) * 1.5);
  border: 1px solid var(--color-border-strong);
  border-radius: var(--radius);
  background: var(--color-surface);
  color: var(--color-text);
  font: inherit;
}

.inline {
  display: inline-flex;
  align-items: center;
  gap: var(--space-unit);
}

.candidates {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: calc(var(--space-unit) * 1.5);
  margin: calc(var(--space-unit) * 1.5) 0;
  padding: calc(var(--space-unit) * 1.5);
  background: var(--color-surface-muted);
  border-radius: var(--radius);
}

.model-table {
  width: 100%;
  border-collapse: collapse;
  margin-top: var(--space-unit);
}

.model-table th,
.model-table td {
  text-align: left;
  padding: calc(var(--space-unit) * 0.8) var(--space-unit);
  border-bottom: 1px solid var(--color-border);
}

.error {
  color: var(--color-danger);
}

.test-message {
  color: var(--color-text-muted);
  font-size: 0.857rem;
}
</style>
