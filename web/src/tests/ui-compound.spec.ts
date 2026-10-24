import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import {
  UiDialog,
  UiFormDialog,
  UiComposer,
  UiScrollFollow,
  UiBubble,
  UiTree,
  UiTreeNode,
} from '../shared/ui/compound'

describe('UiDialog', () => {
  it('renders title and closes on Esc key', async () => {
    const wrapper = mount(UiDialog, {
      props: { title: '测试弹窗' },
      slots: { default: '<p>弹窗内容</p>' },
      attachTo: document.body,
    })
    expect(wrapper.text()).toContain('测试弹窗')
    expect(wrapper.text()).toContain('弹窗内容')

    await wrapper.trigger('keydown', { key: 'Escape' })
    expect(wrapper.emitted('close')).toHaveLength(1)
    wrapper.unmount()
  })
})

describe('UiFormDialog', () => {
  it('renders buttons, error and handles submit and close', async () => {
    const wrapper = mount(UiFormDialog, {
      props: {
        title: '新建项目',
        confirmLabel: '立即创建',
        cancelLabel: '放弃',
        error: '路径不存在',
      },
      slots: { default: '<input id="test-input" />' },
      attachTo: document.body,
    })
    expect(wrapper.text()).toContain('新建项目')
    expect(wrapper.text()).toContain('路径不存在')
    expect(wrapper.text()).toContain('立即创建')
    expect(wrapper.text()).toContain('放弃')

    const confirmBtn = wrapper.find('[data-test="dialog-confirm-btn"]')
    await confirmBtn.trigger('click')
    expect(wrapper.emitted('submit')).toHaveLength(1)

    const cancelBtn = wrapper.find('[data-test="dialog-cancel-btn"]')
    await cancelBtn.trigger('click')
    expect(wrapper.emitted('close')).toHaveLength(1)
    wrapper.unmount()
  })
})

describe('UiComposer', () => {
  it('updates modelValue and renders slots', async () => {
    const wrapper = mount(UiComposer, {
      props: { modelValue: 'hello world' },
      slots: {
        tools: '<button class="tool-btn">工具</button>',
        actions: '<button class="action-btn">发送</button>',
      },
    })
    const textarea = wrapper.find('textarea')
    expect(textarea.element.value).toBe('hello world')
    expect(wrapper.find('.tool-btn').exists()).toBe(true)
    expect(wrapper.find('.action-btn').exists()).toBe(true)

    await textarea.setValue('new message')
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual(['new message'])
  })
})

describe('UiBubble', () => {
  it('renders content, alignment and slots', () => {
    const wrapper = mount(UiBubble, {
      props: {
        align: 'right',
        variant: 'primary',
      },
      slots: {
        header: '<span>发送者</span>',
        default: '这是一条消息',
        actions: '<button class="copy-btn">复制</button>',
      },
    })
    expect(wrapper.text()).toContain('发送者')
    expect(wrapper.text()).toContain('这是一条消息')
    expect(wrapper.find('.copy-btn').exists()).toBe(true)
  })
})

describe('UiScrollFollow', () => {
  it('renders content and exposes scroll methods', () => {
    const wrapper = mount(UiScrollFollow, {
      slots: { default: '<div class="item">内容</div>' },
    })
    expect(wrapper.text()).toContain('内容')
    expect(typeof wrapper.vm.scrollToBottom).toBe('function')
  })
})

describe('UiTree and UiTreeNode', () => {
  it('handles toggle and select events', async () => {
    const wrapper = mount(UiTree, {
      slots: {
        default: `
          <UiTreeNode id="node-1" label="根目录" :expanded="false">
            <template #children>
              <UiTreeNode id="child-1" label="子文件" :is-leaf="true" />
            </template>
          </UiTreeNode>
        `,
      },
      global: {
        components: { UiTreeNode },
      },
    })
    const node = wrapper.findComponent(UiTreeNode)
    expect(node.text()).toContain('根目录')

    // Click toggle button
    const toggleBtn = node.find('button[aria-label="展开"]')
    expect(toggleBtn.exists()).toBe(true)
    await toggleBtn.trigger('click')
    expect(node.emitted('toggle')?.[0]).toEqual(['node-1'])

    // Click node body
    await node.find('.cursor-pointer').trigger('click')
    expect(node.emitted('select')?.[0]).toEqual(['node-1'])
  })
})
