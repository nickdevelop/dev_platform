<script setup lang="ts">
import type { TerminalTab } from '../../stores/terminal'

defineProps<{
  tab: TerminalTab
  active: boolean
}>()

const emit = defineEmits<{
  select: []
  close: []
}>()

function onAuxClick(event: MouseEvent) {
  if (event.button === 1) {
    event.preventDefault()
    emit('close')
  }
}
</script>

<template>
  <div
    class="terminal-tab-item"
    :class="{
      'is-active': active,
      [`status-${tab.status}`]: true,
    }"
    role="tab"
    :aria-selected="active"
    @click="emit('select')"
    @auxclick="onAuxClick"
  >
    <span
      class="status-indicator"
      :class="tab.status"
      :title="`状态: ${tab.status}${tab.errorMessage ? ' - ' + tab.errorMessage : ''}`"
    />
    <span class="tab-title" :title="tab.title">{{ tab.title }}</span>
    <button
      type="button"
      class="tab-close"
      title="关闭终端"
      aria-label="关闭终端"
      @click.stop="emit('close')"
    >
      <svg viewBox="0 0 24 24" class="close-icon" aria-hidden="true">
        <line x1="18" y1="6" x2="6" y2="18" stroke="currentColor" stroke-width="2" stroke-linecap="round"/>
        <line x1="6" y1="6" x2="18" y2="18" stroke="currentColor" stroke-width="2" stroke-linecap="round"/>
      </svg>
    </button>
  </div>
</template>

<style scoped>
.terminal-tab-item {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  height: 36px;
  padding: 0 10px 0 12px;
  background: var(--color-surface);
  color: var(--color-text-muted);
  border-right: 1px solid var(--color-border);
  font-size: 0.929rem;
  cursor: pointer;
  user-select: none;
  position: relative;
  transition: background 0.15s, color 0.15s;
}

.terminal-tab-item:hover {
  background: var(--color-surface-muted);
  color: var(--color-text);
}

.terminal-tab-item.is-active {
  background: var(--color-bg);
  color: var(--color-text);
  font-weight: 500;
  border-bottom: 2px solid var(--color-primary);
}

.status-indicator {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  flex-shrink: 0;
}

.status-indicator.connecting {
  background: #facc15;
  box-shadow: 0 0 4px #facc15;
}

.status-indicator.connected {
  background: #22c55e;
  box-shadow: 0 0 4px #22c55e;
}

.status-indicator.disconnected {
  background: #9ca3af;
}

.status-indicator.error {
  background: #ef4444;
}

.tab-title {
  max-width: 140px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  font-family: var(--font-mono);
  font-size: 0.857rem;
}

.tab-close {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 18px;
  height: 18px;
  padding: 0;
  border: none;
  background: transparent;
  color: inherit;
  border-radius: 3px;
  cursor: pointer;
  opacity: 0.7;
  transition: opacity 0.15s, background 0.15s;
}

.tab-close:hover {
  opacity: 1;
  background: rgba(255, 255, 255, 0.15);
}

.close-icon {
  width: 12px;
  height: 12px;
}
</style>
