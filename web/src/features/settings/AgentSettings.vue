<script setup lang="ts">
// Agent management section: per-backend platform enablement, executable path
// detection, manual path fallback when auto-detection fails, machine-level service
// controller, persisted model catalogs with refresh, and diagnostics.
// Redesigned with design-taste-frontend app-screens guidelines (framed-sheet,
// flat surfaces, zero resting shadows, tabular figures, precise status pills,
// structured sections).
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

// Persisted model snapshots, one per backend.
const models = reactive<Record<string, ModelInfo[]>>({})
const modelsUpdatedAt = reactive<Record<string, string | null>>({})
const modelsLoading = reactive<Record<string, boolean>>({})
const modelsError = reactive<Record<string, string>>({})

const diagnosticsByBackend = computed(() => {
  const byBackend = new Map<string, (typeof agents.diagnostics)[number]>()
  for (const report of agents.diagnostics) byBackend.set(report.backend, report)
  return byBackend
})

const confirmStop = ref<string | null>(null)

onMounted(async () => {
  void settings.loadAgents()
  await agents.loadBackends()
  void agents.loadDiagnostics()
  for (const entry of agents.serviceBackends) {
    void agents.detectService(entry.backend)
  }
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
  await agents.refreshBackends()
}

function onModeChange(backend: string, mode: string) {
  void settings.setAgentMode(backend, mode)
}

function serviceFor(backend: string): AgentServiceStatus | null {
  return agents.services[backend] ?? null
}

function showsService(backend: string): boolean {
  return agents.backendFor(backend)?.serviceSupported === true && serviceFor(backend)?.supported !== false
}

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
  <section class="agents-settings w-full space-y-6" aria-label="Agent 设置">
    <!-- Header row -->
    <header class="pb-4 border-b border-slate-200 dark:border-slate-800">
      <h1 class="text-[22px] font-semibold text-slate-900 dark:text-slate-100 tracking-tight">Agent 设置</h1>
      <p class="text-xs text-slate-500 dark:text-slate-400 mt-1">
        配置平台接入的 AI Coding Agent 后端（OpenCode、Pi 等），管理二进制路径、模型目录与后台服务守护进程。
      </p>
    </header>

    <UiAlert v-if="settings.agentsError" variant="error" class="error">
      {{ settings.agentsError }}
    </UiAlert>
    <UiAlert v-if="agents.diagnosticsError" variant="error" class="error">
      {{ agents.diagnosticsError }}
    </UiAlert>

    <!-- Agent backend cards list -->
    <article
      v-for="agent in settings.agents"
      :key="agent.backend"
      class="agent-card p-5 rounded-lg bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 space-y-5"
      :data-agent="agent.backend"
    >
      <!-- Backend Card Header -->
      <header class="flex items-center justify-between pb-3.5 border-b border-slate-100 dark:border-slate-800">
        <div class="flex items-center gap-3">
          <strong class="text-base text-slate-900 dark:text-slate-100 font-semibold">{{ label(agent.backend) }}</strong>
          <code class="text-[11px] font-mono text-slate-400 dark:text-slate-500">{{ agent.backend }}</code>
        </div>
        <label class="toggle-control inline-flex items-center gap-2 text-xs font-medium text-slate-700 dark:text-slate-300 cursor-pointer select-none">
          <input
            type="checkbox"
            class="w-4 h-4 rounded text-blue-600 focus:ring-blue-500 accent-blue-600"
            :data-agent-enabled="agent.backend"
            :checked="agent.enabled"
            @change="toggle(agent.backend, ($event.target as HTMLInputElement).checked)"
          />
          <span>启用此 Agent</span>
        </label>
      </header>

      <!-- Executable path & detection -->
      <div class="space-y-2">
        <span class="text-xs font-medium text-slate-700 dark:text-slate-300 block">可执行文件路径</span>
        <div class="path-row flex items-center gap-2 flex-wrap">
          <template v-if="agent.pathSource === 'auto'">
            <div class="flex-1 min-w-[16rem] flex items-center gap-2 py-1.5 px-3 bg-slate-50 dark:bg-slate-800/50 rounded border border-slate-200 dark:border-slate-700 text-xs">
              <span class="text-[11px] text-slate-400 select-none">自动检测:</span>
              <code class="path font-mono text-slate-700 dark:text-slate-300 flex-1 truncate" :data-agent-path-display="agent.backend">
                {{ agent.path }}
              </code>
            </div>
          </template>
          <template v-else>
            <UiInput
              class="flex-1 min-w-[16rem] font-mono text-xs"
              :data-agent-path-input="agent.backend"
              :model-value="pathInputs[agent.backend] ?? agent.path"
              placeholder="Agent 可执行文件绝对路径"
              block
              @input="(e: Event) => onPathInput(agent.backend, (e.target as HTMLInputElement).value)"
            />
            <UiButton size="sm" variant="secondary" @click="savePath(agent.backend)">保存路径</UiButton>
          </template>
          <UiButton size="sm" variant="secondary" :data-detect="agent.backend" @click="detect(agent.backend)">重新检测</UiButton>
        </div>

        <div class="status flex items-center gap-3 text-xs text-slate-500 dark:text-slate-400 pt-0.5">
          <UiBadge :variant="agent.available ? 'success' : 'default'" size="sm" dot>
            {{ agent.available ? '已检测到' : '未检测到' }}
          </UiBadge>
          <span v-if="agent.version" class="tabular-nums text-[11px]">版本 {{ agent.version }}</span>
        </div>
      </div>

      <!-- Mode selection row -->
      <div v-if="agent.modes && agent.modes.length > 1" class="mode-row flex items-center gap-3 text-xs text-slate-600 dark:text-slate-400 py-1 border-t border-slate-100 dark:border-slate-800">
        <label class="inline-flex items-center gap-2">
          <span class="font-medium text-slate-700 dark:text-slate-300">传输模式:</span>
          <UiSelect
            size="sm"
            :data-agent-mode="agent.backend"
            :model-value="agent.mode || agent.modes[0]"
            @change="(e: Event) => onModeChange(agent.backend, (e.target as HTMLSelectElement).value)"
          >
            <option v-for="mode in agent.modes" :key="mode" :value="mode">{{ mode }}</option>
          </UiSelect>
        </label>
        <span class="mode-hint text-slate-400 text-[11px]">修改传输模式后需重启平台生效</span>
      </div>

      <!-- Detection alert feedback -->
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

      <!-- Agent available models catalog -->
      <div
        v-if="supportsModels(agent.backend)"
        class="models p-4 rounded-md bg-slate-50 dark:bg-slate-800/40 border border-slate-200 dark:border-slate-700/80 space-y-3"
        :data-test="`agent-models-${agent.backend}`"
      >
        <div class="models-head flex items-center justify-between pb-2 border-b border-slate-200/80 dark:border-slate-700/60">
          <div>
            <strong class="text-xs font-semibold text-slate-800 dark:text-slate-200 block">可用模型目录</strong>
            <span class="models-updated text-[11px] text-slate-400">{{ updatedAtLabel(agent.backend) }}</span>
          </div>
          <UiButton
            size="sm"
            variant="ghost"
            :data-test="`agent-models-refresh-${agent.backend}`"
            :disabled="modelsLoading[agent.backend] === true"
            :loading="modelsLoading[agent.backend] === true"
            @click="loadModels(agent.backend, true)"
          >
            刷新模型
          </UiButton>
        </div>

        <p v-if="modelsLoading[agent.backend]" class="models-line text-xs text-slate-400">正在同步模型目录…</p>
        <UiAlert v-if="modelsError[agent.backend]" variant="error" class="models-line bad">
          {{ modelsError[agent.backend] }}
        </UiAlert>

        <ul v-if="models[agent.backend]?.length" class="model-list space-y-1.5 max-h-64 overflow-y-auto pr-1">
          <li
            v-for="model in models[agent.backend]"
            :key="model.id"
            :data-agent-model="model.id"
            class="model-item p-2.5 rounded bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 text-xs gap-2.5 hover:border-slate-300 dark:hover:border-slate-700 transition-colors flex flex-col sm:flex-row sm:items-center sm:justify-between"
          >
            <div class="flex items-center gap-2.5 min-w-0 flex-1">
              <label class="inline-flex items-center cursor-pointer select-none shrink-0">
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
              <!-- Desktop inline details -->
              <div class="hidden sm:inline-flex items-center gap-2 text-[11px] text-slate-400 min-w-0">
                <code class="model-id text-slate-400 font-mono truncate max-w-[140px]">{{ model.id }}</code>
                <span v-if="model.contextLimit" class="tabular-nums whitespace-nowrap">
                  {{ Math.round(model.contextLimit / 1000) }}k 上下文
                </span>
                <span v-if="model.supportsImages" class="text-blue-500 whitespace-nowrap">图片</span>
                <span v-if="model.variants && model.variants.length" class="model-variants truncate max-w-[140px]">
                  变体：{{ model.variants.join(' / ') }}
                </span>
              </div>
            </div>

            <!-- Mobile subline -->
            <div class="flex sm:hidden items-center gap-2 text-[11px] text-slate-400 pl-6 flex-wrap">
              <code class="model-id text-slate-400 font-mono">{{ model.id }}</code>
              <span v-if="model.contextLimit" class="tabular-nums">
                {{ Math.round(model.contextLimit / 1000) }}k 上下文
              </span>
              <span v-if="model.supportsImages" class="text-blue-500">图片</span>
              <span v-if="model.variants && model.variants.length" class="model-variants">
                变体：{{ model.variants.join(' / ') }}
              </span>
            </div>

            <div class="flex items-center justify-end gap-1 shrink-0 self-end sm:self-center">
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
        <p v-else-if="!modelsLoading[agent.backend]" class="models-line text-xs text-slate-400 py-1">
          尚未同步模型，点击刷新读取该 Agent 当前可用的模型。
        </p>

        <!-- Model edit form inline -->
        <form
          v-if="editingModel && editingModel.backend === agent.backend"
          class="model-edit-form p-4 mt-2 rounded bg-white dark:bg-slate-900 border border-blue-200 dark:border-blue-900/60 space-y-3"
          :data-agent-model-form="editingModel.id"
          @submit.prevent="submitEditModel"
        >
          <div class="flex items-center justify-between pb-1 border-b border-slate-100 dark:border-slate-800">
            <h3 class="text-xs font-semibold text-slate-800 dark:text-slate-200">
              配置模型：<code class="font-mono text-slate-500">{{ editingModel.id }}</code>
            </h3>
            <span class="text-[11px] text-slate-400">自定义友好显示名称与上下文 Token 限制</span>
          </div>

          <UiAlert v-if="editingModel.error" variant="error">
            {{ editingModel.error }}
          </UiAlert>

          <div class="space-y-2.5">
            <div>
              <label class="block text-[11px] font-medium text-slate-600 dark:text-slate-400 mb-1">显示名称</label>
              <UiInput v-model="editingModel.name" placeholder="模型友好名称" required block />
            </div>

            <div class="grid grid-cols-1 sm:grid-cols-2 gap-3 pt-1">
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
            <UiButton size="sm" variant="primary" type="submit" :loading="editingModel.saving">保存配置</UiButton>
          </div>
        </form>
      </div>

      <!-- Service daemon controller -->
      <div
        v-if="showsService(agent.backend)"
        class="service-block p-4 rounded-md bg-slate-50 dark:bg-slate-800/40 border border-slate-200 dark:border-slate-700/80 space-y-3"
      >
        <div class="flex items-center justify-between pb-2 border-b border-slate-200/80 dark:border-slate-700/60">
          <div class="flex items-center gap-2 text-xs">
            <span class="font-medium text-slate-700 dark:text-slate-300">后台 service 守护进程:</span>
            <UiBadge
              :variant="serviceFor(agent.backend)?.state === 'running' ? 'success' : 'default'"
              size="sm"
              :data-test="`service-state-${agent.backend}`"
            >
              {{ stateLabel(agent.backend) }}
            </UiBadge>
          </div>
          <span v-if="serviceFor(agent.backend)?.version" class="tabular-nums text-[11px] text-slate-400">
            版本 {{ serviceFor(agent.backend)?.version }}
          </span>
        </div>

        <div class="space-y-1 text-xs">
          <p v-if="serviceFor(agent.backend)?.endpoint" data-test="service-endpoint" class="service-line text-xs text-slate-600 dark:text-slate-400">
            服务地址 <code class="text-[11px]">{{ serviceFor(agent.backend)?.endpoint }}</code>
          </p>
          <p v-if="serviceFor(agent.backend)?.pid" data-test="service-pid" class="service-line text-xs text-slate-600 dark:text-slate-400">
            进程 PID <span class="tabular-nums text-[11px]">{{ serviceFor(agent.backend)?.pid }}</span>
          </p>
          <p data-test="service-checked-at" class="service-line text-[11px] text-slate-400">
            {{ checkedAt(agent.backend) }}
          </p>
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
        </div>

        <div class="service-actions flex items-center gap-2 pt-1">
          <UiButton size="sm" variant="secondary" data-test="service-detect" @click="detectService(agent.backend)">检测状态</UiButton>
          <UiButton
            size="sm"
            variant="secondary"
            data-test="service-start"
            :disabled="serviceFor(agent.backend)?.state === 'running'"
            @click="startService(agent.backend)"
          >
            启动服务
          </UiButton>
          <UiButton
            size="sm"
            variant="secondary"
            class="text-red-600 hover:text-red-700 dark:text-red-400 border-red-200 dark:border-red-900/60"
            data-test="service-stop"
            :disabled="serviceFor(agent.backend)?.state !== 'running'"
            @click="askStop(agent.backend)"
          >
            停止服务
          </UiButton>
        </div>

        <!-- Stop confirmation dialog inline -->
        <div
          v-if="confirmStop === agent.backend"
          class="service-confirm p-3.5 rounded bg-red-50 dark:bg-red-950/30 border border-red-200 dark:border-red-900 space-y-2.5"
          data-test="service-stop-dialog"
        >
          <p class="text-xs text-red-700 dark:text-red-300 leading-relaxed font-medium">
            这会停止本机所有正在使用该 service 的客户端，包括桌面端 opencode。正在运行的会话会被中断。
          </p>
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

      <!-- Diagnostics accordion -->
      <details
        class="diagnostics text-xs text-slate-700 dark:text-slate-300 pt-1 border-t border-slate-100 dark:border-slate-800"
        :open="diagnosticsOpen(agent.backend)"
        :data-test="`agent-diagnostics-${agent.backend}`"
      >
        <summary class="cursor-pointer font-medium py-1.5 select-none flex items-center justify-between hover:text-slate-900 dark:hover:text-slate-100">
          <div class="flex items-center gap-2">
            <span>后端诊断信息</span>
            <UiBadge
              size="sm"
              :variant="diagnosticsStatus(agent.backend) === 'ok' ? 'success' : diagnosticsStatus(agent.backend) === 'warning' ? 'warning' : 'danger'"
            >
              {{ diagnosticsStatusLabel(agent.backend) }}
            </UiBadge>
          </div>
          <span class="text-slate-400 text-[11px]">展开/收起 ▾</span>
        </summary>
        <div class="pt-2.5">
          <BackendDiagnostics :diagnostics="diagnosticsFor(agent.backend)" compact />
        </div>
      </details>
    </article>
  </section>
</template>
