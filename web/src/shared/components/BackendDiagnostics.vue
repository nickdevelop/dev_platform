<script setup lang="ts">
// Per-backend startup diagnostics panel: overall status, detected vs tested
// version, the individual redacted checks and the declared capability matrix.
// The panel is read-only: it never presents a backend as usable when its
// diagnostics say otherwise. Instances are not shown here — they are an
// internal detail of how a backend serves the workbench, and the operator who
// needs to act on a backend acts on its service, in platform settings.
import type { BackendDiagnostics } from '../../api/agents'

const props = defineProps<{
  diagnostics: BackendDiagnostics[]
  // compact drops the page-level heading and outer margin so the panel can sit
  // inside one agent card. The Agent settings page renders one entry per card.
  compact?: boolean
}>()

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
  stop: '停止会话',
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
  <section class="diagnostics" :class="{ compact: props.compact }" aria-label="后端诊断">
    <h2 v-if="!props.compact">后端诊断</h2>
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
            {{ capabilityLabels[capability] ?? capability }}{{ entry.capabilities[capability] ? '' : ' ✕' }} <!-- taste-skill-ignore -->
          </span>
        </div>
      </li>
      <li v-if="props.diagnostics.length === 0" class="empty">暂无后端诊断信息。</li>
    </ul>

  </section>
</template>

<style scoped>
.diagnostics {
  margin: 1rem 0;
}

.diagnostics.compact {
  margin: 0.5rem 0 0;
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
  border: 1px solid var(--color-border);
  border-radius: 8px;
  padding: 0.6rem 0.75rem;
}

.backend-error {
  border-color: var(--color-danger-border);
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
  border: 1px solid var(--color-border-strong);
  color: var(--color-text-muted);
}

.badge-ok {
  border-color: var(--color-success-border);
  color: var(--color-success);
}

.badge-warning {
  border-color: var(--color-warning-border);
  color: var(--color-warning);
}

.badge-error {
  border-color: var(--color-danger-border);
  color: var(--color-danger);
}

.version {
  color: var(--color-text-muted);
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
  color: var(--color-text);
  min-width: 5.5rem;
}

.check-message {
  color: var(--color-text-muted);
  word-break: break-word;
}

.check-empty {
  color: var(--color-text-muted);
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
  background: var(--color-success-soft);
  color: var(--color-success);
}

.capability.off {
  background: var(--color-surface-muted);
  color: var(--color-text-muted);
}

.empty {
  color: var(--color-text-muted);
}







</style>
