import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { ElMessageBox } from 'element-plus'
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

  it('toggles word wrapping when clicking the wrap button (default: wrap enabled)', async () => {
    const wrapper = mount(CodeView, {
      props: {
        content: sampleCode,
        filePath: 'src/main.ts',
      },
    })

    const host = wrapper.find('[data-test="monaco-editor-host"]')
    const wrapBtn = wrapper.find('[data-test="toggle-wrap-btn"]')

    expect(host.attributes('data-monaco-wrap')).toBe('on')
    expect(wrapBtn.classes()).toContain('is-active')

    await wrapBtn.trigger('click')
    expect(host.attributes('data-monaco-wrap')).toBe('off')
    expect(wrapBtn.classes()).not.toContain('is-active')

    await wrapBtn.trigger('click')
    expect(host.attributes('data-monaco-wrap')).toBe('on')
    expect(wrapBtn.classes()).toContain('is-active')
    wrapper.unmount()
  })

  it('does not have copy and search buttons in top right actions', () => {
    const wrapper = mount(CodeView, {
      props: {
        content: sampleCode,
        filePath: 'src/main.ts',
      },
    })

    const actions = wrapper.find('.code-actions')
    expect(actions.exists()).toBe(true)
    expect(actions.text()).toContain('换行')
    expect(actions.text()).not.toContain('复制')
    expect(actions.text()).not.toContain('查找')
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

  it('supports direct editing, saving, and dirty discard confirmation via ElMessageBox', async () => {
    const confirmSpy = vi.spyOn(ElMessageBox, 'confirm').mockResolvedValue('confirm' as any)

    const wrapper = mount(CodeView, {
      props: {
        content: 'const x = 1',
        filePath: 'app.ts',
        editable: true,
      },
    })

    const editBtn = wrapper.find('[data-test="code-edit-btn"]')
    expect(editBtn.exists()).toBe(true)
    await editBtn.trigger('click')

    const host = wrapper.find('[data-test="monaco-editor-host"]')
    expect(host.attributes('data-monaco-readonly')).toBe('false')

    // Simulate editing inside Monaco model
    monacoTestRegistry.lastEditor.getModel().setValue('const x = 42')
    await wrapper.vm.$nextTick()

    expect(wrapper.find('[data-test="code-dirty-badge"]').exists()).toBe(true)
    expect(wrapper.emitted('dirty-change')?.slice(-1)[0]).toEqual([true])

    await wrapper.find('[data-test="code-save-btn"]').trigger('click')
    expect(wrapper.emitted('save')?.[0]).toEqual(['const x = 42'])

    // Simulate parent updating content after save
    await wrapper.setProps({ content: 'const x = 42' })
    expect(wrapper.find('[data-test="code-dirty-badge"]').exists()).toBe(false)

    // Modify again and test cancel confirmation dialog
    monacoTestRegistry.lastEditor.getModel().setValue('const x = 99')
    await wrapper.vm.$nextTick()
    await wrapper.find('[data-test="code-cancel-btn"]').trigger('click')
    expect(confirmSpy).toHaveBeenCalled()

    confirmSpy.mockRestore()
    wrapper.unmount()
  })
})
