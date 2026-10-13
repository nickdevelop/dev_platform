<script setup lang="ts">
// Agent management section: per-backend platform enablement, executable path
// detection, a manual path fallback when auto-detection fails, and the
// machine-level service block — the one place in the platform that may start or
// stop a third-party service, on an operator's explicit request.
import { onMounted, reactive, ref } from 'vue'
import { useSettingsStore } from '../../stores/settings'
import { useAgentsStore } from '../../stores/agents'
import type { AgentDetection } from '../../api/settings'
import type { AgentServiceStatus } from '../../api/agents'

const settings = useSettingsStore()
const agents = useAgentsStore()
const pathInputs = reactive<Record<string, string>>({})
const detections = reactive<Record<string, AgentDetection | null>>({})
// The backend whose stop is awaiting confirmation, and nothing else. Stopping a
// machine-wide service interrupts every client of it, so the click that opens
// this confirmation is never the click that stops it.
const confirmStop = ref<string | null>(null)

onMounted(async () => {
  void settings.loadAgents()
  await agents.loadBackends()
  // One look per page load, and after that only when the operator presses the
  // button. The service is a third-party process: the platform does not poll it
  // and never acts on its state by itself.
  for (const entry of agents.serviceBackends) {
    void agents.detectService(entry.backend)
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

function toggle(backend: string, value: boolean) {
  void settings.setAgentEnabled(backend, value)
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
    <p class="intro">启用后可在项目中选择并创建新任务。停用只禁止新建，已有任务继续可用。</p>

    <p v-if="settings.agentsError" class="error" role="alert">{{ settings.agentsError }}</p>

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
          <p>这会停止本机所有正在使用该 service 的客户端，包括桌面端 opencode。正在运行的任务会被中断。</p>
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
