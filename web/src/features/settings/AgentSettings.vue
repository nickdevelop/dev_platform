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
  type AgentServiceStatus,
  type ModelInfo,
} from '../../api/agents'
import type { AgentDetection } from '../../api/settings'
import BackendDiagnostics from '../../shared/components/BackendDiagnostics.vue'
import { rememberModels } from '../../shared/modelCache'

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
  <section class="agents" aria-label="Agent 设置">
    <h1>Agent</h1>
    <p class="intro">启用后可在项目中选择并创建新会话。停用只禁止新建，已有会话继续可用。</p>

    <p v-if="settings.agentsError" class="error" role="alert">{{ settings.agentsError }}</p>
    <p v-if="agents.diagnosticsError" class="error" role="alert">{{ agents.diagnosticsError }}</p>

    <article
      v-for="agent in settings.agents"
      :key="agent.backend"
      class="agent-card"
      :data-agent="agent.backend"
    >
      <header>
        <strong>{{ label(agent.backend) }}</strong>
        <label class="inline">
          <input
            type="checkbox"
            :data-agent-enabled="agent.backend"
            :checked="agent.enabled"
            @change="toggle(agent.backend, ($event.target as HTMLInputElement).checked)"
          />
          启用
        </label>
      </header>

      <div class="path-row">
        <template v-if="agent.pathSource === 'auto'">
          <span class="path" :data-agent-path-display="agent.backend">{{ agent.path }}</span>
        </template>
        <template v-else>
          <input
            type="text"
            :data-agent-path-input="agent.backend"
            :value="pathInputs[agent.backend] ?? agent.path"
            placeholder="Agent 可执行文件路径"
            @input="onPathInput(agent.backend, ($event.target as HTMLInputElement).value)"
          />
          <button type="button" @click="savePath(agent.backend)">保存路径</button>
        </template>
        <button type="button" :data-detect="agent.backend" @click="detect(agent.backend)">检测</button>
      </div>

      <p class="status">
        <span>{{ agent.available ? '已检测到' : '未检测到' }}</span>
        <span v-if="agent.version">版本 {{ agent.version }}</span>
      </p>

      <div v-if="agent.modes && agent.modes.length > 1" class="mode-row">
        <label class="inline">
          传输模式
          <select
            :data-agent-mode="agent.backend"
            :value="agent.mode || agent.modes[0]"
            @change="onModeChange(agent.backend, ($event.target as HTMLSelectElement).value)"
          >
            <option v-for="mode in agent.modes" :key="mode" :value="mode">{{ mode }}</option>
          </select>
        </label>
        <span class="mode-hint">修改后需重启平台生效</span>
      </div>

      <p
        v-if="detections[agent.backend]"
        class="detection"
        :data-detection="agent.backend"
        :class="{ bad: detections[agent.backend]?.status !== 'ok' }"
      >
        {{
          detections[agent.backend]?.status === 'ok'
            ? `检测成功：版本 ${detections[agent.backend]?.version}`
            : `检测失败：${detections[agent.backend]?.error}`
        }}
      </p>

      <div v-if="supportsModels(agent.backend)" class="models" :data-test="`agent-models-${agent.backend}`">
        <div class="models-head">
          <strong>模型</strong>
          <button
            type="button"
            :data-test="`agent-models-refresh-${agent.backend}`"
            :disabled="modelsLoading[agent.backend] === true"
            @click="loadModels(agent.backend, true)"
          >
            刷新
          </button>
        </div>
        <p v-if="modelsLoading[agent.backend]" class="models-line">正在读取…</p>
        <p v-if="modelsError[agent.backend]" class="models-line bad" role="alert">
          {{ modelsError[agent.backend] }}
        </p>
        <ul v-if="models[agent.backend]?.length" class="model-list">
          <li
            v-for="model in models[agent.backend]"
            :key="model.id"
            :data-agent-model="model.id"
            class="model-item"
          >
            <span class="model-name">{{ model.name || model.id }}</span>
            <span class="model-id">{{ model.id }}</span>
            <span v-if="model.variants && model.variants.length" class="model-variants">
              变体：{{ model.variants.join(' / ') }}
            </span>
          </li>
        </ul>
        <p v-else-if="!modelsLoading[agent.backend]" class="models-line">
          尚未同步模型，点击刷新读取该 Agent 当前可用的模型。
        </p>
        <p class="models-updated">{{ updatedAtLabel(agent.backend) }}</p>
      </div>

      <div v-if="showsService(agent.backend)" class="service">
        <p class="service-state">
          service
          <strong :data-test="`service-state-${agent.backend}`">{{ stateLabel(agent.backend) }}</strong>
          <span v-if="serviceFor(agent.backend)?.version">版本 {{ serviceFor(agent.backend)?.version }}</span>
        </p>
        <p v-if="serviceFor(agent.backend)?.endpoint" data-test="service-endpoint" class="service-line">
          地址 {{ serviceFor(agent.backend)?.endpoint }}
        </p>
        <p v-if="serviceFor(agent.backend)?.pid" data-test="service-pid" class="service-line">
          进程 {{ serviceFor(agent.backend)?.pid }}
        </p>
        <p data-test="service-checked-at" class="service-line">{{ checkedAt(agent.backend) }}</p>
        <p
          v-if="serviceFor(agent.backend)?.state === 'unreachable' && serviceFor(agent.backend)?.detail"
          data-test="service-detail"
          class="service-line bad"
        >
          {{ serviceFor(agent.backend)?.detail }}
        </p>
        <p v-if="agents.serviceError" data-test="service-error" class="service-line bad" role="alert">
          {{ agents.serviceError }}
        </p>
        <div class="service-actions">
          <button type="button" data-test="service-detect" @click="detectService(agent.backend)">检测</button>
          <button
            type="button"
            data-test="service-start"
            :disabled="serviceFor(agent.backend)?.state === 'running'"
            @click="startService(agent.backend)"
          >
            启动
          </button>
          <button
            type="button"
            data-test="service-stop"
            :disabled="serviceFor(agent.backend)?.state !== 'running'"
            @click="askStop(agent.backend)"
          >
            停止
          </button>
        </div>
        <div v-if="confirmStop === agent.backend" class="service-confirm" data-test="service-stop-dialog">
          <p>这会停止本机所有正在使用该 service 的客户端，包括桌面端 opencode。正在运行的会话会被中断。</p>
          <div class="service-actions">
            <button
              type="button"
              class="danger"
              data-test="service-stop-confirm"
              @click="confirmStopNow(agent.backend)"
            >
              确认停止
            </button>
            <button type="button" data-test="service-stop-cancel" @click="cancelStop()">取消</button>
          </div>
        </div>
      </div>

      <details
        class="diagnostics"
        :open="diagnosticsOpen(agent.backend)"
        :data-test="`agent-diagnostics-${agent.backend}`"
      >
        <summary>
          诊断
          <span class="status-badge" :class="`status-${diagnosticsStatus(agent.backend)}`">
            {{ diagnosticsStatusLabel(agent.backend) }}
          </span>
        </summary>
        <BackendDiagnostics :diagnostics="diagnosticsFor(agent.backend)" compact />
      </details>
    </article>
  </section>
