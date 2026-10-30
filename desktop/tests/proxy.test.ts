import { describe, it, before, after } from 'node:test'
import * as assert from 'node:assert'
import * as http from 'node:http'
import * as fs from 'node:fs'
import * as path from 'node:path'
import * as os from 'node:os'
import { DesktopProxy } from '../src/main/proxy'
import { ConnectionStore } from '../src/main/connection-store'
import { LocalServiceManager } from '../src/main/service-manager'

describe('DesktopProxy', () => {
  let tempUserDataDir: string
  let tempWebDir: string
  let mockRemoteServer: http.Server
  let mockRemotePort: number = 0
  let proxy: DesktopProxy
  let store: ConnectionStore
  let serviceManager: LocalServiceManager

  before(async () => {
    tempUserDataDir = fs.mkdtempSync(path.join(os.tmpdir(), 'desktop-test-proxy-user-'))
    tempWebDir = fs.mkdtempSync(path.join(os.tmpdir(), 'desktop-test-proxy-web-'))

    // Prepare mock web assets
    fs.writeFileSync(path.join(tempWebDir, 'index.html'), '<!DOCTYPE html><html><body>Test UI</body></html>', 'utf-8')
    fs.writeFileSync(path.join(tempWebDir, 'test.js'), 'console.log("hello test");', 'utf-8')

    // Start a mock remote server
    mockRemoteServer = http.createServer((req, res) => {
      if (req.url === '/api/health') {
        res.writeHead(200, { 'Content-Type': 'application/json' })
        res.end(JSON.stringify({ status: 'ok', version: '2.0.0-remote' }))
        return
      }
      if (req.url === '/api/echo' && req.method === 'POST') {
        let body = ''
        req.on('data', (c) => (body += c))
        req.on('end', () => {
          res.writeHead(200, {
            'Content-Type': 'application/json',
            'X-Echo-Host': req.headers.host || '',
            'X-Echo-Origin': (req.headers.origin as string) || '',
          })
          res.end(JSON.stringify({ received: body }))
        })
        return
      }
      res.writeHead(404)
      res.end('Not Found')
    })

    await new Promise<void>((resolve) => {
      mockRemoteServer.listen(0, '127.0.0.1', () => {
        const addr = mockRemoteServer.address() as any
        mockRemotePort = addr.port
        resolve()
      })
    })

    store = new ConnectionStore(tempUserDataDir)
    serviceManager = new LocalServiceManager()
    proxy = new DesktopProxy(store, serviceManager, tempWebDir)
    await proxy.start()
  })

  after(async () => {
    await proxy.stop()
    await new Promise<void>((resolve) => mockRemoteServer.close(() => resolve()))
    try {
      fs.rmSync(tempUserDataDir, { recursive: true, force: true })
      fs.rmSync(tempWebDir, { recursive: true, force: true })
    } catch {}
  })

  it('serves static assets from webDir', async () => {
    const proxyPort = proxy.getPort()
    assert.ok(proxyPort > 0)

    // Request test.js
    const jsContent = await new Promise<string>((resolve, reject) => {
      http.get(`http://127.0.0.1:${proxyPort}/test.js`, (res) => {
        assert.strictEqual(res.statusCode, 200)
        assert.ok(res.headers['content-type']?.includes('application/javascript'))
        let body = ''
        res.on('data', (c) => (body += c))
        res.on('end', () => resolve(body))
      }).on('error', reject)
    })
    assert.strictEqual(jsContent, 'console.log("hello test");')

    // Request non-existent asset route -> falls back to index.html (SPA Fallback)
    const spaContent = await new Promise<string>((resolve, reject) => {
      http.get(`http://127.0.0.1:${proxyPort}/projects/123/sessions`, (res) => {
        assert.strictEqual(res.statusCode, 200)
        assert.ok(res.headers['content-type']?.includes('text/html'))
        let body = ''
        res.on('data', (c) => (body += c))
        res.on('end', () => resolve(body))
      }).on('error', reject)
    })
    assert.ok(spaContent.includes('Test UI'))
  })

  it('returns 503 when local service is not running and active is local', async () => {
    const proxyPort = proxy.getPort()
    const resData = await new Promise<{ status: number; body: string }>((resolve, reject) => {
      http.get(`http://127.0.0.1:${proxyPort}/api/health`, (res) => {
        let body = ''
        res.on('data', (c) => (body += c))
        res.on('end', () => resolve({ status: res.statusCode || 0, body }))
      }).on('error', reject)
    })

    assert.strictEqual(resData.status, 503)
    const parsed = JSON.parse(resData.body)
    assert.strictEqual(parsed.error, 'service_unavailable')
  })

  it('transparently proxies API calls to remote server and rewrites Host/Origin', async () => {
    // Register and switch to remote server
    const remote = store.saveConnection({
      name: '测试远程服',
      type: 'remote',
      url: `http://127.0.0.1:${mockRemotePort}`,
    })
    store.setActiveId(remote.id)

    const proxyPort = proxy.getPort()

    // Test GET /api/health through proxy
    const health = await new Promise<any>((resolve, reject) => {
      http.get(`http://127.0.0.1:${proxyPort}/api/health`, (res) => {
        assert.strictEqual(res.statusCode, 200)
        let body = ''
        res.on('data', (c) => (body += c))
        res.on('end', () => resolve(JSON.parse(body)))
      }).on('error', reject)
    })

    assert.strictEqual(health.status, 'ok')
    assert.strictEqual(health.version, '2.0.0-remote')

    // Test POST /api/echo with custom Origin header
    const postRes = await new Promise<{ headers: http.IncomingHttpHeaders; body: any }>((resolve, reject) => {
      const req = http.request(
        `http://127.0.0.1:${proxyPort}/api/echo`,
        {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            Origin: `http://127.0.0.1:${proxyPort}`,
          },
        },
        (res) => {
          let body = ''
          res.on('data', (c) => (body += c))
          res.on('end', () => resolve({ headers: res.headers, body: JSON.parse(body) }))
        },
      )
      req.on('error', reject)
      req.write(JSON.stringify({ message: 'ping from desktop' }))
      req.end()
    })

    assert.strictEqual(postRes.body.received, JSON.stringify({ message: 'ping from desktop' }))
    // Verify Host and Origin headers were rewritten to match target
    assert.strictEqual(postRes.headers['x-echo-host'], `127.0.0.1:${mockRemotePort}`)
    assert.strictEqual(postRes.headers['x-echo-origin'], `http://127.0.0.1:${mockRemotePort}`)
  })
})
