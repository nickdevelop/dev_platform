import { spawn, ChildProcess } from 'node:child_process'
import * as http from 'node:http'
import * as path from 'node:path'
import * as fs from 'node:fs'
import { app } from 'electron'
import { LocalServiceStatus } from '../shared/types'

export class LocalServiceManager {
  private process: ChildProcess | null = null
  private port: number | null = null
  private status: LocalServiceStatus = { running: false }
  private statusListeners: Array<(status: LocalServiceStatus) => void> = []

  constructor() {}

  public onStatusChange(listener: (status: LocalServiceStatus) => void): () => void {
    this.statusListeners.push(listener)
    return () => {
      this.statusListeners = this.statusListeners.filter((l) => l !== listener)
    }
  }

  private notify() {
    for (const listener of this.statusListeners) {
      listener(this.getStatus())
    }
  }

  public getStatus(): LocalServiceStatus {
    return {
      ...this.status,
      port: this.port ?? undefined,
      pid: this.process?.pid ?? undefined,
    }
  }

  public getPort(): number | null {
    return this.port
  }

  public resolveBinaryPath(): string {
    if (process.env.PLATFORM_BIN_PATH) {
      if (fs.existsSync(process.env.PLATFORM_BIN_PATH)) {
        return process.env.PLATFORM_BIN_PATH
      }
    }

    const exeName = process.platform === 'win32' ? 'platform.exe' : 'platform'

    // Production packaged path
    const packagedPath = path.join(process.resourcesPath, 'bin', exeName)
    if (fs.existsSync(packagedPath)) {
      return packagedPath
    }

    // Development path: check repo root bin/
    const devPath = path.resolve(__dirname, '../../../bin', exeName)
    if (fs.existsSync(devPath)) {
      return devPath
    }

    // Try current working directory bin/
    const cwdPath = path.resolve(process.cwd(), 'bin', exeName)
    if (fs.existsSync(cwdPath)) {
      return cwdPath
    }

    // Parent working directory bin/
    const parentCwdPath = path.resolve(process.cwd(), '../bin', exeName)
    if (fs.existsSync(parentCwdPath)) {
      return parentCwdPath
    }

    return devPath
  }

  public resolveDataDir(): string {
    const dataDir = process.env.PLATFORM_DESKTOP_DATA_DIR || path.join(app.getPath('userData'), 'platform-data')
    if (!fs.existsSync(dataDir)) {
      fs.mkdirSync(dataDir, { recursive: true })
    }
    return dataDir
  }

  public async start(): Promise<number> {
    if (this.process && this.port) {
      return this.port
    }

    const binPath = this.resolveBinaryPath()
    if (!fs.existsSync(binPath)) {
      const err = `Platform binary not found at: ${binPath}. Please run 'make build' first.`
      this.status = { running: false, error: err }
      this.notify()
      throw new Error(err)
    }

    const dataDir = this.resolveDataDir()

    return new Promise<number>((resolve, reject) => {
      let resolved = false
      const timeout = setTimeout(() => {
        if (!resolved) {
          resolved = true
          this.stop().catch(() => {})
          const err = 'Timeout waiting for local platform service to start (15s)'
          this.status = { running: false, error: err }
          this.notify()
          reject(new Error(err))
        }
      }, 15000)

      try {
        const child = spawn(binPath, ['-listen', '127.0.0.1:0', '-data-dir', dataDir], {
          stdio: ['ignore', 'pipe', 'pipe'],
          env: {
            ...process.env,
            PLATFORM_DATA_DIR: dataDir,
          },
        })

        this.process = child
        this.status = { running: false }
        this.notify()

        let stdoutBuffer = ''

        const checkPortMatch = (data: Buffer | string) => {
          stdoutBuffer += data.toString()
          // platform listening on 127.0.0.1:XXXXX or [::1]:XXXXX
          const match = stdoutBuffer.match(/platform listening on (?:127\.0\.0\.1|\[::1\]|localhost):(\d+)/)
          if (match && !resolved) {
            const detectedPort = parseInt(match[1], 10)
            this.port = detectedPort
            this.waitForHealth(detectedPort)
              .then(() => {
                if (!resolved) {
                  resolved = true
                  clearTimeout(timeout)
                  this.status = { running: true }
                  this.notify()
                  resolve(detectedPort)
                }
              })
              .catch((err) => {
                if (!resolved) {
                  resolved = true
                  clearTimeout(timeout)
                  reject(err)
                }
              })
          }
        }

        child.stdout?.on('data', checkPortMatch)
        child.stderr?.on('data', checkPortMatch)

        child.on('error', (err) => {
          if (!resolved) {
            resolved = true
            clearTimeout(timeout)
            this.status = { running: false, error: err.message }
            this.notify()
            reject(err)
          }
        })

        child.on('exit', (code, signal) => {
          const msg = `Local platform process exited with code ${code}, signal ${signal}`
          this.process = null
          this.port = null
          this.status = { running: false, error: code !== 0 ? msg : undefined }
          this.notify()
          if (!resolved) {
            resolved = true
            clearTimeout(timeout)
            reject(new Error(msg))
          }
        })
      } catch (err: any) {
        clearTimeout(timeout)
        this.status = { running: false, error: err.message }
        this.notify()
        reject(err)
      }
    })
  }

  private async waitForHealth(port: number, retries = 30): Promise<void> {
    for (let i = 0; i < retries; i++) {
      const ok = await new Promise<boolean>((resolve) => {
        const req = http.get(`http://127.0.0.1:${port}/api/health`, (res) => {
          resolve(res.statusCode === 200)
        })
        req.on('error', () => resolve(false))
        req.setTimeout(500, () => {
          req.destroy()
          resolve(false)
        })
      })
      if (ok) return
      await new Promise((r) => setTimeout(r, 200))
    }
    throw new Error(`Health check failed on 127.0.0.1:${port}`)
  }

  public async stop(): Promise<void> {
    const child = this.process
    if (!child) {
      this.port = null
      this.status = { running: false }
      this.notify()
      return
    }

    return new Promise<void>((resolve) => {
      let exited = false
      const forceKillTimer = setTimeout(() => {
        if (!exited) {
          try {
            child.kill('SIGKILL')
          } catch {}
          exited = true
          this.process = null
          this.port = null
          this.status = { running: false }
          this.notify()
          resolve()
        }
      }, 5000)

      child.once('exit', () => {
        if (!exited) {
          exited = true
          clearTimeout(forceKillTimer)
          this.process = null
          this.port = null
          this.status = { running: false }
          this.notify()
          resolve()
        }
      })

      try {
        if (process.platform === 'win32') {
          child.kill()
        } else {
          child.kill('SIGTERM')
        }
      } catch {
        child.kill()
      }
    })
  }
}
