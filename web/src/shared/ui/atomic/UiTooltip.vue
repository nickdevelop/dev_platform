<script setup lang="ts">
import { computed, ref } from 'vue'

const props = withDefaults(
  defineProps<{
    content?: string
    placement?: 'top' | 'bottom' | 'left' | 'right'
    disabled?: boolean
  }>(),
  {
    content: '',
    placement: 'top',
    disabled: false,
  },
)

const visible = ref(false)

function show() {
  if (!props.disabled) visible.value = true
}

function hide() {
  visible.value = false
}

const positionClasses = computed(() => {
  switch (props.placement) {
    case 'bottom':
      return 'top-full left-1/2 -translate-x-1/2 mt-1.5'
    case 'left':
      return 'right-full top-1/2 -translate-y-1/2 mr-1.5'
    case 'right':
      return 'left-full top-1/2 -translate-y-1/2 ml-1.5'
    case 'top':
    default:
      return 'bottom-full left-1/2 -translate-x-1/2 mb-1.5'
  }
})
</script>

<template>
  <div
    class="relative inline-flex items-center"
    @mouseenter="show"
    @mouseleave="hide"
    @focusin="show"
    @focusout="hide"
  >
    <slot />

    <Transition
      enter-active-class="transition ease-out duration-100"
      enter-from-class="opacity-0 scale-95"
      enter-to-class="opacity-100 scale-100"
      leave-active-class="transition ease-in duration-75"
      leave-from-class="opacity-100 scale-100"
      leave-to-class="opacity-0 scale-95"
    >
      <div
        v-if="visible && (content || $slots.content)"
        role="tooltip"
        class="absolute z-50 px-2 py-1 text-xs text-white bg-slate-900/95 dark:bg-slate-800 rounded shadow-md whitespace-nowrap pointer-events-none select-none"
        :class="positionClasses"
      >
        <slot name="content">{{ content }}</slot>
      </div>
    </Transition>
  </div>
</template>
