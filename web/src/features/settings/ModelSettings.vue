<script setup lang="ts">
// Model management section: model services (OpenAI-compatible endpoints) and
// their models, with manual add, list-from-service, per-model edit/disable, and
// connectivity test.
// Redesigned with design-taste-frontend app-screens guidelines (framed-sheet,
// flat surfaces, zero resting shadows, tabular figures for tokens and latency,
// sentence-case 12px table headers, pale status pills).
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
  <section class="models-settings w-full space-y-6" aria-label="模型设置">
    <!-- Header row -->
    <header class="flex items-center justify-between pb-4 border-b border-slate-200 dark:border-slate-800 gap-4 flex-wrap">
      <div>
        <h1 class="text-[22px] font-semibold text-slate-900 dark:text-slate-100 tracking-tight">模型管理</h1>
        <p class="text-xs text-slate-500 dark:text-slate-400 mt-1">
          配置 OpenAI 兼容端点服务，按服务发现与管理供平台 Chat 与 Agent 调用的模型。
        </p>
      </div>
      <UiButton variant="primary" size="sm" @click="openCreate">
        新增服务
      </UiButton>
    </header>

    <!-- Service creation / edition form -->
    <form
      v-if="showServiceForm"
      class="service-form p-5 rounded-lg bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 space-y-4"
      @submit.prevent="submitService"
    >
      <div class="flex items-center justify-between border-b border-slate-100 dark:border-slate-800 pb-2.5">
        <h2 class="text-sm font-semibold text-slate-800 dark:text-slate-200">
          {{ editingServiceId === null ? '新增服务' : '编辑服务' }}
        </h2>
        <span class="text-[11px] text-slate-400">支持接入任何兼容 OpenAI Chat Completion 协议的模型接口</span>
      </div>

      <div class="space-y-3">
        <label class="block space-y-1">
          <span class="text-xs font-medium text-slate-700 dark:text-slate-300">服务名称</span>
          <UiInput v-model="serviceForm.name" placeholder="例如：DeepSeek 官方 / Ollama 本地 / SiliconFlow" required block />
        </label>

        <label class="block space-y-1">
          <span class="text-xs font-medium text-slate-700 dark:text-slate-300">API 地址</span>
          <UiInput v-model="serviceForm.baseUrl" placeholder="API 地址（如 https://api.openai.com/v1）" required block />
        </label>

        <label class="block space-y-1">
          <span class="text-xs font-medium text-slate-700 dark:text-slate-300">API Key</span>
          <UiInput
            v-model="serviceForm.apiKey"
            type="password"
            placeholder="API Key（留空保留已保存的 Key，填写新值则替换）"
            block
          />
          <span class="text-[11px] text-slate-400 block pt-0.5">敏感凭据采用写优先存储，已保存的密钥不会在界面明文回显。</span>
        </label>

        <label class="inline-flex items-center gap-2 text-xs text-slate-700 dark:text-slate-300 cursor-pointer select-none pt-1">
          <input v-model="serviceForm.enabled" type="checkbox" class="w-4 h-4 rounded text-blue-600 focus:ring-blue-500 accent-blue-600" />
          <span>启用此服务</span>
        </label>
      </div>

      <div class="actions flex items-center justify-end gap-2 pt-3 border-t border-slate-100 dark:border-slate-800">
        <UiButton variant="secondary" size="sm" type="button" @click="showServiceForm = false">取消</UiButton>
        <UiButton variant="primary" size="sm" type="submit">保存</UiButton>
      </div>
    </form>

    <UiAlert v-if="models.error" variant="error" class="error">
      {{ models.error }}
    </UiAlert>

    <!-- Services empty state -->
    <div
      v-if="models.services.length === 0 && !showServiceForm"
      class="p-12 text-center rounded-lg bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 space-y-3"
    >
      <div class="text-sm font-medium text-slate-700 dark:text-slate-300">暂未配置模型服务</div>
      <p class="text-xs text-slate-400 dark:text-slate-500 max-w-md mx-auto">
        点击右上角「新增服务」接入 OpenAI、DeepSeek、Ollama 等模型 API，接入后即可一键获取远端模型列表。
      </p>
      <UiButton variant="primary" size="sm" @click="openCreate">
        立即新增服务
      </UiButton>
    </div>

    <!-- Services list cards -->
    <article
      v-for="service in models.services"
      :key="service.id"
      class="service p-5 rounded-lg bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 space-y-4"
      :data-service="service.id"
    >
      <header class="flex items-center justify-between gap-4 flex-wrap pb-3 border-b border-slate-100 dark:border-slate-800">
        <div class="flex items-center gap-2.5 min-w-0">
          <strong class="text-sm text-slate-900 dark:text-slate-100 font-semibold">{{ service.name }}</strong>
          <code class="text-[11px] text-slate-500 dark:text-slate-400 px-1.5 py-0.5 rounded bg-slate-100 dark:bg-slate-800 border border-slate-200 dark:border-slate-700 truncate max-w-[280px]">
            {{ service.baseUrl }}
          </code>
          <span class="text-[11px] text-slate-400 tabular-nums">
            {{ service.models?.length || 0 }} 个模型
          </span>
          <UiBadge v-if="!service.enabled" variant="default" size="sm">已停用</UiBadge>
        </div>
        <div class="service-actions flex items-center gap-1.5">
          <UiButton size="sm" variant="secondary" :data-fetch-models="service.id" @click="fetchCandidates(service)">
            获取模型
          </UiButton>
          <UiButton size="sm" variant="secondary" @click="openEdit(service)">编辑</UiButton>
          <UiButton size="sm" variant="danger" @click="removeService(service)">删除</UiButton>
        </div>
      </header>

      <!-- Remote candidates selection panel -->
      <div
        v-if="candidates[service.id]?.length"
        class="candidates p-3.5 rounded-md bg-slate-50 dark:bg-slate-800/40 border border-blue-200 dark:border-blue-900/40 space-y-2.5"
      >
        <div class="flex items-center justify-between">
          <div class="text-xs font-semibold text-slate-800 dark:text-slate-200">勾选要添加至本服务的模型：</div>
          <span class="text-[11px] text-slate-400 tabular-nums">{{ candidates[service.id].length }} 个远端模型</span>
        </div>
        <div class="flex flex-wrap gap-2 max-h-48 overflow-y-auto p-1">
          <label
            v-for="candidate in candidates[service.id]"
            :key="candidate.id"
            :data-candidate="candidate.id"
            class="inline-flex items-center gap-1.5 px-2.5 py-1.5 rounded bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-700 text-xs text-slate-700 dark:text-slate-200 cursor-pointer select-none hover:border-blue-400 transition-colors"
          >
            <input
              type="checkbox"
              class="w-3.5 h-3.5 rounded text-blue-600 accent-blue-600"
              :checked="isSelected(service.id, candidate.id)"
              @change="selected[service.id][candidate.id] = ($event.target as HTMLInputElement).checked"
            />
            <code class="truncate max-w-[220px] text-[11px]">{{ candidate.name || candidate.id }}</code>
          </label>
        </div>
        <div class="flex justify-end pt-1">
          <UiButton size="sm" variant="primary" :data-add-selected="service.id" @click="addSelected(service)">
            添加选中
          </UiButton>
        </div>
      </div>

      <!-- Models Table -->
      <div class="overflow-x-auto rounded border border-slate-100 dark:border-slate-800">
        <table class="model-table w-full text-left text-xs border-collapse">
          <thead class="bg-slate-50 dark:bg-slate-800/50 text-slate-500 dark:text-slate-400 text-[11px] font-medium border-b border-slate-100 dark:border-slate-800">
            <tr>
              <th class="py-2.5 px-3">模型 ID</th>
              <th class="py-2.5 px-3">名称</th>
              <th class="py-2.5 px-3">能力</th>
              <th class="py-2.5 px-3 text-right">上下文</th>
              <th class="py-2.5 px-3 text-right">输出</th>
              <th class="py-2.5 px-3">状态</th>
              <th class="py-2.5 px-3 text-right">操作</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-slate-100 dark:divide-slate-800 text-slate-700 dark:text-slate-300">
            <tr
              v-for="model in service.models"
              :key="model.id"
              :data-model="model.id"
              class="hover:bg-slate-50/50 dark:hover:bg-slate-800/30 transition-colors"
            >
              <td class="py-2.5 px-3 text-[11px] text-slate-800 dark:text-slate-200">
                <code>{{ model.modelId }}</code>
              </td>
              <td class="py-2.5 px-3 font-medium text-slate-800 dark:text-slate-200">{{ model.name }}</td>
              <td class="py-2.5 px-3">
                <div class="inline-flex items-center gap-1 text-[10px]">
                  <span v-if="model.supportsText" class="px-1.5 py-0.5 rounded bg-slate-100 dark:bg-slate-800 text-slate-600 dark:text-slate-300">文本</span>
                  <span v-if="model.supportsImage" class="px-1.5 py-0.5 rounded bg-blue-50 dark:bg-blue-950/40 text-blue-600 dark:text-blue-300">图片</span>
                  <span v-if="model.supportsTools" class="px-1.5 py-0.5 rounded bg-amber-50 dark:bg-amber-950/40 text-amber-600 dark:text-amber-300">工具</span>
                </div>
              </td>
              <td class="py-2.5 px-3 text-right tabular-nums text-slate-600 dark:text-slate-400">
                {{ model.contextTokens ? `${model.contextTokens.toLocaleString()}` : '-' }}
              </td>
              <td class="py-2.5 px-3 text-right tabular-nums text-slate-600 dark:text-slate-400">
                {{ model.outputTokens ? `${model.outputTokens.toLocaleString()}` : '-' }}
              </td>
              <td class="py-2.5 px-3">
                <UiBadge v-if="model.usable" variant="success" size="sm">可用</UiBadge>
                <UiBadge v-else variant="default" size="sm">不可用</UiBadge>
              </td>
              <td class="py-2.5 px-3 text-right">
                <div class="model-actions inline-flex items-center justify-end gap-1">
                  <UiButton size="sm" variant="ghost" :data-test-model="model.id" @click="runTest(service, model)">测试</UiButton>
                  <UiButton size="sm" variant="ghost" @click="openModelEdit(model)">编辑</UiButton>
                  <UiButton size="sm" variant="ghost" class="text-red-600 hover:text-red-700 dark:text-red-400" @click="removeModel(model)">删除</UiButton>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <!-- Test results feedback -->
      <div v-for="model in service.models" :key="`t-${model.id}`">
        <p
          v-show="testMessage[model.id]"
          class="test-message text-xs px-3 py-1.5 rounded bg-slate-50 dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700 text-slate-700 dark:text-slate-300 flex items-center justify-between"
        >
          <span>{{ model.name }}：{{ testMessage[model.id] }}</span>
          <button type="button" class="text-slate-400 hover:text-slate-600 dark:hover:text-slate-200 text-xs" @click="testMessage[model.id] = ''">×</button>
        </p>
      </div>
    </article>

    <!-- Model edit form -->
    <form
      v-if="editingModelId !== null"
      class="model-form p-5 rounded-lg bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 space-y-4"
      @submit.prevent="submitModel"
    >
      <div class="flex items-center justify-between border-b border-slate-100 dark:border-slate-800 pb-2.5">
        <h2 class="text-sm font-semibold text-slate-800 dark:text-slate-200">编辑模型参数</h2>
        <span class="text-[11px] text-slate-400">调整模型标识、友好名称与输入输出 Token 限制</span>
      </div>

      <div class="space-y-3">
        <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
          <label class="block space-y-1">
            <span class="text-xs font-medium text-slate-700 dark:text-slate-300">模型 ID</span>
            <UiInput v-model="modelForm.modelId" placeholder="模型 ID（API 实际入参）" required block />
          </label>
          <label class="block space-y-1">
            <span class="text-xs font-medium text-slate-700 dark:text-slate-300">友好名称</span>
            <UiInput v-model="modelForm.name" placeholder="界面展示名称" required block />
          </label>
        </div>

        <div class="space-y-1 pt-1">
          <span class="text-xs font-medium text-slate-700 dark:text-slate-300 block">模型能力支持</span>
          <div class="flex flex-wrap gap-4 text-xs text-slate-700 dark:text-slate-300">
            <label class="inline-flex items-center gap-1.5 cursor-pointer select-none">
              <input v-model="modelForm.supportsText" type="checkbox" class="w-4 h-4 rounded text-blue-600 accent-blue-600" />
              <span>文本输入</span>
            </label>
            <label class="inline-flex items-center gap-1.5 cursor-pointer select-none">
              <input v-model="modelForm.supportsImage" type="checkbox" class="w-4 h-4 rounded text-blue-600 accent-blue-600" />
              <span>图片输入</span>
            </label>
            <label class="inline-flex items-center gap-1.5 cursor-pointer select-none">
              <input v-model="modelForm.supportsTools" type="checkbox" class="w-4 h-4 rounded text-blue-600 accent-blue-600" />
              <span>工具调用 (Function Call)</span>
            </label>
          </div>
        </div>

        <div class="grid grid-cols-2 gap-3 pt-1">
          <label class="flex flex-col gap-1 text-xs text-slate-600 dark:text-slate-400">
            <span class="font-medium text-slate-700 dark:text-slate-300">上下文 Token 限制</span>
            <UiInput v-model.number="modelForm.contextTokens" type="number" min="0" placeholder="0 表示不限制" block />
          </label>
          <label class="flex flex-col gap-1 text-xs text-slate-600 dark:text-slate-400">
            <span class="font-medium text-slate-700 dark:text-slate-300">最大输出 Token 限制</span>
            <UiInput v-model.number="modelForm.outputTokens" type="number" min="0" placeholder="0 表示默认" block />
          </label>
        </div>

        <label class="inline-flex items-center gap-2 text-xs text-slate-700 dark:text-slate-300 cursor-pointer select-none pt-1">
          <input v-model="modelForm.enabled" type="checkbox" class="w-4 h-4 rounded text-blue-600 accent-blue-600" />
          <span>启用此模型</span>
        </label>
      </div>

      <div class="actions flex items-center justify-end gap-2 pt-3 border-t border-slate-100 dark:border-slate-800">
        <UiButton variant="secondary" size="sm" type="button" @click="editingModelId = null">取消</UiButton>
        <UiButton variant="primary" size="sm" type="submit">保存模型</UiButton>
      </div>
    </form>
  </section>
</template>
