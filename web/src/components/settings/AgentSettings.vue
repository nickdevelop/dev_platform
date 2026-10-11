<script setup lang="ts">
// Agent management section: per-backend platform enablement, executable path
// detection, and a manual path fallback when auto-detection fails. An
// auto-detected path is read-only; a missing or manually-set one is editable.
import { onMounted, reactive } from 'vue'
import { useSettingsStore } from '../../stores/settings'
import type { AgentDetection } from '../../api/settings'

const settings = useSettingsStore()
const pathInputs = reactive<Record<string, string>>({})
const detections = reactive<Record<string, AgentDetection | null>>({})

onMounted(() => {
  void settings.loadAgents()
})

function label(backend: string): string {
  if (backend === 'opencode-v2') return 'OpenCode'
  if (backend === 'pi') return 'Pi Agent'
  return backend
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
</style>
