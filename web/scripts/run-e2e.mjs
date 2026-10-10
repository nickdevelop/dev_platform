// Orchestrates the Playwright E2E run against the real stack:
//
//   1. builds the Go server and the E2E password-setup helper,
//   2. prepares an isolated scratch directory (SQLite data dir, authorized
//      workspace root, XDG data home so server-side trash deletes stay local),
//   3. starts the backend and the Vite dev server (same-origin /api proxy),
//   4. runs Playwright, forwarding any CLI arguments (e.g. a spec filter),
//   5. tears everything down and removes the scratch directory.
//
// Usage: npm run test:e2e [-- <playwright args...>]
import { spawn, spawnSync } from 'node:child_process'
import { mkdirSync, mkdtempSync, rmSync } from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const webDir = path.dirname(path.dirname(fileURLToPath(import.meta.url)))
const repoDir = path.dirname(webDir)
// Fake agent executables (pi, opencode) let the workbench E2E drive the real
// adapters without the installed CLIs. They are prepended to the server's PATH
// so the adapters resolve them exactly as they would the real binaries.
const fixtureBin = path.join(webDir, 'e2e', 'fixtures', 'bin')
// The environment pre-creates /tmp/opencode for scratch data; fall back to
// the platform temp dir elsewhere.
const scratchRoot = process.env.TMPDIR ?? '/tmp/opencode'

// A throwaway credential for a database that is deleted after the run. It is
// handed to the server process and Playwright through the environment and is
// never written to disk or logs.
const E2E_PASSWORD = `e2e-${Date.now().toString(36)}-${process.pid.toString(36)}`
// The harness moves the backend off the default :8080 (often occupied on
// developer machines) and points the Vite /api proxy at the same address.
const apiAddr = process.env.E2E_API_ADDR ?? '127.0.0.1:18080'

const scratch = mkdtempSync(path.join(scratchRoot, 'platform-e2e-'))
const dataDir = path.join(scratch, 'data')
const workspaceRoot = path.join(scratch, 'workspace')
const xdgDataHome = path.join(scratch, 'xdg-data')
const trashDir = path.join(xdgDataHome, 'Trash')
// The pi adapter discovers and replays sessions from this directory, and the
// fake pi executable writes to it. Pointing it into the scratch directory
// keeps the run from reading or writing the developer's real pi sessions.
const piSessionDir = path.join(scratch, 'pi-sessions')
const serverBin = path.join(scratch, 'platform-server')
const setupBin = path.join(scratch, 'platform-setup')
mkdirSync(workspaceRoot, { recursive: true })
mkdirSync(xdgDataHome, { recursive: true })
mkdirSync(piSessionDir, { recursive: true })

const children = []
let shuttingDown = false

function start(name, command, args, options = {}) {
  const child = spawn(command, args, { stdio: 'inherit', ...options })
  child.on('exit', (code, signal) => {
    if (!shuttingDown && signal !== 'SIGTERM') {
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
    try {
      child.kill('SIGTERM')
    } catch {
      // Already exited.
    }
  }
  // The Go server may not finish reaping an agent it was stopping when it is
  // signaled, so any fixture agent that outlived it is cleaned up directly.
  try {
    spawnSync('pkill', ['-f', fixtureBin])
  } catch {
    // pkill is best-effort; its absence is not a harness failure.
  }
  rmSync(scratch, { recursive: true, force: true })
  process.exit(exitCode)
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

  start('server', serverBin, ['-data-dir', dataDir], {
    env: {
      ...process.env,
      XDG_DATA_HOME: xdgDataHome,
      PLATFORM_LISTEN_ADDR: apiAddr,
      PI_CODING_AGENT_SESSION_DIR: piSessionDir,
      PATH: `${fixtureBin}${path.delimiter}${process.env.PATH ?? ''}`,
    },
  })
  await waitFor(`http://${apiAddr}/api/health`, 'platform server')

  // Run vite directly under node (not via npm) so SIGTERM reaches it and no
  // dev-server process survives the harness.
  start(
    'vite',
    process.execPath,
    [
      path.join(webDir, 'node_modules', 'vite', 'bin', 'vite.js'),
      '--port',
      '5173',
      '--strictPort',
      '--host',
      '127.0.0.1',
    ],
    { cwd: webDir, env: { ...process.env, E2E_API_PROXY: `http://${apiAddr}` } },
  )
  await waitFor('http://127.0.0.1:5173/', 'vite dev server')

  const playwrightArgs = process.argv.slice(2)
  const result = spawnSync('npx', ['playwright', 'test', ...playwrightArgs], {
    cwd: webDir,
    stdio: 'inherit',
    env: {
      ...process.env,
      E2E_PASSWORD,
      E2E_WORKSPACE_ROOT: workspaceRoot,
      E2E_TRASH_DIR: trashDir,
    },
  })
  shutdown(result.status ?? 1)
} catch (error) {
  console.error(error instanceof Error ? error.message : error)
  shutdown(1)
}
