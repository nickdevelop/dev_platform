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
          <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
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
          <button type="button" class="action-btn primary group" @click="goToNewSession">
            <span class="action-symbol">
              <svg viewBox="0 0 24 24" width="15" height="15" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round">
                <polyline points="9 18 15 12 9 6" />
              </svg>
            </span>
            <span class="action-label">新建 Agent 会话</span>
            <kbd class="key-hint">Ctrl+N</kbd>
          </button>
          <button type="button" class="action-btn group" @click="goToTerminal">
            <span class="action-symbol">
              <svg viewBox="0 0 24 24" width="15" height="15" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <polyline points="4 17 10 11 4 5" />
                <line x1="12" y1="19" x2="20" y2="19" />
              </svg>
            </span>
            <span class="action-label">打开系统终端</span>
            <kbd class="key-hint">PTY</kbd>
          </button>
        </div>
      </div>

      <div class="terminal-footer">
        <span class="meta-item">BACKENDS: OPENCODE · PI</span>
        <span class="meta-divider">/</span>
        <span class="meta-item">STREAM: SSE TYPED</span>
        <span class="meta-divider">/</span>
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
  border-radius: 8px;
  background: var(--color-surface);
  box-shadow: 0 4px 20px -2px rgba(0, 0, 0, 0.08), 0 2px 6px -1px rgba(0, 0, 0, 0.04);
  overflow: hidden;
  transition: box-shadow 0.2s ease, border-color 0.2s ease;
}

.terminal-card:hover {
  border-color: var(--color-border-strong);
  box-shadow: 0 8px 30px -4px rgba(0, 0, 0, 0.12), 0 4px 10px -2px rgba(0, 0, 0, 0.06);
}

.terminal-header {
  display: flex;
  align-items: center;
  gap: 0.6rem;
  padding: 0.65rem 1rem;
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
  letter-spacing: 0.06em;
  font-family: var(--font-mono, monospace);
  color: var(--color-text);
  font-size: var(--text-size-11);
}

.header-status {
  margin-left: auto;
  font-size: var(--text-size-meta);
  font-family: var(--font-mono, monospace);
  font-weight: 600;
  color: var(--color-success);
  padding: 2px 8px;
  border: 1px solid var(--color-success-border, var(--color-border));
  border-radius: 9999px;
  background: var(--color-success-soft, var(--color-surface));
}

.terminal-body {
  padding: 1.75rem;
  display: flex;
  flex-direction: column;
  gap: 0.85rem;
}

.main-title {
  margin: 0;
  font-size: 1.25rem;
  font-weight: 600;
  color: var(--color-text);
  letter-spacing: -0.01em;
  line-height: 1.3;
}

.subtitle {
  margin: 0;
  font-size: 0.875rem;
  color: var(--color-text-muted);
  line-height: 1.6;
}

.quick-actions {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  margin-top: 0.5rem;
}

.action-btn {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  padding: 0.65rem 0.95rem;
  border: 1px solid var(--color-border);
  border-radius: 6px;
  background: var(--color-surface);
  color: var(--color-text);
  cursor: pointer;
  font: inherit;
  font-size: 0.875rem;
  text-align: left;
  border-left: 3px solid transparent;
  transition: transform 0.15s ease, background-color 0.15s ease, border-color 0.15s ease, box-shadow 0.15s ease;
}

.action-btn:hover {
  background: var(--color-surface-muted);
  border-color: var(--color-border-strong);
  border-left-color: var(--color-accent);
  transform: translateY(-1px);
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.05);
}

.action-btn:active {
  transform: translateY(0);
}

.action-btn.primary {
  border-left-color: var(--color-accent);
  background: var(--color-surface);
}

.action-btn.primary:hover {
  background: var(--color-surface-muted);
}

.action-symbol {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  color: var(--color-accent);
  width: 1.2rem;
  flex-shrink: 0;
}

.action-label {
  flex: 1;
  font-weight: 500;
}

.key-hint {
  font-size: var(--text-size-11);
  font-family: var(--font-mono, monospace);
  padding: 2px 6px;
  border: 1px solid var(--color-border-strong);
  border-radius: 4px;
  background: var(--color-surface-muted);
  color: var(--color-text-muted);
  box-shadow: 0 1px 0 rgba(0, 0, 0, 0.05);
}

.terminal-footer {
  display: flex;
  align-items: center;
  gap: 0.6rem;
  padding: 0.55rem 1rem;
  border-top: 1px solid var(--color-border);
  background: var(--color-surface-muted);
  font-size: var(--text-size-meta);
  font-family: var(--font-mono, monospace);
  color: var(--color-text-muted);
}

.meta-divider {
  color: var(--color-border-strong);
}
</style>
