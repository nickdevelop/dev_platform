import { describe, expect, it, beforeEach } from 'vitest'
import {
  getPersistedSendShortcut,
  setPersistedSendShortcut,
  shouldSubmitOnKeydown,
} from '../features/chat/composer/sendShortcut'

describe('sendShortcut', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  describe('persistence', () => {
    it('defaults to enter', () => {
      expect(getPersistedSendShortcut()).toBe('enter')
    })

    it('persists ctrl-enter and enter', () => {
      setPersistedSendShortcut('ctrl-enter')
      expect(getPersistedSendShortcut()).toBe('ctrl-enter')
      setPersistedSendShortcut('enter')
      expect(getPersistedSendShortcut()).toBe('enter')
    })
  })

  describe('shouldSubmitOnKeydown', () => {
    it('returns false for non-Enter keys', () => {
      const event = new KeyboardEvent('keydown', { key: 'a' })
      expect(shouldSubmitOnKeydown(event, 'enter')).toBe(false)
      expect(shouldSubmitOnKeydown(event, 'ctrl-enter')).toBe(false)
    })

    describe('enter shortcut mode', () => {
      it('submits on Enter without modifiers', () => {
        const event = new KeyboardEvent('keydown', { key: 'Enter' })
        expect(shouldSubmitOnKeydown(event, 'enter')).toBe(true)
      })

      it('does not submit on Shift+Enter', () => {
        const event = new KeyboardEvent('keydown', { key: 'Enter', shiftKey: true })
        expect(shouldSubmitOnKeydown(event, 'enter')).toBe(false)
      })

      it('does not submit on Ctrl+Enter or Cmd+Enter in enter mode', () => {
        const ctrlEvent = new KeyboardEvent('keydown', { key: 'Enter', ctrlKey: true })
        const metaEvent = new KeyboardEvent('keydown', { key: 'Enter', metaKey: true })
        expect(shouldSubmitOnKeydown(ctrlEvent, 'enter')).toBe(false)
        expect(shouldSubmitOnKeydown(metaEvent, 'enter')).toBe(false)
      })
    })

    describe('ctrl-enter shortcut mode', () => {
      it('submits on Ctrl+Enter', () => {
        const event = new KeyboardEvent('keydown', { key: 'Enter', ctrlKey: true })
        expect(shouldSubmitOnKeydown(event, 'ctrl-enter')).toBe(true)
      })

      it('submits on Cmd+Enter (metaKey)', () => {
        const event = new KeyboardEvent('keydown', { key: 'Enter', metaKey: true })
        expect(shouldSubmitOnKeydown(event, 'ctrl-enter')).toBe(true)
      })

      it('does not submit on plain Enter or Shift+Enter', () => {
        const plainEvent = new KeyboardEvent('keydown', { key: 'Enter' })
        const shiftEvent = new KeyboardEvent('keydown', { key: 'Enter', shiftKey: true })
        expect(shouldSubmitOnKeydown(plainEvent, 'ctrl-enter')).toBe(false)
        expect(shouldSubmitOnKeydown(shiftEvent, 'ctrl-enter')).toBe(false)
      })
    })
  })
})
