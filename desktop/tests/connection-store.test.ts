import { describe, it, before, after } from 'node:test'
import * as assert from 'node:assert'
import * as fs from 'node:fs'
import * as path from 'node:path'
import * as os from 'node:os'
import { ConnectionStore } from '../src/main/connection-store'

describe('ConnectionStore', () => {
  let tempDir: string

  before(() => {
    tempDir = fs.mkdtempSync(path.join(os.tmpdir(), 'desktop-test-connections-'))
  })

  after(() => {
    try {
      fs.rmSync(tempDir, { recursive: true, force: true })
    } catch {}
  })

  it('initializes with default local connection', () => {
    const store = new ConnectionStore(tempDir)
    const connections = store.getConnections()
    assert.strictEqual(connections.length, 1)
    assert.strictEqual(connections[0].id, 'local')
    assert.strictEqual(connections[0].type, 'local')
    assert.strictEqual(connections[0].name, '本地内置服务')

    const active = store.getActiveConnection()
    assert.strictEqual(active.id, 'local')
  })

  it('saves and encrypts/decrypts token for remote connection', () => {
    const store = new ConnectionStore(tempDir)
    const saved = store.saveConnection({
      name: '研发服务器-A',
      type: 'remote',
      url: 'http://192.168.1.88:8080',
      token: 'secret-password-123',
    })

    assert.ok(saved.id)
    assert.strictEqual(saved.token, '***') // Sanitized

    const list = store.getConnections()
    assert.strictEqual(list.length, 2)
    const remote = list.find((c) => c.id === saved.id)
    assert.ok(remote)
    assert.strictEqual(remote.name, '研发服务器-A')
    assert.strictEqual(remote.token, '***')

    // Raw connection retrieves decrypted token
    const raw = store.getRawConnection(saved.id)
    assert.ok(raw)
    assert.strictEqual(raw.resolvedToken, 'secret-password-123')
  })

  it('switches active connection and updates lastConnectedAt', () => {
    const store = new ConnectionStore(tempDir)
    const remote = store.saveConnection({
      name: '测试服',
      type: 'remote',
      url: 'http://test.internal:8080',
    })

    const ok = store.setActiveId(remote.id)
    assert.strictEqual(ok, true)

    const active = store.getActiveConnection()
    assert.strictEqual(active.id, remote.id)
    assert.ok(active.lastConnectedAt)
  })

  it('protects local connection from removal and safely cleans up removed active connection', () => {
    const store = new ConnectionStore(tempDir)
    // Refuses to remove local
    const removedLocal = store.removeConnection('local')
    assert.strictEqual(removedLocal, false)

    const remote = store.saveConnection({
      name: '待删除服务器',
      type: 'remote',
      url: 'http://delete.me:8080',
    })

    store.setActiveId(remote.id)
    assert.strictEqual(store.getActiveConnection().id, remote.id)

    // Remove the currently active remote connection
    const removedRemote = store.removeConnection(remote.id)
    assert.strictEqual(removedRemote, true)

    // Automatically falls back to local
    const fallbackActive = store.getActiveConnection()
    assert.strictEqual(fallbackActive.id, 'local')
  })
})
