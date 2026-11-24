<script setup lang="ts">
// OrgSidebar: The middle column for the Organization center.
// Second-level navigation: AI Assistants, AI Employees, and AI Teams.
// Pure object governance with zero transactional work.
// Follows design-taste-frontend (Data Terminal / App Screens) compact rules:
//   - Aligned directly with AppShell's sidebar-head (no redundant double header)
//   - The Move: active navigation strictly uses 2px accent indicator on the left edge
import { RouterLink } from 'vue-router'

const links = [
  {
    name: 'org-assistants',
    label: 'AI 助手',
    desc: '日常人设与自治工具管理',
    icon: 'assistant',
  },
  {
    name: 'org-employees',
    label: 'AI 员工',
    desc: '专职数字员工与执行引擎分工',
    icon: 'employee',
  },
  {
    name: 'org-teams',
    label: 'AI 团队',
    desc: '中控大脑调度与多专家协同',
    icon: 'team',
  },
]
</script>

<template>
  <nav class="org-sidebar org-nav sidebar-panel" data-test="org-sidebar" aria-label="组织架构管理">
    <!-- Zone C: Category navigation links list -->
    <div class="org-links sidebar-list-container" role="list">
      <RouterLink
        v-for="link in links"
        :key="link.name"
        :to="{ name: link.name }"
        class="org-link sidebar-card group"
        role="listitem"
      >
        <div class="sidebar-card-row-top">
          <div class="flex items-center gap-2 min-w-0 flex-1">
            <span class="link-icon" aria-hidden="true">
              <!-- AI Assistant Icon -->
              <svg v-if="link.icon === 'assistant'" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
                <path d="M12 2a2 2 0 0 1 2 2v2a2 2 0 0 1-2 2 2 2 0 0 1-2-2V4a2 2 0 0 1 2-2z" />
                <rect x="4" y="8" width="16" height="12" rx="2" />
                <circle cx="9" cy="13" r="1.25" fill="currentColor" />
                <circle cx="15" cy="13" r="1.25" fill="currentColor" />
                <path d="M9 17h6" />
                <path d="M2 14h2M20 14h2" />
              </svg>
              <!-- AI Employee / Role Icon -->
              <svg v-else-if="link.icon === 'employee'" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
                <path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2" />
                <circle cx="9" cy="7" r="4" />
                <path d="M23 21v-2a4 4 0 0 0-3-3.87" />
                <path d="M16 3.13a4 4 0 0 1 0 7.75" />
              </svg>
              <!-- AI Team / Organization Icon -->
              <svg v-else-if="link.icon === 'team'" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
                <circle cx="12" cy="5" r="3" />
                <circle cx="5" cy="19" r="3" />
                <circle cx="19" cy="19" r="3" />
                <path d="M12 8v4M7.5 17.5 10 13.5M16.5 17.5 14 13.5" />
              </svg>
            </span>
            <span class="link-label sidebar-card-title">{{ link.label }}</span>
          </div>
          <span class="link-chevron" aria-hidden="true">
            <svg viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
              <polyline points="6 12 10 8 6 4" />
            </svg>
          </span>
        </div>
        <div class="sidebar-card-row-bottom">
          <span class="link-desc sidebar-card-meta">{{ link.desc }}</span>
        </div>
      </RouterLink>
    </div>
  </nav>
</template>

<style scoped>
.org-sidebar {
  display: flex;
  flex-direction: column;
  min-height: 0;
  flex: 1;
  overflow: hidden;
  background: var(--color-surface);
}

.org-links {
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

.link-icon svg {
  width: 14px;
  height: 14px;
}

.link-chevron {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 14px;
  height: 14px;
  color: var(--color-text-muted);
  opacity: 0;
  transform: translateX(-2px);
  transition: opacity 0.12s ease, transform 0.12s ease;
  flex-shrink: 0;
}

.link-chevron svg {
  width: 11px;
  height: 11px;
}

.org-link:hover .link-chevron {
  opacity: 0.6;
  transform: translateX(0);
}

.org-link.router-link-active .link-icon {
  color: var(--color-accent, #4a8ad6);
}

.org-link.router-link-active .link-chevron {
  opacity: 1;
  color: var(--color-accent, #4a8ad6);
  transform: translateX(0);
}
</style>
