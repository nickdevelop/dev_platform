import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import CodeView from '../features/files/CodeView.vue'
import { monacoTestRegistry } from './setup/monaco'

describe('CodeView', () => {
  const sampleCode = 'const count: number = 10;\nfunction test() {\n  return count;\n}'

  it('mounts monaco editor with content and resolved language', () => {
    const wrapper = mount(CodeView, {
      props: {
        content: sampleCode,
        filePath: 'src/main.ts',
      },
    })

    const host = wrapper.find('[data-test="monaco-editor-host"]')
    expect(host.exists()).toBe(true)
    expect(host.attributes('data-monaco-value')).toBe(sampleCode)
    expect(host.attributes('data-monaco-language')).toBe('typescript')
    expect(host.attributes('data-monaco-readonly')).toBe('true')
    wrapper.unmount()
  })

  it('toggles word wrapping with Alt+Z (default: wrap enabled)', async () => {
    const wrapper = mount(CodeView, {
      props: {
        content: sampleCode,
        filePath: 'src/main.ts',
      },
    })

    const host = wrapper.find('[data-test="monaco-editor-host"]')
    const view = wrapper.find('[data-test="code-view"]')

    expect(host.attributes('data-monaco-wrap')).toBe('on')

    await view.trigger('keydown', { key: 'z', altKey: true })
    expect(host.attributes('data-monaco-wrap')).toBe('off')

    await view.trigger('keydown', { key: 'z', altKey: true })
    expect(host.attributes('data-monaco-wrap')).toBe('on')
    wrapper.unmount()
  })

  it('renders no action buttons in the top right corner (chips only)', () => {
    const wrapper = mount(CodeView, {
      props: {
        content: sampleCode,
        filePath: 'src/main.ts',
        editable: true,
      },
    })

    const actions = wrapper.find('.code-actions')
    expect(actions.exists()).toBe(true)
    expect(actions.findAll('button')).toHaveLength(0)
    expect(actions.text()).not.toContain('编辑')
    expect(actions.text()).not.toContain('退出')
    expect(actions.text()).not.toContain('保存')
    expect(actions.text()).not.toContain('换行')
    expect(wrapper.find('[data-test="code-edit-btn"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="toggle-wrap-btn"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('focuses initialLine when passed as prop', async () => {
    const wrapper = mount(CodeView, {
      props: {
        content: sampleCode,
        filePath: 'src/main.ts',
        initialLine: 2,
      },
    })

    await wrapper.vm.$nextTick()
    const host = wrapper.find('[data-test="monaco-editor-host"]')
    expect(host.attributes('data-focused-line')).toBe('2')
    expect(monacoTestRegistry.lastEditor.revealLineInCenter).toHaveBeenCalledWith(2)
    wrapper.unmount()
  })

  it('shows + button on selection end line and opens inline comment editor on click', async () => {
    const wrapper = mount(CodeView, {
      props: {
        content: 'line 1\nline 2\nline 3\nline 4\nline 5\nline 6\nline 7\nline 8',
        filePath: 'src/main.ts',
      },
    })
    await wrapper.vm.$nextTick()

    monacoTestRegistry.lastEditor._triggerSelection(3, 8)
    await wrapper.vm.$nextTick()

    const addBtn = wrapper.find('[data-test="line-add-comment-btn"]')
    expect(addBtn.exists()).toBe(true)
    expect(addBtn.attributes('aria-label')).toBe('添加选区备注')

    await addBtn.trigger('click')
    await wrapper.vm.$nextTick()

    const inlineRow = wrapper.find('[data-test="inline-comment-row"]')
    expect(inlineRow.exists()).toBe(true)

    const textarea = inlineRow.find<HTMLTextAreaElement>('[data-test="inline-comment-textarea"]')
    await textarea.setValue('检查循环性能')
    await inlineRow.find('[data-test="inline-submit-btn"]').trigger('click')
    await wrapper.vm.$nextTick()

    const emitted = wrapper.emitted('add-comment')
    expect(emitted).toBeTruthy()
    expect(emitted![0][0]).toMatchObject({
      filePath: 'src/main.ts',
      fileName: 'main.ts',
      startLine: 3,
      endLine: 8,
      codeSnippet: 'line 3\nline 4\nline 5\nline 6\nline 7\nline 8',
      comment: '检查循环性能',
    })

    expect(wrapper.find('[data-test="inline-comment-row"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('starts editable by default, tracks dirty state and saves via Ctrl+S', async () => {
    const wrapper = mount(CodeView, {
      props: {
        content: 'const x = 1',
        filePath: 'app.ts',
        editable: true,
      },
    })

    const host = wrapper.find('[data-test="monaco-editor-host"]')
    // No edit toggle: an editable file mounts with the editor unlocked
    expect(host.attributes('data-monaco-readonly')).toBe('false')
    expect(wrapper.find('[data-test="code-dirty-badge"]').exists()).toBe(false)

    // Simulate typing inside the Monaco model
    monacoTestRegistry.lastEditor.getModel().setValue('const x = 42')
    await wrapper.vm.$nextTick()

    expect(wrapper.find('[data-test="code-dirty-badge"]').exists()).toBe(true)
    expect(wrapper.emitted('dirty-change')?.slice(-1)[0]).toEqual([true])

    await wrapper.find('[data-test="code-view"]').trigger('keydown', { key: 's', ctrlKey: true })
    expect(wrapper.emitted('save')?.[0]).toEqual(['const x = 42'])

    // Parent updates content after save → dirty clears
    await wrapper.setProps({ content: 'const x = 42' })
    await wrapper.vm.$nextTick()
    expect(wrapper.find('[data-test="code-dirty-badge"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('does not clobber unsaved local edits when external content refreshes', async () => {
    const wrapper = mount(CodeView, {
      props: {
        content: 'const x = 1',
        filePath: 'app.ts',
        editable: true,
      },
    })

    monacoTestRegistry.lastEditor.getModel().setValue('const x = 42')
    await wrapper.vm.$nextTick()

    // External refresh while dirty: local draft survives
    await wrapper.setProps({ content: 'const x = 7' })
    await wrapper.vm.$nextTick()
    expect(monacoTestRegistry.lastEditor.getModel().getValue()).toBe('const x = 42')
    expect(wrapper.find('[data-test="code-dirty-badge"]').exists()).toBe(true)
    wrapper.unmount()
  })
})
