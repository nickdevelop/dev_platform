import { describe, it } from 'node:test'
import * as assert from 'node:assert'
import * as fs from 'node:fs'
import * as path from 'node:path'
import { getAppIconPath } from '../src/main/icon'

function findDesktopRoot(startDir: string): string {
  let cur = startDir
  while (cur !== path.dirname(cur)) {
    const pkgPath = path.join(cur, 'package.json')
    if (fs.existsSync(pkgPath)) {
      try {
        const pkg = JSON.parse(fs.readFileSync(pkgPath, 'utf-8'))
        if (pkg.name === 'dev-platform-desktop') {
          return cur
        }
      } catch {}
    }
    cur = path.dirname(cur)
  }
  return path.resolve(startDir, '../..')
}

describe('Application Icons and Assets', () => {
  const desktopRoot = findDesktopRoot(__dirname)
  const projectRoot = path.resolve(desktopRoot, '..')

  it('provides all required desktop build icons', () => {
    const buildPng = path.join(desktopRoot, 'build', 'icon.png')
    const buildIco = path.join(desktopRoot, 'build', 'icon.ico')
    const buildIcns = path.join(desktopRoot, 'build', 'icon.icns')

    assert.ok(fs.existsSync(buildPng), 'desktop/build/icon.png must exist')
    assert.ok(fs.existsSync(buildIco), 'desktop/build/icon.ico must exist')
    assert.ok(fs.existsSync(buildIcns), 'desktop/build/icon.icns must exist')

    assert.ok(fs.statSync(buildPng).size > 0, 'desktop/build/icon.png must not be empty')
    assert.ok(fs.statSync(buildIco).size > 0, 'desktop/build/icon.ico must not be empty')
    assert.ok(fs.statSync(buildIcns).size > 0, 'desktop/build/icon.icns must not be empty')
  })

  it('provides the runtime desktop icon in desktop/assets', () => {
    const assetIcon = path.join(desktopRoot, 'assets', 'icon.png')
    assert.ok(fs.existsSync(assetIcon), 'desktop/assets/icon.png must exist')
    assert.ok(fs.statSync(assetIcon).size > 0, 'desktop/assets/icon.png must not be empty')
  })

  it('provides web public favicons', () => {
    const faviconIco = path.join(projectRoot, 'web', 'public', 'favicon.ico')
    const faviconPng = path.join(projectRoot, 'web', 'public', 'favicon.png')

    assert.ok(fs.existsSync(faviconIco), 'web/public/favicon.ico must exist')
    assert.ok(fs.existsSync(faviconPng), 'web/public/favicon.png must exist')

    assert.ok(fs.statSync(faviconIco).size > 0, 'web/public/favicon.ico must not be empty')
    assert.ok(fs.statSync(faviconPng).size > 0, 'web/public/favicon.png must not be empty')
  })

  it('resolves the app icon path via getAppIconPath()', () => {
    const resolvedPath = getAppIconPath(__dirname)
    assert.ok(resolvedPath, 'getAppIconPath() should resolve a valid path')
    assert.ok(fs.existsSync(resolvedPath!), 'resolved icon path must exist on disk')
    assert.ok(resolvedPath!.endsWith('icon.png'), 'resolved icon path should end with icon.png')
  })

  it('declares icon configurations and assets packaging in electron-builder.yml', () => {
    const configPath = path.join(desktopRoot, 'electron-builder.yml')
    assert.ok(fs.existsSync(configPath), 'electron-builder.yml must exist')
    const content = fs.readFileSync(configPath, 'utf-8')

    assert.ok(content.includes('assets/**/*'), 'files should bundle assets directory')
    assert.ok(content.includes('buildResources: build'), 'buildResources should point to build')
    assert.ok(content.includes('icon: build/icon.png'), 'linux should configure build/icon.png')
    assert.ok(content.includes('icon: build/icon.ico'), 'win should configure build/icon.ico')
    assert.ok(content.includes('icon: build/icon.icns'), 'mac should configure build/icon.icns')
  })
})
