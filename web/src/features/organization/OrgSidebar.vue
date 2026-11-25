<script setup lang="ts">
// OrgSidebar: the organization center's second-level navigation.
//
// It is an `el-menu` in router mode: the library owns the keyboard behaviour
// (roving tabindex, aria roles, expand/collapse) that a hand-rolled list never
// had. Two entries today would have been a plain list, but the structure below
// already renders `el-sub-menu` for any link that declares children, so adding a
// third level is a data change rather than a rewrite.
//
// The visual contract (neutral hover, 2px accent edge on the active row, compact
// row height) lives in `theme/element-tokens.css` — never here. This file only
// decorates the slot content, which is our own markup under our own classes.
import { computed, type Component } from 'vue'
import { useRoute } from 'vue-router'
import { ChatDotRound, Connection, EditPen, User } from '@element-plus/icons-vue'

interface OrgLink {
  // Route name, kept for tests and for future programmatic navigation.
  name: string
  // el-menu navigates by index, and in router mode the index is the path.
  path: string
  label: string
  desc: string
  icon: Component
  children?: Omit<OrgLink, 'children'>[]
}

const links: OrgLink[] = [
  {
    name: 'org-prompts',
    path: '/org/prompts',
    label: 'AI提示词',
    desc: '角色人设与流程协同规范',
    icon: EditPen,
  },
  {
    name: 'org-assistants',
    path: '/org/assistants',
    label: 'AI 助手',
    desc: '日常人设与自治工具管理',
    icon: ChatDotRound,
  },
  {
    name: 'org-employees',
    path: '/org/employees',
    label: 'AI 员工',
    desc: '专职数字员工与执行引擎分工',
    icon: User,
  },
  {
    name: 'org-teams',
    path: '/org/teams',
    label: 'AI 团队',
    desc: '中控大脑调度与多专家协同',
    icon: Connection,
  },
]

const route = useRoute()

// The menu highlights the current route; `/org/assistants?edit=3` still belongs
// to the assistants entry, so match on the path and ignore the query.
const activeIndex = computed(() => route.path)
</script>

<template>
  <nav class="org-nav sidebar-panel" data-test="org-sidebar" aria-label="组织架构管理">
    <div class="org-menu-scroll">
      <el-menu
        class="org-menu"
        router
        :default-active="activeIndex"
        :default-openeds="[]"
        data-test="org-menu"
      >
        <template v-for="link in links" :key="link.name">
          <el-sub-menu v-if="link.children" :index="link.path">
            <template #title>
              <el-icon class="link-icon"><component :is="link.icon" /></el-icon>
              <span class="link-label">{{ link.label }}</span>
            </template>
            <el-menu-item v-for="child in link.children" :key="child.path" :index="child.path">
              <span class="link-label">{{ child.label }}</span>
            </el-menu-item>
          </el-sub-menu>

          <el-menu-item v-else :index="link.path" :data-test="`org-link-${link.name}`">
            <el-icon class="link-icon"><component :is="link.icon" /></el-icon>
            <div class="link-body">
              <span class="link-label">{{ link.label }}</span>
              <span class="link-desc">{{ link.desc }}</span>
            </div>
          </el-menu-item>
        </template>
      </el-menu>
    </div>
  </nav>
</template>

<style scoped>
.org-nav {
  background: var(--color-surface);
}

/* The scroller is ours so the menu keeps its own block layout: putting the
   scroll container on the menu root would fight the library's list styles. */
.org-menu-scroll {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  overflow-x: hidden;
  padding: 4px 6px 12px 6px;
}

.org-menu {
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
