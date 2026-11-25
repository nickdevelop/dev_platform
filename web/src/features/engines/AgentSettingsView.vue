<script setup lang="ts">
// Agent management section: per-backend platform enablement, executable path
// detection, manual path fallback when auto-detection fails, machine-level service
// controller, persisted model catalogs with refresh, and diagnostics.
//
// Element Plus migration (整页): the backend switcher is `el-tabs`, forms are
// `el-form` + `el-input / el-input-number / el-select / el-switch / el-checkbox`,
// the model catalog is a small `el-table`, model editing opens in the shared
// `EntityFormDialog`, stopping the service goes through `ElMessageBox`, and
// every result — success or failure — is reported once through `ElMessage`
// (the old page-level alert banners are retired; the watchers below are the
// single place store errors surface).
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useSettingsStore } from '../../stores/settings'
import { useAgentsStore } from '../../stores/agents'
import { describeApiError } from '../../api/client'
import {
  listAgentBackendModels,
  refreshAgentBackendModels,
  setAllAgentBackendModelsEnabled,
  updateAgentBackendModel,
  type AgentServiceStatus,
  type ModelInfo,
} from '../../api/agents'
import BackendDiagnostics from '../../shared/components/BackendDiagnostics.vue'
import BackendIcon from '../../shared/components/BackendIcon.vue'
import BuiltinDataPanel from '../settings/BuiltinDataPanel.vue'
import AgentUsagePanel from '../settings/AgentUsagePanel.vue'
import EntityFormDialog from '../../shared/components/EntityFormDialog.vue'
import { rememberModels } from '../../shared/modelCache'

const settings = useSettingsStore()
const agents = useAgentsStore()
const pathInputs = reactive<Record<string, string>>({})

// Tab selection state for switching between backends
const selectedBackend = ref<string>('general')

const activeAgent = computed(() => {
  if (selectedBackend.value && selectedBackend.value !== 'general') {
    const found = settings.agents.find((a) => a.backend === selectedBackend.value)
    if (found) return found
  }
  return settings.agents[0] ?? null
})

// General agent settings state
const generalPromptInput = ref<string>('')

const generalUpdatedAtLabel = computed(() => {
  const raw = settings.agentGeneral?.updatedAt
  if (!raw) return ''
  const parsed = new Date(raw)
  if (Number.isNaN(parsed.getTime())) return ''
  return `上次保存于 ${parsed.toLocaleString()}`
})

async function saveGeneralSettings() {
  const ok = await settings.saveAgentGeneral(generalPromptInput.value)
  if (ok) {
    ElMessage.success('全局系统提示词已成功保存并同步')
  } else {
    ElMessage.error(settings.agentGeneralError || '保存全局设置失败')
  }
}

// Store failures surface once through ElMessage — no page banners.
watch(
  () => settings.agentsError,
  (msg) => {
    if (msg) ElMessage.error(msg)
  },
)
watch(
  () => agents.diagnosticsError,
  (msg) => {
    if (msg) ElMessage.error(msg)
  },
)
watch(
  () => agents.serviceError,
  (msg) => {
    if (msg) ElMessage.error(msg)
  },
)

// Persisted model snapshots, one per backend.
const models = reactive<Record<string, ModelInfo[]>>({})
const modelsUpdatedAt = reactive<Record<string, string | null>>({})
const modelsLoading = reactive<Record<string, boolean>>({})
const modelSearch = reactive<Record<string, string>>({})
const modelsBulkSaving = reactive<Record<string, boolean>>({})

// The settings list keeps enabled models at the top so a long catalog stays
// readable after the operator has chosen what to expose. Search filters by the
// friendly name and the wire id, and the sort is a stable partition: both
// groups keep the backend's reported order.
const visibleModels = computed<Record<string, ModelInfo[]>>(() => {
  const result: Record<string, ModelInfo[]> = {}
  for (const [backend, list] of Object.entries(models)) {
    const query = (modelSearch[backend] ?? '').trim().toLowerCase()
    const filtered = query
      ? list.filter(
          (model) =>
            (model.name || '').toLowerCase().includes(query) ||
            model.id.toLowerCase().includes(query),
        )
      : list
    result[backend] = [...filtered].sort(
      (a, b) => Number(b.enabled !== false) - Number(a.enabled !== false),
    )
  }
  return result
})

