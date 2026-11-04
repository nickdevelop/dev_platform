import { ref, type Ref } from 'vue'
import type { ImageAttachment } from '../../../api/messageTypes'
import {
  imageAttachmentFromFile,
  imageValidationError,
  VALID_IMAGE_TYPES,
} from './imageFiles'

export const MAX_IMAGES_COUNT = 10
export const MAX_TEXT_FILE_SIZE_BYTES = 1024 * 1024 // 1MB

export function readFileAsText(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve((reader.result as string) || '')
    reader.onerror = () => reject(reader.error)
    reader.readAsText(file)
  })
}

export interface AttachmentUploaderOptions {
  initialImages?: ImageAttachment[]
  onNotice?: (msg: string) => void
}

export function useAttachmentUploader(options: AttachmentUploaderOptions = {}) {
  const attachedImages = ref<ImageAttachment[]>(
    options.initialImages ? [...options.initialImages] : [],
  )
  const previewModalImage = ref<ImageAttachment | null>(null)
  const fileInputRef = ref<HTMLInputElement | null>(null)

  function emitNotice(msg: string) {
    if (options.onNotice) {
      options.onNotice(msg)
    }
  }

  function removeImage(id: string) {
    attachedImages.value = attachedImages.value.filter((img) => img.id !== id)
  }

  function openImagePreview(img: ImageAttachment) {
    previewModalImage.value = img
  }

  function triggerFileInput() {
    fileInputRef.value?.click()
  }

  async function addImageFiles(files: FileList | File[]) {
    const list = Array.from(files)
    for (const file of list) {
      if (attachedImages.value.length >= MAX_IMAGES_COUNT) {
        emitNotice(`单次最多添加 ${MAX_IMAGES_COUNT} 张图片`)
        break
      }
      const validationError = imageValidationError(file)
      if (validationError !== null) {
        emitNotice(validationError)
        continue
      }
      try {
        attachedImages.value.push(await imageAttachmentFromFile(file))
      } catch {
        emitNotice(`读取图片失败: ${file.name || '未命名图片'}`)
      }
    }
  }

  async function handleUploadedFiles(
    files: FileList | File[],
    onAppendText?: (snippet: string) => void,
  ) {
    const list = Array.from(files)
    const imageFiles: File[] = []
    const textFiles: File[] = []

    for (const file of list) {
      if (file.type.startsWith('image/') || VALID_IMAGE_TYPES[file.type]) {
        imageFiles.push(file)
      } else {
        textFiles.push(file)
      }
    }

    if (imageFiles.length > 0) {
      await addImageFiles(imageFiles)
    }

    for (const file of textFiles) {
      if (file.size > MAX_TEXT_FILE_SIZE_BYTES) {
        emitNotice(`文本文件超过 1MB 限制: ${file.name}`)
        continue
      }
      try {
        const text = await readFileAsText(file)
        if (onAppendText) {
          onAppendText(`--- 文件: ${file.name} ---\n${text}\n---`)
        }
      } catch {
        emitNotice(`读取文件失败: ${file.name}`)
      }
    }
  }

  function onFileInput(event: Event, onAppendText?: (snippet: string) => void) {
    const target = event.target as HTMLInputElement
    if (target.files && target.files.length > 0) {
      void handleUploadedFiles(target.files, onAppendText)
      target.value = ''
    }
  }

  function onPaste(event: ClipboardEvent) {
    const items = event.clipboardData?.items
    if (!items) return
    const imageFiles: File[] = []
    for (const item of Array.from(items)) {
      if (item.kind === 'file' && item.type.startsWith('image/')) {
        const file = item.getAsFile()
        if (file) {
          const ext = file.type.split('/')[1]?.split('+')[0] || 'png'
          const rawName = file.name || ''
          const isGeneric =
            !rawName || rawName === 'image.png' || rawName === 'blob' || !rawName.includes('.')
          const name = isGeneric ? `pasted-image-${Date.now()}.${ext}` : rawName
          imageFiles.push(new File([file], name, { type: file.type }))
        }
      }
    }
    if (imageFiles.length > 0) {
      event.preventDefault()
      void addImageFiles(imageFiles)
    }
  }

  function onDragOver(event: DragEvent) {
    if (
      event.dataTransfer?.types?.includes('Files') ||
      event.dataTransfer?.types?.includes('text/plain')
    ) {
      event.dataTransfer.dropEffect = 'copy'
    }
  }

  function onDrop(event: DragEvent, onAppendText?: (snippet: string) => void) {
    if (!event.dataTransfer) return
    event.preventDefault()
    if (event.dataTransfer.files && event.dataTransfer.files.length > 0) {
      void handleUploadedFiles(event.dataTransfer.files, onAppendText)
    }
  }

  return {
    attachedImages,
    previewModalImage,
    fileInputRef,
    removeImage,
    openImagePreview,
    triggerFileInput,
    addImageFiles,
    handleUploadedFiles,
    onFileInput,
    onPaste,
    onDragOver,
    onDrop,
  }
}
