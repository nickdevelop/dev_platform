// Monaco Editor environment setup (Vite ?worker bundling) and file extension -> language mapping.
import editorWorker from 'monaco-editor/editor/editor.worker?worker'
import jsonWorker from 'monaco-editor/language/json/json.worker?worker'
import cssWorker from 'monaco-editor/language/css/css.worker?worker'
import htmlWorker from 'monaco-editor/language/html/html.worker?worker'
import tsWorker from 'monaco-editor/language/typescript/ts.worker?worker'

let environmentInitialized = false

export function ensureMonacoEnvironment(): void {
  if (environmentInitialized || typeof self === 'undefined') return
  environmentInitialized = true

  const g = self as unknown as {
    MonacoEnvironment?: {
      getWorker: (_moduleId: string, label: string) => Worker
    }
  }

  if (!g.MonacoEnvironment) {
    g.MonacoEnvironment = {
      getWorker(_moduleId: string, label: string): Worker {
        if (label === 'json') return new jsonWorker()
        if (label === 'css' || label === 'scss' || label === 'less') return new cssWorker()
        if (label === 'html' || label === 'handlebars' || label === 'razor') return new htmlWorker()
        if (label === 'typescript' || label === 'javascript') return new tsWorker()
        return new editorWorker()
      },
    }
  }
}

const EXTENSION_LANGUAGE_MAP: Record<string, string> = {
  ts: 'typescript',
  tsx: 'typescript',
  mts: 'typescript',
  cts: 'typescript',
  js: 'javascript',
  jsx: 'javascript',
  mjs: 'javascript',
  cjs: 'javascript',
  vue: 'html',
  html: 'html',
  htm: 'html',
  xml: 'xml',
  svg: 'xml',
  css: 'css',
  scss: 'scss',
  less: 'less',
  json: 'json',
  jsonc: 'json',
  md: 'markdown',
  markdown: 'markdown',
  go: 'go',
  py: 'python',
  rs: 'rust',
  java: 'java',
  kt: 'kotlin',
  kts: 'kotlin',
  c: 'c',
  h: 'c',
  cpp: 'cpp',
  cc: 'cpp',
  cxx: 'cpp',
  hpp: 'cpp',
  cs: 'csharp',
  rb: 'ruby',
  php: 'php',
  swift: 'swift',
  sh: 'shell',
  bash: 'shell',
  zsh: 'shell',
  ps1: 'powershell',
  sql: 'sql',
  yaml: 'yaml',
  yml: 'yaml',
  toml: 'ini',
  ini: 'ini',
  conf: 'ini',
  dockerfile: 'dockerfile',
  graphql: 'graphql',
  gql: 'graphql',
  lua: 'lua',
  r: 'r',
  dart: 'dart',
  puml: 'plaintext',
  plantuml: 'plaintext',
  pu: 'plaintext',
  uml: 'plaintext',
}

export function resolveMonacoLanguage(filePath?: string): string {
  if (!filePath) return 'plaintext'
  const normalized = filePath.replace(/\\/g, '/').toLowerCase()
  const baseName = normalized.split('/').pop() || ''
  if (baseName === 'dockerfile' || baseName.startsWith('dockerfile.')) {
    return 'dockerfile'
  }
  if (baseName === 'makefile' || baseName === 'gnumakefile') {
    return 'plaintext'
  }
  const dotIdx = baseName.lastIndexOf('.')
  if (dotIdx === -1) return 'plaintext'
  const ext = baseName.slice(dotIdx + 1)
  return EXTENSION_LANGUAGE_MAP[ext] || 'plaintext'
}
