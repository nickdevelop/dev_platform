import { beforeEach, describe, expect, it } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useViewportStore } from '../stores/viewport'

describe('useViewportStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('records and retrieves session scroll state', () => {
    const store = useViewportStore()

    expect(store.getSessionScroll('sess-1')).toBeUndefined()

    store.saveSessionScroll('sess-1', 450, false)
    expect(store.getSessionScroll('sess-1')).toEqual({ scrollTop: 450, pinned: false })

    store.saveSessionScroll('sess-1', 1200, true)
    expect(store.getSessionScroll('sess-1')).toEqual({ scrollTop: 1200, pinned: true })
  })

  it('records and retrieves activity log scroll state independently', () => {
    const store = useViewportStore()

    store.saveSessionScroll('sess-1', 300, false)
    store.saveActivityScroll('sess-1', 150, true)

    expect(store.getSessionScroll('sess-1')).toEqual({ scrollTop: 300, pinned: false })
    expect(store.getActivityScroll('sess-1')).toEqual({ scrollTop: 150, pinned: true })

    store.clearSession('sess-1')
    expect(store.getSessionScroll('sess-1')).toBeUndefined()
    expect(store.getActivityScroll('sess-1')).toBeUndefined()
  })

  it('records and retrieves pure chat scroll state', () => {
    const store = useViewportStore()

    expect(store.getChatScroll(10)).toBeUndefined()

    store.saveChatScroll(10, 800, false)
    expect(store.getChatScroll(10)).toEqual({ scrollTop: 800, pinned: false })

    store.clearChat(10)
    expect(store.getChatScroll(10)).toBeUndefined()
  })

  it('safely handles empty or invalid keys', () => {
    const store = useViewportStore()

    store.saveSessionScroll('', 100, true)
    expect(store.getSessionScroll('')).toBeUndefined()

    store.saveChatScroll(NaN, 100, true)
    expect(store.getChatScroll(NaN)).toBeUndefined()
  })
})
