<script setup lang="ts">
// GroupsSidebar: The middle column for the Team Groups center.
// Fully aligns with shared sidebar.css standards (Data Terminal / App Screens):
// - Zone A: Dedicated action button (+ 新建团队)
// - Zone B: Compact search bar with 1.5px magnifier icon and clear button
// - Zone C: Standard compact entity cards with 2px accent indicator (The Move)
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useGroupsStore } from '../../stores/groups'

const route = useRoute()
const router = useRouter()
const groupsStore = useGroupsStore()

const keyword = ref('')

onMounted(async () => {
  await groupsStore.loadGroups()
})

const isNewRoute = computed(() => route.name === 'group-new')

const activeGroupId = computed<number | null>(() => {
  if (isNewRoute.value) return null
  if (route.params.groupId !== undefined) {
    const id = Number(route.params.groupId)
    return isNaN(id) ? null : id
  }
  return groupsStore.activeGroupId
})

const filteredGroups = computed(() => {
  const q = keyword.value.trim().toLowerCase()
  if (!q) return groupsStore.groups
  return groupsStore.groups.filter(
    (g) =>
      g.name.toLowerCase().includes(q) ||
      (g.description && g.description.toLowerCase().includes(q)),
  )
})

function openNewGroup() {
  void router.push({ name: 'group-new' })
}

function selectGroup(groupId: number) {
  void router.push({ name: 'group-detail', params: { groupId } })
}
</script>

<template>
  <div class="groups-sidebar sidebar-panel" aria-label="团队列表">
    <!-- Zone A: Top dedicated action button -->
    <div class="sidebar-actions">
      <button
        type="button"
        class="action-btn"
        data-test="btn-new-group"
        title="新建团队"
        @click="openNewGroup"
      >
        <svg
          viewBox="0 0 16 16"
          width="12"
          height="12"
          fill="none"
          stroke="currentColor"
          stroke-width="1.8"
          stroke-linecap="round"
          stroke-linejoin="round"
          aria-hidden="true"
        >
          <line x1="8" y1="3" x2="8" y2="13" />
          <line x1="3" y1="8" x2="13" y2="8" />
        </svg>
        <span>新建团队</span>
      </button>
    </div>

    <!-- Zone B: Compact search bar -->
    <div class="sidebar-search-bar">
      <svg
        class="sidebar-search-icon"
        viewBox="0 0 16 16"
        fill="none"
        stroke="currentColor"
        stroke-width="1.8"
        aria-hidden="true"
      >
        <circle cx="7" cy="7" r="4.5" />
        <line x1="10.5" y1="10.5" x2="14" y2="14" stroke-linecap="round" />
      </svg>
      <input
        v-model="keyword"
        type="search"
        class="sidebar-search-input"
        placeholder="搜索团队名称或描述"
        aria-label="搜索团队名称或描述"
      />
      <button
        v-if="keyword"
        type="button"
        class="sidebar-search-clear"
        title="清空搜索"
        aria-label="清空搜索"
        @click="keyword = ''"
      >
        <svg
          viewBox="0 0 16 16"
          width="10"
          height="10"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
          stroke-linecap="round"
        >
          <line x1="4" y1="4" x2="12" y2="12" />
          <line x1="12" y1="4" x2="4" y2="12" />
        </svg>
      </button>
    </div>

    <!-- Zone C: Standard card container & entity list -->
    <div class="sidebar-list-container" role="list">
      <div
        v-for="group in filteredGroups"
        :key="group.id"
        class="sidebar-card"
        :class="{ active: group.id === activeGroupId }"
        role="listitem"
        tabindex="0"
        @click="selectGroup(group.id)"
        @keydown.enter="selectGroup(group.id)"
      >
        <div class="sidebar-card-row-top">
          <span class="sidebar-card-title truncate" :title="group.name">
            {{ group.name }}
          </span>
          <span class="shrink-0 text-2xs font-mono px-1.5 py-0.5 rounded bg-surface-muted text-foreground border border-border">
            {{ group.orchestratorModel }}
          </span>
        </div>

        <p v-if="group.description" class="text-2xs text-[var(--color-text-muted)] truncate mt-0.5">
          {{ group.description }}
        </p>

        <div class="flex items-center justify-between text-2xs text-[var(--color-text-muted)] mt-1 pt-1 border-t border-[var(--color-border)] font-mono">
          <span>成员: {{ group.memberCount ?? 0 }} 位专家</span>
        </div>
      </div>

      <div
        v-if="filteredGroups.length === 0"
        class="py-8 text-center text-2xs text-[var(--color-text-muted)]"
      >
        {{ keyword ? '未找到匹配的团队' : '暂无团队' }}
      </div>
    </div>
  </div>
</template>
