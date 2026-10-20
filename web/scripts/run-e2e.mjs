// Orchestrates the Playwright E2E run against the real stack:
//
//   1. builds the Go server and the E2E password-setup helper,
//   2. prepares an isolated scratch directory (SQLite data dir, authorized
//      workspace root, XDG data home so the platform data directory stays local),
//   3. starts the mock opencode machine-wide service, then the backend and the
//      Vite dev server (same-origin /api proxy),
//   4. runs Playwright, forwarding any CLI arguments (e.g. a spec filter),
//   5. tears everything down and removes the scratch directory.
//
// Usage: npm run test:e2e [-- <playwright args...>]
import { spawn, spawnSync } from 'node:child_process'
import { mkdirSync, mkdtempSync, readFileSync, rmSync } from 'node:fs'
import os from 'node:os'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const isWindows = os.platform() === 'win32'
const exeSuffix = isWindows ? '.exe' : ''

const webDir = path.dirname(path.dirname(fileURLToPath(import.meta.url)))
const repoDir = path.dirname(webDir)
// Fake agent executables (pi, opencode) let the workbench E2E drive the real
// adapters without the installed CLIs. They are prepended to the server's PATH
// so the adapters resolve them exactly as they would the real binaries.
const fixtureBin = path.join(webDir, 'e2e', 'fixtures', 'bin')
// The environment pre-creates /tmp/opencode for scratch data; fall back to
// the platform temp dir elsewhere.
const scratchRoot = process.env.TMPDIR ?? os.tmpdir()

// A throwaway credential for a database that is deleted after the run. It is
// handed to the server process and Playwright through the environment and is
// never written to disk or logs.
const E2E_PASSWORD = `e2e-${Date.now().toString(36)}-${process.pid.toString(36)}`
// The harness moves the backend off the default :8080 (often occupied on
// developer machines) and points the Vite /api proxy at the same address.
// It deliberately avoids 18080, which is dev.sh's default port: a developer
// running the platform locally must not collide with (or be killed by) an E2E
// run. Override with E2E_API_ADDR.
const apiAddr = process.env.E2E_API_ADDR ?? '127.0.0.1:18090'

const scratch = mkdtempSync(path.join(scratchRoot, 'platform-e2e-'))
const dataDir = path.join(scratch, 'data')
const workspaceRoot = path.join(scratch, 'workspace')
const xdgDataHome = path.join(scratch, 'xdg-data')
// The pi adapter discovers and replays sessions from this directory, and the
// fake pi executable writes to it. Pointing it into the scratch directory
// keeps the run from reading or writing the developer's real pi sessions.
const piSessionDir = path.join(scratch, 'pi-sessions')
// The mock opencode server keeps session data on disk (like the real one), so
// the store lives in the scratch directory too.
const opencodeStoreDir = path.join(scratch, 'opencode-store')
// `opencode serve --service` is a machine-wide third-party process the platform
// never starts or stops itself — so the harness starts it the way an operator
// (or the desktop app) would, and the platform adopts its registration file.
// Pointing XDG_STATE_HOME into the scratch directory keeps the run from reading
// or writing the developer's own opencode service.
const xdgStateHome = path.join(scratch, 'xdg-state')
const serviceRegistration = path.join(xdgStateHome, 'opencode', 'service.json')
const serverBin = path.join(scratch, 'platform-server' + exeSuffix)
const setupBin = path.join(scratch, 'platform-setup' + exeSuffix)
mkdirSync(workspaceRoot, { recursive: true })
mkdirSync(xdgDataHome, { recursive: true })
mkdirSync(piSessionDir, { recursive: true })
mkdirSync(xdgStateHome, { recursive: true })

const children = []
let shuttingDown = false

// start spawns a helper process for the run. `tolerateExit` is for the opencode
// service: it is a third-party process the suite is allowed to stop and start,
// so its death is a test action, not a harness failure.
function start(name, command, args, options = {}) {
  const { tolerateExit = false, ...spawnOptions } = options
  const child = spawn(command, args, { stdio: 'inherit', ...spawnOptions })
  child.on('exit', (code, signal) => {
    if (!tolerateExit && !shuttingDown && signal !== 'SIGTERM') {
      console.error(`e2e: ${name} exited unexpectedly (code=${code} signal=${signal})`)
      shutdown(1)
    }
  })
  children.push({ name, child })
  return child
}

function shutdown(exitCode) {
  if (shuttingDown) return
  shuttingDown = true
  for (const { child } of children) {
    killTree(child)
  }
  if (!isWindows) {
    // The Go server may not finish reaping an agent it was stopping when it
    // is signaled, so any fixture agent that outlived it is cleaned up
    // directly. On Windows the taskkill trees above already cover this.
    try {
      spawnSync('pkill', ['-f', fixtureBin])
    } catch {
      // pkill is best-effort; its absence is not a harness failure.
    }
  }
  rmSync(scratch, { recursive: true, force: true })
  process.exit(exitCode)
}

// killTree stops a harness child and everything below it. Unix signals the
// child (the Go server then reaps its own process groups gracefully); on
// Windows child.kill cannot deliver Ctrl+C, so taskkill /T /F ends the whole
// tree the way the platform's own Terminate does.
function killTree(child) {
  try {
    if (!isWindows) {
      child.kill('SIGTERM')
      return
    }
    if (child.pid !== undefined) {
      spawnSync('taskkill', ['/T', '/F', '/PID', String(child.pid)], { stdio: 'ignore' })
    }
  } catch {
    // Already exited.
  }
}

