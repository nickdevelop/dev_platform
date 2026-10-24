import { CloseOutlined, PictureOutlined, SendOutlined, StopOutlined } from '@ant-design/icons'
import { Button, Image, Input, Space } from 'antd'
import type { TextAreaRef } from 'antd/es/input/TextArea'
import { useRef, useState, type ClipboardEvent, type ReactNode } from 'react'
import type { ImageAttachment } from '../../api/messageTypes'
import { imageAttachmentFromFile, imageValidationError } from './composer/imageFiles'
import styles from './ChatComposer.module.css'

interface ChatComposerProps {
  running: boolean
  disabled?: boolean
  allowImages?: boolean
  mentionOptions?: string[]
  toolbar?: ReactNode
  onSend: (text: string, images: ImageAttachment[]) => Promise<void>
  onStop: () => Promise<void>
}

export function ChatComposer({
  running,
  disabled = false,
  allowImages = false,
  mentionOptions = [],
  toolbar,
  onSend,
  onStop,
}: ChatComposerProps) {
  const [text, setText] = useState('')
  const [images, setImages] = useState<ImageAttachment[]>([])
  const [imageError, setImageError] = useState('')
  const [busy, setBusy] = useState(false)
  const fileRef = useRef<HTMLInputElement | null>(null)
  const textareaRef = useRef<TextAreaRef | null>(null)
  const [mentionQuery, setMentionQuery] = useState<string | null>(null)

  async function addFiles(files: File[]) {
    const accepted: ImageAttachment[] = []
    for (const file of files) {
      const validation = imageValidationError(file)
      if (validation) {
        setImageError(validation)
        continue
      }
      accepted.push(await imageAttachmentFromFile(file))
    }
    if (accepted.length > 0) {
      setImages((current) => [...current, ...accepted])
      setImageError('')
    }
  }

  function updateMention(value: string, caret: number | null) {
    const position = caret ?? value.length
    const before = value.slice(0, position)
    const at = before.lastIndexOf('@')
    if (at === -1 || /\s/.test(before.slice(at + 1))) {
      setMentionQuery(null)
      return
    }
    setMentionQuery(before.slice(at + 1))
  }

  function chooseMention(path: string) {
    const textarea = textareaRef.current?.resizableTextArea?.textArea
    const caret = textarea?.selectionStart ?? text.length
    const before = text.slice(0, caret)
    const at = before.lastIndexOf('@')
    if (at === -1) return
    const next = `${text.slice(0, at)}@${path} ${text.slice(caret)}`
    setText(next)
    setMentionQuery(null)
    requestAnimationFrame(() => {
      textarea?.focus()
      textarea?.setSelectionRange(at + path.length + 2, at + path.length + 2)
    })
  }

  function onPaste(event: ClipboardEvent<HTMLTextAreaElement>) {
    if (!allowImages) return
    const files = Array.from(event.clipboardData.files).filter((file) => file.type.startsWith('image/'))
    if (files.length > 0) {
      event.preventDefault()
      void addFiles(files)
    }
  }

  async function send() {
    const value = text.trim()
    if ((value === '' && images.length === 0) || busy || disabled) return
    setBusy(true)
    try {
      await onSend(value, images)
      setText('')
      setImages([])
      setImageError('')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className={styles.composer}>
      {toolbar && <div className={styles.toolbar}>{toolbar}</div>}
      {images.length > 0 && (
        <div className={styles.tray}>
          {images.map((image) => (
            <div key={image.id} className={styles.thumb}>
              <Image
                src={`data:${image.mediaType};base64,${image.data}`}
                alt={image.name}
                width={56}
                height={56}
                style={{ objectFit: 'cover' }}
                preview={false}
              />
              <button
                type="button"
                aria-label={`移除 ${image.name}`}
                className={styles.remove}
                onClick={() => setImages((current) => current.filter((entry) => entry.id !== image.id))}
              >
                <CloseOutlined />
              </button>
            </div>
          ))}
        </div>
      )}
      {imageError !== '' && <span className={styles.error}>{imageError}</span>}
      {mentionQuery !== null && mentionOptions.length > 0 && (
        <div className={styles.mentions} aria-label="文件提及">
          {mentionOptions
            .filter((option) => option.toLowerCase().includes(mentionQuery.toLowerCase()))
            .slice(0, 8)
            .map((option) => (
              <button
                key={option}
                type="button"
                className={styles.mention}
                onMouseDown={(event) => {
                  event.preventDefault()
                  chooseMention(option)
                }}
              >
                {option}
              </button>
            ))}
        </div>
      )}
      <Input.TextArea
        data-test="message-input"
        aria-label="发送消息"
        ref={textareaRef}
        value={text}
        onChange={(event) => {
          setText(event.target.value)
          updateMention(event.target.value, event.target.selectionStart)
        }}
        onPaste={onPaste}
        onKeyDown={(event) => {
          if (event.key === 'Enter' && !event.shiftKey && !event.nativeEvent.isComposing) {
            event.preventDefault()
            void send()
          }
        }}
        autoSize={{ minRows: 2, maxRows: 8 }}
        placeholder="向 Agent 发送消息… (Enter 发送，Shift+Enter 换行)"
        disabled={disabled}
      />
      <div className={styles.bar}>
        <span className={styles.hint}>Enter 发送 · Shift+Enter 换行</span>
        <Space>
          {allowImages && (
            <>
              <input
                ref={fileRef}
                type="file"
                accept="image/png,image/jpeg,image/webp,image/gif"
                multiple
                hidden
                onChange={(event) => {
                  void addFiles(Array.from(event.target.files ?? []))
                  event.target.value = ''
                }}
              />
              <Button icon={<PictureOutlined />} onClick={() => fileRef.current?.click()} disabled={disabled}>
                图片
              </Button>
            </>
          )}
          {running && (
            <Button icon={<StopOutlined />} onClick={() => void onStop()} danger>
              停止
            </Button>
          )}
          <Button
            type="primary"
            icon={<SendOutlined />}
            loading={busy}
            disabled={disabled || (text.trim() === '' && images.length === 0)}
            onClick={() => void send()}
          >
            发送
          </Button>
        </Space>
      </div>
    </div>
  )
}
