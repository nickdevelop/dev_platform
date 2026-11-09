// One image can arrive inline (base64, e.g. a fresh upload or an older
// history response) or as a URL served on demand by the history image
// endpoint. Rendering code should not care which: this is the single place
// that turns either shape into a usable <img src>.
import type { ImageAttachment } from '../api/messageTypes'

export function imageSource(image: ImageAttachment): string {
  if (image.url) return image.url
  if (!image.data) return ''
  return image.data.startsWith('data:')
    ? image.data
    : `data:${image.mediaType};base64,${image.data}`
}
