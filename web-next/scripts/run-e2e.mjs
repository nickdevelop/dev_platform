// Deterministic React E2E harness: build the real Go server, seed a throwaway
// password, start the same-origin Vite server, then run Playwright. It does not
// start the machine-wide opencode service: the migration specs use the fake pi
// fixture only, so no external service can make the run hang.
import { mkdtempSync, mkdirSync, rmSync } from 'node:fs'
import { spawn, spawnSync } from 'node:child_process'
import { tmpdir } from 'node:os'
import path from 'node:path'
import process from 'node:process'

const webDir = path.resolve(import.meta.dirname, '..')
const repoDir = path.resolve(webDir, '..')
const fixtureBin = path.join(webDir, 'e2e', 'fixtures', 'bin')
const vitePort = process.env.E2E_VITE_PORT ?? '15178'
const apiPort = process.env.E2E_API_PORT ?? '18092'
const password = process.env.E2E_PASSWORD ?? 'e2e-password'
const scratch = mkdtempSync(path.join(tmpdir(), 'devplatform-react-e2e-'))
const dataDir = path.join(scratch, 'data')
const workspaceRoot = path.join(scratch, 'workspace')
const xdgData = path.join(scratch, 'xdg-data')
const xdgState = path.join(scratch, 'xdg-state')
const piSessions = path.join(scratch, 'pi-sessions')
const opencodeStore = path.join(scratch, 'opencode-store')
const tmp = path.join(scratch, 'tmp')
for (const dir of [dataDir, workspaceRoot, xdgData, xdgState, piSessions, opencodeStore, tmp]) {
  mkdirSync(dir, { recursive: true })
}

const children = []
let shuttingDown = false

function log(message) {
  console.log(`[e2e] ${message}`)
}

function start(name, command, args, options = {}) {
  const child = spawn(command, args, {
    cwd: options.cwd ?? repoDir,
    env: options.env ?? process.env,
    stdio: options.stdio ?? 'inherit',
  })
  children.push(child)
  child.on('exit', (code) => {
    if (!shuttingDown && code !== 0) console.error(`[e2e] ${name} exited with ${code}`)
  })
  return child
}

function shutdown(code) {
  if (shuttingDown) return
  shuttingDown = true
  for (const child of children) {
    try { child.kill('SIGTERM') } catch { /* already gone */ }
  }
  try { rmSync(scratch, { recursive: true, force: true }) } catch { /* best effort */ }
  process.exit(code)
}

process.on('SIGINT', () => shutdown(130))
process.on('SIGTERM', () => shutdown(143))

async function waitFor(url, label, timeoutMs = 60_000) {
  const deadline = Date.now() + timeoutMs
  while (Date.now() < deadline) {
    try {
      const response = await fetch(url)
      if (response.ok) return
    } catch { /* not ready */ }
    await new Promise((resolve) => setTimeout(resolve, 250))
  }
  throw new Error(`[e2e] ${label} did not become ready at ${url}`)
}

try {
  log('build Go server and setup')
  const buildEnv = { ...process.env, TMPDIR: tmp }
  for (const [output, pkg] of [
    [path.join(scratch, 'platform'), './cmd/platform'],
    [path.join(scratch, 'setup'), './e2e/setup'],
  ]) {
    const result = spawnSync('go', ['build', '-o', output, pkg], {
      cwd: repoDir,
      env: buildEnv,
      stdio: 'inherit',
    })
    if (result.status !== 0) throw new Error(`[e2e] go build ${pkg} failed`)
  }

  log('seed throwaway password')
  const setup = spawnSync(path.join(scratch, 'setup'), ['-data-dir', dataDir], {
    cwd: repoDir,
    env: { ...buildEnv, TMPDIR: tmp },
    input: `${password}\n`,
    stdio: ['pipe', 'inherit', 'inherit'],
  })
  if (setup.status !== 0) throw new Error('[e2e] password setup failed')

  log(`start platform on 127.0.0.1:${apiPort}`)
  start('server', path.join(scratch, 'platform'), [
    '-data-dir', dataDir,
    '-listen', `127.0.0.1:${apiPort}`,
  ], {
    env: {
      ...process.env,
      TMPDIR: tmp,
      PATH: `${fixtureBin}${path.delimiter}${process.env.PATH ?? ''}`,
      XDG_DATA_HOME: xdgData,
      XDG_STATE_HOME: xdgState,
      PI_CODING_AGENT_SESSION_DIR: piSessions,
      OPENCODE_MOCK_STORE_DIR: opencodeStore,
    },
  })
  await waitFor(`http://127.0.0.1:${apiPort}/api/health`, 'platform server')

  log(`start Vite on 127.0.0.1:${vitePort}`)
  start('vite', process.execPath, [
    path.join(webDir, 'node_modules', 'vite', 'bin', 'vite.js'),
    '--port', vitePort,
    '--strictPort',
    '--host', '127.0.0.1',
  ], {
    cwd: webDir,
    env: { ...process.env, TMPDIR: tmp, E2E_API_PROXY: `http://127.0.0.1:${apiPort}` },
  })
  await waitFor(`http://127.0.0.1:${vitePort}/`, 'vite dev server')

  log('run Playwright')
  const playwright = path.join(webDir, 'node_modules', '.bin', 'playwright')
  const result = spawnSync(playwright, ['test', ...process.argv.slice(2)], {
    cwd: webDir,
    stdio: 'inherit',
    env: {
      ...process.env,
      E2E_PASSWORD: password,
      E2E_WORKSPACE_ROOT: workspaceRoot,
      E2E_VITE_PORT: vitePort,
      TMPDIR: tmp,
    },
  })
  shutdown(result.status ?? 1)
} catch (error) {
  console.error(error instanceof Error ? error.message : error)
  shutdown(1)
}
