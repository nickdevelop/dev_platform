// Shared image attachment rules used by the agent composer and the pure-chat
// composer: accepted media types, the per-image size limit, and base64
// encoding in the wire shape both APIs expect. Keeping this here avoids two
// copies of the same validation drifting apart.

import type { ImageAttachment, ImageMediaType } from '../../../api/messageTypes'

export const MAX_IMAGE_SIZE_BYTES = 5 * 1024 * 1024

export const VALID_IMAGE_TYPES: Record<string, ImageMediaType> = {
  'image/jpeg': 'image/jpeg',
  'image/png': 'image/png',
  'image/webp': 'image/webp',
  'image/gif': 'image/gif',
}

// imageValidationError returns a user-facing reason the file cannot be
// attached, or null when it is acceptable.
export function imageValidationError(file: File): string | null {
  const cleanType = (file.type || '').split(';')[0].trim().toLowerCase()
  if (!VALID_IMAGE_TYPES[cleanType]) {
    return `不支持的文件类型: ${file.name || '未命名图片'} (仅支持 PNG/JPEG/WebP/GIF)`
  }
  if (file.size > MAX_IMAGE_SIZE_BYTES) {
    return `图片超过 5MB 限制: ${file.name || '未命名图片'}`
  }
  return null
}

export function readFileAsBase64(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => {
      const result = reader.result as string
      const commaIndex = result.indexOf(',')
      resolve(commaIndex !== -1 ? result.slice(commaIndex + 1) : result)
    }
    reader.onerror = () => reject(reader.error)
    reader.readAsDataURL(file)
  })
}

export async function imageAttachmentFromFile(file: File): Promise<ImageAttachment> {
  const cleanType = (file.type || '').split(';')[0].trim().toLowerCase()
  const mediaType = VALID_IMAGE_TYPES[cleanType]!
  const base64 = await readFileAsBase64(file)
  return {
    id: `img-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`,
    name: file.name || `image-${Date.now()}.png`,
    mediaType,
    data: base64,
    size: file.size,
  }
}
