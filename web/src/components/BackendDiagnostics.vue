<script setup lang="ts">
// Per-backend startup diagnostics panel: overall status, detected vs tested
// version, the individual redacted checks and the declared capability matrix.
// The panel is read-only: it never presents a backend as usable when its
// diagnostics say otherwise.
import type { AgentInstance, BackendDiagnostics, BackendId, Capabilities } from '../api/agents'

const props = defineProps<{
  diagnostics: BackendDiagnostics[]
  // Running instances of the project. They are listed here, and only here:
  // the workbench hides instances from the task flow and keeps the start/stop
  // verbs in the pane an operator opens on purpose.
  instances?: AgentInstance[]
  // capabilitiesFor gates the stop control: a backend that cannot stop an
  // instance must not be offered one.
  capabilitiesFor?: (backend: BackendId) => Capabilities | undefined
}>()

const emit = defineEmits<{ stop: [instanceId: string] }>()

const instancesOf = (): AgentInstance[] => props.instances ?? []

function canStop(instance: AgentInstance): boolean {
  if (instance.status !== 'running' && instance.status !== 'starting') return false
  return props.capabilitiesFor?.(instance.backend)?.stop === true
}

const statusLabels: Record<string, string> = {
  ok: '正常',
  warning: '警告',
  error: '错误',
  unknown: '未知',
}

const kindLabels: Record<string, string> = {
  executable: '可执行文件',
  version: '版本兼容',
  api: 'API 连通',
  event_stream: '事件流',
  adapter: '适配器',
}

const capabilityLabels: Record<string, string> = {
  streaming: '流式输出',
  sessionDiscovery: '会话发现',
  sessionContinue: '会话续聊',
  stop: '停止实例',
  modelSelection: '模型选择',
  toolCallEvents: '工具事件',
  toolConfirmation: '工具确认',
}

const capabilityOrder = [
  'streaming',
  'sessionDiscovery',
  'sessionContinue',
  'stop',
  'modelSelection',
  'toolCallEvents',
  'toolConfirmation',
] as const

function versionLine(entry: BackendDiagnostics): string {
  if (entry.version && entry.testedVersion) {
    return `检测到 ${entry.version}（已验证 ${entry.testedVersion}）`
  }
  if (entry.testedVersion) {
    return `已验证 ${entry.testedVersion}`
  }
  return ''
}

// checksOf tolerates a payload without a checks array (for example an older
// or hand-written response) so the panel never throws on a missing array.
function checksOf(entry: BackendDiagnostics): BackendDiagnostics['checks'] {
  return entry.checks ?? []
}
</script>

<template>
  <section class="diagnostics" aria-label="后端诊断">
    <h2>后端诊断</h2>
    <ul class="backend-list">
      <li
        v-for="entry in props.diagnostics"
        :key="entry.backend"
        class="backend-item"
        :class="`backend-${entry.status}`"
        :data-backend-diagnostics="entry.backend"
      >
        <div class="backend-head">
          <span class="backend-name">{{ entry.backend }}</span>
          <span class="badge" :class="`badge-${entry.status}`" :data-diagnostics-status="entry.status">
            {{ statusLabels[entry.status] ?? entry.status }}
          </span>
          <span v-if="versionLine(entry)" class="version">{{ versionLine(entry) }}</span>
        </div>

        <ul class="checks">
          <li
            v-for="(check, index) in checksOf(entry)"
            :key="`${entry.backend}-${check.kind}-${index}`"
            class="check"
            :data-diagnostic-kind="check.kind"
          >
            <span class="check-kind">{{ kindLabels[check.kind] ?? check.kind }}</span>
            <span class="badge" :class="`badge-${check.status}`">{{ statusLabels[check.status] ?? check.status }}</span>
            <span class="check-message">{{ check.message }}</span>
          </li>
          <li v-if="checksOf(entry).length === 0" class="check check-empty">该后端未提供诊断信息。</li>
        </ul>

        <div v-if="entry.capabilities" class="capabilities">
          <span
            v-for="capability in capabilityOrder"
            :key="capability"
            class="capability"
            :class="{ off: !entry.capabilities[capability] }"
            :title="entry.capabilities[capability] ? '支持' : '该后端不支持此能力'"
          >
            {{ capabilityLabels[capability] ?? capability }}{{ entry.capabilities[capability] ? '' : ' ✕' }}
          </span>
        </div>
      </li>
      <li v-if="props.diagnostics.length === 0" class="empty">暂无后端诊断信息。</li>
    </ul>

    <div v-if="instancesOf().length > 0" class="instances" data-test="instance-list">
      <h3>运行中实例</h3>
      <ul class="instance-items">
        <li v-for="instance in instancesOf()" :key="instance.id" class="instance-item">
          <span class="instance-backend">{{ instance.backend }}</span>
          <span class="instance-status">{{ instance.status }}</span>
          <span class="instance-id">{{ instance.id }}</span>
          <span class="instance-directory">{{ instance.directory }}</span>
          <button
            v-if="canStop(instance)"
            type="button"
            data-test="stop-instance"
            @click="emit('stop', instance.id)"
          >
            停止服务
          </button>
        </li>
      </ul>
    </div>
  </section>
