<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, onUpdated, ref, watch } from 'vue'

const props = withDefaults(
  defineProps<{
    ariaLabel?: string
    activeKey?: string | null
  }>(),
  {
    ariaLabel: '标签栏',
    activeKey: null,
  },
)

const scrollerRef = ref<HTMLElement | null>(null)
const hasOverflow = ref(false)
const thumbWidth = ref(24)
const thumbLeft = ref(0)
const isDraggingThumb = ref(false)

let resizeObserver: ResizeObserver | null = null
let mutationObserver: MutationObserver | null = null
let dragStartX = 0
let dragStartScrollLeft = 0

function syncScrollbar() {
  const el = scrollerRef.value
  if (!el) return
  const clientWidth = el.clientWidth
  const scrollWidth = el.scrollWidth
  const overflow = scrollWidth > clientWidth + 1
  hasOverflow.value = overflow
  if (!overflow || clientWidth <= 0) {
    thumbWidth.value = 0
    thumbLeft.value = 0
    return
  }
  const ratio = clientWidth / scrollWidth
  const w = Math.max(24, Math.min(clientWidth, Math.round(clientWidth * ratio)))
  const maxScroll = Math.max(1, scrollWidth - clientWidth)
  const maxLeft = Math.max(0, clientWidth - w)
  thumbWidth.value = w
  thumbLeft.value = Math.round((el.scrollLeft / maxScroll) * maxLeft)
}

function scrollActiveIntoView() {
  const el = scrollerRef.value
  if (!el) return
  const activeEl = el.querySelector('.active, .is-active') as HTMLElement | null
  if (activeEl && typeof activeEl.scrollIntoView === 'function') {
    activeEl.scrollIntoView({ block: 'nearest', inline: 'nearest' })
  }
  syncScrollbar()
}

function onWheel(e: WheelEvent) {
  const el = scrollerRef.value
  if (!el || el.scrollWidth <= el.clientWidth) return
  const rawDelta = Math.abs(e.deltaY) >= Math.abs(e.deltaX) ? e.deltaY : e.deltaX
  if (rawDelta === 0) return
  e.preventDefault()
  const multiplier = e.deltaMode === 1 ? 16 : 1
  el.scrollLeft += rawDelta * multiplier
  syncScrollbar()
}

function onTrackMouseDown(e: MouseEvent) {
  const el = scrollerRef.value
  if (!el || !hasOverflow.value) return
  const rect = (e.currentTarget as HTMLElement).getBoundingClientRect()
  const clickX = e.clientX - rect.left
  const maxLeft = Math.max(1, el.clientWidth - thumbWidth.value)
  const targetThumbLeft = Math.max(0, Math.min(maxLeft, clickX - thumbWidth.value / 2))
  const maxScroll = Math.max(0, el.scrollWidth - el.clientWidth)
  el.scrollLeft = (targetThumbLeft / maxLeft) * maxScroll
  syncScrollbar()
}

function onThumbMouseDown(e: MouseEvent) {
  if (e.button !== 0) return
  const el = scrollerRef.value
  if (!el) return
  e.preventDefault()
  isDraggingThumb.value = true
  dragStartX = e.clientX
  dragStartScrollLeft = el.scrollLeft

  const onMouseMove = (ev: MouseEvent) => {
    if (!isDraggingThumb.value || !scrollerRef.value) return
    const curEl = scrollerRef.value
    const maxScroll = Math.max(0, curEl.scrollWidth - curEl.clientWidth)
    const maxLeft = Math.max(1, curEl.clientWidth - thumbWidth.value)
    const dx = ev.clientX - dragStartX
    curEl.scrollLeft = dragStartScrollLeft + (dx / maxLeft) * maxScroll
    syncScrollbar()
  }

  const onMouseUp = () => {
    isDraggingThumb.value = false
    window.removeEventListener('mousemove', onMouseMove)
    window.removeEventListener('mouseup', onMouseUp)
  }

  window.addEventListener('mousemove', onMouseMove)
  window.addEventListener('mouseup', onMouseUp)
}

watch(
  () => props.activeKey,
  () => {
    void nextTick(() => scrollActiveIntoView())
  },
)

onMounted(() => {
  syncScrollbar()
  const el = scrollerRef.value
  if (!el) return
  if (typeof ResizeObserver !== 'undefined') {
    resizeObserver = new ResizeObserver(() => syncScrollbar())
    resizeObserver.observe(el)
  }
  if (typeof MutationObserver !== 'undefined') {
    mutationObserver = new MutationObserver(() => syncScrollbar())
    mutationObserver.observe(el, { childList: true, subtree: true, characterData: true })
  }
})

onUpdated(() => {
  syncScrollbar()
})

onBeforeUnmount(() => {
  resizeObserver?.disconnect()
  resizeObserver = null
  mutationObserver?.disconnect()
  mutationObserver = null
})

defineExpose({
  syncScrollbar,
  scrollerRef,
})
</script>

<template>
  <div class="wb-tab-strip">
    <div
      class="wb-tab-scroll-wrap"
      :class="{ 'has-overflow': hasOverflow, 'is-dragging-scrollbar': isDraggingThumb }"
    >
      <div
        ref="scrollerRef"
        class="wb-tab-scroller"
        role="tablist"
        :aria-label="props.ariaLabel"
        @scroll="syncScrollbar"
        @wheel="onWheel"
      >
        <slot />
      </div>

      <div
        v-if="hasOverflow"
        class="wb-scrollbar-track"
        data-test="tab-floating-scrollbar"
        @mousedown="onTrackMouseDown"
      >
        <div
          class="wb-scrollbar-thumb"
          data-test="tab-floating-scrollbar-thumb"
          :style="{ width: `${thumbWidth}px`, transform: `translateX(${thumbLeft}px)` }"
          @mousedown.stop="onThumbMouseDown"
        />
      </div>
    </div>

    <div v-if="$slots.trailing" class="wb-tab-trailing">
      <slot name="trailing" />
    </div>
  </div>
</template>
