<script setup lang="ts">
// Model management section: model services (OpenAI-compatible endpoints) and
// their models, with manual add, list-from-service, per-model edit/disable, and
// a connectivity test. API keys are write-only: the form shows whether a key is
// stored, and an empty key field on save keeps the stored key untouched.
import { onMounted, reactive, ref } from 'vue'
import { useModelsStore } from '../../stores/models'
import type { ModelDto, RemoteModel, ServiceDto } from '../../api/models'
import { UiButton, UiInput, UiBadge, UiAlert } from '../../shared/ui'

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
  <section class="models max-w-4xl space-y-6" aria-label="模型设置">
    <div class="head flex items-center justify-between">
      <h1 class="text-xl font-bold text-slate-800 dark:text-slate-100">模型管理</h1>
      <UiButton variant="primary" @click="openCreate">新增服务</UiButton>
    </div>

    <!-- 新增/编辑服务表单 -->
    <form
      v-if="showServiceForm"
      class="service-form p-4 rounded bg-slate-50 dark:bg-slate-800/40 border border-slate-200 dark:border-slate-700 space-y-3"
      @submit.prevent="submitService"
    >
      <h2 class="text-sm font-semibold text-slate-800 dark:text-slate-200">
        {{ editingServiceId === null ? '新增服务' : '编辑服务' }}
      </h2>
      <div class="space-y-2">
        <UiInput v-model="serviceForm.name" placeholder="服务名称" required block />
        <UiInput v-model="serviceForm.baseUrl" placeholder="API 地址（如 https://api.openai.com/v1）" required block />
        <UiInput
          v-model="serviceForm.apiKey"
          type="password"
          placeholder="API Key（留空保留已保存的 Key，填写新值则替换）"
          block
        />
        <label class="inline-flex items-center gap-2 text-sm text-slate-700 dark:text-slate-300 cursor-pointer select-none">
          <input v-model="serviceForm.enabled" type="checkbox" class="w-4 h-4 rounded text-blue-600 focus:ring-blue-500 accent-blue-600" />
          <span>启用此服务</span>
        </label>
      </div>
      <div class="actions flex items-center justify-end gap-2 pt-2 border-t border-slate-200 dark:border-slate-700">
        <UiButton variant="secondary" type="button" @click="showServiceForm = false">取消</UiButton>
        <UiButton variant="primary" type="submit">保存</UiButton>
      </div>
    </form>

    <UiAlert v-if="models.error" variant="error" class="error">
      {{ models.error }}
    </UiAlert>

    <!-- 服务卡片列表 -->
    <article
      v-for="service in models.services"
      :key="service.id"
      class="service p-5 rounded bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 shadow-sm space-y-4"
      :data-service="service.id"
    >
      <header class="flex items-center justify-between gap-4 flex-wrap">
        <div class="flex items-center gap-3">
          <strong class="text-base text-slate-800 dark:text-slate-100 font-semibold">{{ service.name }}</strong>
          <span class="muted text-xs text-slate-400 dark:text-slate-500 font-mono">{{ service.baseUrl }}</span>
          <UiBadge v-if="!service.enabled" variant="default" size="sm">已停用</UiBadge>
        </div>
        <div class="service-actions flex items-center gap-2">
          <UiButton size="sm" variant="secondary" :data-fetch-models="service.id" @click="fetchCandidates(service)">
            获取模型
          </UiButton>
          <UiButton size="sm" variant="secondary" @click="openEdit(service)">编辑</UiButton>
          <UiButton size="sm" variant="danger" @click="removeService(service)">删除</UiButton>
        </div>
      </header>

      <!-- 远端候选模型挑选列表 -->
      <div
        v-if="candidates[service.id]?.length"
        class="candidates p-3 rounded bg-blue-50/50 dark:bg-blue-950/20 border border-blue-200 dark:border-blue-900/40 space-y-2.5"
      >
        <div class="text-xs font-semibold text-blue-800 dark:text-blue-300">勾选要添加至本服务的模型：</div>
        <div class="flex flex-wrap gap-2.5 max-h-48 overflow-y-auto">
          <label
            v-for="candidate in candidates[service.id]"
            :key="candidate.id"
            :data-candidate="candidate.id"
            class="inline-flex items-center gap-1.5 px-2 py-1 rounded bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 text-xs text-slate-700 dark:text-slate-200 cursor-pointer select-none"
          >
            <input
              type="checkbox"
              class="w-3.5 h-3.5 rounded text-blue-600 accent-blue-600"
              :checked="isSelected(service.id, candidate.id)"
              @change="selected[service.id][candidate.id] = ($event.target as HTMLInputElement).checked"
            />
            <span class="truncate max-w-[200px]">{{ candidate.name || candidate.id }}</span>
          </label>
        </div>
        <div class="flex justify-end pt-1">
          <UiButton size="sm" variant="primary" :data-add-selected="service.id" @click="addSelected(service)">
            添加选中
          </UiButton>
        </div>
      </div>

      <!-- 服务下属模型表格 -->
      <div class="overflow-x-auto rounded border border-slate-100 dark:border-slate-800">
        <table class="model-table w-full text-left text-sm border-collapse">
          <thead class="bg-slate-50 dark:bg-slate-800/60 text-slate-500 dark:text-slate-400 text-xs font-medium border-b border-slate-100 dark:border-slate-800">
            <tr>
              <th class="py-2.5 px-3">模型 ID</th>
              <th class="py-2.5 px-3">名称</th>
              <th class="py-2.5 px-3">能力</th>
              <th class="py-2.5 px-3">上下文</th>
              <th class="py-2.5 px-3">输出</th>
              <th class="py-2.5 px-3">状态</th>
              <th class="py-2.5 px-3 text-right">操作</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-slate-100 dark:divide-slate-800 text-xs text-slate-700 dark:text-slate-300">
            <tr v-for="model in service.models" :key="model.id" :data-model="model.id" class="hover:bg-slate-50/50 dark:hover:bg-slate-800/30 transition-colors">
              <td class="py-2 px-3 font-mono">{{ model.modelId }}</td>
              <td class="py-2 px-3 font-medium text-slate-800 dark:text-slate-200">{{ model.name }}</td>
              <td class="py-2 px-3 text-slate-400">
                <span v-if="model.supportsText" class="mr-1">文本</span>
                <span v-if="model.supportsImage" class="mr-1">图片</span>
                <span v-if="model.supportsTools">工具</span>
              </td>
              <td class="py-2 px-3 font-mono">{{ model.contextTokens || '-' }}</td>
              <td class="py-2 px-3 font-mono">{{ model.outputTokens || '-' }}</td>
              <td class="py-2 px-3">
                <UiBadge v-if="model.usable" variant="success" size="sm">可用</UiBadge>
                <UiBadge v-else variant="default" size="sm">不可用</UiBadge>
              </td>
              <td class="py-2 px-3 text-right">
                <div class="model-actions inline-flex items-center justify-end gap-1.5">
                  <UiButton size="sm" variant="ghost" :data-test-model="model.id" @click="runTest(service, model)">测试</UiButton>
                  <UiButton size="sm" variant="ghost" @click="openModelEdit(model)">编辑</UiButton>
                  <UiButton size="sm" variant="ghost" class="text-red-600 hover:text-red-700 dark:text-red-400" @click="removeModel(model)">删除</UiButton>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <!-- 测试结果反馈 -->
      <div v-for="model in service.models" :key="`t-${model.id}`">
        <p
          v-show="testMessage[model.id]"
          class="test-message text-xs px-2.5 py-1.5 rounded bg-slate-100 dark:bg-slate-800 text-slate-600 dark:text-slate-300 font-mono"
        >
          {{ testMessage[model.id] }}
        </p>
      </div>
    </article>

    <!-- 编辑单个模型弹层/表单 -->
    <form
      v-if="editingModelId !== null"
      class="model-form p-4 rounded bg-slate-50 dark:bg-slate-800/40 border border-slate-200 dark:border-slate-700 space-y-3"
      @submit.prevent="submitModel"
    >
      <h2 class="text-sm font-semibold text-slate-800 dark:text-slate-200">编辑模型</h2>
      <div class="space-y-2">
        <UiInput v-model="modelForm.modelId" placeholder="模型 ID" required block />
        <UiInput v-model="modelForm.name" placeholder="模型名称" required block />
        <div class="flex flex-wrap gap-4 pt-1 text-sm text-slate-700 dark:text-slate-300">
          <label class="inline-flex items-center gap-1.5 cursor-pointer">
            <input v-model="modelForm.supportsText" type="checkbox" class="w-4 h-4 rounded text-blue-600 accent-blue-600" />
            <span>文本输入</span>
          </label>
          <label class="inline-flex items-center gap-1.5 cursor-pointer">
            <input v-model="modelForm.supportsImage" type="checkbox" class="w-4 h-4 rounded text-blue-600 accent-blue-600" />
            <span>图片输入</span>
          </label>
          <label class="inline-flex items-center gap-1.5 cursor-pointer">
            <input v-model="modelForm.supportsTools" type="checkbox" class="w-4 h-4 rounded text-blue-600 accent-blue-600" />
            <span>工具调用</span>
          </label>
        </div>
        <div class="grid grid-cols-2 gap-3 pt-1">
          <label class="flex flex-col gap-1 text-xs text-slate-600 dark:text-slate-400">
            <span>上下文 Token 限制</span>
            <UiInput v-model.number="modelForm.contextTokens" type="number" min="0" block />
          </label>
          <label class="flex flex-col gap-1 text-xs text-slate-600 dark:text-slate-400">
            <span>输出 Token 限制</span>
            <UiInput v-model.number="modelForm.outputTokens" type="number" min="0" block />
          </label>
        </div>
        <label class="inline-flex items-center gap-2 text-sm text-slate-700 dark:text-slate-300 cursor-pointer select-none pt-1">
          <input v-model="modelForm.enabled" type="checkbox" class="w-4 h-4 rounded text-blue-600 accent-blue-600" />
          <span>启用此模型</span>
        </label>
      </div>
      <div class="actions flex items-center justify-end gap-2 pt-2 border-t border-slate-200 dark:border-slate-700">
        <UiButton variant="secondary" type="button" @click="editingModelId = null">取消</UiButton>
        <UiButton variant="primary" type="submit">保存</UiButton>
      </div>
    </form>
  </section>
</template>
