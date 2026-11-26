<script setup lang="ts">
// EnginesSidebar: the engines center's second-level navigation.
//
// Same shape as OrgSidebar: an `el-menu` in router mode, so the library owns
// the keyboard behaviour (roving tabindex, aria roles) that a hand-rolled
// RouterLink list never had. The visual contract (neutral hover, 2px accent
// edge on the active row, compact row height) lives in
// `theme/element-tokens.css` — never here; this file only decorates the slot
// content with our own classes.
import { computed, type Component } from 'vue'
import { useRoute } from 'vue-router'
import { ChatDotRound, Connection, Cpu, Monitor, Share } from '@element-plus/icons-vue'

interface EngineLink {
  // Route name, kept for tests and for future programmatic navigation.
  name: string
  // el-menu navigates by index, and in router mode the index is the path.
  path: string
  label: string
  desc: string
  icon: Component
}

const links: EngineLink[] = [
  {
    name: 'engines-assistant',
    path: '/engines/assistant',
    label: '助手引擎',
    desc: '对话助手人设与平台管理工具',
    icon: ChatDotRound,
  },
  {
    name: 'engines-coder',
    path: '/engines/coder',
    label: '会话引擎',
    desc: '工作区代码生成与终端执行',
    icon: Monitor,
  },
  {
    name: 'engines-orchestrator',
    path: '/engines/orchestrator',
    label: '协同引擎',
    desc: '多 Agent 流程拆解与分派',
    icon: Share,
  },
  {
    name: 'engines-bot',
    path: '/engines/bot',
    label: 'Bot 引擎',
    desc: '外部 IM ReAct 执行底座与微信接入',
    icon: Connection,
  },
  {
    name: 'engines-agents',
    path: '/engines/agents',
    label: 'Agent配置',
    desc: '异构 Agent 后端与服务管理',
    icon: Cpu,
  },
]

const route = useRoute()

// The menu highlights the current route; match on the path so query strings
// (if any are added later) never drop the active marker.
const activeIndex = computed(() => route.path)
</script>

<template>
  <nav class="engines-nav sidebar-panel" data-test="engines-nav" aria-label="引擎分类">
    <div class="engines-menu-scroll">
      <el-menu
        class="engines-menu"
        router
        :default-active="activeIndex"
        :default-openeds="[]"
        data-test="engines-menu"
      >
        <el-menu-item
          v-for="link in links"
          :key="link.path"
          :index="link.path"
          :data-test="`engines-link-${link.name}`"
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
.engines-nav {
  display: flex;
  flex-direction: column;
  min-height: 0;
  flex: 1;
  overflow: hidden;
  background: var(--color-surface);
}

/* The scroller is ours so the menu keeps its own block layout: putting the
   scroll container on the menu root would fight the library's list styles. */
.engines-menu-scroll {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  overflow-x: hidden;
  padding: 4px 6px 12px 6px;
}

.engines-menu {
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
