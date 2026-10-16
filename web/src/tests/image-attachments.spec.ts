import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import ComposerImageTray from '../features/chat/composer/ComposerImageTray.vue'
import ImagePreviewModal from '../features/chat/rendering/ImagePreviewModal.vue'
import MessageView from '../features/chat/rendering/MessageView.vue'
import type { ImageAttachment } from '../api/sessions'
import type { ChatEntry } from '../stores/sessions'

describe('ComposerImageTray', () => {
  const dummyImages: ImageAttachment[] = [
    {
      id: 'img-1',
      name: 'screenshot.png',
      mediaType: 'image/png',
      data: 'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==',
      size: 1024 * 150, // 150 KB
    },
    {
      id: 'img-2',
      name: 'diagram.jpg',
      mediaType: 'image/jpeg',
      data: 'abc123',
      size: 1024 * 1024 * 2.5, // 2.5 MB
    },
  ]

  it('renders image cards and formatted sizes', () => {
    const wrapper = mount(ComposerImageTray, {
      props: { images: dummyImages },
    })

    const cards = wrapper.findAll('.image-card')
    expect(cards).toHaveLength(2)
    expect(cards[0].find('.image-name').text()).toBe('screenshot.png')
    expect(cards[0].find('.image-size').text()).toBe('150 KB')
    expect(cards[1].find('.image-name').text()).toBe('diagram.jpg')
    expect(cards[1].find('.image-size').text()).toBe('2.5 MB')
  })

  it('emits remove when clicking close button', async () => {
    const wrapper = mount(ComposerImageTray, {
      props: { images: dummyImages },
    })

    await wrapper.findAll('.image-remove')[0].trigger('click')
    expect(wrapper.emitted('remove')?.[0]).toEqual(['img-1'])
  })

  it('emits preview when clicking thumbnail', async () => {
    const wrapper = mount(ComposerImageTray, {
      props: { images: dummyImages },
    })

    await wrapper.findAll('.image-thumb')[0].trigger('click')
    expect(wrapper.emitted('preview')?.[0]).toEqual([dummyImages[0]])
  })
})

describe('ImagePreviewModal', () => {
  const sampleImage: ImageAttachment = {
    id: 'img-1',
    name: 'architecture.png',
    mediaType: 'image/png',
    data: 'rawdata',
    size: 2048,
  }

  it('renders image modal and handles close', async () => {
    const wrapper = mount(ImagePreviewModal, {
      props: {
        image: sampleImage,
        visible: true,
      },
    })

    const modal = document.body.querySelector('.image-modal-backdrop')
    expect(modal).not.toBeNull()
    expect(modal?.querySelector('.modal-title')?.textContent).toBe('architecture.png')

    // Close button
    const closeBtn = modal?.querySelector('.modal-close') as HTMLElement
    closeBtn?.click()
    expect(wrapper.emitted('close')).toBeTruthy()

    wrapper.unmount()
  })

  it('closes on Escape keydown', async () => {
    const wrapper = mount(ImagePreviewModal, {
      props: {
        image: sampleImage,
        visible: true,
      },
    })

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    expect(wrapper.emitted('close')).toBeTruthy()

    wrapper.unmount()
  })
})

describe('MessageView image rendering', () => {
  it('renders user attached images grid inside bubble', async () => {
    const dummyImage: ImageAttachment = {
      id: 'img-test',
      name: 'test.png',
      mediaType: 'image/png',
      data: 'base64test',
      size: 512,
    }

    const messages: ChatEntry[] = [
      {
        id: 'msg-1',
        kind: 'message',
        role: 'user',
        text: '请看这张架构图',
        images: [dummyImage],
        streaming: false,
      },
    ]

    const wrapper = mount(MessageView, {
      props: {
        messages,
        display: 'messages',
      },
    })

    const imagesGrid = wrapper.find('[data-test="message-images"]')
    expect(imagesGrid.exists()).toBe(true)
    const items = imagesGrid.findAll('.message-image-item')
    expect(items).toHaveLength(1)
    expect(items[0].attributes('title')).toBe('test.png')

    // Clicking image opens preview modal
    await items[0].trigger('click')
    const modal = document.body.querySelector('.image-modal-backdrop')
    expect(modal).not.toBeNull()
    expect(modal?.querySelector('.modal-title')?.textContent).toBe('test.png')

    wrapper.unmount()
  })
})
