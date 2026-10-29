<script setup lang="ts">
// Agent management section: per-backend platform enablement, executable path
// detection, a manual path fallback when auto-detection fails, the
// machine-level service block — the one place in the platform that may start or
// stop a third-party service — persisted model catalogs with manual refresh,
// and each backend's diagnostics. Backend diagnostics live here, not in project
// settings: they describe the machine, not a project.
import { computed, onMounted, reactive, ref } from 'vue'
import { useSettingsStore } from '../../stores/settings'
import { useAgentsStore } from '../../stores/agents'
import { describeApiError } from '../../api/client'
import {
  listAgentBackendModels,
  refreshAgentBackendModels,
  updateAgentBackendModel,
  type AgentServiceStatus,
  type ModelInfo,
} from '../../api/agents'
import type { AgentDetection } from '../../api/settings'
import BackendDiagnostics from '../../shared/components/BackendDiagnostics.vue'
import { rememberModels } from '../../shared/modelCache'
import { UiButton, UiInput, UiSelect, UiBadge, UiAlert } from '../../shared/ui'

const settings = useSettingsStore()
const agents = useAgentsStore()
const pathInputs = reactive<Record<string, string>>({})
const detections = reactive<Record<string, AgentDetection | null>>({})

// Persisted model snapshots, one per backend. A read never touches the agent;
// only the refresh button asks the backend for its current catalog.
const models = reactive<Record<string, ModelInfo[]>>({})
const modelsUpdatedAt = reactive<Record<string, string | null>>({})
const modelsLoading = reactive<Record<string, boolean>>({})
const modelsError = reactive<Record<string, string>>({})

const diagnosticsByBackend = computed(() => {
  const byBackend = new Map<string, (typeof agents.diagnostics)[number]>()
  for (const report of agents.diagnostics) byBackend.set(report.backend, report)
  return byBackend
})
// The backend whose stop is awaiting confirmation, and nothing else. Stopping a
// machine-wide service interrupts every client of it, so the click that opens
// this confirmation is never the click that stops it.
const confirmStop = ref<string | null>(null)

onMounted(async () => {
  void settings.loadAgents()
  await agents.loadBackends()
  // Diagnostics are point-in-time and refetched on every visit; nothing polls.
  void agents.loadDiagnostics()
  // One look per page load, and after that only when the operator presses the
  // button. The service is a third-party process: the platform does not poll it
  // and never acts on its state by itself.
  for (const entry of agents.serviceBackends) {
    void agents.detectService(entry.backend)
  }
  // Read the persisted snapshot only; the agent is not contacted until the
  // operator presses refresh.
  for (const entry of agents.orderedBackends) {
    if (entry.capabilities?.modelSelection) void loadModels(entry.backend, false)
  }
})

function label(backend: string): string {
  return agents.backendLabel(backend)
}

function onPathInput(backend: string, value: string) {
  pathInputs[backend] = value
}

async function savePath(backend: string) {
  const ok = await settings.setAgentPath(backend, pathInputs[backend] ?? '')
  if (ok) pathInputs[backend] = ''
}

async function detect(backend: string) {
  detections[backend] = await settings.detectAgent(backend)
}

async function toggle(backend: string, value: boolean) {
  await settings.setAgentEnabled(backend, value)
  // The workbench reads the platform switch from the agents store; reload it
  // so the new state applies without a page reload.
  await agents.refreshBackends()
}

function onModeChange(backend: string, mode: string) {
  void settings.setAgentMode(backend, mode)
}

function serviceFor(backend: string): AgentServiceStatus | null {
  return agents.services[backend] ?? null
}

function showsService(backend: string): boolean {
  // A backend listed here is one the platform has a service controller for; a
  // server answer of `supported: false` overrides it, because the server is
  // right and the platform must not offer a control that does not apply.
  return agents.backendFor(backend)?.serviceSupported === true && serviceFor(backend)?.supported !== false
}

// stateLabel is the operator's wording for what detection found. "未启动" and
// "无法连接" are deliberately different: the first means nothing is registered,
// the second means something is there that does not answer.
function stateLabel(backend: string): string {
  switch (serviceFor(backend)?.state) {
    case 'running':
      return '运行中'
    case 'unreachable':
      return '无法连接'
    default:
      return '未启动'
  }
}

function supportsModels(backend: string): boolean {
  return agents.backendFor(backend)?.capabilities.modelSelection === true
}

