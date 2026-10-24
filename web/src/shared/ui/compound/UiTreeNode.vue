<script setup lang="ts">
const props = withDefaults(
  defineProps<{
    id: string | number
    label: string
    level?: number
    isLeaf?: boolean
    expanded?: boolean
    selected?: boolean
    disabled?: boolean
  }>(),
  {
    level: 0,
    isLeaf: false,
    expanded: false,
    selected: false,
    disabled: false,
  },
)

const emit = defineEmits<{
  toggle: [id: string | number]
  select: [id: string | number]
}>()

function onToggleClick(e: MouseEvent) {
  e.stopPropagation()
  if (props.isLeaf || props.disabled) return
  emit('toggle', props.id)
}

function onNodeClick() {
  if (props.disabled) return
  emit('select', props.id)
}
</script>

<template>
  <li
    class="select-none list-none"
    role="treeitem"
    :aria-expanded="isLeaf ? undefined : expanded"
    :aria-selected="selected"
  >
    <div
      class="group flex items-center h-8 px-2 rounded cursor-pointer transition-colors duration-100 text-sm"
      :class="[
        selected
          ? 'bg-blue-50 dark:bg-blue-950/40 text-blue-700 dark:text-blue-300 font-medium'
          : 'text-slate-700 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800',
        disabled ? 'opacity-50 cursor-not-allowed' : '',
      ]"
      :style="{ paddingLeft: `${Math.max(8, level * 16 + 8)}px` }"
      @click="onNodeClick"
    >
      <!-- Caret toggle -->
      <button
        v-if="!isLeaf"
        type="button"
        class="w-4 h-4 mr-1 flex items-center justify-center text-slate-400 hover:text-slate-600 dark:hover:text-slate-200 transition-transform"
        tabindex="-1"
        :aria-label="expanded ? '收起' : '展开'"
        @click="onToggleClick"
      >
        <svg
          class="w-3.5 h-3.5 transition-transform duration-150"
          :class="[expanded ? 'rotate-90' : '']"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
        >
          <path d="M9 18l6-6-6-6" />
        </svg>
      </button>
      <span v-else class="w-4 h-4 mr-1 shrink-0" />

      <!-- Prefix / Icon slot -->
      <div v-if="$slots.icon" class="mr-1.5 shrink-0 text-slate-400 dark:text-slate-500">
        <slot name="icon" />
      </div>

      <!-- Label slot / default text -->
      <span class="truncate flex-1">
        <slot>{{ label }}</slot>
      </span>

      <!-- Right actions slot (hover visible) -->
      <div
        v-if="$slots.actions"
        class="opacity-0 group-hover:opacity-100 focus-within:opacity-100 flex items-center gap-1 transition-opacity shrink-0 ml-1"
        @click.stop
      >
        <slot name="actions" />
      </div>
    </div>

    <!-- Children container if expanded -->
    <ul
      v-if="expanded && !isLeaf && $slots.children"
      class="pl-0 m-0"
      role="group"
    >
      <slot name="children" />
    </ul>
  </li>
</template>
