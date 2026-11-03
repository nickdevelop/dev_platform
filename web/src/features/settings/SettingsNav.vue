<script setup lang="ts">
// The settings center's middle column: category links ride in the shell's
// sidebar in place of the project/task tree. Navigation recedes to flat,
// quiet neutral states with clear icon affordances and zero resting shadows.
// Conforms to design-taste-frontend (Data Terminal / App Screens) compact rules:
//   - Aligned directly with AppShell's sidebar-head (no redundant double header)
//   - The Move: active navigation strictly uses 2px accent indicator on the left edge
import { RouterLink } from 'vue-router'

const links = [
  {
    name: 'settings-appearance',
    label: '外观',
    desc: '主题、字号与显示密度',
    icon: 'palette',
  },
  {
    name: 'settings-roles',
    label: '角色管理',
    desc: '协同专家与智能体分工',
    icon: 'users',
  },
  {
    name: 'settings-prompts',
    label: '提示词管理',
    desc: '角色提示词与流程范式',
    icon: 'prompt',
  },
  {
    name: 'settings-models',
    label: '模型',
    desc: '服务端点与可用模型',
    icon: 'cube',
  },
  {
    name: 'settings-agents',
    label: 'Agent',
    desc: '执行引擎与后台服务',
    icon: 'cpu',
  },
  {
    name: 'settings-proxies',
    label: '服务代理',
    desc: '二级路径与端口转发',
    icon: 'network',
  },
  {
    name: 'settings-connections',
    label: '服务连接',
    desc: '本地与远程服务连接管理',
    icon: 'connection',
  },
]
</script>

<template>
  <nav class="settings-nav sidebar-panel" data-test="settings-nav" aria-label="设置分类">
    <!-- Zone C: Category navigation links list -->
    <div class="settings-links sidebar-list-container" role="list">
      <RouterLink
        v-for="link in links"
        :key="link.name"
        :to="{ name: link.name }"
        class="settings-link sidebar-card group"
        role="listitem"
      >
        <div class="sidebar-card-row-top">
          <div class="flex items-center gap-2 min-w-0 flex-1">
            <span class="link-icon" aria-hidden="true">
              <!-- Palette / Appearance -->
              <svg v-if="link.icon === 'palette'" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
                <circle cx="13.5" cy="6.5" r=".5" fill="currentColor" />
                <circle cx="17.5" cy="10.5" r=".5" fill="currentColor" />
                <circle cx="8.5" cy="7.5" r=".5" fill="currentColor" />
                <circle cx="6.5" cy="12.5" r=".5" fill="currentColor" />
                <path d="M12 2C6.5 2 2 6.5 2 12s4.5 10 10 10c.926 0 1.648-.746 1.648-1.688 0-.437-.18-.835-.437-1.125-.29-.289-.438-.652-.438-1.125a1.64 1.64 0 0 1 1.668-1.668h1.996c3.051 0 5.563-2.512 5.563-5.563C21.437 6.208 17.199 2 12 2z" />
              </svg>
              <!-- Users / Roles -->
              <svg v-else-if="link.icon === 'users'" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
                <path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2" />
                <circle cx="9" cy="7" r="4" />
                <path d="M23 21v-2a4 4 0 0 0-3-3.87" />
                <path d="M16 3.13a4 4 0 0 1 0 7.75" />
              </svg>
              <!-- Prompt / Prompts -->
              <svg v-else-if="link.icon === 'prompt'" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
                <path d="M12 20h9" />
                <path d="M16.5 3.5a2.121 2.121 0 0 1 3 3L7 19l-4 1 1-4L16.5 3.5z" />
              </svg>
              <!-- Cube / Models -->
              <svg v-else-if="link.icon === 'cube'" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
                <path d="m21 16-9 5-9-5V8l9-5 9 5v8z" />
                <path d="m3.27 6.96 8.73 4.99 8.73-4.99" />
                <path d="M12 22.08V12" />
              </svg>
              <!-- CPU / Agents -->
              <svg v-else-if="link.icon === 'cpu'" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
                <rect x="4" y="4" width="16" height="16" rx="2" />
                <rect x="9" y="9" width="6" height="6" />
                <path d="M9 1v3M15 1v3M9 20v3M15 20v3M20 9h3M20 14h3M1 9h3M1 14h3" />
              </svg>
              <!-- Network / Proxies -->
              <svg v-else-if="link.icon === 'network'" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
                <rect x="2" y="5" width="8" height="5" rx="1" />
                <rect x="14" y="5" width="8" height="5" rx="1" />
                <rect x="8" y="14" width="8" height="5" rx="1" />
                <path d="M6 10v2a2 2 0 0 0 2 2h8a2 2 0 0 0 2-2v-2M12 14v-2" />
              </svg>
              <!-- Connection / Link -->
              <svg v-else-if="link.icon === 'connection'" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
                <path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71" />
                <path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71" />
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
.settings-nav {
  display: flex;
  flex-direction: column;
  min-height: 0;
  flex: 1;
  overflow: hidden;
}

.settings-links {
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

.settings-link:hover .link-chevron {
  opacity: 0.6;
  transform: translateX(0);
}

.settings-link.router-link-active .link-icon {
  color: var(--color-accent, #4a8ad6);
}

.settings-link.router-link-active .link-chevron {
  opacity: 1;
  color: var(--color-accent, #4a8ad6);
  transform: translateX(0);
}
</style>
