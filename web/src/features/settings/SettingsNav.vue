<script setup lang="ts">
// SettingsNav: the settings center's second-level navigation.
//
// Same shape as OrgSidebar and EnginesSidebar: an `el-menu` in router mode, so
// the library owns the keyboard behaviour (roving tabindex, aria roles) that a
// hand-rolled RouterLink list never had. The visual contract (neutral hover,
// 2px accent edge on the active row, compact row height) lives in
// `theme/element-tokens.css` — never here; this file only decorates the slot
// content with our own classes.
//
// The nine hand-drawn SVGs this file used to inline are gone: the library's
// glyph set covers every entry here, and v1's "vector only" rule is satisfied
// by `@element-plus/icons-vue` without a private icon folder to keep in sync.
import { computed, type Component } from 'vue'
import { useRoute } from 'vue-router'
import { Box, Brush, Connection, DataLine, Monitor, Share } from '@element-plus/icons-vue'

interface SettingsLink {
  // Route name, kept for tests and for future programmatic navigation.
  name: string
  // el-menu navigates by index, and in router mode the index is the path.
  path: string
  label: string
  desc: string
  icon: Component
}

// Menu order is part of the product contract: appearance, models, proxies,
// connections, terminal, debug (Role and Prompt moved to the organization
// center, so nothing here points at them any more).
const links: SettingsLink[] = [
  {
    name: 'settings-appearance',
    path: '/settings/appearance',
    label: '外观',
    desc: '主题、字号与显示密度',
    icon: Brush,
  },
  {
    name: 'settings-models',
    path: '/settings/models',
    label: '模型',
    desc: '服务端点与可用模型',
    icon: Box,
  },
  {
    name: 'settings-proxies',
    path: '/settings/proxies',
    label: '服务代理',
    desc: '二级路径与端口转发',
    icon: Share,
  },
  {
    name: 'settings-connections',
    path: '/settings/connections',
    label: '服务连接',
    desc: '本地与远程服务连接管理',
    icon: Connection,
  },
  {
    name: 'settings-terminal',
    path: '/settings/terminal',
    label: '终端服务器',
    desc: '本地与远程 SSH 服务器终端维护',
    icon: Monitor,
  },
  {
    name: 'settings-debug',
    path: '/settings/debug',
    label: '调试面板',
    desc: 'SSE 总线消息实时调试',
    icon: DataLine,
  },
]

const route = useRoute()

// The menu highlights the current route; match on the path so query strings
// (if any are added later) never drop the active marker.
const activeIndex = computed(() => route.path)
</script>

<template>
  <nav class="settings-nav sidebar-panel" data-test="settings-nav" aria-label="设置分类">
    <div class="settings-menu-scroll">
      <el-menu
        class="settings-menu"
        router
        :default-active="activeIndex"
        :default-openeds="[]"
        data-test="settings-menu"
      >
        <el-menu-item
          v-for="link in links"
          :key="link.path"
          :index="link.path"
          :data-test="`settings-link-${link.name}`"
        >
          <el-icon class="link-icon"><component :is="link.icon" /></el-icon>
          <div class="link-body">
            <span class="link-label">{{ link.label }}</span>
            <span class="link-desc">{{ link.desc }}</span>
          </div>
        </el-menu-item>
      </el-menu>
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
  background: var(--color-surface);
}

/* The scroller is ours so the menu keeps its own block layout: putting the
   scroll container on the menu root would fight the library's list styles. */
.settings-menu-scroll {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  overflow-x: hidden;
  padding: 4px 6px 12px 6px;
}

.settings-menu {
  width: 100%;
}

.link-icon {
  color: var(--color-text-muted);
  font-size: var(--text-size-14);
  transition: color 0.12s ease-out;
}

.link-body {
  display: flex;
  flex-direction: column;
  gap: 1px;
  min-width: 0;
  line-height: 1.35;
}

.link-label {
  font-size: var(--text-size-13);
  color: var(--color-text);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.link-desc {
  font-size: var(--text-size-11);
  color: var(--color-text-muted);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
</style>
