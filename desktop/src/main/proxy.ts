import * as http from 'node:http'
import * as https from 'node:https'
import * as net from 'node:net'
import * as tls from 'node:tls'
import * as stream from 'node:stream'
import * as fs from 'node:fs'
import * as path from 'node:path'
import * as url from 'node:url'
import { app } from 'electron'
import { ConnectionStore } from './connection-store'
import { LocalServiceManager } from './service-manager'

const MIME_TYPES: Record<string, string> = {
  '.html': 'text/html; charset=utf-8',
  '.js': 'application/javascript; charset=utf-8',
  '.mjs': 'application/javascript; charset=utf-8',
  '.css': 'text/css; charset=utf-8',
  '.json': 'application/json; charset=utf-8',
  '.svg': 'image/svg+xml',
  '.png': 'image/png',
  '.jpg': 'image/jpeg',
  '.jpeg': 'image/jpeg',
  '.gif': 'image/gif',
  '.ico': 'image/x-icon',
  '.woff': 'font/woff',
  '.woff2': 'font/woff2',
  '.ttf': 'font/ttf',
}

export class DesktopProxy {
  private server: http.Server
  private port: number = 0
  private webDir: string

  constructor(
    private connectionStore: ConnectionStore,
    private serviceManager: LocalServiceManager,
    customWebDir?: string,
  ) {
    this.webDir = customWebDir || this.resolveWebDir()
    this.server = http.createServer((req, res) => this.handleHttpRequest(req, res))
    this.server.on('upgrade', (req, socket, head) => this.handleUpgrade(req, socket, head))
  }

  private resolveWebDir(): string {
    // 1. Packaged directory
    const packagedPath = path.join(process.resourcesPath, 'web')
    if (fs.existsSync(path.join(packagedPath, 'index.html'))) {
      return packagedPath
    }

    // 2. Development directory
    const devPath = path.resolve(__dirname, '../../../web/dist')
    if (fs.existsSync(path.join(devPath, 'index.html'))) {
      return devPath
    }

    const cwdPath = path.resolve(process.cwd(), 'web/dist')
    if (fs.existsSync(path.join(cwdPath, 'index.html'))) {
      return cwdPath
    }

    return devPath
  }

  public async start(): Promise<number> {
    return new Promise((resolve, reject) => {
      this.server.listen(0, '127.0.0.1', () => {
        const addr = this.server.address() as net.AddressInfo
        this.port = addr.port
        resolve(this.port)
      })
      this.server.on('error', reject)
    })
  }

  public getPort(): number {
    return this.port
  }

  public async stop(): Promise<void> {
    return new Promise((resolve) => {
      this.server.close(() => resolve())
    })
  }

  private getTarget(): { host: string; port: number; isHttps: boolean; basePath: string } | null {
    const active = this.connectionStore.getActiveConnection()
    if (active.type === 'local') {
      const localPort = this.serviceManager.getPort()
      if (!localPort) return null
      return { host: '127.0.0.1', port: localPort, isHttps: false, basePath: '' }
    }

    if (active.type === 'remote' && active.url) {
      try {
        const parsed = new url.URL(active.url)
        const isHttps = parsed.protocol === 'https:'
        const port = parsed.port ? parseInt(parsed.port, 10) : (isHttps ? 443 : 80)
        return {
          host: parsed.hostname,
          port,
          isHttps,
          basePath: parsed.pathname.replace(/\/+$/, ''),
        }
      } catch (e) {
        console.error('[DesktopProxy] Invalid remote URL:', active.url, e)
      }
    }
    return null
  }

  private handleHttpRequest(req: http.IncomingMessage, res: http.ServerResponse): void {
    const reqUrl = req.url || '/'
    const parsedUrl = url.parse(reqUrl)
    const pathname = parsedUrl.pathname || '/'

    // Pass through API and Service Proxy requests to the active backend
    if (pathname.startsWith('/api') || pathname.startsWith('/proxy')) {
      this.proxyHttpToBackend(req, res, pathname, parsedUrl.search || '')
      return
    }

    // Serve local static web UI assets
    this.serveStatic(req, res, pathname)
  }

  private serveStatic(req: http.IncomingMessage, res: http.ServerResponse, pathname: string): void {
    let filePath = path.join(this.webDir, pathname.replace(/^\/+/, ''))

    // Check if path exists and is a file
    if (fs.existsSync(filePath) && fs.statSync(filePath).isFile()) {
      this.sendFile(res, filePath)
      return
    }

    // Single Page Application Fallback: serve index.html for non-asset routes
    const indexPath = path.join(this.webDir, 'index.html')
    if (fs.existsSync(indexPath)) {
      this.sendFile(res, indexPath)
      return
    }

    res.writeHead(404, { 'Content-Type': 'text/plain' })
    res.end('Web UI not found. Please build the frontend first (cd web && npm run build).')
  }

