import { describe, expect, it } from 'vitest'
import { imageSource } from '../shared/imageSource'
import type { ImageAttachment } from '../api/messageTypes'

function image(overrides: Partial<ImageAttachment> = {}): ImageAttachment {
  return {
    id: 'img-1',
    name: 'chart.png',
    mediaType: 'image/png',
    data: '',
    size: 128,
    ...overrides,
  }
}

describe('imageSource', () => {
  it('prefers a lazy history URL over inline data', () => {
    expect(imageSource(image({ url: '/api/projects/1/sessions/s1/history/images/img-1', data: 'aGVsbG8=' }))).toBe(
      '/api/projects/1/sessions/s1/history/images/img-1',
    )
  })

  it('builds a data URL from base64', () => {
    expect(imageSource(image({ data: 'aGVsbG8=' }))).toBe('data:image/png;base64,aGVsbG8=')
  })

  it('keeps an existing data URL intact', () => {
    expect(imageSource(image({ data: 'data:image/png;base64,aGVsbG8=' }))).toBe('data:image/png;base64,aGVsbG8=')
  })

  it('returns an empty source when neither url nor data exists', () => {
    expect(imageSource(image())).toBe('')
  })
})
