<script setup lang="ts">
import { computed, ref } from 'vue'
import DOMPurify from 'dompurify'
import { Marked } from 'marked'
import { getDirectory, isExternalUrl, resolveRelativePath } from './pathUtils'
import ArtifactToolbar from './ArtifactToolbar.vue'
import CodeView from './CodeView.vue'

const props = defineProps<{
  projectId: number
  filePath: string
  content: string
  fileSize?: number
  lines?: number
}>()

const emit = defineEmits<{
  'open-file': [path: string]
}>()

const mode = ref<'preview' | 'source'>('preview')

const renderedHtml = computed(() => {
  if (!props.content) return ''
  const baseDir = getDirectory(props.filePath)

  const customMarked = new Marked({
    gfm: true,
    breaks: true,
  })

  // Override image renderer to resolve relative paths to raw API
  customMarked.use({
    renderer: {
      image(token) {
        let src = token.href
        if (!isExternalUrl(src)) {
          const resolved = resolveRelativePath(baseDir, src)
          src = `/api/projects/${props.projectId}/files/raw?path=${encodeURIComponent(resolved)}`
        }
        const titleAttr = token.title ? ` title="${token.title}"` : ''
        const altAttr = token.text ? ` alt="${token.text}"` : ''
        return `<img src="${src}"${altAttr}${titleAttr} class="markdown-image" />`
      },
    },
  })

  const rawHtml = customMarked.parse(props.content, { async: false }) as string
  return DOMPurify.sanitize(rawHtml)
})

function onPreviewClick(event: MouseEvent) {
  const target = event.target as HTMLElement | null
  const link = target?.closest('a')
  if (!link) return

  const href = link.getAttribute('href')
  if (!href) return

  if (isExternalUrl(href)) {
    if (href.startsWith('http://') || href.startsWith('https://')) {
      event.preventDefault()
      window.open(href, '_blank', 'noopener,noreferrer')
    }
    return
  }

  // Internal workspace file link
  event.preventDefault()
  const baseDir = getDirectory(props.filePath)
  const resolved = resolveRelativePath(baseDir, href)
  emit('open-file', resolved)
}
</script>

<template>
  <div class="markdown-artifact-view" data-test="markdown-artifact-view">
    <ArtifactToolbar
      v-model:mode="mode"
      :size="props.fileSize"
      :lines="props.lines"
      :file-path="props.filePath"
    />

    <div class="view-content">
      <CodeView
        v-if="mode === 'source'"
        :content="props.content"
        :file-path="props.filePath"
      />

      <div
        v-else
        class="markdown-preview-scroll"
        data-test="markdown-preview"
        @click="onPreviewClick"
      >
        <article class="markdown-body" v-html="renderedHtml"></article>
      </div>
    </div>
  </div>
</template>

<style scoped>
.markdown-artifact-view {
  display: flex;
  flex-direction: column;
  height: 100%;
  background: var(--color-surface, #ffffff);
  overflow: hidden;
}

.view-content {
  flex: 1;
  min-height: 0;
  position: relative;
}

.markdown-preview-scroll {
  height: 100%;
  overflow-y: auto;
  padding: 24px 32px;
}

.markdown-body {
  max-width: 860px;
  margin: 0 auto;
  font-family: var(--font-family);
  font-size: 0.92rem;
  line-height: 1.68;
  color: var(--color-text);
  word-wrap: break-word;
}

:deep(.markdown-body h1),
:deep(.markdown-body h2),
:deep(.markdown-body h3),
:deep(.markdown-body h4) {
  margin-top: 1.4em;
  margin-bottom: 0.6em;
  font-weight: 600;
  line-height: 1.35;
  color: var(--color-text);
}

:deep(.markdown-body h1) {
  font-size: 1.6rem;
  border-bottom: 1px solid var(--color-border, #e3e6eb);
  padding-bottom: 0.3em;
}

:deep(.markdown-body h2) {
  font-size: 1.3rem;
  border-bottom: 1px solid var(--color-border, #e3e6eb);
  padding-bottom: 0.2em;
}

:deep(.markdown-body h3) {
  font-size: 1.1rem;
}

:deep(.markdown-body p) {
  margin-top: 0;
  margin-bottom: 1em;
}

:deep(.markdown-body a) {
  color: var(--color-primary, #2563eb);
  text-decoration: underline;
  text-underline-offset: 2px;
  cursor: pointer;
}

:deep(.markdown-body a:hover) {
  color: var(--color-primary-hover, #1d4ed8);
}

:deep(.markdown-body code) {
  padding: 0.15em 0.35em;
  font-size: 0.85em;
  background-color: var(--color-surface-muted, #f1f5f9);
  border-radius: 4px;
  font-family: var(--font-mono);
}

:deep(.markdown-body pre) {
  padding: 12px 16px;
  overflow: auto;
  font-size: 0.85em;
  line-height: 1.5;
  background-color: var(--color-surface-muted, #f8fafc);
  border-radius: 6px;
  border: 1px solid var(--color-border, #e3e6eb);
  margin-bottom: 1em;
}

:deep(.markdown-body pre code) {
  padding: 0;
  background: transparent;
  border: none;
}

:deep(.markdown-body blockquote) {
  padding: 0 1em;
  color: var(--color-text-muted);
  border-left: 0.25em solid var(--color-border-strong, #cbd5e1);
  margin: 0 0 1em 0;
}

:deep(.markdown-body ul),
:deep(.markdown-body ol) {
  padding-left: 1.5em;
  margin-bottom: 1em;
}

:deep(.markdown-body li) {
  margin-bottom: 0.3em;
}

:deep(.markdown-body table) {
  border-collapse: collapse;
  width: 100%;
  margin-bottom: 1em;
}

:deep(.markdown-body th),
:deep(.markdown-body td) {
  padding: 6px 12px;
  border: 1px solid var(--color-border, #e3e6eb);
  font-size: 0.85rem;
}

:deep(.markdown-body th) {
  font-weight: 600;
  background-color: var(--color-surface-muted, #f8fafc);
}

:deep(.markdown-image) {
  max-width: 100%;
  height: auto;
  border-radius: 4px;
  margin: 8px 0;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.08);
}
</style>
