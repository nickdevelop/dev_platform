<script setup lang="ts">
// The settings center's middle column: category links ride in the shell's
// sidebar in place of the project/task tree. Navigation recedes to flat,
// quiet neutral states with clear icon affordances and zero resting shadows.
import { RouterLink } from 'vue-router'

const links = [
  {
    name: 'settings-appearance',
    label: '外观',
    desc: '主题、字号与显示密度',
    icon: 'palette',
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
  <nav class="settings-nav" data-test="settings-nav" aria-label="设置分类">
    <header class="settings-nav-header">
      <span class="settings-nav-title">平台设置</span>
      <span class="settings-nav-sub">系统偏好与基础服务配置</span>
    </header>

    <div class="settings-links" role="list">
      <RouterLink
        v-for="link in links"
        :key="link.name"
        :to="{ name: link.name }"
        class="settings-link"
        role="listitem"
      >
        <span class="link-icon" aria-hidden="true">
          <!-- Palette / Appearance -->
          <svg v-if="link.icon === 'palette'" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round">
            <circle cx="13.5" cy="6.5" r=".5" fill="currentColor" />
            <circle cx="17.5" cy="10.5" r=".5" fill="currentColor" />
            <circle cx="8.5" cy="7.5" r=".5" fill="currentColor" />
            <circle cx="6.5" cy="12.5" r=".5" fill="currentColor" />
            <path d="M12 2C6.5 2 2 6.5 2 12s4.5 10 10 10c.926 0 1.648-.746 1.648-1.688 0-.437-.18-.835-.437-1.125-.29-.289-.438-.652-.438-1.125a1.64 1.64 0 0 1 1.668-1.668h1.996c3.051 0 5.563-2.512 5.563-5.563C21.437 6.208 17.199 2 12 2z" />
          </svg>
          <!-- Cube / Models -->
          <svg v-else-if="link.icon === 'cube'" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round">
            <path d="m21 16-9 5-9-5V8l9-5 9 5v8z" />
            <path d="m3.27 6.96 8.73 4.99 8.73-4.99" />
            <path d="M12 22.08V12" />
          </svg>
          <!-- CPU / Agents -->
          <svg v-else-if="link.icon === 'cpu'" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round">
            <rect x="4" y="4" width="16" height="16" rx="2" />
            <rect x="9" y="9" width="6" height="6" />
            <path d="M9 1v3M15 1v3M9 20v3M15 20v3M20 9h3M20 14h3M1 9h3M1 14h3" />
          </svg>
          <!-- Network / Proxies -->
          <svg v-else-if="link.icon === 'network'" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round">
            <rect x="2" y="5" width="8" height="5" rx="1" />
            <rect x="14" y="5" width="8" height="5" rx="1" />
            <rect x="8" y="14" width="8" height="5" rx="1" />
            <path d="M6 10v2a2 2 0 0 0 2 2h8a2 2 0 0 0 2-2v-2M12 14v-2" />
          </svg>
          <!-- Connection / Link -->
          <svg v-else-if="link.icon === 'connection'" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round">
            <path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71" />
            <path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71" />
          </svg>
        </span>
        <div class="link-content">
          <span class="link-label">{{ link.label }}</span>
          <span class="link-desc">{{ link.desc }}</span>
        </div>
        <span class="link-chevron" aria-hidden="true">
          <svg viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round">
            <polyline points="6 12 10 8 6 4" />
          </svg>
        </span>
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
  overflow-y: auto;
  padding: 16px 12px;
}

.settings-nav-header {
  display: flex;
  flex-direction: column;
  gap: 3px;
  padding: 4px 8px 16px;
  margin-bottom: 8px;
  border-bottom: 1px solid var(--color-border);
}

.settings-nav-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--color-text);
  letter-spacing: -0.01em;
}

.settings-nav-sub {
  font-size: 11px;
  color: var(--color-text-muted);
}

.settings-links {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.settings-link {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 10px;
  border: 1px solid transparent;
  border-radius: var(--radius, 6px);
  color: var(--color-text);
  text-decoration: none;
  background: transparent;
  transition: background-color 0.15s ease, border-color 0.15s ease, color 0.15s ease;
  user-select: none;
}

.settings-link:hover {
  background: var(--color-surface-muted);
  border-color: var(--color-border);
}

.link-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  color: var(--color-text-muted);
  flex-shrink: 0;
  transition: color 0.15s ease;
}

.link-icon svg {
  width: 16px;
  height: 16px;
}

.link-content {
  display: flex;
  flex-direction: column;
  min-width: 0;
  flex: 1;
}

.link-label {
  font-size: 13px;
  font-weight: 500;
  color: var(--color-text);
  line-height: 1.25;
}

.link-desc {
  font-size: 11px;
  color: var(--color-text-muted);
  line-height: 1.3;
  margin-top: 1px;
}

.link-chevron {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 14px;
  height: 14px;
  color: var(--color-text-muted);
  opacity: 0;
  transform: translateX(-3px);
  transition: opacity 0.15s ease, transform 0.15s ease;
  flex-shrink: 0;
}

.link-chevron svg {
  width: 12px;
  height: 12px;
}

.settings-link:hover .link-chevron {
  opacity: 0.6;
  transform: translateX(0);
}

/* Active navigation state: flat neutral surface, 1px border, no box-shadow */
.settings-link.router-link-active {
  background: var(--color-surface-muted);
  border-color: var(--color-border-strong, var(--color-border));
  color: var(--color-text);
}

.settings-link.router-link-active .link-icon {
  color: var(--color-primary);
}

.settings-link.router-link-active .link-label {
  font-weight: 600;
}

.settings-link.router-link-active .link-chevron {
  opacity: 1;
  color: var(--color-primary);
  transform: translateX(0);
}
</style>
