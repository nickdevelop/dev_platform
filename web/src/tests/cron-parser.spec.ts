import { describe, expect, it } from 'vitest'
import {
  buildCron,
  calculateNextRuns,
  createDefaultCronStates,
  describeCron,
  formatDateTime,
  parseCron,
} from '../shared/utils/cronParser'

describe('cronParser utility', () => {
  it('parses 5-part cron expressions (minute hour day month weekday)', () => {
    const states = parseCron('15 10 * * 1-5')
    expect(states).not.toBeNull()
    if (!states) return

    expect(states.second.specific).toEqual([0])
    expect(states.minute.specific).toEqual([15])
    expect(states.hour.specific).toEqual([10])
    expect(states.day.type).toBe('all')
    expect(states.month.type).toBe('all')
    expect(states.week.type).toBe('range')
    expect(states.week.start).toBe(1)
    expect(states.week.end).toBe(5)
    expect(states.year.type).toBe('all')
  })

  it('parses 6-part cron expressions (second minute hour day month weekday)', () => {
    const states = parseCron('30 */15 * * * ?')
    expect(states).not.toBeNull()
    if (!states) return

    expect(states.second.specific).toEqual([30])
    expect(states.minute.type).toBe('step')
    expect(states.minute.step).toBe(15)
    expect(states.week.type).toBe('none')
    expect(states.year.type).toBe('all')
  })

  it('parses standard 7-part cron expressions (second minute hour day month weekday year)', () => {
    const states = parseCron('0 0 12 ? * 1,3,5 2026')
    expect(states).not.toBeNull()
    if (!states) return

    expect(states.second.specific).toEqual([0])
    expect(states.minute.specific).toEqual([0])
    expect(states.hour.specific).toEqual([12])
    expect(states.day.type).toBe('none')
    expect(states.month.type).toBe('all')
    expect(states.week.type).toBe('specific')
    expect(states.week.specific).toEqual([1, 3, 5])
    expect(states.year.type).toBe('specific')
    expect(states.year.specific).toEqual([2026])
  })

  it('builds standard 7-part cron expression from states', () => {
    const states = createDefaultCronStates()
    states.second.specific = [10]
    states.minute.specific = [30]
    states.hour.specific = [8]
    states.day.type = 'all'
    states.month.type = 'all'
    states.week.type = 'none'
    states.year.type = 'all'

    const expr = buildCron(states)
    expect(expr).toBe('10 30 8 * * ? *')
  })

  it('calculates next 10 runs accurately', () => {
    // Every day at 09:00:00
    const from = new Date('2026-05-01T08:00:00Z')
    const runs = calculateNextRuns('0 0 9 * * ? *', 10, from)
    expect(runs).toHaveLength(10)

    for (let i = 0; i < runs.length; i++) {
      expect(runs[i].getHours()).toBe(9)
      expect(runs[i].getMinutes()).toBe(0)
      expect(runs[i].getSeconds()).toBe(0)
      if (i > 0) {
        // Consecutive days: difference should be ~24 hours
        const diffMs = runs[i].getTime() - runs[i - 1].getTime()
        expect(diffMs).toBe(24 * 3600 * 1000)
      }
    }
  })

  it('calculates next runs with second-level step interval', () => {
    const from = new Date('2026-05-01T10:00:00Z')
    const runs = calculateNextRuns('*/15 * * * * ? *', 5, from)
    expect(runs).toHaveLength(5)
    for (let i = 1; i < runs.length; i++) {
      const diffSec = (runs[i].getTime() - runs[i - 1].getTime()) / 1000
      expect(diffSec).toBe(15)
    }
  })

  it('returns empty array when year is expired in the past', () => {
    const from = new Date('2026-05-01T10:00:00Z')
    const runs = calculateNextRuns('0 0 9 * * ? 2020', 10, from)
    expect(runs).toHaveLength(0)
  })

  it('formats dates nicely into human friendly text', () => {
    const d = new Date(2026, 4, 1, 9, 0, 0)
    const formatted = formatDateTime(d)
    expect(formatted).toContain('2026-05-01 09:00:00')
  })

  it('describes cron expressions in Chinese', () => {
    expect(describeCron('0 0 9 * * ? *')).toContain('每天 09:00:00 执行一次')
    expect(describeCron('0 30 8 * * 1-5 *')).toContain('工作日')
    expect(describeCron('0 */15 * * * ? *')).toContain('每隔 15 分钟执行一次')
    expect(describeCron('invalid-cron')).toContain('格式错误')
  })
})
