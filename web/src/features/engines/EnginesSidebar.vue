<script setup lang="ts">
// Engines navigation sidebar in AppShell's middle column.
// Conforms to design-taste-frontend (Data Terminal / App Screens) compact rules:
// - Direct alignment with AppShell's sidebar-head
// - Active navigation uses 2px accent indicator on the left edge
// - Flat surfaces, zero resting shadows, tabular descriptions
import { RouterLink } from 'vue-router'

const links = [
  {
    name: 'engines-assistant',
    label: '助手引擎',
    desc: '对话助手人设与平台管理工具',
    icon: 'assistant',
  },
  {
    name: 'engines-coder',
    label: '会话引擎',
    desc: '工作区代码生成与终端执行',
    icon: 'coder',
  },
  {
    name: 'engines-orchestrator',
    label: '协同引擎',
    desc: '多 Agent 流程拆解与分派',
    icon: 'orchestrator',
  },
  {
    name: 'engines-agents',
    label: 'Agent配置',
    desc: '异构 Agent 后端与服务管理',
    icon: 'agents',
  },
]
</script>

<template>
  <nav class="engines-nav sidebar-panel" data-test="engines-nav" aria-label="引擎分类">
    <div class="engines-links sidebar-list-container" role="list">
      <RouterLink
        v-for="link in links"
        :key="link.name"
        :to="{ name: link.name }"
        class="engines-link sidebar-card group"
        role="listitem"
      >
        <div class="sidebar-card-row-top">
          <div class="flex items-center gap-2 min-w-0 flex-1">
            <span class="link-icon" aria-hidden="true">
              <!-- SVG Icons -->
              <svg v-if="link.icon === 'assistant'" class="w-3.5 h-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M21 11.5a8.38 8.38 0 0 1-.9 3.8 8.5 8.5 0 0 1-7.6 4.7 8.38 8.38 0 0 1-3.8-.9L3 21l1.9-5.7a8.38 8.38 0 0 1-.9-3.8 8.5 8.5 0 0 1 4.7-7.6 8.38 8.38 0 0 1 3.8-.9h.5a8.48 8.48 0 0 1 8 8z" />
              </svg>
              <svg v-else-if="link.icon === 'coder'" class="w-3.5 h-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <polyline points="16 18 22 12 16 6" />
                <polyline points="8 6 2 12 8 18" />
              </svg>
              <svg v-else-if="link.icon === 'orchestrator'" class="w-3.5 h-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <circle cx="12" cy="12" r="3" />
                <circle cx="19" cy="5" r="2" />
                <circle cx="5" cy="5" r="2" />
                <circle cx="19" cy="19" r="2" />
                <circle cx="5" cy="19" r="2" />
                <line x1="12" y1="9" x2="12" y2="3" />
                <line x1="14" y1="13.5" x2="17.5" y2="17.5" />
                <line x1="10" y1="13.5" x2="6.5" y2="17.5" />
              </svg>
              <svg v-else-if="link.icon === 'agents'" class="w-3.5 h-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <rect x="4" y="4" width="16" height="16" rx="2" />
                <rect x="9" y="9" width="6" height="6" />
                <line x1="9" y1="1" x2="9" y2="4" />
                <line x1="15" y1="1" x2="15" y2="4" />
                <line x1="9" y1="20" x2="9" y2="23" />
                <line x1="15" y1="20" x2="15" y2="23" />
                <line x1="20" y1="9" x2="23" y2="9" />
                <line x1="20" y1="14" x2="23" y2="14" />
                <line x1="1" y1="9" x2="4" y2="9" />
                <line x1="1" y1="14" x2="4" y2="14" />
              </svg>
              <svg v-else class="w-3.5 h-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <rect x="2" y="2" width="20" height="20" rx="4" />
                <path d="M9 9h6v6H9z" />
                <path d="M9 2v3M15 2v3M9 19v3M15 19v3M2 9h3M2 15h3M19 9h3M19 15h3" />
              </svg>
            </span>
            <span class="link-label sidebar-card-title">{{ link.label }}</span>
          </div>
        </div>
        <p class="link-desc sidebar-card-desc">{{ link.desc }}</p>
      </RouterLink>
    </div>
  </nav>
</template>

<style scoped>
.engines-nav {
  display: flex;
  flex-direction: column;
  min-height: 0;
  flex: 1;
  overflow: hidden;
}

.engines-links {
  display: flex;
  flex-direction: column;
  gap: 3px;
  padding: 4px 8px 16px 8px;
}

.link-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 16px;
  height: 16px;
  color: var(--color-text-muted);
  flex-shrink: 0;
  transition: color 0.12s ease;
}

.engines-link.router-link-active .link-icon {
  color: var(--color-accent);
}

.sidebar-card-title {
  font-size: 0.8125rem;
  font-weight: 500;
  color: var(--color-text);
}

.sidebar-card-desc {
  font-size: 0.6875rem;
  color: var(--color-text-muted);
  line-height: 1.3;
  margin: 0;
  padding-left: 24px;
}
</style>