</template>

<style scoped>
.agents h1 {
  font-size: 1.25rem;
  margin: 0 0 var(--space-unit);
}

.intro {
  color: var(--color-text-muted);
  margin: 0 0 calc(var(--space-unit) * 2);
}

.agent-card {
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  background: var(--color-surface);
  padding: calc(var(--space-unit) * 2);
  margin-bottom: calc(var(--space-unit) * 2);
}

.agent-card header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: calc(var(--space-unit) * 1.5);
}

.path-row {
  display: flex;
  gap: var(--space-unit);
  align-items: center;
  flex-wrap: wrap;
}

.path-row input {
  flex: 1;
  min-width: 16rem;
  height: var(--control-height);
  padding: 0 calc(var(--space-unit) * 1.5);
  border: 1px solid var(--color-border-strong);
  border-radius: var(--radius);
  background: var(--color-surface);
  color: var(--color-text);
  font: inherit;
}

.path {
  flex: 1;
  min-width: 16rem;
  color: var(--color-text-muted);
}

button {
  height: var(--control-height);
  padding: 0 calc(var(--space-unit) * 1.5);
  border: 1px solid var(--color-border-strong);
  border-radius: var(--radius);
  background: var(--color-surface);
  color: var(--color-text);
  cursor: pointer;
  font: inherit;
}

