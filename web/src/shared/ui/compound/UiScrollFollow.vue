<script setup lang="ts">
import { nextTick, onMounted, ref, watch } from 'vue'

const props = withDefaults(
  defineProps<{
    // 监听数据变化以触发自动贴底
    watchSignal?: unknown
    threshold?: number
    jumpButtonText?: string
  }>(),
  {
    watchSignal: undefined,
    threshold: 60,
    jumpButtonText: '↓ 滚动到底部',
  },
)

const emit = defineEmits<{
  scroll: [event: Event]
}>()

const scrollerRef = ref<HTMLElement | null>(null)
const isPinned = ref(true)
const showJump = ref(false)

function checkScrollPosition() {
  const el = scrollerRef.value
  if (!el) return
  const distanceFromBottom = el.scrollHeight - el.scrollTop - el.clientHeight
  const atBottom = distanceFromBottom <= props.threshold
  isPinned.value = atBottom
  showJump.value = !atBottom
}

function scrollToBottom(smooth = false) {
  const el = scrollerRef.value
  if (!el) return
  isPinned.value = true
  showJump.value = false
  if (typeof el.scrollTo === 'function') {
    el.scrollTo({
      top: el.scrollHeight,
      behavior: smooth ? 'smooth' : 'auto',
    })
  } else {
    el.scrollTop = el.scrollHeight
  }
}

function onScroll(event: Event) {
  checkScrollPosition()
  emit('scroll', event)
}

watch(
  () => props.watchSignal,
  () => {
    if (isPinned.value) {
      void nextTick(() => {
        scrollToBottom(false)
      })
    }
  },
  { deep: true },
)

onMounted(() => {
  scrollToBottom(false)
})

defineExpose({
  scrollerRef,
  isPinned,
  showJump,
  scrollToBottom,
})
</script>

<template>
  <div class="relative w-full h-full flex flex-col min-h-0 overflow-hidden">
    <div
      ref="scrollerRef"
      class="flex-1 w-full h-full overflow-y-auto overflow-x-hidden leading-relaxed"
      @scroll.passive="onScroll"
    >
      <slot />
    </div>

    <!-- 浮动在底部的跳转按钮 -->
    <Transition
      enter-active-class="transition ease-out duration-150"
      enter-from-class="opacity-0 translate-y-2"
      enter-to-class="opacity-100 translate-y-0"
      leave-active-class="transition ease-in duration-100"
      leave-from-class="opacity-100 translate-y-0"
      leave-to-class="opacity-0 translate-y-2"
    >
      <div
        v-if="showJump"
        class="absolute bottom-4 left-1/2 -translate-x-1/2 z-20 select-none"
      >
        <slot name="jump-button" :scroll-to-bottom="scrollToBottom">
          <button
            type="button"
            class="px-3 py-1.5 rounded-full text-xs font-medium bg-slate-900/90 text-white dark:bg-slate-700/95 dark:text-slate-100 shadow-md hover:bg-slate-800 dark:hover:bg-slate-600 transition-colors flex items-center gap-1.5"
            @click="scrollToBottom(true)"
          >
            {{ jumpButtonText }}
          </button>
        </slot>
      </div>
    </Transition>
  </div>
</template>
