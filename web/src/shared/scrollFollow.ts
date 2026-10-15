// Generic "follow while at the bottom" scroll logic, reusable by any log view:
// the view scrolls to the newest content only while the user is at the bottom;
// scrolling away parks the view and reveals a jump button instead. The scroller
// geometry is read and written through the element itself so tests can drive it
// by hand (jsdom does not perform layout).

import { nextTick, ref, watch, type Ref, type WatchSource } from 'vue'

// "At the bottom" tolerates a small rounding gap so subpixel layouts, and a
// wheel that stops a hair short, still count as following the conversation.
export const SCROLL_FOLLOW_THRESHOLD_PX = 24

export function useBottomFollow(
  scroller: Ref<HTMLElement | null>,
  changeSignal: WatchSource<unknown>,
) {
  const pinned = ref(true)
  const showJump = ref(false)

  // onScroll tracks where the user is: reaching the bottom resumes following
  // and retires the jump button; scrolling away stops following so a new entry
  // can never yank the view.
  function onScroll() {
    const el = scroller.value
    if (!el) return
    const distance = el.scrollHeight - el.scrollTop - el.clientHeight
    const atBottom = distance <= SCROLL_FOLLOW_THRESHOLD_PX
    pinned.value = atBottom
    if (atBottom) showJump.value = false
  }

  function scrollToBottom(smooth: boolean) {
    const el = scroller.value
    if (!el) return
    // The click animates; following the stream lands each delta immediately, so
    // a fast token stream never queues a pile of smooth scrolls.
    if (smooth && typeof el.scrollTo === 'function') {
      el.scrollTo({ top: el.scrollHeight, behavior: 'smooth' })
    } else {
      el.scrollTop = el.scrollHeight
    }
    pinned.value = true
    showJump.value = false
  }

  watch(changeSignal, () => {
    // Wait for the entry to be in the DOM before measuring it.
    void nextTick(() => {
      const el = scroller.value
      if (!el) return
      if (pinned.value) {
        el.scrollTop = el.scrollHeight
        showJump.value = false
      } else {
        showJump.value = true
      }
    })
  })

  return { pinned, showJump, onScroll, scrollToBottom }
}
