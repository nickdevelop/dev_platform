<script setup lang="ts">
import { useRouter } from 'vue-router'

const router = useRouter()

function goToNewSession() {
  void router.push({ name: 'session-new-global' })
}

function goToTerminal() {
  void router.push({ name: 'terminal' })
}
</script>

<template>
  <section class="empty-workspace" aria-label="未选择内容">
    <div class="terminal-card">
      <div class="terminal-header">
        <div class="header-glyph">
          <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="1.8">
            <polyline points="4 17 10 11 4 5" />
            <line x1="12" y1="19" x2="20" y2="19" />
          </svg>
        </div>
        <span class="header-tag">DEV_PLATFORM // HARNESS</span>
        <span class="header-status">STANDBY</span>
      </div>

      <div class="terminal-body">
        <h1 class="main-title">工作台就绪</h1>
        <p class="subtitle">从左侧选择一个项目或会话，或立即开启新的 Agent 协作环境。</p>

        <div class="quick-actions">
          <button type="button" class="action-btn primary" @click="goToNewSession">
            <span class="action-symbol">>_</span>
            <span class="action-label">新建 Agent 会话</span>
            <kbd class="key-hint">Ctrl+N</kbd>
          </button>
          <button type="button" class="action-btn" @click="goToTerminal">
            <span class="action-symbol">$</span>
            <span class="action-label">打开系统终端</span>
            <kbd class="key-hint">PTY</kbd>
          </button>
        </div>
      </div>

      <div class="terminal-footer">
        <span class="meta-item">BACKENDS: OPENCODE · PI</span>
        <span class="meta-divider">|</span>
        <span class="meta-item">STREAM: SSE TYPED</span>
        <span class="meta-divider">|</span>
        <span class="meta-item">STATE: READY</span>
      </div>
    </div>
  </section>
</template>

<style scoped>
.empty-workspace {
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 1.5rem;
  background: var(--color-bg);
  user-select: none;
}

.terminal-card {
  width: min(34rem, 100%);
  border: 1px solid var(--color-border);
  border-radius: 4px;
  background: var(--color-surface);
  overflow: hidden;
}

.terminal-header {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.5rem 0.85rem;
  border-bottom: 1px solid var(--color-border);
  background: var(--color-surface-muted);
  font-size: 0.75rem;
  color: var(--color-text-muted);
}

.header-glyph {
  display: flex;
  align-items: center;
  color: var(--color-accent);
}

.header-tag {
  font-weight: 600;
  letter-spacing: 0.04em;
  color: var(--color-text);
}

.header-status {
  margin-left: auto;
  font-size: 0.7rem;
  color: var(--color-success);
  padding: 1px 6px;
  border: 1px solid var(--color-border);
  border-radius: 2px;
  background: var(--color-surface);
}

.terminal-body {
  padding: 1.5rem;
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.main-title {
  margin: 0;
  font-size: 1.15rem;
  font-weight: 600;
  color: var(--color-text);
  line-height: 1.3;
}

.subtitle {
  margin: 0;
  font-size: 0.85rem;
  color: var(--color-text-muted);
  line-height: 1.5;
}

.quick-actions {
  display: flex;
  flex-direction: column;
  gap: 0.45rem;
  margin-top: 0.5rem;
}

.action-btn {
  display: flex;
  align-items: center;
  gap: 0.65rem;
  padding: 0.55rem 0.85rem;
  border: 1px solid var(--color-border);
  border-radius: 3px;
  background: var(--color-surface-muted);
  color: var(--color-text);
  cursor: pointer;
  font: inherit;
  font-size: 0.85rem;
  text-align: left;
  border-left: 2px solid transparent;
  transition: color 0.12s ease-out, background-color 0.12s ease-out, border-color 0.12s ease-out;
}

.action-btn:hover {
  background: var(--color-surface);
  border-left-color: var(--color-accent);
  border-color: var(--color-border-strong);
}

.action-btn.primary {
  border-left-color: var(--color-accent);
}

.action-symbol {
  color: var(--color-accent);
  font-weight: 600;
  width: 1.2rem;
}

.action-label {
  flex: 1;
}

.key-hint {
  font-size: 0.72rem;
  padding: 1px 5px;
  border: 1px solid var(--color-border-strong);
  border-radius: 2px;
  background: var(--color-surface);
  color: var(--color-text-muted);
}

.terminal-footer {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.45rem 0.85rem;
  border-top: 1px solid var(--color-border);
  background: var(--color-surface-muted);
  font-size: 0.7rem;
  color: var(--color-text-muted);
}

.meta-divider {
  color: var(--color-border);
}
</style>