function visibleModelsFor(backend: string): ModelInfo[] {
  return visibleModels.value[backend] ?? []
}

function enabledModelCount(backend: string): number {
  return (models[backend] ?? []).filter((model) => model.enabled !== false).length
}

const diagnosticsByBackend = computed(() => {
  const byBackend = new Map<string, (typeof agents.diagnostics)[number]>()
  for (const report of agents.diagnostics) byBackend.set(report.backend, report)
  return byBackend
})

// Idle timeout state per backend
const idleTimeoutInputs = reactive<Record<string, number | null>>({})
const idleTimeoutSaving = reactive<Record<string, boolean>>({})



function supportsIdleTimeout(backend: string): boolean {
  return backend === 'deepseek' || settings.agents.find((a) => a.backend === backend)?.idleTimeoutMinutes !== undefined
}

const debugModeSaving = reactive<Record<string, boolean>>({})

function supportsDebugMode(backend: string): boolean {
  return backend === 'builtin'
}

async function toggleDebugMode(backend: string, enabled: boolean) {
  debugModeSaving[backend] = true
  const ok = await settings.setAgentDebugMode(backend, enabled)
  // 失败已经进了 agentsError，由 watcher 统一报一次。
  if (ok) {
    ElMessage.success(enabled ? '调试日志落盘已开启' : '调试日志落盘已关闭')
  }
  debugModeSaving[backend] = false
}

function setIdleTimeoutPreset(backend: string, minutes: number) {
  idleTimeoutInputs[backend] = minutes
}

async function saveIdleTimeout(backend: string) {
  const agent = settings.agents.find((a) => a.backend === backend)
  const minutes = idleTimeoutInputs[backend] ?? agent?.idleTimeoutMinutes ?? 30
  idleTimeoutSaving[backend] = true
  const ok = await settings.setAgentIdleTimeout(backend, minutes)
  if (ok) {
    ElMessage.success('空闲断开时间已成功保存')
  }
  // 失败已经进了 agentsError，由 watcher 统一报一次。
  idleTimeoutSaving[backend] = false
}