</template>

<style scoped>
.diagnostics {
  margin: 1rem 0;
}

.diagnostics h2 {
  font-size: 1.05rem;
  margin: 0 0 0.5rem;
}

.backend-list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 0.6rem;
}

.backend-item {
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  padding: 0.6rem 0.75rem;
}

.backend-error {
  border-color: #fca5a5;
}

.backend-head {
  display: flex;
  align-items: baseline;
  gap: 0.6rem;
  flex-wrap: wrap;
}

.backend-name {
  font-weight: 600;
}

.badge {
  font-size: 0.75rem;
  padding: 0.05rem 0.45rem;
  border-radius: 999px;
  border: 1px solid #cbd5e1;
  color: #475569;
}

.badge-ok {
  border-color: #86efac;
  color: #15803d;
}

.badge-warning {
  border-color: #fcd34d;
  color: #92400e;
}

.badge-error {
  border-color: #fca5a5;
  color: #b91c1c;
}

.version {
  color: #64748b;
  font-size: 0.85rem;
}

.checks {
  list-style: none;
  margin: 0.5rem 0 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
}

.check {
  display: flex;
  gap: 0.5rem;
  align-items: baseline;
  font-size: 0.85rem;
}

.check-kind {
  color: #334155;
  min-width: 5.5rem;
}

.check-message {
  color: #475569;
  word-break: break-word;
}

.check-empty {
  color: #94a3b8;
}

.capabilities {
  display: flex;
  flex-wrap: wrap;
  gap: 0.35rem;
  margin-top: 0.6rem;
}

.capability {
  font-size: 0.75rem;
  padding: 0.1rem 0.45rem;
  border-radius: 6px;
  background: #ecfdf5;
  color: #047857;
}

.capability.off {
  background: #f1f5f9;
  color: #94a3b8;
}

.empty {
  color: #64748b;
}

.instances {
  margin-top: 0.75rem;
}

.instances h3 {
  font-size: 0.95rem;
  margin: 0 0 0.4rem;
}

.instance-items {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 0.3rem;
}

.instance-item {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  flex-wrap: wrap;
  font-size: 0.85rem;
  color: #475569;
}

.instance-backend,
.instance-status {
  padding: 0.05rem 0.4rem;
  border-radius: 999px;
  background: #f1f5f9;
}

.instance-id,
.instance-directory {
  font-family: monospace;
  color: #64748b;
}

.instance-item button {
  padding: 0.15rem 0.5rem;
  border: 1px solid #fca5a5;
  border-radius: 6px;
  background: #fff;
  color: #b91c1c;
  cursor: pointer;
}
</style>
