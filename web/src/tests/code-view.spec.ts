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
})
