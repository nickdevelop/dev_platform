<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'

const props = withDefaults(
  defineProps<{
    align?: 'left' | 'right'
    widthClass?: string
    closeOnClick?: boolean
  }>(),
  {
    align: 'left',
    widthClass: 'min-w-[140px]',
    closeOnClick: true,
  },
)

const isOpen = ref(false)
const dropdownRef = ref<HTMLElement | null>(null)

function toggle() {
  isOpen.value = !isOpen.value
}

function open() {
  isOpen.value = true
}

function close() {
  isOpen.value = false
}

function onDocumentClick(e: MouseEvent) {
  if (!dropdownRef.value) return
  if (!dropdownRef.value.contains(e.target as Node)) {
    close()
  }
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape' && isOpen.value) {
    close()
    e.stopPropagation()
  }
}

function handleMenuClick() {
  if (props.closeOnClick) {
    close()
  }
}

onMounted(() => {
  window.addEventListener('click', onDocumentClick, true)
  window.addEventListener('keydown', onKeydown)
})

onBeforeUnmount(() => {
  window.removeEventListener('click', onDocumentClick, true)
  window.removeEventListener('keydown', onKeydown)
})

defineExpose({
  isOpen,
  open,
  close,
  toggle,
})
</script>

<template>
  <div ref="dropdownRef" class="relative inline-block text-left">
    <div @click="toggle">
      <slot name="trigger" :is-open="isOpen" :toggle="toggle" />
    </div>

    <Transition
      enter-active-class="transition ease-out duration-100"
      enter-from-class="transform opacity-0 scale-95"
      enter-to-class="transform opacity-100 scale-100"
      leave-active-class="transition ease-in duration-75"
      leave-from-class="transform opacity-100 scale-100"
      leave-to-class="transform opacity-0 scale-95"
    >
      <div
        v-if="isOpen"
        class="absolute z-50 mt-1 rounded bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 shadow-lg py-1 text-sm text-slate-700 dark:text-slate-200 focus:outline-none"
        :class="[
          align === 'right' ? 'right-0' : 'left-0',
          widthClass,
        ]"
        @click="handleMenuClick"
      >
        <slot :close="close" />
      </div>
    </Transition>
  </div>
</template>