async function loadModels(backend: string, refresh: boolean) {
  modelsLoading[backend] = true
  try {
    const list = refresh
      ? await refreshAgentBackendModels(backend)
      : await listAgentBackendModels(backend)
    models[backend] = list.models
    modelsUpdatedAt[backend] = list.updatedAt
    rememberModels(backend, list.models)
    modelsError[backend] = ''
  } catch (error) {
    modelsError[backend] = describeApiError(error)
  } finally {
    modelsLoading[backend] = false
  }
}

function updatedAtLabel(backend: string): string {
  const raw = modelsUpdatedAt[backend]
  if (!raw) return '尚未同步模型'
  const parsed = new Date(raw)
  if (Number.isNaN(parsed.getTime())) return '上次同步时间未知'
  return `上次同步于 ${parsed.toLocaleString()}`
}

interface EditingModelState {
  backend: string
  id: string
  name: string
  contextLimit: number | null
  supportsImages: boolean
  enabled: boolean
  saving: boolean
  error: string
}
const editingModel = ref<EditingModelState | null>(null)

function openEditModel(backend: string, model: ModelInfo) {
  editingModel.value = {
    backend,
    id: model.id,
    name: model.name || model.id,
    contextLimit: model.contextLimit ?? null,
    supportsImages: model.supportsImages === true,
    enabled: model.enabled !== false,
    saving: false,
    error: '',
  }
}

function cancelEditModel() {
  editingModel.value = null
}

async function submitEditModel() {
  if (!editingModel.value) return
  const cur = editingModel.value
  cur.saving = true
  cur.error = ''
  try {
    const updated = await updateAgentBackendModel(cur.backend, cur.id, {
      name: cur.name.trim(),
      contextLimit: cur.contextLimit && cur.contextLimit > 0 ? Number(cur.contextLimit) : undefined,
      supportsImages: cur.supportsImages,
      enabled: cur.enabled,
    })
    const list = models[cur.backend] ?? []
    const idx = list.findIndex((m) => m.id === cur.id)
    if (idx !== -1) {
      list[idx] = { ...list[idx], ...updated }
      rememberModels(cur.backend, list)
    }
    editingModel.value = null
  } catch (error) {
    cur.error = describeApiError(error)
  } finally {
    if (editingModel.value) editingModel.value.saving = false
  }
}

async function toggleModelEnabled(backend: string, model: ModelInfo, enabled: boolean) {
  const prev = model.enabled !== false
  model.enabled = enabled
  try {
    await updateAgentBackendModel(backend, model.id, { enabled })
    const list = models[backend] ?? []
    rememberModels(backend, list)
  } catch (error) {
    model.enabled = prev
    modelsError[backend] = describeApiError(error)
  }
}

const diagnosticsStatusLabels: Record<string, string> = {
  ok: '正常',
  warning: '警告',
  error: '错误',
  unknown: '未知',
}

function diagnosticsFor(backend: string) {
  const report = diagnosticsByBackend.value.get(backend)
  return report ? [report] : []
}

function diagnosticsStatus(backend: string): string {
  return diagnosticsByBackend.value.get(backend)?.status ?? 'unknown'
}

function diagnosticsStatusLabel(backend: string): string {
  if (!diagnosticsByBackend.value.has(backend)) return '未检测'
  return diagnosticsStatusLabels[diagnosticsStatus(backend)] ?? '未知'
}

function diagnosticsOpen(backend: string): boolean {
  const status = diagnosticsStatus(backend)
  return status === 'warning' || status === 'error'
}

function checkedAt(backend: string): string {
  const raw = serviceFor(backend)?.checkedAt
  if (!raw) return '未检测'
  const parsed = new Date(raw)
  if (Number.isNaN(parsed.getTime())) return '最后检测于'
  return `最后检测于 ${parsed.toLocaleTimeString()}`
}

function detectService(backend: string) {
  void agents.detectService(backend)
}

function startService(backend: string) {
  void agents.startService(backend)
}

// askStop only opens the confirmation. The service may be serving the desktop
// app or another client; saying so is the point of the dialog.
function askStop(backend: string) {
  confirmStop.value = backend
}

function confirmStopNow(backend: string) {
  confirmStop.value = null
  void agents.stopService(backend)
}

function cancelStop() {
  confirmStop.value = null
}
</script>

