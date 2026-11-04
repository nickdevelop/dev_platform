import { describe, it } from 'node:test'
import * as assert from 'node:assert'
import { matchReloadShortcut } from '../src/main/shortcuts'

describe('Shortcuts: matchReloadShortcut', () => {
  it('ignores non-keyDown events', () => {
    const res = matchReloadShortcut({
      type: 'keyUp',
      key: 'r',
      control: true,
    })
    assert.strictEqual(res.isReload, false)
    assert.strictEqual(res.isForceReload, false)
  })

  it('matches Ctrl+R as normal reload', () => {
    const res = matchReloadShortcut({
      type: 'keyDown',
      key: 'r',
      control: true,
    })
    assert.strictEqual(res.isReload, true)
    assert.strictEqual(res.isForceReload, false)
  })

  it('matches Cmd+R (meta) as normal reload', () => {
    const res = matchReloadShortcut({
      type: 'keyDown',
      key: 'R',
      meta: true,
    })
    assert.strictEqual(res.isReload, true)
    assert.strictEqual(res.isForceReload, false)
  })

  it('matches Ctrl+Shift+R as force reload', () => {
    const res = matchReloadShortcut({
      type: 'keyDown',
      key: 'r',
      control: true,
      shift: true,
    })
    assert.strictEqual(res.isReload, false)
    assert.strictEqual(res.isForceReload, true)
  })

  it('matches Cmd+Shift+R as force reload', () => {
    const res = matchReloadShortcut({
      type: 'keyDown',
      key: 'R',
      meta: true,
      shift: true,
    })
    assert.strictEqual(res.isReload, false)
    assert.strictEqual(res.isForceReload, true)
  })

  it('matches F5 as normal reload', () => {
    const res = matchReloadShortcut({
      type: 'keyDown',
      key: 'F5',
    })
    assert.strictEqual(res.isReload, true)
    assert.strictEqual(res.isForceReload, false)
  })

  it('matches Ctrl+F5 or Shift+F5 as force reload', () => {
    const resCtrl = matchReloadShortcut({
      type: 'keyDown',
      key: 'F5',
      control: true,
    })
    assert.strictEqual(resCtrl.isReload, false)
    assert.strictEqual(resCtrl.isForceReload, true)

    const resShift = matchReloadShortcut({
      type: 'keyDown',
      key: 'f5',
      shift: true,
    })
    assert.strictEqual(resShift.isReload, false)
    assert.strictEqual(resShift.isForceReload, true)
  })

  it('ignores unrelated keys or combinations', () => {
    const res1 = matchReloadShortcut({
      type: 'keyDown',
      key: 'r',
      control: false,
      meta: false,
    })
    assert.strictEqual(res1.isReload, false)
    assert.strictEqual(res1.isForceReload, false)

    const res2 = matchReloadShortcut({
      type: 'keyDown',
      key: 'c',
      control: true,
    })
    assert.strictEqual(res2.isReload, false)
    assert.strictEqual(res2.isForceReload, false)
  })
})
