// Guard: a settings-centre table must fit the canvas it sits on.
//
// `el-table` does not wrap: once the declared column widths exceed the panel it
// gets a horizontal scrollbar, and a `fixed="right"` 操作 column then slides over
// whatever is underneath it. That looked like "the 更新时间 column disappeared"
// and "the status chip is cut in half" on the 服务代理 page — visible in a real
// browser, invisible to every unit test, because the markup was all *there*.
//
// The budget is the table's measured client width at the reference viewport
// (1365×720, project rail open): the 1080px canvas minus the page padding, the
// panel padding and the scrollbar gutter leaves 866px. Widening the canvas is a
// design decision; adding a column to a settings table is not, so the check
// lives here.
import { readFileSync } from 'node:fs'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const srcDir = resolve(dirname(fileURLToPath(import.meta.url)), '..')

const CANVAS_TABLE_BUDGET = 866

const TABLES: Array<{ file: string; label: string }> = [
  { file: 'features/settings/ProxySettings.vue', label: '服务代理' },
  { file: 'features/settings/TerminalSettingsView.vue', label: '远程 SSH 服务器' },
  { file: 'features/settings/ConnectionSettings.vue', label: '服务连接' },
  { file: 'features/settings/ModelSettings.vue', label: '模型列表' },
]

// One `el-table-column` opening tag, its attributes only.
const COLUMN_TAG = /<el-table-column\b([^>]*)>/g

function columnWidths(text: string): Array<{ label: string; width: number }> {
  const columns: Array<{ label: string; width: number }> = []
  for (const match of text.matchAll(COLUMN_TAG)) {
    const attrs = match[1]
    const label = /label="([^"]*)"/.exec(attrs)?.[1] ?? '?'
    // `width` is a hard column; `min-width` is the floor the column shrinks to,
    // so it is the number that decides whether the table overflows.
    const px = /(?:^|\s)(?:min-)?width="(\d+)"/.exec(attrs)?.[1]
    columns.push({ label, width: px ? Number(px) : 80 })
  }
  return columns
}

describe('settings-centre tables fit the canvas', () => {
  it('finds the tables it is meant to measure', () => {
    // An empty scan would make the budget check below vacuously true.
    const found = TABLES.filter(({ file }) => {
      const text = readFileSync(join(srcDir, file), 'utf8')
      return text.includes('<el-table')
    })
    expect(found.length).toBe(TABLES.length)
  })

  for (const { file, label } of TABLES) {
    it(`${label} stays within ${CANVAS_TABLE_BUDGET}px of column budget`, () => {
      const columns = columnWidths(readFileSync(join(srcDir, file), 'utf8'))
      const total = columns.reduce((sum, c) => sum + c.width, 0)
      const report = columns.map((c) => `${c.label}=${c.width}`).join(' + ')
      expect(total, `${label}: ${report} = ${total}px > ${CANVAS_TABLE_BUDGET}px`).toBeLessThanOrEqual(
        CANVAS_TABLE_BUDGET,
      )
    })
  }
})
