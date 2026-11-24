<script setup lang="ts">
// GroupDetailView: Displays team info, orchestrator brain configuration,
// and member experts from role library.
import { computed, ref, watch } from 'vue'
import { useGroupsStore } from '../../stores/groups'
import BackendTag from '../../shared/components/BackendTag.vue'
import { UiButton, UiAlert, UiConfirmDialog } from '../../shared/ui'

const props = defineProps<{
  groupId: number
}>()

const emit = defineEmits<{
  edit: []
  deleted: []
}>()

const groupsStore = useGroupsStore()

const showPromptExpanded = ref(false)
const pendingDeleteGroup = ref<boolean>(false)
const actionError = ref('')

const group = computed(() => groupsStore.activeGroupDetail)

watch(
  () => props.groupId,
  async (newId) => {
    if (!newId) return
    actionError.value = ''
    await groupsStore.loadGroup(newId)
  },
  { immediate: true },
)

async function confirmDeleteGroup() {
  actionError.value = ''
  try {
    await groupsStore.deleteGroup(props.groupId)
    pendingDeleteGroup.value = false
    emit('deleted')
  } catch (err: any) {
    actionError.value = err?.message || '删除团队失败'
  }
}
</script>

<template>
  <div class="h-full flex flex-col min-h-0 overflow-y-auto bg-[var(--color-bg)] p-6 space-y-6">
    <UiAlert v-if="actionError" variant="error">
      {{ actionError }}
    </UiAlert>

    <!-- 1. Header Bar: Team Info & Quick Actions -->
    <div class="bg-[var(--color-surface)] border border-[var(--color-border)] rounded-[var(--radius,6px)] p-5">
      <div class="flex flex-col md:flex-row md:items-center justify-between gap-4">
        <div class="space-y-1">
          <div class="flex items-center gap-2.5">
            <h1 class="text-lg font-semibold text-[var(--color-text)] tracking-tight">
              {{ group?.name || '团队' }}
            </h1>
            <span class="inline-flex items-center gap-1 px-2 py-0.5 rounded text-2xs font-mono font-medium bg-surface-muted text-foreground border border-border">
              <svg viewBox="0 0 16 16" width="12" height="12" fill="none" stroke="currentColor" stroke-width="1.8">
                <circle cx="8" cy="8" r="3" />
                <path d="M8 1v2M8 13v2M1 8h2M13 8h2" />
              </svg>
              中控大脑: {{ group?.orchestratorModel }}
            </span>
          </div>
          <p class="text-xs text-[var(--color-text-muted)] leading-relaxed">
            {{ group?.description || '暂无团队描述' }}
          </p>
        </div>

        <div class="flex items-center gap-2">
          <UiButton
            variant="secondary"
            class="text-xs"
            @click="emit('edit')"
          >
            编辑团队设置
          </UiButton>
          <UiButton
            variant="secondary"
            class="text-xs text-red-500 hover:text-red-600"
            @click="pendingDeleteGroup = true"
          >
            删除团队
          </UiButton>
        </div>
      </div>

      <!-- Orchestrator System Prompt Preview / Collapsible -->
      <div v-if="group?.orchestratorPrompt" class="mt-4 pt-3 border-t border-[var(--color-border)]">
        <button
          type="button"
          class="flex items-center gap-1.5 text-2xs font-medium text-[var(--color-text-muted)] hover:text-[var(--color-text)] transition-colors"
          @click="showPromptExpanded = !showPromptExpanded"
        >
          <svg
            viewBox="0 0 16 16"
            width="12"
            height="12"
            fill="none"
            stroke="currentColor"
            stroke-width="1.8"
            class="transition-transform"
            :class="[showPromptExpanded ? 'rotate-90' : '']"
          >
            <polyline points="6 3 11 8 6 13" />
          </svg>
          <span>中控调度大脑专属角色提示词与团队规范</span>
        </button>
        <div v-if="showPromptExpanded" class="mt-2 p-3 rounded bg-[var(--color-surface-muted)] text-2xs font-mono whitespace-pre-wrap leading-relaxed text-[var(--color-text)] border border-[var(--color-border)]">
          {{ group.orchestratorPrompt }}
        </div>
      </div>
    </div>

    <!-- 2. Team Members Strip: Roles from Role Library -->
    <div class="bg-[var(--color-surface)] border border-[var(--color-border)] rounded-[var(--radius,6px)] p-5 space-y-3">
      <div class="flex items-center justify-between">
        <div class="flex items-center gap-2">
          <span class="text-xs font-semibold text-[var(--color-text)] uppercase tracking-wider">团队专家成员</span>
          <span class="text-2xs text-[var(--color-text-muted)] font-mono">({{ group?.members?.length || 0 }})</span>
        </div>
        <UiButton
          variant="secondary"
          class="text-xs h-7 px-2.5"
          @click="emit('edit')"
        >
          调整成员
        </UiButton>
      </div>

      <div class="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-3">
        <div
          v-for="m in (group?.members || [])"
          :key="m.id"
          class="p-3 rounded border border-[var(--color-border)] bg-[var(--color-surface-muted)] flex flex-col justify-between space-y-2 hover:border-[var(--color-border-strong)] transition-colors"
        >
          <div>
            <div class="flex items-center justify-between gap-1">
              <span class="text-xs font-medium text-[var(--color-text)] truncate">{{ m.name }}</span>
              <BackendTag :backend="m.backend" />
            </div>
            <p class="text-2xs text-[var(--color-text-muted)] line-clamp-2 mt-1">
              {{ m.description || m.systemPrompt || '暂无设定' }}
            </p>
          </div>
          <div class="text-2xs text-[var(--color-text-muted)] font-mono truncate pt-1 border-t border-[var(--color-border)]">
            模型: {{ m.model }}
          </div>
        </div>

        <div
          v-if="!group?.members || group.members.length === 0"
          class="col-span-full py-6 text-center text-xs text-[var(--color-text-muted)]"
        >
          当前团队暂未添加任何专家成员，请点击右上角「调整成员」从角色库添加。
        </div>
      </div>
    </div>

    <!-- Delete Group Confirm -->
    <UiConfirmDialog
      v-if="pendingDeleteGroup"
      title="确认删除团队？"
      message="此操作将永久删除该团队配置，操作不可撤销。"
      confirm-label="确认删除"
      variant="danger"
      @confirm="confirmDeleteGroup"
      @close="pendingDeleteGroup = false"
    />
  </div>
</template>
