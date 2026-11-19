import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import CodeView from '../features/files/CodeView.vue'

describe('CodeView', () => {
  const sampleCode = 'const count: number = 10;\nfunction test() {\n  return count;\n}'

  it('renders code lines and line numbers', () => {
    const wrapper = mount(CodeView, {
      props: {
        content: sampleCode,
        filePath: 'src/main.ts',
      },
    })

    const rows = wrapper.findAll('.code-row')
    expect(rows.length).toBe(4)

    const lineNumbers = wrapper.findAll('.line-number')
    expect(lineNumbers.length).toBe(4)
    expect(lineNumbers[0].text()).toBe('1')
    expect(lineNumbers[1].text()).toBe('2')
    expect(lineNumbers[2].text()).toBe('3')
    expect(lineNumbers[3].text()).toBe('4')

    // Accessibility check: line-number container must NOT have aria-hidden="true",
    // while the text span itself should have aria-hidden="true"
    expect(lineNumbers[0].attributes('aria-hidden')).toBeUndefined()
    const numTexts = wrapper.findAll('.line-num-text')
    expect(numTexts.length).toBe(4)
    expect(numTexts[0].attributes('aria-hidden')).toBe('true')
  })

  it('applies syntax highlighting according to file extension', () => {
    const wrapper = mount(CodeView, {
      props: {
        content: sampleCode,
        filePath: 'src/main.ts',
      },
    })

    expect(wrapper.html()).toContain('class="hljs-keyword"')
    expect(wrapper.html()).toContain('const')
    expect(wrapper.html()).toContain('class="hljs-number"')
  })

  it('toggles word wrapping when clicking the wrap button', async () => {
    const wrapper = mount(CodeView, {
      props: {
        content: sampleCode,
        filePath: 'src/main.ts',
      },
    })

    const table = wrapper.find('.code-table')
    const wrapBtn = wrapper.find('[data-test="toggle-wrap-btn"]')

    expect(table.classes()).not.toContain('is-wrapped')
    expect(wrapBtn.classes()).not.toContain('is-active')

    // Click to enable wrap
    await wrapBtn.trigger('click')
    expect(table.classes()).toContain('is-wrapped')
    expect(wrapBtn.classes()).toContain('is-active')

    // Click again to disable wrap
    await wrapBtn.trigger('click')
    expect(table.classes()).not.toContain('is-wrapped')
    expect(wrapBtn.classes()).not.toContain('is-active')
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
  })

  it('highlights search matches and highlights current match on search', async () => {
    const wrapper = mount(CodeView, {
      props: {
        content: sampleCode,
        filePath: 'src/main.ts',
      },
    })

    // Trigger Ctrl+F
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'f', ctrlKey: true }))
    await wrapper.vm.$nextTick()

    const searchBar = wrapper.find('[data-test="code-search-bar"]')
    expect(searchBar.exists()).toBe(true)

    // Type query "count"
    const input = searchBar.find('input')
    await input.setValue('count')
    await wrapper.vm.$nextTick()

    // Matches should be highlighted with <mark class="search-match">
    const matches = wrapper.findAll('mark.search-match')
    expect(matches.length).toBe(2)
    // The first match is current
    expect(matches[0].classes()).toContain('is-current')
    expect(matches[1].classes()).not.toContain('is-current')

    // Navigate to next match
    const nextBtn = searchBar.findAll('button').find((b) => b.text() === '↓')
    await nextBtn?.trigger('click')
    await wrapper.vm.$nextTick()

    const matchesAfter = wrapper.findAll('mark.search-match')
    expect(matchesAfter[0].classes()).not.toContain('is-current')
    expect(matchesAfter[1].classes()).toContain('is-current')
  })

  it('opens goto-line toolbar with Ctrl+G and jumps to specified line', async () => {
    const wrapper = mount(CodeView, {
      props: {
        content: sampleCode,
        filePath: 'src/main.ts',
      },
    })

    // Trigger Ctrl+G
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'g', ctrlKey: true }))
    await wrapper.vm.$nextTick()

    const gotoBar = wrapper.find('[data-test="goto-line-bar"]')
    expect(gotoBar.exists()).toBe(true)

    const input = gotoBar.find<HTMLInputElement>('[data-test="goto-line-input"]')
    await input.setValue('3')
    await wrapper.find('[data-test="goto-line-submit-btn"]').trigger('click')
    await wrapper.vm.$nextTick()

    const rows = wrapper.findAll('.code-row')
    expect(rows[2].classes()).toContain('is-focused-line')
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
    await wrapper.vm.$nextTick()

    const rows = wrapper.findAll('.code-row')
    expect(rows[1].classes()).toContain('is-focused-line')
  })

  it('shows + button on selection end line and opens inline comment editor on click', async () => {
    const wrapper = mount(CodeView, {
      props: {
        content: 'line 1\nline 2\nline 3\nline 4\nline 5\nline 6\nline 7\nline 8',
        filePath: 'src/main.ts',
      },
    })
    await wrapper.vm.$nextTick()

    // Mock active selection from line 3 to 8
    const vm = wrapper.vm as any
    vm.selectedStartLine = 3
    vm.selectedEndLine = 8
    vm.selectedSnippet = 'line 3\nline 4\nline 5\nline 6\nline 7\nline 8'
    vm.activeSelectionEndLine = 8
    await wrapper.vm.$nextTick()

    // Check that line 8 has the add comment button
    const row8 = wrapper.find('[data-line="8"]')
    const lineNum8 = row8.find('.line-number')
    expect(lineNum8.attributes('aria-hidden')).toBeUndefined()
    const addBtn = row8.find('[data-test="line-add-comment-btn"]')
    expect(addBtn.exists()).toBe(true)
    expect(addBtn.attributes('aria-label')).toBe('添加选区备注')

    // Lines 3 to 8 should have is-in-selection class
    expect(wrapper.find('[data-line="3"]').classes()).toContain('is-in-selection')
    expect(wrapper.find('[data-line="8"]').classes()).toContain('is-in-selection')
    expect(wrapper.find('[data-line="2"]').classes()).not.toContain('is-in-selection')

    // Click the + button to open inline comment editor
    await addBtn.trigger('click')
    await wrapper.vm.$nextTick()

    const inlineRow = wrapper.find('[data-test="inline-comment-row"]')
    expect(inlineRow.exists()).toBe(true)

    // Type comment and submit
    const textarea = inlineRow.find<HTMLTextAreaElement>('[data-test="inline-comment-textarea"]')
    await textarea.setValue('检查循环性能')
    await inlineRow.find('[data-test="inline-submit-btn"]').trigger('click')
    await wrapper.vm.$nextTick()

    // add-comment event emitted
    const emitted = wrapper.emitted('add-comment')
    expect(emitted).toBeTruthy()
    expect(emitted![0][0]).toMatchObject({
      filePath: 'src/main.ts',
      fileName: 'main.ts',
      startLine: 3,
      endLine: 8,
      comment: '检查循环性能',
    })

    // Inline editor should now be closed
    expect(wrapper.find('[data-test="inline-comment-row"]').exists()).toBe(false)
  })
})