onMounted(async () => {
  void settings.loadAgents()
  void settings.loadAgentGeneral().then(() => {
    generalPromptInput.value = settings.agentGeneral?.systemPrompt || ''
  })
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
  if (ok) {
    pathInputs[backend] = ''
    ElMessage.success('可执行文件路径已保存')
  }
}

async function detect(backend: string) {
  const result = await settings.detectAgent(backend)
  if (!result) return // 失败已经进了 agentsError，由 watcher 统一报一次。
  if (result.status === 'ok') {
    ElMessage.success(`检测成功：版本 ${result.version}`)
  } else {
    ElMessage.error(`检测失败：${result.error}`)
  }
}

async function toggle(backend: string, value: boolean) {
  await settings.setAgentEnabled(backend, value)
  await agents.refreshBackends()
}

function onModeChange(backend: string, mode: string) {
  void settings.setAgentMode(backend, mode)
}

async function onPermissionModeChange(backend: string, mode: string) {
  try {
    await settings.setAgentPermissionMode(backend, mode)
    void settings.loadAgents()
  } catch {
    // The select falls back to the persisted value on the next reload.
  }
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

function serviceTagType(backend: string): 'success' | 'danger' | 'info' {
  const state = serviceFor(backend)?.state
  if (state === 'running') return 'success'
  if (state === 'unreachable') return 'danger'
  return 'info'
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
  } catch (error) {
    ElMessage.error(describeApiError(error))
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

// One click enables or clears every model of the backend. The server applies
// both states atomically, so the UI never shows a half-applied selection.
async function setAllModelsEnabled(backend: string, enabled: boolean) {
  if (!(models[backend] ?? []).length) return
  modelsBulkSaving[backend] = true
  try {
    const list = await setAllAgentBackendModelsEnabled(backend, enabled)
    models[backend] = list.models
    modelsUpdatedAt[backend] = list.updatedAt
    rememberModels(backend, list.models)
  } catch (error) {
    ElMessage.error(describeApiError(error))
  } finally {
    modelsBulkSaving[backend] = false
  }
}

interface EditingModelState {
  backend: string
  id: string
  name: string
  contextLimit: number | null
  supportsImages: boolean
  enabled: boolean
  saving: boolean
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
  }
}

function cancelEditModel() {
  editingModel.value = null
}

async function submitEditModel() {
  if (!editingModel.value) return
  const cur = editingModel.value
  cur.saving = true
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
    ElMessage.success(`模型「${cur.name.trim() || cur.id}」配置已保存`)
    editingModel.value = null
  } catch (error) {
    cur.saving = false
    ElMessage.error(describeApiError(error))
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
    ElMessage.error(describeApiError(error))
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

function diagnosticsTagType(backend: string): 'success' | 'warning' | 'danger' | 'info' {
  const status = diagnosticsStatus(backend)
  if (status === 'ok') return 'success'
  if (status === 'warning') return 'warning'
  if (status === 'error') return 'danger'
  return 'info'
}

// Warnings and errors open the accordion by default; a healthy backend stays
// collapsed. One item per collapse, so the model is a plain string array.
function diagnosticsActive(backend: string): string[] {
  const status = diagnosticsStatus(backend)
  return status === 'warning' || status === 'error' ? ['diagnostics'] : []
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

// Stopping a shared service kills it for every client on the machine, so the
// question goes through the library's confirmation; a rejection (cancel or
// Esc) simply does nothing.
async function askStop(backend: string) {
  try {
    await ElMessageBox.confirm(
      '这会停止本机所有正在使用该 service 的客户端，包括桌面端 opencode。正在运行的会话会被中断。',
      '停止后台服务',
      {
        type: 'warning',
        confirmButtonText: '确认停止',
        cancelButtonText: '取消',
        confirmButtonType: 'danger',
      },
    )
  } catch {
    return
  }
  void agents.stopService(backend)
}
</script>

<template>
  <section class="agents-settings w-full space-y-6" aria-label="Agent 设置">
    <!-- Header row -->
    <header class="panel-header pb-4 border-b border-platform-border/70">
      <div class="flex items-center gap-2.5">
        <h1 class="text-xl font-semibold tracking-tight text-platform-text">Agent 配置</h1>
        <span class="text-xs px-2 py-0.5 rounded border border-platform-border bg-platform-surface-muted text-platform-text-muted font-mono">Infrastructure</span>
      </div>
      <p class="text-xs text-platform-text-muted mt-1">
        配置平台接入的 AI Coding Agent 后端（OpenCode、Pi、DeepSeek 等），管理二进制路径、模型目录、空闲断开策略与后台服务守护进程。
      </p>
    </header>

    <el-tabs v-model="selectedBackend" class="agent-tabs" data-test="agent-tabs">
      <!-- General Settings -->
      <el-tab-pane name="general">
        <template #label>
          <span data-test="agent-tab-general" data-agent-tab="general">全局设置</span>
        </template>

        <article
          class="agent-card p-5 rounded-lg bg-platform-surface-muted/40 border border-platform-border/80 space-y-5"
          data-agent="general"
          data-test="agent-card-general"
        >
          <header class="flex items-center justify-between pb-3.5 border-b border-platform-border/40">
            <div>
              <strong class="text-base text-platform-text font-semibold">全局通用配置</strong>
              <p class="text-xs text-platform-text-muted mt-0.5">
                配置跨所有 AI Coding Agent（OpenCode、Pi、DeepSeek 等）生效的通用规则与系统指令。
              </p>
            </div>
          </header>

          <!-- System Prompt / AGENTS.md section -->
          <div class="space-y-3">
            <div class="flex items-center justify-between flex-wrap gap-2">
              <div>
                <label for="agent-general-system-prompt" class="text-xs font-semibold text-platform-text block">
                  全局系统提示词 (AGENTS.md)
                </label>
                <span class="text-2xs text-platform-text-muted block mt-0.5">
                  作为全局系统规约落盘于平台数据目录，各 Agent 原生作为底层系统级指令加载，绝不污染对话消息流。
                </span>
              </div>
              <span v-if="settings.agentGeneral.path" class="text-2xs text-platform-text-muted font-mono" :title="settings.agentGeneral.path">
                {{ settings.agentGeneral.path }}
              </span>
            </div>

            <el-input
              id="agent-general-system-prompt"
              v-model="generalPromptInput"
              type="textarea"
              :rows="10"
              resize="vertical"
              class="agent-general-prompt font-mono"
              placeholder="# 全局规范&#10;&#10;- 与用户交互始终使用简体中文&#10;- 代码、命令原文保持原样，不翻译"
              data-test="agent-general-prompt-input"
            />

            <div class="flex items-center justify-between pt-1 flex-wrap gap-2">
              <div class="flex items-center gap-3">
                <span class="text-2xs text-platform-text-muted tabular-nums">
                  {{ (generalPromptInput || '').length }} 字符
                </span>
                <span v-if="generalUpdatedAtLabel" class="text-2xs text-platform-text-muted">
                  {{ generalUpdatedAtLabel }}
                </span>
              </div>

              <div class="flex items-center gap-2">
                <el-button
                  type="primary"
                  :loading="settings.agentGeneralSaving"
                  data-test="save-agent-general"
                  @click="saveGeneralSettings"
                >
                  保存全局设置
                </el-button>
              </div>
            </div>
          </div>
        </article>
      </el-tab-pane>

      <!-- One pane per agent backend -->
      <el-tab-pane v-for="agent in settings.agents" :key="agent.backend" :name="agent.backend">
        <template #label>
          <span
            class="inline-flex items-center gap-2"
            :data-agent-tab="agent.backend"
            :data-test="`agent-tab-${agent.backend}`"
          >
            <BackendIcon :backend="agent.backend" size="sm" />
            <span>{{ label(agent.backend) }}</span>
            <span
              class="w-1.5 h-1.5 rounded-full"
              :class="agent.enabled ? 'bg-emerald-500' : 'bg-slate-300 dark:bg-slate-600'"
              :title="agent.enabled ? '已启用' : '未启用'"
            ></span>
          </span>
        </template>

        <article
          class="agent-card p-5 rounded-lg bg-platform-surface-muted/40 border border-platform-border/80 space-y-5"
          :data-agent="agent.backend"
        >
          <!-- Backend Card Header -->
          <header class="flex items-center justify-between pb-3.5 border-b border-platform-border/40">
            <div class="flex items-center gap-3">
              <BackendIcon :backend="agent.backend" />
              <strong class="text-base text-platform-text font-semibold">{{ label(agent.backend) }}</strong>
              <code class="text-2xs font-mono text-platform-text-muted">{{ agent.backend }}</code>
            </div>
            <div class="flex items-center gap-2">
              <el-switch
                :model-value="agent.enabled"
                size="small"
                :data-agent-enabled="agent.backend"
                @change="toggle(agent.backend, $event as boolean)"
              />
              <span class="text-xs font-medium text-platform-text select-none">启用此 Agent</span>
            </div>
          </header>

          <!-- Executable path & detection (hidden for in-process backends) -->
          <div v-if="agent.pathSource !== 'embedded'" class="space-y-2">
            <span class="text-xs font-medium text-platform-text block">可执行文件路径</span>
            <div class="path-row flex items-center gap-2 flex-wrap">
              <template v-if="agent.pathSource === 'auto'">
                <div class="flex-1 min-w-[16rem] flex items-center gap-2 py-1.5 px-3 bg-platform-bg/60 rounded border border-platform-border/70 text-xs">
                  <span class="text-2xs text-platform-text-muted select-none">自动检测:</span>
                  <code class="path font-mono text-platform-text flex-1 truncate" :data-agent-path-display="agent.backend">
                    {{ agent.path }}
                  </code>
                </div>
              </template>
              <template v-else>
                <el-input
                  class="flex-1 min-w-[16rem] font-mono text-xs"
                  :data-agent-path-input="agent.backend"
                  :model-value="pathInputs[agent.backend] ?? agent.path"
                  placeholder="Agent 可执行文件绝对路径"
                  @input="(value: string) => onPathInput(agent.backend, value)"
                />
                <el-button size="small" @click="savePath(agent.backend)">保存路径</el-button>
              </template>
              <el-button size="small" :data-detect="agent.backend" @click="detect(agent.backend)">重新检测</el-button>
            </div>

            <div class="status flex items-center gap-3 text-xs text-platform-text-muted pt-0.5">
              <el-tag :type="agent.available ? 'success' : 'info'" size="small">
                {{ agent.available ? '已检测到' : '未检测到' }}
              </el-tag>
              <span v-if="agent.version" class="tabular-nums text-2xs">版本 {{ agent.version }}</span>
            </div>
          </div>

          <!-- Mode selection row -->
          <div v-if="agent.modes && agent.modes.length > 1" class="mode-row flex items-center gap-3 text-xs text-platform-text-muted py-1 border-t border-platform-border/40">
            <label class="inline-flex items-center gap-2">
              <span class="font-medium text-platform-text">传输模式:</span>
              <el-select
                size="small"
                class="agent-mode-select"
                :data-agent-mode="agent.backend"
                :model-value="agent.mode || agent.modes[0]"
                @change="(mode: string) => onModeChange(agent.backend, mode)"
              >
                <el-option v-for="mode in agent.modes" :key="mode" :label="mode" :value="mode" />
              </el-select>
            </label>
            <span class="mode-hint text-platform-text-muted text-2xs">修改传输模式后需重启平台生效</span>
          </div>

          <!-- Tool permission mode (backends that support the control) -->
          <div
            v-if="agent.permissionModes && agent.permissionModes.length > 0"
            class="permission-row flex items-center gap-3 text-xs text-platform-text-muted py-1 border-t border-platform-border/40"
          >
            <label class="inline-flex items-center gap-2">
              <span class="font-medium text-platform-text">工具权限模式:</span>
              <el-select
                size="small"
                class="agent-permission-select"
                :data-agent-permission-mode="agent.backend"
                :model-value="agent.permissionMode || agent.permissionModes[0]"
                @change="(mode: string) => onPermissionModeChange(agent.backend, mode)"
              >
                <el-option label="需确认（写文件与命令前弹出审批卡片）" value="confirm" />
                <el-option label="自动放行（所有工具调用直接执行）" value="auto" />
              </el-select>
            </label>
            <span class="text-platform-text-muted text-2xs">自下一轮对话开始生效</span>
          </div>

          <!-- Debug Mode Settings (specifically for builtin agent) -->
          <div
            v-if="supportsDebugMode(agent.backend)"
            class="debug-mode-block p-4 rounded-md bg-platform-bg/60 border border-platform-border/70 space-y-3"
            :data-test="`agent-debug-mode-${agent.backend}`"
          >
            <div class="flex items-center justify-between pb-2 border-b border-platform-border/40">
              <div>
                <strong class="text-xs font-semibold text-platform-text block">交互调试记录落盘</strong>
                <span class="text-2xs text-platform-text-muted">开启后将内置 Agent 的所有 LLM 请求/响应及工具执行详情落盘保存至 dataDir，可随时在会话头部导出调试日志文件。</span>
              </div>
              <span class="tabular-nums text-2xs text-platform-text-muted">
                当前状态：{{ agent.debugMode ? '已开启' : '已关闭' }}
              </span>
            </div>

            <div class="flex items-center gap-3 text-xs">
              <el-checkbox
                :model-value="agent.debugMode ?? false"
                :disabled="debugModeSaving[agent.backend]"
                :data-test="`debug-mode-checkbox-${agent.backend}`"
                @change="(checked: string | number | boolean) => toggleDebugMode(agent.backend, Boolean(checked))"
              >
                <span class="text-platform-text font-medium">开启调试日志落盘</span>
              </el-checkbox>
              <span v-if="debugModeSaving[agent.backend]" class="text-platform-text-muted text-2xs">保存中...</span>
            </div>
          </div>

          <!-- Idle Timeout Settings (specifically for dsh / deepseek or backends with idle reclamation) -->
          <div
            v-if="supportsIdleTimeout(agent.backend)"
            class="idle-timeout-block p-4 rounded-md bg-platform-bg/60 border border-platform-border/70 space-y-3"
            :data-test="`agent-idle-timeout-${agent.backend}`"
          >
            <div class="flex items-center justify-between pb-2 border-b border-platform-border/40">
              <div>
                <strong class="text-xs font-semibold text-platform-text block">DSH 进程空闲断开时间</strong>
                <span class="text-2xs text-platform-text-muted">设置无会话活跃时共享 dsh 守护进程的自动释放时长</span>
              </div>
              <span v-if="agent.idleTimeoutMinutes !== undefined" class="tabular-nums text-2xs text-platform-text-muted">
                当前设定：{{ agent.idleTimeoutMinutes < 0 ? '从不断开' : `${agent.idleTimeoutMinutes} 分钟` }}
              </span>
            </div>

            <div class="space-y-3 text-xs">
              <div class="flex items-center gap-3 flex-wrap">
                <div class="flex items-center gap-2">
                  <label :for="`idle-timeout-input-${agent.backend}`" class="text-platform-text-muted font-medium">空闲断开时长:</label>
                  <el-input-number
                    :id="`idle-timeout-input-${agent.backend}`"
                    :min="-1"
                    :step="1"
                    :controls="false"
                    size="small"
                    class="idle-timeout-number w-24"
                    :data-test="`idle-timeout-input-${agent.backend}`"
                    :model-value="idleTimeoutInputs[agent.backend] ?? agent.idleTimeoutMinutes ?? 30"
                    @update:model-value="(val: number | undefined) => (idleTimeoutInputs[agent.backend] = val ?? null)"
                  />
                  <span class="text-platform-text-muted text-xs">分钟（-1 表示从不断开）</span>
                </div>

                <div class="preset-buttons flex items-center gap-1.5">
                  <el-button size="small" plain @click="setIdleTimeoutPreset(agent.backend, 15)">15m</el-button>
                  <el-button size="small" plain @click="setIdleTimeoutPreset(agent.backend, 30)">30m</el-button>
                  <el-button size="small" plain @click="setIdleTimeoutPreset(agent.backend, 60)">60m</el-button>
                  <el-button size="small" plain @click="setIdleTimeoutPreset(agent.backend, -1)">从不断开</el-button>
                </div>

                <el-button
                  type="primary"
                  size="small"
                  :data-test="`save-idle-timeout-${agent.backend}`"
                  :loading="idleTimeoutSaving[agent.backend]"
                  @click="saveIdleTimeout(agent.backend)"
                >
                  保存设置
                </el-button>
              </div>
            </div>
          </div>

          <!-- Session data management (platform-persisted transcripts) -->
          <BuiltinDataPanel v-if="agent.sessionDataOwner" :backend="agent.backend" />

          <!-- Agent usage and quota statistics -->
          <AgentUsagePanel v-if="agent.supportsUsage" :backend="agent.backend" />

          <!-- Agent available models catalog -->
          <div
            v-if="supportsModels(agent.backend)"
            class="models p-4 rounded-md bg-platform-bg/60 border border-platform-border/70 space-y-3"
            :data-test="`agent-models-${agent.backend}`"
          >
            <div class="models-head flex items-center justify-between pb-2 border-b border-platform-border/40">
              <div>
                <strong class="text-xs font-semibold text-platform-text block">可用模型目录</strong>
                <span class="models-updated text-2xs text-platform-text-muted">{{ updatedAtLabel(agent.backend) }}</span>
              </div>
              <el-button
                size="small"
                text
                :data-test="`agent-models-refresh-${agent.backend}`"
                :disabled="modelsLoading[agent.backend] === true"
                :loading="modelsLoading[agent.backend] === true"
                @click="loadModels(agent.backend, true)"
              >
                刷新模型
              </el-button>
            </div>

            <div class="model-toolbar flex items-center gap-2 flex-wrap">
              <el-input
                v-model="modelSearch[agent.backend]"
                size="small"
                clearable
                placeholder="按模型名称搜索"
                class="flex-1 min-w-[12rem]"
                :data-test="`agent-models-search-${agent.backend}`"
              />
              <el-button
                size="small"
                :data-test="`agent-models-select-all-${agent.backend}`"
                :disabled="
                  modelsLoading[agent.backend] === true ||
                  modelsBulkSaving[agent.backend] === true ||
                  !(models[agent.backend] ?? []).length ||
                  enabledModelCount(agent.backend) === (models[agent.backend] ?? []).length
                "
                :loading="modelsBulkSaving[agent.backend] === true"
                @click="setAllModelsEnabled(agent.backend, true)"
              >
                全选
              </el-button>
              <el-button
                size="small"
                :data-test="`agent-models-clear-all-${agent.backend}`"
                :disabled="
                  modelsLoading[agent.backend] === true ||
                  modelsBulkSaving[agent.backend] === true ||
                  enabledModelCount(agent.backend) === 0
                "
                @click="setAllModelsEnabled(agent.backend, false)"
              >
                清空
              </el-button>
              <span class="text-2xs text-platform-text-muted tabular-nums">
                已启用 {{ enabledModelCount(agent.backend) }}/{{ (models[agent.backend] ?? []).length }}
              </span>
            </div>

            <el-table
              v-if="visibleModelsFor(agent.backend).length || modelsLoading[agent.backend]"
              :data="visibleModelsFor(agent.backend)"
              size="small"
              max-height="256"
              row-key="id"
              class="model-table"
              :empty-text="modelsLoading[agent.backend] ? '正在同步模型目录…' : '没有匹配的模型。'"
            >
              <el-table-column width="56">
                <template #default="{ row }">
                  <el-switch
                    size="small"
                    :model-value="row.enabled !== false"
                    :data-agent-model-toggle="row.id"
                    @change="toggleModelEnabled(agent.backend, row as ModelInfo, $event as boolean)"
                  />
                </template>
              </el-table-column>
              <el-table-column label="模型" min-width="240">
                <template #default="{ row }">
                  <div class="flex items-center gap-2 min-w-0">
                    <el-tag :type="row.enabled !== false ? 'success' : 'info'" size="small">
                      {{ row.enabled !== false ? '已启用' : '已停用' }}
                    </el-tag>
                    <span class="model-name text-xs font-medium text-platform-text truncate">{{ row.name || row.id }}</span>
                  </div>
                  <div class="flex items-center gap-2 text-2xs text-platform-text-muted min-w-0 mt-0.5">
                    <code class="model-id font-mono truncate max-w-[160px]">{{ row.id }}</code>
                    <span v-if="row.contextLimit" class="tabular-nums whitespace-nowrap">
                      {{ Math.round(row.contextLimit / 1000) }}k 上下文
                    </span>
                    <span v-if="row.supportsImages" class="text-platform-primary whitespace-nowrap">图片</span>
                    <span v-if="row.variants && row.variants.length" class="model-variants truncate max-w-[160px]">
                      变体：{{ row.variants.join(' / ') }}
                    </span>
                  </div>
                </template>
              </el-table-column>
              <el-table-column label="操作" width="72" align="right">
                <template #default="{ row }">
                  <el-button
                    size="small"
                    link
                    :data-agent-model-edit="row.id"
                    @click="openEditModel(agent.backend, row as ModelInfo)"
                  >
                    编辑
                  </el-button>
                </template>
              </el-table-column>
            </el-table>

            <p
              v-else-if="!modelsLoading[agent.backend] && !(models[agent.backend] ?? []).length"
              class="models-line text-xs text-platform-text-muted py-1"
            >
              尚未同步模型，点击刷新读取该 Agent 当前可用的模型。
            </p>

            <!-- Model edit dialog: the shared editor shell, same chrome as every
                 other 编辑 X form on the platform. -->
            <EntityFormDialog
              v-if="editingModel"
              :key="`${editingModel.backend}-${editingModel.id}`"
              title="编辑模型"
              subtitle="自定义友好显示名称与上下文 Token 限制"
              :identifier="editingModel.id"
              save-label="保存"
              :saving="editingModel.saving"
              :width="520"
              save-test="agent-model-form-submit"
              data-test="agent-model-form"
              @save="submitEditModel"
              @cancel="cancelEditModel"
            >
              <el-form class="entity-form" label-position="top" @submit.prevent>
                <el-form-item label="显示名称">
                  <el-input v-model="editingModel.name" placeholder="模型友好名称" data-test="agent-model-edit-name" />
                </el-form-item>
                <el-form-item label="上下文 Token 限制（留空按后端默认）">
                  <el-input-number
                    v-model="editingModel.contextLimit"
                    :min="0"
                    :controls="false"
                    class="w-full"
                    placeholder="如 128000"
                    data-test="agent-model-edit-context"
                  />
                </el-form-item>
                <el-form-item>
                  <el-checkbox v-model="editingModel.supportsImages">支持图片输入</el-checkbox>
                  <el-checkbox v-model="editingModel.enabled" class="ml-4">启用此模型</el-checkbox>
                </el-form-item>
              </el-form>
            </EntityFormDialog>
          </div>

          <!-- Service daemon controller -->
          <div
            v-if="showsService(agent.backend)"
            class="service-block p-4 rounded-md bg-platform-bg/60 border border-platform-border/70 space-y-3"
          >
            <div class="flex items-center justify-between pb-2 border-b border-platform-border/40">
              <div class="flex items-center gap-2 text-xs">
                <span class="font-medium text-platform-text">后台 service 守护进程:</span>
                <el-tag
                  :type="serviceTagType(agent.backend)"
                  size="small"
                  :data-test="`service-state-${agent.backend}`"
                >
                  {{ stateLabel(agent.backend) }}
                </el-tag>
              </div>
              <span v-if="serviceFor(agent.backend)?.version" class="tabular-nums text-2xs text-platform-text-muted">
                版本 {{ serviceFor(agent.backend)?.version }}
              </span>
            </div>

            <div class="space-y-1 text-xs">
              <p v-if="serviceFor(agent.backend)?.endpoint" data-test="service-endpoint" class="service-line text-xs text-platform-text-muted">
                服务地址 <code class="text-2xs">{{ serviceFor(agent.backend)?.endpoint }}</code>
              </p>
              <p v-if="serviceFor(agent.backend)?.pid" data-test="service-pid" class="service-line text-xs text-platform-text-muted">
                进程 PID <span class="tabular-nums text-2xs">{{ serviceFor(agent.backend)?.pid }}</span>
              </p>
              <p data-test="service-checked-at" class="service-line text-2xs text-platform-text-muted">
                {{ checkedAt(agent.backend) }}
              </p>
              <p
                v-if="serviceFor(agent.backend)?.state === 'unreachable' && serviceFor(agent.backend)?.detail"
                data-test="service-detail"
                class="service-line text-xs text-platform-danger"
              >
                {{ serviceFor(agent.backend)?.detail }}
              </p>
            </div>

            <div class="service-actions flex items-center gap-2 pt-1">
              <el-button size="small" data-test="service-detect" @click="detectService(agent.backend)">检测状态</el-button>
              <el-button
                size="small"
                data-test="service-start"
                :disabled="serviceFor(agent.backend)?.state === 'running'"
                @click="startService(agent.backend)"
              >
                启动服务
              </el-button>
              <el-button
                size="small"
                type="danger"
                plain
                data-test="service-stop"
                :disabled="serviceFor(agent.backend)?.state !== 'running'"
                @click="askStop(agent.backend)"
              >
                停止服务
              </el-button>
            </div>
          </div>

          <!-- Diagnostics accordion -->
          <el-collapse
            class="diagnostics text-xs text-platform-text pt-1 border-t border-platform-border/40"
            :model-value="diagnosticsActive(agent.backend)"
            :data-test="`agent-diagnostics-${agent.backend}`"
          >
            <el-collapse-item name="diagnostics">
              <template #title>
                <div class="flex items-center gap-2 font-medium select-none">
                  <span>后端诊断信息</span>
                  <el-tag size="small" :type="diagnosticsTagType(agent.backend)">
                    {{ diagnosticsStatusLabel(agent.backend) }}
                  </el-tag>
                </div>
              </template>
              <div class="pt-1">
                <BackendDiagnostics :diagnostics="diagnosticsFor(agent.backend)" compact />
              </div>
            </el-collapse-item>
          </el-collapse>
        </article>
      </el-tab-pane>
    </el-tabs>
  </section>
</template>

<style scoped>
/* 库部件的样式干预只能写在 theme/element-tokens.css；这里只碰原生元素。 */
.agent-general-prompt :deep(textarea) {
  font-family: var(--font-mono);
  font-size: var(--text-size-12);
}

.idle-timeout-number :deep(input) {
  font-variant-numeric: tabular-nums;
}
</style>