.inline {
  display: inline-flex;
  align-items: center;
  gap: var(--space-unit);
}

.status {
  display: flex;
  gap: calc(var(--space-unit) * 1.5);
  color: var(--color-text-muted);
  font-size: 0.857rem;
  margin: calc(var(--space-unit) * 1.5) 0 0;
}

.detection {
  margin: var(--space-unit) 0 0;
  color: var(--color-success);
}

.detection.bad,
.error {
  color: var(--color-danger);
}

.models {
  margin-top: calc(var(--space-unit) * 1.5);
  padding-top: calc(var(--space-unit) * 1.5);
  border-top: 1px solid var(--color-border);
}

.models-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-unit);
}

.model-list {
  list-style: none;
  margin: var(--space-unit) 0 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: calc(var(--space-unit) * 0.4);
}

.model-item {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  gap: calc(var(--space-unit) * 0.75);
  font-size: 0.85rem;
}

.model-name {
  color: var(--color-text);
}

.model-id {
  color: var(--color-text-muted);
  font-size: 0.78rem;
}

.model-variants {
  color: var(--color-text-muted);
  font-size: 0.78rem;
}

.models-line,
.models-updated {
  margin: var(--space-unit) 0 0;
  color: var(--color-text-muted);
  font-size: 0.8rem;
}

.models-line.bad {
  color: var(--color-danger);
}

.diagnostics {
  margin-top: calc(var(--space-unit) * 1.5);
  padding-top: calc(var(--space-unit) * 1.5);
  border-top: 1px solid var(--color-border);
}

.diagnostics summary {
  cursor: pointer;
  display: flex;
  align-items: center;
  gap: var(--space-unit);
  color: var(--color-text-muted);
  font-size: 0.85rem;
}

.status-badge {
  padding: 0.05rem 0.45rem;
  border-radius: 999px;
  border: 1px solid var(--color-border-strong);
  font-size: 0.75rem;
}

.status-ok {
  border-color: var(--color-success-border);
  color: var(--color-success);
}

.status-warning {
  border-color: var(--color-warning-border);
  color: var(--color-warning);
}

.status-error {
  border-color: var(--color-danger-border);
  color: var(--color-danger);
}

.service {
  margin-top: calc(var(--space-unit) * 1.5);
  padding-top: calc(var(--space-unit) * 1.5);
  border-top: 1px solid var(--color-border);
}

.service-state {
  display: flex;
  gap: var(--space-unit);
  align-items: baseline;
  margin: 0;
  color: var(--color-text-muted);
  font-size: 0.857rem;
}

.service-state strong {
  color: var(--color-text);
}

.service-line {
  margin: calc(var(--space-unit) * 0.5) 0 0;
  color: var(--color-text-muted);
  font-size: 0.8rem;
}

.service-line.bad {
  color: var(--color-danger);
}

.service-actions {
  display: flex;
  gap: var(--space-unit);
  margin-top: var(--space-unit);
}

.service-confirm {
  margin-top: var(--space-unit);
  padding: var(--space-unit);
  border: 1px solid var(--color-danger);
  border-radius: var(--radius);
}

.service-confirm p {
  margin: 0 0 var(--space-unit);
}

button.danger {
  border-color: var(--color-danger);
  color: var(--color-danger);
}

button:disabled {
  cursor: not-allowed;
  opacity: 0.5;
}
</style>