<template>
  <section class="agents max-w-4xl space-y-6" aria-label="Agent 设置">
    <div>
      <h1 class="text-xl font-bold text-slate-800 dark:text-slate-100 mb-1">Agent 设置</h1>
      <p class="intro text-xs text-slate-500 dark:text-slate-400">启用后可在项目中选择并创建新会话。停用只禁止新建，已有会话继续可用。</p>
    </div>

    <UiAlert v-if="settings.agentsError" variant="error" class="error">
      {{ settings.agentsError }}
    </UiAlert>
    <UiAlert v-if="agents.diagnosticsError" variant="error" class="error">
      {{ agents.diagnosticsError }}
    </UiAlert>

    <article
      v-for="agent in settings.agents"
      :key="agent.backend"
      class="agent-card p-5 rounded bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 shadow-sm space-y-4"
      :data-agent="agent.backend"
    >
      <header class="flex items-center justify-between pb-3 border-b border-slate-100 dark:border-slate-800">
        <strong class="text-base text-slate-800 dark:text-slate-100 font-semibold">{{ label(agent.backend) }}</strong>
        <label class="inline-flex items-center gap-2 text-sm text-slate-700 dark:text-slate-300 cursor-pointer select-none">
          <input
            type="checkbox"
            class="w-4 h-4 rounded text-blue-600 focus:ring-blue-500 accent-blue-600"
            :data-agent-enabled="agent.backend"
            :checked="agent.enabled"
            @change="toggle(agent.backend, ($event.target as HTMLInputElement).checked)"
          />
          <span>启用</span>
        </label>
      </header>

      <div class="path-row flex items-center gap-2 flex-wrap">
        <template v-if="agent.pathSource === 'auto'">
          <span class="path flex-1 min-w-[16rem] text-xs font-mono text-slate-500 dark:text-slate-400 py-1.5 px-2 bg-slate-50 dark:bg-slate-800/40 rounded border border-slate-200 dark:border-slate-700" :data-agent-path-display="agent.backend">
            {{ agent.path }}
          </span>
        </template>
        <template v-else>
          <UiInput
            class="flex-1 min-w-[16rem]"
            :data-agent-path-input="agent.backend"
            :model-value="pathInputs[agent.backend] ?? agent.path"
            placeholder="Agent 可执行文件路径"
            block
            @input="(e: Event) => onPathInput(agent.backend, (e.target as HTMLInputElement).value)"
          />
          <UiButton size="md" variant="secondary" @click="savePath(agent.backend)">保存路径</UiButton>
        </template>
        <UiButton size="md" variant="secondary" :data-detect="agent.backend" @click="detect(agent.backend)">检测</UiButton>
      </div>

      <p class="status flex items-center gap-3 text-xs text-slate-500 dark:text-slate-400">
        <UiBadge :variant="agent.available ? 'success' : 'default'" size="sm" dot>
          {{ agent.available ? '已检测到' : '未检测到' }}
        </UiBadge>
        <span v-if="agent.version" class="font-mono">版本 {{ agent.version }}</span>
      </p>

      <div v-if="agent.modes && agent.modes.length > 1" class="mode-row flex items-center gap-3 text-xs text-slate-600 dark:text-slate-400">
        <label class="inline-flex items-center gap-2">
          <span>传输模式</span>
          <UiSelect
            size="sm"
            :data-agent-mode="agent.backend"
            :model-value="agent.mode || agent.modes[0]"
            @change="(e: Event) => onModeChange(agent.backend, (e.target as HTMLSelectElement).value)"
          >
            <option v-for="mode in agent.modes" :key="mode" :value="mode">{{ mode }}</option>
          </UiSelect>
        </label>
        <span class="mode-hint text-slate-400">修改后需重启平台生效</span>
      </div>

      <UiAlert
        v-if="detections[agent.backend]"
        :variant="detections[agent.backend]?.status === 'ok' ? 'success' : 'error'"
        class="detection"
        :data-detection="agent.backend"
      >
        {{
          detections[agent.backend]?.status === 'ok'
            ? `检测成功：版本 ${detections[agent.backend]?.version}`
            : `检测失败：${detections[agent.backend]?.error}`
        }}
      </UiAlert>

      <!-- 模型列表 -->
      <div v-if="supportsModels(agent.backend)" class="models p-3 rounded bg-slate-50 dark:bg-slate-800/40 border border-slate-200 dark:border-slate-700 space-y-2.5" :data-test="`agent-models-${agent.backend}`">
        <div class="models-head flex items-center justify-between">
          <strong class="text-xs font-semibold text-slate-700 dark:text-slate-300">可用模型</strong>
          <UiButton
            size="sm"
            variant="ghost"
            :data-test="`agent-models-refresh-${agent.backend}`"
            :disabled="modelsLoading[agent.backend] === true"
            :loading="modelsLoading[agent.backend] === true"
            @click="loadModels(agent.backend, true)"
          >
            刷新
          </UiButton>
        </div>
        <p v-if="modelsLoading[agent.backend]" class="models-line text-xs text-slate-400">正在读取…</p>
        <UiAlert v-if="modelsError[agent.backend]" variant="error" class="models-line bad">
          {{ modelsError[agent.backend] }}
        </UiAlert>
        <ul v-if="models[agent.backend]?.length" class="model-list space-y-1.5 max-h-60 overflow-y-auto">
          <li
            v-for="model in models[agent.backend]"
            :key="model.id"
            :data-agent-model="model.id"
            class="model-item flex items-center justify-between p-2 rounded bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 text-xs gap-2"
          >
            <div class="flex items-center gap-2 flex-1 min-w-0">
              <label class="inline-flex items-center cursor-pointer select-none">
                <input
                  type="checkbox"
                  class="w-3.5 h-3.5 rounded text-blue-600 focus:ring-blue-500 accent-blue-600 cursor-pointer"
                  :data-agent-model-toggle="model.id"
                  :checked="model.enabled !== false"
                  @change="toggleModelEnabled(agent.backend, model, ($event.target as HTMLInputElement).checked)"
                />
              </label>
              <UiBadge :variant="model.enabled !== false ? 'success' : 'default'" size="sm">
                {{ model.enabled !== false ? '已启用' : '已停用' }}
              </UiBadge>
              <span class="model-name font-medium text-slate-800 dark:text-slate-200 truncate">{{ model.name || model.id }}</span>
              <span class="model-id text-slate-400 font-mono truncate text-[11px]">{{ model.id }}</span>
              <span v-if="model.contextLimit" class="text-slate-400 text-[11px] whitespace-nowrap">
                {{ Math.round(model.contextLimit / 1000) }}k 上下文
              </span>
              <span v-if="model.supportsImages" class="text-blue-500 text-[11px] whitespace-nowrap">图片</span>
              <span v-if="model.variants && model.variants.length" class="model-variants text-slate-400 text-[11px] truncate">
                变体：{{ model.variants.join(' / ') }}
              </span>
            </div>
            <div class="flex items-center gap-1.5 shrink-0">
              <UiButton
                size="sm"
                variant="ghost"
                :data-agent-model-edit="model.id"
                @click="openEditModel(agent.backend, model)"
              >
                编辑
              </UiButton>
            </div>
          </li>
        </ul>
        <p v-else-if="!modelsLoading[agent.backend]" class="models-line text-xs text-slate-400">
          尚未同步模型，点击刷新读取该 Agent 当前可用的模型。
        </p>
        <p class="models-updated text-[11px] text-slate-400">{{ updatedAtLabel(agent.backend) }}</p>

        <!-- 模型配置编辑表单 -->
        <form
          v-if="editingModel && editingModel.backend === agent.backend"
          class="model-edit-form p-3.5 mt-2 rounded bg-white dark:bg-slate-900 border border-blue-200 dark:border-blue-900/60 shadow-sm space-y-3"
          :data-agent-model-form="editingModel.id"
          @submit.prevent="submitEditModel"
        >
          <div class="flex items-center justify-between">
            <h3 class="text-xs font-semibold text-slate-800 dark:text-slate-200">
              配置模型：<span class="font-mono text-slate-500">{{ editingModel.id }}</span>
            </h3>
            <span class="text-[11px] text-slate-400">可自定义模型显示名称与参数</span>
          </div>

          <UiAlert v-if="editingModel.error" variant="error">
            {{ editingModel.error }}
          </UiAlert>

          <div class="space-y-2">
            <div>
              <label class="block text-[11px] font-medium text-slate-600 dark:text-slate-400 mb-1">显示名称</label>
              <UiInput v-model="editingModel.name" placeholder="模型友好名称" required block />
            </div>

            <div class="grid grid-cols-2 gap-2 pt-1">
              <div>
                <label class="block text-[11px] font-medium text-slate-600 dark:text-slate-400 mb-1">上下文 Token 限制 (留空按默认)</label>
                <UiInput v-model.number="editingModel.contextLimit" type="number" min="0" placeholder="如 128000" block />
              </div>
              <div class="flex flex-col justify-end gap-1.5 pb-1">
                <label class="inline-flex items-center gap-1.5 text-xs text-slate-700 dark:text-slate-300 cursor-pointer select-none">
                  <input v-model="editingModel.supportsImages" type="checkbox" class="w-3.5 h-3.5 rounded text-blue-600 accent-blue-600" />
                  <span>支持图片输入</span>
                </label>
                <label class="inline-flex items-center gap-1.5 text-xs text-slate-700 dark:text-slate-300 cursor-pointer select-none">
                  <input v-model="editingModel.enabled" type="checkbox" class="w-3.5 h-3.5 rounded text-blue-600 accent-blue-600" />
                  <span>启用此模型</span>
                </label>
              </div>
            </div>
          </div>

          <div class="flex items-center justify-end gap-2 pt-2 border-t border-slate-100 dark:border-slate-800">
            <UiButton size="sm" variant="secondary" type="button" @click="cancelEditModel">取消</UiButton>
            <UiButton size="sm" variant="primary" type="submit" :loading="editingModel.saving">保存</UiButton>
          </div>
        </form>
      </div>

      <!-- Service 状态与控制 -->
      <div v-if="showsService(agent.backend)" class="service p-3.5 rounded bg-slate-50 dark:bg-slate-800/40 border border-slate-200 dark:border-slate-700 space-y-2">
        <p class="service-state flex items-center gap-2 text-xs text-slate-700 dark:text-slate-300">
          <span>service 状态:</span>
          <UiBadge
            :variant="serviceFor(agent.backend)?.state === 'running' ? 'success' : 'default'"
            size="sm"
            :data-test="`service-state-${agent.backend}`"
          >
            {{ stateLabel(agent.backend) }}
          </UiBadge>
          <span v-if="serviceFor(agent.backend)?.version" class="font-mono text-slate-400">
            版本 {{ serviceFor(agent.backend)?.version }}
          </span>
        </p>
        <p v-if="serviceFor(agent.backend)?.endpoint" data-test="service-endpoint" class="service-line text-xs font-mono text-slate-500">
          地址 {{ serviceFor(agent.backend)?.endpoint }}
        </p>
        <p v-if="serviceFor(agent.backend)?.pid" data-test="service-pid" class="service-line text-xs font-mono text-slate-500">
          进程 {{ serviceFor(agent.backend)?.pid }}
        </p>
        <p data-test="service-checked-at" class="service-line text-[11px] text-slate-400">{{ checkedAt(agent.backend) }}</p>
        <p
          v-if="serviceFor(agent.backend)?.state === 'unreachable' && serviceFor(agent.backend)?.detail"
          data-test="service-detail"
          class="service-line text-xs text-red-500"
        >
          {{ serviceFor(agent.backend)?.detail }}
        </p>
        <UiAlert v-if="agents.serviceError" data-test="service-error" variant="error" class="service-line bad">
          {{ agents.serviceError }}
        </UiAlert>
        <div class="service-actions flex items-center gap-2 pt-1">
          <UiButton size="sm" variant="secondary" data-test="service-detect" @click="detectService(agent.backend)">检测</UiButton>
          <UiButton
            size="sm"
            variant="secondary"
            data-test="service-start"
            :disabled="serviceFor(agent.backend)?.state === 'running'"
            @click="startService(agent.backend)"
          >
            启动
          </UiButton>
          <UiButton
            size="sm"
            variant="danger"
            data-test="service-stop"
            :disabled="serviceFor(agent.backend)?.state !== 'running'"
            @click="askStop(agent.backend)"
          >
            停止
          </UiButton>
        </div>

        <div v-if="confirmStop === agent.backend" class="service-confirm p-3 rounded bg-red-50 dark:bg-red-950/30 border border-red-200 dark:border-red-900 space-y-2" data-test="service-stop-dialog">
          <p class="text-xs text-red-700 dark:text-red-300 leading-relaxed">这会停止本机所有正在使用该 service 的客户端，包括桌面端 opencode。正在运行的会话会被中断。</p>
          <div class="service-actions flex items-center gap-2">
            <UiButton
              size="sm"
              variant="danger"
              data-test="service-stop-confirm"
              @click="confirmStopNow(agent.backend)"
            >
              确认停止
            </UiButton>
            <UiButton size="sm" variant="secondary" data-test="service-stop-cancel" @click="cancelStop()">取消</UiButton>
          </div>
        </div>
      </div>

      <!-- 诊断详情 -->
      <details
        class="diagnostics text-xs text-slate-700 dark:text-slate-300"
        :open="diagnosticsOpen(agent.backend)"
        :data-test="`agent-diagnostics-${agent.backend}`"
      >
        <summary class="cursor-pointer font-medium py-1 select-none flex items-center gap-2">
          <span>诊断信息</span>
          <UiBadge
            size="sm"
            :variant="diagnosticsStatus(agent.backend) === 'ok' ? 'success' : diagnosticsStatus(agent.backend) === 'warning' ? 'warning' : 'danger'"
          >
            {{ diagnosticsStatusLabel(agent.backend) }}
          </UiBadge>
        </summary>
        <div class="pt-2">
          <BackendDiagnostics :diagnostics="diagnosticsFor(agent.backend)" compact />
        </div>
      </details>
    </article>
  </section>
</template>