process.on('SIGINT', () => shutdown(130))
process.on('SIGTERM', () => shutdown(143))

async function waitFor(url, label, timeoutMs = 60_000) {
  const deadline = Date.now() + timeoutMs
  while (Date.now() < deadline) {
    try {
      const response = await fetch(url)
      if (response.ok) return
    } catch {
      // Not up yet; retry until the deadline.
    }
    await new Promise((resolve) => setTimeout(resolve, 250))
  }
  throw new Error(`e2e: ${label} did not become ready at ${url}`)
}

// waitForServiceRegistration polls the opencode service registration file — the
// file the real binary writes when it starts its machine-wide service — and then
// authenticates against the endpoint it names. Readiness is "a client can reach
// the service with the published credential", which is exactly what the platform
// will do, rather than a port guess the mock never publishes ahead of time.
async function waitForServiceRegistration(file, timeoutMs = 30_000) {
  const deadline = Date.now() + timeoutMs
  while (Date.now() < deadline) {
    try {
      const registration = JSON.parse(readFileSync(file, 'utf8'))
      const credentials = Buffer.from(`opencode:${registration.password}`).toString('base64')
      const response = await fetch(`${registration.url}/api/info`, { headers: { authorization: `Basic ${credentials}` } })
      if (response.ok) return registration
    } catch {
      // Not registered yet, or not answering yet.
    }
    await new Promise((resolve) => setTimeout(resolve, 100))
  }
  throw new Error(`e2e: the mock opencode service never became reachable via ${file}`)
}

try {
  const build = spawnSync(
    'go',
    ['build', '-o', serverBin, './cmd/platform'],
    { cwd: repoDir, stdio: 'inherit' },
  )
  if (build.status !== 0) throw new Error('e2e: building the platform server failed')

  const buildSetup = spawnSync(
    'go',
    ['build', '-o', setupBin, './e2e/setup'],
    { cwd: repoDir, stdio: 'inherit' },
  )
  if (buildSetup.status !== 0) throw new Error('e2e: building the setup helper failed')

  const setup = spawnSync(setupBin, ['-data-dir', dataDir], {
    cwd: repoDir,
    input: `${E2E_PASSWORD}\n`,
    stdio: ['pipe', 'inherit', 'inherit'],
  })
  if (setup.status !== 0) throw new Error('e2e: configuring the throwaway password failed')

  // The service exists before the platform does, and independently of it: no
  // business path in the platform may bring it up, so the harness must not give
  // the platform the chance to appear to do so.
  start(
    'opencode-service',
    process.execPath,
    [path.join(fixtureBin, 'opencode'), 'serve', '--service', '--hostname', '127.0.0.1', '--port', '0'],
    {
      env: { ...process.env, XDG_STATE_HOME: xdgStateHome, OPENCODE_MOCK_STORE_DIR: opencodeStoreDir },
      stdio: 'ignore',
      // The suite stops and starts this service on purpose; its death is not a
      // harness failure and must not tear the run down.
      tolerateExit: true,
    },
  )
  await waitForServiceRegistration(serviceRegistration)

  start('server', serverBin, ['-data-dir', dataDir], {
    env: {
      ...process.env,
      XDG_DATA_HOME: xdgDataHome,
      XDG_STATE_HOME: xdgStateHome,
      PLATFORM_LISTEN_ADDR: apiAddr,
      PI_CODING_AGENT_SESSION_DIR: piSessionDir,
      OPENCODE_MOCK_STORE_DIR: opencodeStoreDir,
      PATH: `${fixtureBin}${path.delimiter}${process.env.PATH ?? ''}`,
    },
  })
  await waitFor(`http://${apiAddr}/api/health`, 'platform server')

  // Run vite directly under node (not via npm) so SIGTERM reaches it and no
  // dev-server process survives the harness. The port defaults to 5173 and can
  // be moved with E2E_VITE_PORT when another dev server wants that port — the
  // Playwright config reads the same variable for its baseURL.
  const vitePort = process.env.E2E_VITE_PORT ?? '5173'
  start(
    'vite',
    process.execPath,
    [
      path.join(webDir, 'node_modules', 'vite', 'bin', 'vite.js'),
      '--port',
      vitePort,
      '--strictPort',
      '--host',
      '127.0.0.1',
    ],
    { cwd: webDir, env: { ...process.env, E2E_API_PROXY: `http://${apiAddr}` } },
  )
  await waitFor(`http://127.0.0.1:${vitePort}/`, 'vite dev server')

  const playwrightArgs = process.argv.slice(2)
  const result = spawnSync('npx', ['playwright', 'test', ...playwrightArgs], {
    cwd: webDir,
    stdio: 'inherit',
    env: {
      ...process.env,
      E2E_PASSWORD,
      E2E_WORKSPACE_ROOT: workspaceRoot,
    },
  })
  shutdown(result.status ?? 1)
} catch (error) {
  console.error(error instanceof Error ? error.message : error)
  shutdown(1)
}
