<script setup lang="ts">
// GroupsView: Main workbench panel for Team Groups, rendered in AppShell's router-view.
// Manages display of GroupDetailView, GroupEditPanel (for create/edit), and empty state.
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useGroupsStore } from '../../stores/groups'
import { useProjectsStore } from '../../stores/projects'
import GroupDetailView from './GroupDetailView.vue'
import GroupEditPanel from './GroupEditPanel.vue'
import { UiButton } from '../../shared/ui'

const route = useRoute()
const router = useRouter()
const groupsStore = useGroupsStore()
const projectsStore = useProjectsStore()

// If editing a group in-place
const editingGroupId = ref<number | null>(null)

onMounted(async () => {
  await Promise.all([groupsStore.loadGroups(), projectsStore.load()])
})

const isNewRoute = computed(() => route.name === 'group-new')

const activeGroupId = computed<number | null>(() => {
  if (isNewRoute.value) return null
  if (route.params.groupId !== undefined) {
    const id = Number(route.params.groupId)
    return isNaN(id) ? null : id
  }
  return groupsStore.groups.length > 0 ? groupsStore.groups[0].id : null
})

// Auto navigate to first group if on plain /groups route and groups exist
watch(
  [() => route.name, () => groupsStore.groups.length],
  ([routeName, count]) => {
    if (routeName === 'groups' && count > 0 && !route.params.groupId) {
      void router.replace({ name: 'group-detail', params: { groupId: groupsStore.groups[0].id } })
    }
  },
  { immediate: true },
)

function onGroupSaved(groupId: number) {
  editingGroupId.value = null
  void router.push({ name: 'group-detail', params: { groupId } })
}

function onCancelEdit() {
  editingGroupId.value = null
  if (groupsStore.groups.length > 0) {
    const fallbackId = activeGroupId.value || groupsStore.groups[0].id
    void router.push({ name: 'group-detail', params: { groupId: fallbackId } })
  } else {
    void router.push({ name: 'groups' })
  }
}

function onEditGroup(groupId: number) {
  editingGroupId.value = groupId
}

function onGroupDeleted() {
  editingGroupId.value = null
  if (groupsStore.groups.length > 0) {
    void router.push({ name: 'group-detail', params: { groupId: groupsStore.groups[0].id } })
  } else {
    void router.push({ name: 'groups' })
  }
}
</script>

<template>
  <main class="flex-1 min-w-0 h-full flex flex-col overflow-hidden bg-[var(--color-bg)]" data-test="groups-workbench">
    <!-- 1. Group Creation or Editing Panel (Full workspace pane, no modal overlay) -->
    <template v-if="isNewRoute || editingGroupId !== null">
      <GroupEditPanel
        :group="editingGroupId !== null ? groupsStore.groupById(editingGroupId) : null"
        @saved="onGroupSaved"
        @cancel="onCancelEdit"
      />
    </template>

    <!-- 2. Active Group Detail View -->
    <template v-else-if="activeGroupId">
      <GroupDetailView
        :group-id="activeGroupId"
        @edit="onEditGroup(activeGroupId)"
        @deleted="onGroupDeleted"
      />
    </template>

    <!-- 3. Empty Guidance -->
    <template v-else>
      <div class="flex items-center justify-center h-full p-6 md:p-12 overflow-y-auto">
        <div class="max-w-xl w-full bg-[var(--color-surface)] border border-[var(--color-border)] rounded-[var(--radius,6px)] p-8 text-center space-y-4">
          <div class="inline-flex p-3 rounded-full bg-accent/10 text-accent mx-auto">
            <svg viewBox="0 0 24 24" width="28" height="28" fill="none" stroke="currentColor" stroke-width="1.8">
              <path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2" />
              <circle cx="9" cy="7" r="4" />
              <path d="M23 21v-2a4 4 0 0 0-3-3.87" />
              <path d="M16 3.13a4 4 0 0 1 0 7.75" />
            </svg>
          </div>
          <h2 class="text-base font-semibold text-[var(--color-text)] tracking-tight">
            团队中心
          </h2>
          <p class="text-xs text-[var(--color-text-muted)] leading-relaxed max-w-md mx-auto">
            定义专业协作团队，配置中控调度大脑模型与角色提示词规范，纳管专家角色。在项目中创建协同会话时，团队各专家在中控大脑的统筹规划下协同执行任务。
          </p>
          <div class="pt-2">
            <UiButton
              variant="primary"
              class="text-xs px-4 py-2 font-medium"
              @click="router.push({ name: 'group-new' })"
            >
              + 创建首个团队
            </UiButton>
          </div>
        </div>
      </div>
    </template>
  </main>
</template>
