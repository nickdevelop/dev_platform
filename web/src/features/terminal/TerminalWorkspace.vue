<script setup lang="ts">
import { computed } from 'vue'
import { useTerminalStore } from '../../stores/terminal'
import TerminalTabItem from './TerminalTabItem.vue'
import XtermView from './XtermView.vue'

const props = withDefaults(
  defineProps<{
    visible?: boolean
  }>(),
  {
    visible: true,
  },
)

const terminalStore = useTerminalStore()

const tabs = computed(() => terminalStore.tabs)
const activeTabId = computed(() => terminalStore.activeTabId)

function onSelectTab(tabId: string) {
  terminalStore.setActiveTab(tabId)
}

function onCloseTab(tabId: string) {
  terminalStore.closeTab(tabId)
}

function onNewTab() {
  const targetServerId = terminalStore.activeServerId ?? 0
  terminalStore.openTerminal(targetServerId)
}

function onQuickConnectLocal() {
  terminalStore.selectServer(0)
  terminalStore.openTerminal(0)
}
</script>

<template>
  <div class="terminal-workspace" data-test="terminal-workspace">
    <!-- 1. Terminal Tab Strip (when tabs are open) -->
    <div v-if="tabs.length > 0" class="terminal-tab-strip" role="tablist">
      <div class="tab-list">
        <TerminalTabItem
          v-for="tab in tabs"
          :key="tab.id"
          :tab="tab"
          :active="tab.id === activeTabId"
          @select="onSelectTab(tab.id)"
          @close="onCloseTab(tab.id)"
        />
      </div>
      <button
        type="button"
        class="btn-new-tab"
        title="打开新终端标签"
        @click="onNewTab"
      >
        <svg viewBox="0 0 24 24" class="plus-icon" aria-hidden="true">
          <line x1="12" y1="5" x2="12" y2="19" stroke="currentColor" stroke-width="2" stroke-linecap="round"/>
          <line x1="5" y1="12" x2="19" y2="12" stroke="currentColor" stroke-width="2" stroke-linecap="round"/>
        </svg>
      </button>
    </div>

    <!-- 2. Terminal Panes (Preserves active WebSocket & DOM using v-show) -->
    <div v-if="tabs.length > 0" class="terminal-viewport">
      <div
        v-for="tab in tabs"
        :key="tab.id"
        v-show="tab.id === activeTabId"
        class="terminal-pane"
      >
        <XtermView
          :tab-id="tab.id"
          :server-id="tab.serverId"
          :active="tab.id === activeTabId"
          :visible="props.visible"
        />
      </div>
    </div>

    <!-- 3. Empty State (when no tabs are open) -->
    <div v-else class="empty-terminal-state">
      <div class="empty-card">
        <div class="empty-icon">
          <svg viewBox="0 0 24 24" class="console-icon">
            <polyline points="4 17 10 11 4 5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
            <line x1="12" y1="19" x2="20" y2="19" stroke="currentColor" stroke-width="2" stroke-linecap="round"/>
          </svg>
        </div>
        <h3>终端控制台</h3>
        <p class="empty-desc">
          在平台上直接通过命令行方式连接管理服务器。请在左侧列表中选择服务器并点击「连接」以打开终端。
        </p>

        <div v-if="!terminalStore.isSupported" class="platform-alert flex items-center justify-center gap-1.5" role="alert">
          <svg class="shrink-0 text-amber-500" viewBox="0 0 16 16" width="14" height="14" fill="none" stroke="currentColor" stroke-width="1.8">
            <path d="M8 2l6.5 11.5H1.5L8 2z" />
            <line x1="8" y1="6" x2="8" y2="9.5" />
            <circle cx="8" cy="11.5" r="0.75" fill="currentColor" />
          </svg>
          <span>当前环境非 Linux，本地终端暂不支持；您仍可添加并连接第三方 SSH 服务器。</span>
        </div>

        <div class="empty-actions">
          <button
            type="button"
            class="btn-primary-action"
            :disabled="!terminalStore.isSupported"
            @click="onQuickConnectLocal"
          >
            连接本机终端
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.terminal-workspace {
  display: flex;
  flex-direction: column;
  width: 100%;
  height: 100%;
  background: var(--color-bg, #14151a);
  overflow: hidden;
}

.terminal-tab-strip {
  display: flex;
  align-items: center;
  background: var(--color-surface, #1a1b22);
  border-bottom: 1px solid var(--color-border, #2d2e38);
  overflow-x: auto;
  overflow-y: hidden;
  height: 36px;
  flex-shrink: 0;
}

.tab-list {
  display: flex;
  align-items: center;
  height: 100%;
}

.btn-new-tab {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  margin-left: 6px;
  background: transparent;
  border: none;
  color: var(--color-text-muted, #9ca3af);
  border-radius: 4px;
  cursor: pointer;
  transition: color 0.15s, background 0.15s;
}

.btn-new-tab:hover {
  color: var(--color-text, #fff);
  background: rgba(255, 255, 255, 0.08);
}

.plus-icon {
  width: 14px;
  height: 14px;
}

.terminal-viewport {
  flex: 1;
  width: 100%;
  height: calc(100% - 36px);
  position: relative;
  overflow: hidden;
}

.terminal-pane {
  width: 100%;
  height: 100%;
}

.empty-terminal-state {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 32px;
}

.empty-card {
  max-width: 440px;
  text-align: center;
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  padding: 36px 32px;
  box-shadow: 0 4px 20px rgba(0, 0, 0, 0.15);
}

.empty-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 56px;
  height: 56px;
  background: var(--color-primary-soft);
  color: var(--color-primary);
  border-radius: var(--radius);
  margin-bottom: 16px;
}

.console-icon {
  width: 28px;
  height: 28px;
}

.empty-card h3 {
  font-size: 1.25rem;
  font-weight: 600;
  margin: 0 0 10px 0;
  color: var(--color-text);
}

.empty-desc {
  font-size: 0.929rem;
  color: var(--color-text-muted);
  line-height: 1.6;
  margin: 0 0 20px 0;
}

.platform-alert {
  padding: 10px 14px;
  background: var(--color-warning-soft);
  border: 1px solid var(--color-warning-border);
  border-radius: var(--radius);
  font-size: 0.8rem;
  color: var(--color-warning);
  line-height: 1.5;
  margin-bottom: 20px;
  text-align: left;
}

.empty-actions {
  display: flex;
  justify-content: center;
  gap: 12px;
}

.btn-primary-action {
  padding: 8px 20px;
  background: var(--color-primary);
  color: #fff;
  border: none;
  border-radius: var(--radius);
  font-size: 0.929rem;
  font-weight: 500;
  cursor: pointer;
  transition: opacity 0.15s;
}

.btn-primary-action:hover:not(:disabled) {
  opacity: 0.9;
}

.btn-primary-action:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
</style>
