import { CloseOutlined, PushpinFilled, PushpinOutlined } from '@ant-design/icons'
import { Dropdown, Tooltip } from 'antd'
import type { MenuProps } from 'antd'
import { tabKey, type OpenTab } from '../../stores/openTabs'
import styles from './SessionTabs.module.css'

interface SessionTabsProps {
  tabs: OpenTab[]
  activeKey: string | null
  onSelect: (tab: OpenTab) => void
  onClose: (tab: OpenTab) => void
  onTogglePin: (tab: OpenTab) => void
  onCloseOthers: (tab: OpenTab) => void
  onCloseAll: () => void
}

function label(tab: OpenTab): string {
  switch (tab.kind) {
    case 'session':
      return tab.title || tab.sessionId
    case 'project-settings':
      return '项目设置'
    case 'new-session':
      return '新建会话'
    case 'task':
      return tab.title || `任务 #${tab.taskId}`
    case 'platform-settings':
      return '平台设置'
  }
}

function menuFor(
  tab: OpenTab,
  handlers: {
    onClose: (tab: OpenTab) => void
    onTogglePin: (tab: OpenTab) => void
    onCloseOthers: (tab: OpenTab) => void
    onCloseAll: () => void
  },
): MenuProps {
  return {
    items: [
      {
        key: 'pin',
        label: tab.pinned === true ? '取消固定' : '固定标签页',
      },
      { key: 'close', label: '关闭标签页', disabled: tab.pinned === true },
      { key: 'others', label: '关闭其他标签页' },
      { key: 'all', label: '关闭全部标签页' },
    ],
    onClick: ({ key, domEvent }) => {
      domEvent.stopPropagation()
      if (key === 'pin') handlers.onTogglePin(tab)
      if (key === 'close') handlers.onClose(tab)
      if (key === 'others') handlers.onCloseOthers(tab)
      if (key === 'all') handlers.onCloseAll()
    },
  }
}

export function SessionTabs({
  tabs,
  activeKey,
  onSelect,
  onClose,
  onTogglePin,
  onCloseOthers,
  onCloseAll,
}: SessionTabsProps) {
  if (tabs.length === 0) return null
  const handlers = { onClose, onTogglePin, onCloseOthers, onCloseAll }

  return (
    <div className={styles.strip} role="tablist" aria-label="已打开的页面" data-test="session-tabs">
      {tabs.map((tab) => {
        const key = tabKey(tab)
        const active = key === activeKey
        return (
          <Dropdown key={key} trigger={['contextMenu']} menu={menuFor(tab, handlers)}>
            <div
              className={`${styles.tab} ${active ? styles.active : ''}`}
              role="tab"
              aria-selected={active}
              tabIndex={0}
              data-test="open-tab"
              data-tab-key={key}
              data-pinned={tab.pinned === true ? 'true' : undefined}
              onClick={() => onSelect(tab)}
              onKeyDown={(event) => {
                if (event.key === 'Enter' || event.key === ' ') {
                  event.preventDefault()
                  onSelect(tab)
                }
              }}
            >
              <span className={styles.pin}>
                {tab.pinned === true ? <PushpinFilled /> : <PushpinOutlined />}
              </span>
              <span className={styles.label}>{label(tab)}</span>
              {tab.pinned !== true && (
                <Tooltip title="关闭">
                  <button
                    type="button"
                    className={styles.close}
                    aria-label={`关闭 ${label(tab)}`}
                    data-test="close-tab"
                    onClick={(event) => {
                      event.stopPropagation()
                      onClose(tab)
                    }}
                  >
                    <CloseOutlined />
                  </button>
                </Tooltip>
              )}
            </div>
          </Dropdown>
        )
      })}
    </div>
  )
}
