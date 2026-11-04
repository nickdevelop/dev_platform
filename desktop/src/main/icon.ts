import * as path from 'node:path'
import * as fs from 'node:fs'

/**
 * Resolves the application window icon path across dev, test, and packaged environments.
 * Returns the absolute path to icon.png if found, or undefined if missing.
 */
export function getAppIconPath(baseDir: string = __dirname): string | undefined {
  const resourcesPath = typeof process !== 'undefined' && process.resourcesPath ? process.resourcesPath : ''
  const cwd = process.cwd()

  const candidates = [
    // Packaged asar & extra resources
    ...(resourcesPath
      ? [
          path.join(resourcesPath, 'assets/icon.png'),
          path.join(resourcesPath, 'app.asar/assets/icon.png'),
        ]
      : []),
    // Relative to compiled module directory (__dirname)
    path.resolve(baseDir, '../../assets/icon.png'),
    path.resolve(baseDir, '../../../assets/icon.png'),
    path.resolve(baseDir, '../assets/icon.png'),
    path.resolve(baseDir, 'assets/icon.png'),
    // Relative to process.cwd()
    path.resolve(cwd, 'assets/icon.png'),
    path.resolve(cwd, 'desktop/assets/icon.png'),
    // Fallback to build resources
    path.resolve(baseDir, '../../build/icon.png'),
    path.resolve(baseDir, '../../../build/icon.png'),
    path.resolve(cwd, 'build/icon.png'),
    path.resolve(cwd, 'desktop/build/icon.png'),
  ]

  for (const candidate of candidates) {
    if (candidate && fs.existsSync(candidate)) {
      return candidate
    }
  }
  return undefined
}
