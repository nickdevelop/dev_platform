import { app, BrowserWindow, ipcMain, dialog } from 'electron'
import * as path from 'node:path'
import * as http from 'node:http'
import * as https from 'node:https'
import * as url from 'node:url'
import { LocalServiceManager } from './service-manager'
import { ConnectionStore } from './connection-store'
import { DesktopProxy } from './proxy'
import { DesktopStatus, ServerConnection, TestConnectionResult } from '../shared/types'

let mainWindow: BrowserWindow | null = null
let serviceManager: LocalServiceManager
let connectionStore: ConnectionStore
let proxy: DesktopProxy

const isSingleInstance = app.requestSingleInstanceLock()
if (!isSingleInstance) {
  app.quit()
  process.exit(0)
}

app.on('second-instance', () => {
  if (mainWindow) {
    if (mainWindow.isMinimized()) mainWindow.restore()
    mainWindow.focus()
  }
})

function broadcastStatus(status: DesktopStatus) {
  if (mainWindow && !mainWindow.isDestroyed()) {
    mainWindow.webContents.send('desktop:status-changed', status)
  }
}

function getDesktopStatus(): DesktopStatus {
  const activeConn = connectionStore.getActiveConnection()
  const localStatus = serviceManager.getStatus()
  const proxyPort = proxy ? proxy.getPort() : 0

  let connectionState: DesktopStatus['connectionState'] = 'connected'
  let statusMessage: string | undefined = undefined

  if (activeConn.type === 'local') {
    if (!localStatus.running) {
      connectionState = localStatus.error ? 'error' : 'connecting'
      statusMessage = localStatus.error || '正在启动本地服务...'
    }
  }

  return {
    activeConnectionId: activeConn.id,
    connectionState,
    statusMessage,
    localService: localStatus,
    proxyPort,
  }
}

async function testTargetHealth(targetUrl: string, _token?: string): Promise<TestConnectionResult> {
  const startTime = Date.now()
  try {
    const parsed = new url.URL(targetUrl)
    const isHttps = parsed.protocol === 'https:'
    const port = parsed.port ? parseInt(parsed.port, 10) : (isHttps ? 443 : 80)
    const client = isHttps ? https : http

    const basePath = parsed.pathname.replace(/\/+$/, '')
    const healthPath = (basePath ? basePath : '') + '/api/health'

    return new Promise<TestConnectionResult>((resolve) => {
      const req = client.request(
        {
          host: parsed.hostname,
          port,
          path: healthPath,
          method: 'GET',
          headers: { Host: `${parsed.hostname}:${port}` },
          rejectUnauthorized: false,
        },
        (res) => {
          let body = ''
          res.on('data', (chunk) => (body += chunk))
          res.on('end', () => {
            const latencyMs = Date.now() - startTime
            if (res.statusCode === 200) {
              let version = 'unknown'
              try {
                const parsedBody = JSON.parse(body)
                if (parsedBody.version) version = parsedBody.version
              } catch {}
              resolve({ ok: true, status: 200, latencyMs, version })
            } else {
              resolve({
                ok: false,
                status: res.statusCode,
                message: `HTTP ${res.statusCode}`,
                latencyMs,
              })
            }
          })
        },
      )

      req.on('error', (err) => {
        resolve({
          ok: false,
          message: err.message,
          latencyMs: Date.now() - startTime,
        })
      })

      req.setTimeout(5000, () => {
        req.destroy()
        resolve({
          ok: false,
          message: '连接超时（5秒）',
          latencyMs: Date.now() - startTime,
        })
      })

      req.end()
    })
  } catch (e: any) {
    return { ok: false, message: e.message, latencyMs: 0 }
  }
}

function registerIpcHandlers() {
  ipcMain.handle('desktop:get-connections', () => {
    return connectionStore.getConnections()
  })

  ipcMain.handle('desktop:save-connection', (_event, connection) => {
    const saved = connectionStore.saveConnection(connection)
    broadcastStatus(getDesktopStatus())
    return saved
  })

  ipcMain.handle('desktop:remove-connection', (_event, id) => {
    const ok = connectionStore.removeConnection(id)
    broadcastStatus(getDesktopStatus())
    return ok
  })

  ipcMain.handle('desktop:switch-connection', async (_event, id) => {
    connectionStore.setActiveId(id)
    const active = connectionStore.getActiveConnection()

    if (active.type === 'local') {
      try {
        await serviceManager.start()
      } catch (e: any) {
        console.error('[Main] Failed to start local service on switch:', e.message)
      }
    }

    const status = getDesktopStatus()
    broadcastStatus(status)
    return status
  })

  ipcMain.handle('desktop:get-status', () => {
    return getDesktopStatus()
  })

  ipcMain.handle('desktop:test-connection', (_event, testUrl, token) => {
    return testTargetHealth(testUrl, token)
  })

  ipcMain.handle('desktop:open-directory-dialog', async (_event, defaultPath) => {
    if (!mainWindow) return null
    const result = await dialog.showOpenDialog(mainWindow, {
      title: '选择工作区目录',
      defaultPath: defaultPath || undefined,
      properties: ['openDirectory', 'createDirectory'],
    })
    if (result.canceled || result.filePaths.length === 0) {
      return null
    }
    return result.filePaths[0]
  })
}

async function createWindow(proxyPort: number) {
  mainWindow = new BrowserWindow({
    width: 1280,
    height: 800,
    minWidth: 1024,
    minHeight: 650,
    title: 'AI-Dev-Platform',
    webPreferences: {
      preload: path.join(__dirname, '../preload/index.js'),
      nodeIntegration: false,
      contextIsolation: true,
      sandbox: false,
    },
  })

  mainWindow.on('closed', () => {
    mainWindow = null
  })

  // Load the transparent local proxy URL
  await mainWindow.loadURL(`http://127.0.0.1:${proxyPort}`)
}

async function init() {
  await app.whenReady()

  connectionStore = new ConnectionStore()
  serviceManager = new LocalServiceManager()

  serviceManager.onStatusChange((_status) => {
    broadcastStatus(getDesktopStatus())
  })

  proxy = new DesktopProxy(connectionStore, serviceManager)

  const active = connectionStore.getActiveConnection()
  if (active.type === 'local') {
    try {
      await serviceManager.start()
    } catch (err: any) {
      console.error('[Main] Local service auto-start warning:', err.message)
    }
  }

  const proxyPort = await proxy.start()
  registerIpcHandlers()
  await createWindow(proxyPort)

  app.on('activate', async () => {
    if (BrowserWindow.getAllWindows().length === 0) {
      await createWindow(proxy.getPort())
    }
  })
}

app.on('window-all-closed', () => {
  if (process.platform !== 'darwin') {
    app.quit()
  }
})

let isShuttingDown = false
app.on('before-quit', async (e) => {
  if (isShuttingDown) return
  isShuttingDown = true
  e.preventDefault()

  try {
    if (serviceManager) {
      await serviceManager.stop()
    }
    if (proxy) {
      await proxy.stop()
    }
  } catch (err) {
    console.error('[Main] Shutdown error:', err)
  } finally {
    app.exit(0)
  }
})

init().catch((err) => {
  console.error('[Main] Application failed to initialize:', err)
  app.exit(1)
})
