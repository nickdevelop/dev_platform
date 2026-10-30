<script setup lang="ts">
import { computed, inject, type Ref } from 'vue'

const props = withDefaults(
  defineProps<{
    value: string | number
    label?: string
    count?: number | string
    disabled?: boolean
    closable?: boolean
  }>(),
  {
    label: '',
    count: undefined,
    disabled: false,
    closable: false,
  },
)

const emit = defineEmits<{
  close: [event: MouseEvent]
}>()

interface TabsContext {
  activeValue: Ref<string | number>
  variant: Ref<'line' | 'pill'>
  size: Ref<'sm' | 'md'>
  selectTab: (val: string | number) => void
}

const context = inject<TabsContext>('uiTabsContext')

const isActive = computed(() => context?.activeValue.value === props.value)
const isPill = computed(() => context?.variant.value === 'pill')
const isSm = computed(() => context?.size.value === 'sm')

function onClick() {
  if (props.disabled) return
  context?.selectTab(props.value)
}

function onClose(e: MouseEvent) {
  emit('close', e)
}
</script>

<template>
  <button
    type="button"
    role="tab"
    :aria-selected="isActive"
    :disabled="disabled"
    class="inline-flex items-center transition-all duration-150 select-none outline-none focus-visible:ring-2 focus-visible:ring-blue-500/30 disabled:opacity-50 disabled:cursor-not-allowed"
    :class="[
      isSm ? 'px-2 py-1 text-xs gap-1.5' : 'px-3 py-1.5 text-sm gap-2',
      isPill
        ? [
            'rounded font-medium',
            isActive
              ? 'bg-white dark:bg-slate-700 text-slate-900 dark:text-slate-100 shadow-sm'
              : 'text-slate-500 dark:text-slate-400 hover:text-slate-700 dark:hover:text-slate-200 hover:bg-white/50 dark:hover:bg-slate-700/50',
          ]
        : [
            'border-b-2 font-medium -mb-px',
            isActive
              ? 'border-blue-600 text-blue-600 dark:text-blue-400'
              : 'border-transparent text-slate-500 dark:text-slate-400 hover:text-slate-700 dark:hover:text-slate-200 hover:border-slate-300 dark:hover:border-slate-700',
          ],
    ]"
    @click="onClick"
  >
    <slot name="icon" />
    <slot>{{ label }}</slot>
    <span
      v-if="count !== undefined"
      class="px-1.5 py-0.2 text-[10px] font-mono rounded-full bg-slate-100 dark:bg-slate-800 text-slate-600 dark:text-slate-300 border border-slate-200 dark:border-slate-700"
    >
      {{ count }}
    </span>
    <button
      v-if="closable"
      type="button"
      class="hover:text-red-500 p-0.5 rounded focus:outline-none inline-flex items-center justify-center"
      title="关闭"
      aria-label="关闭标签"
      @click.stop="onClose"
    >
      <svg viewBox="0 0 16 16" width="10" height="10" fill="none" stroke="currentColor" stroke-width="1.8">
        <line x1="3" y1="3" x2="13" y2="13" />
        <line x1="13" y1="3" x2="3" y2="13" />
      </svg>
    </button>
  </button>
</template>