  private sendFile(res: http.ServerResponse, filePath: string): void {
    const ext = path.extname(filePath).toLowerCase()
    const contentType = MIME_TYPES[ext] || 'application/octet-stream'
    res.writeHead(200, {
      'Content-Type': contentType,
      'Cache-Control': ext === '.html' ? 'no-cache' : 'public, max-age=31536000, immutable',
    })
    fs.createReadStream(filePath).pipe(res)
  }

  private proxyHttpToBackend(
    req: http.IncomingMessage,
    res: http.ServerResponse,
    pathname: string,
    search: string,
  ): void {
    const target = this.getTarget()
    if (!target) {
      res.writeHead(503, { 'Content-Type': 'application/json' })
      res.end(JSON.stringify({ error: 'service_unavailable', message: 'Backend service is not running or not configured' }))
      return
    }

    const targetPath = (target.basePath ? target.basePath : '') + pathname + search
    const headers = { ...req.headers }
    headers.host = `${target.host}:${target.port}`

    // Align Origin header to match target Host, preserving strict CSRF & Origin checks
    if (headers.origin) {
      headers.origin = `${target.isHttps ? 'https' : 'http'}://${target.host}:${target.port}`
    }

    const transport = target.isHttps ? https : http
    const proxyReq = transport.request(
      {
        host: target.host,
        port: target.port,
        path: targetPath,
        method: req.method,
        headers,
        rejectUnauthorized: false, // Allow self-signed certificates in internal dev servers
      },
      (targetRes) => {
        // Forward status and headers
        const resHeaders = { ...targetRes.headers }
        res.writeHead(targetRes.statusCode || 500, resHeaders)
        // Stream response body (works for JSON, SSE, binaries)
        targetRes.pipe(res)
      },
    )

    proxyReq.on('error', (err) => {
      console.error('[DesktopProxy] HTTP proxy error:', err.message)
      if (!res.headersSent) {
        res.writeHead(502, { 'Content-Type': 'application/json' })
        res.end(JSON.stringify({ error: 'bad_gateway', message: `Failed to connect to backend: ${err.message}` }))
      }
    })

    req.pipe(proxyReq)
  }

  private handleUpgrade(req: http.IncomingMessage, socket: stream.Duplex, head: Buffer): void {
    const target = this.getTarget()
    if (!target) {
      socket.write('HTTP/1.1 503 Service Unavailable\r\n\r\n')
      socket.destroy()
      return
    }

    const reqUrl = req.url || '/'
    const parsedUrl = url.parse(reqUrl)
    const targetPath = (target.basePath ? target.basePath : '') + (parsedUrl.pathname || '/') + (parsedUrl.search || '')

    // Reconstruct HTTP upgrade request line and headers
    const headers = { ...req.headers }
    headers.host = `${target.host}:${target.port}`
    // Crucial for terminal's checkSameOrigin: ensure Origin exactly matches target Host
    headers.origin = `${target.isHttps ? 'https' : 'http'}://${target.host}:${target.port}`

    let requestHeaderLines = `${req.method} ${targetPath} HTTP/1.1\r\n`
    for (const [key, value] of Object.entries(headers)) {
      if (Array.isArray(value)) {
        for (const v of value) {
          requestHeaderLines += `${key}: ${v}\r\n`
        }
      } else if (value !== undefined) {
        requestHeaderLines += `${key}: ${value}\r\n`
      }
    }
    requestHeaderLines += '\r\n'

    const onConnected = (targetSocket: stream.Duplex) => {
      // Send upgrade handshake to target server
      targetSocket.write(requestHeaderLines)
      if (head && head.length > 0) {
        targetSocket.write(head)
      }
      // Bi-directional streaming for WebSocket
      socket.pipe(targetSocket)
      targetSocket.pipe(socket)

      targetSocket.on('error', (err: Error) => {
        console.error('[DesktopProxy] WebSocket target socket error:', err.message)
        socket.destroy()
      })
    }

    const targetSocket = target.isHttps
      ? tls.connect({ host: target.host, port: target.port, rejectUnauthorized: false }, () => onConnected(targetSocket))
      : net.connect({ host: target.host, port: target.port }, () => onConnected(targetSocket))

    socket.on('error', (err: Error) => {
      console.error('[DesktopProxy] WebSocket client socket error:', err.message)
      targetSocket.destroy()
    })
  }
}
