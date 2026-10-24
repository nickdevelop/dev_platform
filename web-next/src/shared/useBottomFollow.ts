import { useCallback, useEffect, useRef, useState } from 'react'

export const SCROLL_FOLLOW_THRESHOLD_PX = 24

export function useBottomFollow(changeKey: string) {
  const scrollerRef = useRef<HTMLDivElement | null>(null)
  const pinnedRef = useRef(true)
  const [showJump, setShowJump] = useState(false)

  const onScroll = useCallback(() => {
    const el = scrollerRef.current
    if (!el) return
    const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight <= SCROLL_FOLLOW_THRESHOLD_PX
    pinnedRef.current = atBottom
    if (atBottom) setShowJump(false)
  }, [])

  const scrollToBottom = useCallback((smooth: boolean) => {
    const el = scrollerRef.current
    if (!el) return
    if (smooth && typeof el.scrollTo === 'function') {
      el.scrollTo({ top: el.scrollHeight, behavior: 'smooth' })
    } else {
      el.scrollTop = el.scrollHeight
    }
    pinnedRef.current = true
    setShowJump(false)
  }, [])

  useEffect(() => {
    const frame = requestAnimationFrame(() => {
      const el = scrollerRef.current
      if (!el) return
      if (pinnedRef.current) {
        el.scrollTop = el.scrollHeight
        setShowJump(false)
      } else {
        setShowJump(true)
      }
    })
    return () => cancelAnimationFrame(frame)
  }, [changeKey])

  return { scrollerRef, showJump, onScroll, scrollToBottom }
}
