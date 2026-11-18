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
          <svg viewBox="0 0 24 24" width="15" height="15" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <polyline points="4 17 10 11 4 5" />
            <line x1="12" y1="19" x2="20" y2="19" />
          </svg>
        </div>
        <span class="header-tag">工程工作台</span>
        <span class="header-status">
          <span class="status-dot" aria-hidden="true"></span>
          就绪
        </span>
      </div>

      <div class="terminal-body">
        <h1 class="main-title">工作台就绪</h1>
        <p class="subtitle">从左侧选择一个项目或会话，或通过快捷方式直接开启协作</p>

        <div class="quick-actions">
          <button type="button" class="action-btn primary" @click="goToNewSession">
            <span class="action-symbol">
              <svg viewBox="0 0 24 24" width="15" height="15" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <polyline points="9 18 15 12 9 6" />
              </svg>
            </span>
            <span class="action-label">新建 Agent 会话</span>
            <kbd class="key-hint">Ctrl+N</kbd>
          </button>
          <button type="button" class="action-btn" @click="goToTerminal">
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
        <span class="meta-item">快捷键</span>
        <span class="meta-divider">·</span>
        <span class="meta-item">Ctrl+N 新建会话</span>
        <span class="meta-divider">·</span>
        <span class="meta-item">多智能体协同就绪</span>
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
  width: min(32rem, 100%);
  border: 1px solid var(--color-border);
  border-radius: 8px;
  background: var(--color-surface);
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.04);
  overflow: hidden;
  transition: border-color 0.15s ease;
}

.terminal-card:hover {
  border-color: var(--color-border-strong);
}

.terminal-header {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.65rem 1rem;
  border-bottom: 1px solid var(--color-border);
  background: var(--color-surface-muted);
  font-size: 0.75rem;
  color: var(--color-text-muted);
}

.header-glyph {
  display: flex;
  align-items: center;
  color: var(--color-primary);
}

.header-tag {
  font-weight: 500;
  color: var(--color-text);
  font-size: var(--text-size-12);
}

.header-status {
  margin-left: auto;
  display: inline-flex;
  align-items: center;
  gap: 0.35rem;
  font-size: var(--text-size-meta);
  font-weight: 500;
  color: var(--color-text-muted);
  padding: 1px 6px;
  border: 1px solid var(--color-border);
  border-radius: 4px;
  background: var(--color-surface);
}

.status-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--color-success);
  display: inline-block;
}

.terminal-body {
  padding: 1.5rem 1.75rem;
  display: flex;
  flex-direction: column;
  gap: 0.85rem;
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
  font-size: 0.875rem;
  color: var(--color-text-muted);
  line-height: 1.5;
}

.quick-actions {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  margin-top: 0.35rem;
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
  transition: background-color 0.15s ease, border-color 0.15s ease;
}

.action-btn:hover {
  background: var(--color-surface-muted);
  border-color: var(--color-border-strong);
}

.action-btn:active {
  background: var(--color-border);
}

.action-symbol {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  color: var(--color-text-muted);
  width: 1.2rem;
  flex-shrink: 0;
  transition: color 0.15s ease;
}

.action-btn:hover .action-symbol {
  color: var(--color-primary);
}

.action-label {
  flex: 1;
  font-weight: 500;
}

.key-hint {
  font-size: var(--text-size-11);
  font-family: var(--font-mono, monospace);
  padding: 2px 6px;
  border: 1px solid var(--color-border);
  border-radius: 4px;
  background: var(--color-surface-muted);
  color: var(--color-text-muted);
}

.terminal-footer {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.5rem 1rem;
  border-top: 1px solid var(--color-border);
  background: var(--color-surface-muted);
  font-size: var(--text-size-meta);
  color: var(--color-text-muted);
}

.meta-divider {
  color: var(--color-border);
}
</style>
