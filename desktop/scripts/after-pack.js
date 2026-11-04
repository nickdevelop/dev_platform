const fs = require('node:fs')
const path = require('node:path')
const { load } = require('resedit/cjs')

/**
 * Patches a Windows PE executable with the given .ico icon data using resedit (pure JS, no Wine needed).
 *
 * @param {string} exePath - Absolute path to the .exe file
 * @param {string} iconPath - Absolute path to the .ico file
 */
async function patchWindowsExeIcon(exePath, iconPath) {
  if (!fs.existsSync(exePath)) {
    throw new Error(`Target executable not found: ${exePath}`)
  }
  if (!fs.existsSync(iconPath)) {
    throw new Error(`Icon file not found: ${iconPath}`)
  }

  const ResEdit = await load()
  const exeBuf = fs.readFileSync(exePath)
  const exe = ResEdit.NtExecutable.from(exeBuf, { ignoreCert: true })
  const res = ResEdit.NtExecutableResource.from(exe)

  const iconBuf = fs.readFileSync(iconPath)
  const iconFile = ResEdit.Data.IconFile.from(iconBuf)

  const iconGroupEntries = ResEdit.Resource.IconGroupEntry.fromEntries(res.entries)
  const iconGroupId = iconGroupEntries.length > 0 ? iconGroupEntries[0].id : 1
  const lang = iconGroupEntries.length > 0 ? iconGroupEntries[0].lang : 1033

  ResEdit.Resource.IconGroupEntry.replaceIconsForResource(
    res.entries,
    iconGroupId,
    lang,
    iconFile.icons.map((item) => item.data),
  )

  res.outputResource(exe)
  const newExeBuf = Buffer.from(exe.generate())
  fs.writeFileSync(exePath, newExeBuf)
}

/**
 * electron-builder afterPack hook.
 * Invoked after packaging platform files into appOutDir and before archive generation (e.g. zip).
 */
async function afterPack(context) {
  if (context.electronPlatformName !== 'win32') {
    return
  }

  const appOutDir = context.appOutDir
  const productFilename = context.packager.appInfo.productFilename
  const exePath = path.join(appOutDir, `${productFilename}.exe`)

  if (!fs.existsSync(exePath)) {
    console.warn(`[after-pack] Executable ${exePath} not found, skipping icon patch.`)
    return
  }

  const iconPath = path.join(context.packager.projectDir, 'build/icon.ico')
  if (!fs.existsSync(iconPath)) {
    console.warn(`[after-pack] Windows icon ${iconPath} not found, skipping icon patch.`)
    return
  }

  try {
    await patchWindowsExeIcon(exePath, iconPath)
    console.log(`[after-pack] Successfully injected custom icon into: ${exePath}`)
  } catch (err) {
    console.error(`[after-pack] Failed to inject custom icon into ${exePath}:`, err)
    throw err
  }
}

module.exports = afterPack
module.exports.patchWindowsExeIcon = patchWindowsExeIcon
