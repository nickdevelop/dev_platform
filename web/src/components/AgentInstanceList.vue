<script setup lang="ts">
// Lists a project's agent instances. Each instance is independent; the stop
// control appears only when that backend declares the stop capability.
import type { AgentInstance, Capabilities } from '../api/agents'

const props = defineProps<{
  instances: AgentInstance[]
  capabilitiesFor: (backend: string) => Capabilities | null
}>()

const emit = defineEmits<{ stop: [instanceId: string]; discover: [instanceId: string] }>()

const statusLabels: Record<string, string> = {
  starting: '启动中',
  running: '运行中',
  stopped: '已停止',
  failed: '启动失败',
}

function canStop(backend: string, status: string): boolean {
  return status === 'running' && props.capabilitiesFor(backend)?.stop === true
}

function canDiscover(backend: string, status: string): boolean {
  return status === 'running' && props.capabilitiesFor(backend)?.sessionDiscovery === true
}
</script>

<template>
  <ul class="instance-list">
    <li v-for="instance in props.instances" :key="instance.id" class="instance-item">
      <div class="instance-info">
        <span class="backend">{{ instance.backend }}</span>
        <span class="status" :class="`status-${instance.status}`">
          {{ statusLabels[instance.status] ?? instance.status }}
        </span>
        <span class="directory">{{ instance.directory }}</span>
        <span v-if="instance.error" class="instance-error" role="alert">{{ instance.error }}</span>
      </div>
      <div class="instance-actions">
        <button
          v-if="canDiscover(instance.backend, instance.status)"
          type="button"
          class="discover"
          data-test="discover-sessions"
          @click="emit('discover', instance.id)"
        >
          发现原生会话
        </button>
        <button v-if="canStop(instance.backend, instance.status)" type="button" class="stop" @click="emit('stop', instance.id)">
          停止
        </button>
      </div>
    </li>
    <li v-if="props.instances.length === 0" class="empty">尚未启动任何 Agent 实例。</li>
  </ul>
</template>

<style scoped>
.instance-list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
}

.instance-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 1rem;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  padding: 0.5rem 0.75rem;
}

.instance-info {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
  align-items: baseline;
  min-width: 0;
}

.backend {
  font-weight: 600;
}

.status {
  font-size: 0.8rem;
  color: #475569;
}

.status-running {
  color: #15803d;
}

.status-failed {
  color: #b91c1c;
}

.directory {
  color: #64748b;
  font-size: 0.85rem;
  word-break: break-all;
}

.instance-error {
  color: #b91c1c;
  font-size: 0.85rem;
  width: 100%;
}

.instance-actions {
  display: flex;
  align-items: center;
  gap: 0.4rem;
}

.instance-item button {
  padding: 0.3rem 0.65rem;
  border: 1px solid #fca5a5;
  border-radius: 6px;
  background: #fff;
  color: #b91c1c;
  cursor: pointer;
}

.instance-item button.discover {
  color: #1d4ed8;
  border-color: #bfdbfe;
}

.empty {
  color: #64748b;
}
</style>
