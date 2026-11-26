import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import DiffViewer from '../features/files/DiffViewer.vue'
import * as filesApi from '../api/files'
import type { GitFileDiffResponse } from '../features/files/types'

describe('DiffViewer.vue', () => {
  const sampleDiff: GitFileDiffResponse = {
    path: 'src/main.ts',
    status: 'modified',
    additions: 2,
    deletions: 1,
    isBinary: false,
    originalContent: 'import { createApp } from \'vue\'\nconst port = 3000\nconst app = createApp({})\n',
    modifiedContent: 'import { createApp } from \'vue\'\nconst port = 8080\nconsole.log(\'started\')\nconst app = createApp({})\n',
    patch: `diff --git a/src/main.ts b/src/main.ts
index 1234..5678 100644
--- a/src/main.ts
+++ b/src/main.ts
@@ -1,3 +1,4 @@
 import { createApp } from 'vue'
-const port = 3000
+const port = 8080
+console.log('started')
 const app = createApp({})
`,
  }

  it('fetches and renders monaco diff editor with stats and mode toggle', async () => {
    const diffSpy = vi.spyOn(filesApi, 'getGitFileDiff').mockResolvedValue(sampleDiff)

    const wrapper = mount(DiffViewer, {
      props: {
        projectId: 1,
        filePath: 'src/main.ts',
        commitRef: 'a1b2c3d4e5f6',
        commitMessage: 'fix: update server port',
      },
    })

    await vi.waitFor(() => expect(wrapper.find('[data-test="monaco-diff-host"]').exists()).toBe(true))

    expect(diffSpy).toHaveBeenCalledWith(1, 'src/main.ts', 'a1b2c3d4e5f6', undefined)

    // Expect additions and deletions stats
    expect(wrapper.text()).toContain('+2')
    expect(wrapper.text()).toContain('-1')

    // Expect commit badge info
    expect(wrapper.text()).toContain('a1b2c3d')
    expect(wrapper.text()).toContain('fix: update server port')

    const diffHost = wrapper.find('[data-test="monaco-diff-host"]')
    expect(diffHost.attributes('data-language')).toBe('typescript')
    expect(diffHost.attributes('data-original')).toContain('const port = 3000')
    expect(diffHost.attributes('data-modified')).toContain('const port = 8080')
    expect(diffHost.attributes('data-side-by-side')).toBe('false')

    // Toggle side-by-side mode
    const toggleBtn = wrapper.find('[data-test="diff-toggle-mode-btn"]')
    expect(toggleBtn.exists()).toBe(true)
    await toggleBtn.trigger('click')
    expect(diffHost.attributes('data-side-by-side')).toBe('true')

    wrapper.unmount()
  })

  it('handles binary files with friendly notice', async () => {
    vi.spyOn(filesApi, 'getGitFileDiff').mockResolvedValue({
      path: 'assets/logo.png',
      status: 'modified',
      additions: 0,
      deletions: 0,
      isBinary: true,
      patch: 'Binary files a/assets/logo.png and b/assets/logo.png differ\n',
    })

    const wrapper = mount(DiffViewer, {
      props: {
        projectId: 1,
        filePath: 'assets/logo.png',
      },
    })

    await vi.waitFor(() => expect(wrapper.text()).toContain('二进制文件变更'))
    expect(wrapper.text()).toContain('无法在文本编辑器中展示二进制文件的差异')
    wrapper.unmount()
  })
})
