// Cron expression parser, generator, human-readable translator, and
// next execution times forecaster supporting standard 7 fields:
// [second] [minute] [hour] [day] [month] [week] [year]

export type FieldType = 'all' | 'none' | 'range' | 'step' | 'specific'

export interface FieldState {
  type: FieldType
  start?: number
  end?: number
  step?: number
  specific: number[]
}

export interface CronFieldStates {
  second: FieldState
  minute: FieldState
  hour: FieldState
  day: FieldState
  month: FieldState
  week: FieldState
  year: FieldState
}

export interface FieldConfig {
  name: string
  min: number
  max: number
  allowNone?: boolean // Allows '?' (day of month, day of week)
}

export const FIELD_CONFIGS: Record<keyof CronFieldStates, FieldConfig> = {
  second: { name: '秒', min: 0, max: 59 },
  minute: { name: '分', min: 0, max: 59 },
  hour: { name: '时', min: 0, max: 23 },
  day: { name: '日', min: 1, max: 31, allowNone: true },
  month: { name: '月', min: 1, max: 12 },
  week: { name: '周', min: 1, max: 7, allowNone: true }, // 1=Mon, 7=Sun
  year: { name: '年', min: 1970, max: 2099 },
}

export function createDefaultFieldState(type: FieldType = 'all', specific: number[] = []): FieldState {
  return {
    type,
    start: undefined,
    end: undefined,
    step: undefined,
    specific,
  }
}

export function createDefaultCronStates(): CronFieldStates {
  return {
    second: { type: 'specific', specific: [0] },
    minute: { type: 'specific', specific: [0] },
    hour: { type: 'specific', specific: [9] },
    day: { type: 'all', specific: [] },
    month: { type: 'all', specific: [] },
    week: { type: 'none', specific: [] },
    year: { type: 'all', specific: [] },
  }
}

// Convert a single FieldState into its cron token representation
export function formatField(state: FieldState, config: FieldConfig): string {
  switch (state.type) {
    case 'all':
      return '*'
    case 'none':
      return config.allowNone ? '?' : '*'
    case 'range': {
      const start = state.start ?? config.min
      const end = state.end ?? config.max
      return `${start}-${end}`
    }
    case 'step': {
      const start = state.start ?? config.min
      const step = state.step && state.step > 0 ? state.step : 1
      return `${start}/${step}`
    }
    case 'specific': {
      if (!state.specific || state.specific.length === 0) {
        return String(config.min)
      }
      const sorted = [...new Set(state.specific)].sort((a, b) => a - b)
      return sorted.join(',')
    }
    default:
      return '*'
  }
}

// Build a full 7-field cron expression: second minute hour day month week year
export function buildCron(states: CronFieldStates): string {
  const sec = formatField(states.second, FIELD_CONFIGS.second)
  const min = formatField(states.minute, FIELD_CONFIGS.minute)
  const hr = formatField(states.hour, FIELD_CONFIGS.hour)
  const day = formatField(states.day, FIELD_CONFIGS.day)
  const mon = formatField(states.month, FIELD_CONFIGS.month)
  const wk = formatField(states.week, FIELD_CONFIGS.week)
  const yr = formatField(states.year, FIELD_CONFIGS.year)

  // Standard 7-field representation
  return `${sec} ${min} ${hr} ${day} ${mon} ${wk} ${yr}`.trim()
}

// Parse a single cron field token into FieldState
export function parseFieldToken(token: string, config: FieldConfig): FieldState {
  const t = token.trim()
  if (!t || t === '*') {
    return { type: 'all', specific: [] }
  }
  if (t === '?') {
    return { type: 'none', specific: [] }
  }

  // Step: 0/5 or */5
  if (t.includes('/')) {
    const [startStr, stepStr] = t.split('/')
    const start = startStr === '*' ? config.min : parseInt(startStr, 10)
    const step = parseInt(stepStr, 10)
    if (!Number.isNaN(step) && step > 0) {
      return {
        type: 'step',
        start: Number.isNaN(start) ? config.min : start,
        step,
        specific: [],
      }
    }
  }

  // Range: 1-5
  if (t.includes('-')) {
    const [startStr, endStr] = t.split('-')
    const start = parseInt(startStr, 10)
    const end = parseInt(endStr, 10)
    if (!Number.isNaN(start) && !Number.isNaN(end)) {
      return {
        type: 'range',
        start,
        end,
        specific: [],
      }
    }
  }

  // Specific list: 1,2,3 or single number
  const parts = t.split(',').map((p) => parseInt(p.trim(), 10))
  const valid = parts.filter((n) => !Number.isNaN(n) && n >= config.min && n <= config.max)
  if (valid.length > 0) {
    return {
      type: 'specific',
      specific: [...new Set(valid)].sort((a, b) => a - b),
    }
  }

  // Fallback to all
  return { type: 'all', specific: [] }
}

// Reverse parse incoming expression into 7-field states
// Supports 5-part (min hr day mon week), 6-part (sec min hr day mon week), or 7-part (sec min hr day mon week year)
export function parseCron(expr: string): CronFieldStates | null {
  if (!expr || typeof expr !== 'string') return null
  const parts = expr.trim().split(/\s+/)
  if (parts.length < 5 || parts.length > 7) {
    return null
  }

  let secToken = '0'
  let minToken = '0'
  let hrToken = '0'
  let dayToken = '*'
  let monToken = '*'
  let weekToken = '?'
  let yearToken = '*'

  if (parts.length === 5) {
    // 5-part: minute hour day month week
    ;[minToken, hrToken, dayToken, monToken, weekToken] = parts
    secToken = '0'
    yearToken = '*'
  } else if (parts.length === 6) {
    // 6-part: second minute hour day month week
    ;[secToken, minToken, hrToken, dayToken, monToken, weekToken] = parts
    yearToken = '*'
  } else if (parts.length === 7) {
    // 7-part: second minute hour day month week year
    ;[secToken, minToken, hrToken, dayToken, monToken, weekToken, yearToken] = parts
  }

  return {
    second: parseFieldToken(secToken, FIELD_CONFIGS.second),
    minute: parseFieldToken(minToken, FIELD_CONFIGS.minute),
    hour: parseFieldToken(hrToken, FIELD_CONFIGS.hour),
    day: parseFieldToken(dayToken, FIELD_CONFIGS.day),
    month: parseFieldToken(monToken, FIELD_CONFIGS.month),
    week: parseFieldToken(weekToken, FIELD_CONFIGS.week),
    year: parseFieldToken(yearToken, FIELD_CONFIGS.year),
  }
}

// Evaluates whether a value matches a given FieldState
export function matchesField(val: number, state: FieldState, config: FieldConfig): boolean {
  switch (state.type) {
    case 'all':
      return true
    case 'none':
      return true
    case 'range': {
      const start = state.start ?? config.min
      const end = state.end ?? config.max
      return val >= start && val <= end
    }
    case 'step': {
      const start = state.start ?? config.min
      const step = state.step && state.step > 0 ? state.step : 1
      if (val < start) return false
      return (val - start) % step === 0
    }
    case 'specific':
      return state.specific.includes(val)
    default:
      return true
  }
}

// Convert JS Day of week (0=Sun, 1=Mon, ..., 6=Sat) to 1=Mon .. 7=Sun
export function jsDayToCronWeek(jsDay: number): number {
  return jsDay === 0 ? 7 : jsDay
}

function pad2(n: number): string {
  return String(n).padStart(2, '0')
}

export function formatDateTime(d: Date): string {
  const y = d.getFullYear()
  const m = pad2(d.getMonth() + 1)
  const day = pad2(d.getDate())
  const hr = pad2(d.getHours())
  const min = pad2(d.getMinutes())
  const sec = pad2(d.getSeconds())
  const weekMap = ['', '周一', '周二', '周三', '周四', '周五', '周六', '周日']
  const wk = weekMap[jsDayToCronWeek(d.getDay())] ?? ''
  return `${y}-${m}-${day} ${hr}:${min}:${sec} (${wk})`
}

// Fast Next-Run Forecaster: predicts up to `count` upcoming execution dates
export function calculateNextRuns(expr: string, count = 10, fromDate = new Date()): Date[] {
  const states = parseCron(expr)
  if (!states) return []

  const results: Date[] = []
  // Start from next second
  const cur = new Date(fromDate.getTime())
  cur.setMilliseconds(0)
  cur.setSeconds(cur.getSeconds() + 1)

  const currentYear = cur.getFullYear()
  const maxYear = currentYear + 5 // Bound search within 5 years
  let iterations = 0
  const maxIterations = 50000 // Guard against infinite loop

  while (results.length < count && cur.getFullYear() <= maxYear && iterations < maxIterations) {
    iterations++
    const y = cur.getFullYear()

    // If specific years are defined and all are in the past, no future runs possible
    if (states.year.type === 'specific' && states.year.specific.length > 0) {
      const maxSpecified = Math.max(...states.year.specific)
      if (y > maxSpecified) {
        break
      }
    } else if (states.year.type === 'range' && states.year.end !== undefined) {
      if (y > states.year.end) {
        break
      }
    }

    if (!matchesField(y, states.year, FIELD_CONFIGS.year)) {
      cur.setFullYear(y + 1, 0, 1)
      cur.setHours(0, 0, 0, 0)
      continue
    }

    const mon = cur.getMonth() + 1
    if (!matchesField(mon, states.month, FIELD_CONFIGS.month)) {
      cur.setMonth(cur.getMonth() + 1, 1)
      cur.setHours(0, 0, 0, 0)
      continue
    }

    const d = cur.getDate()
    const w = jsDayToCronWeek(cur.getDay())

    let dayMatch = false
    const dayNone = states.day.type === 'none'
    const weekNone = states.week.type === 'none'

    if (dayNone && weekNone) {
      dayMatch = true
    } else if (dayNone) {
      dayMatch = matchesField(w, states.week, FIELD_CONFIGS.week)
    } else if (weekNone) {
      dayMatch = matchesField(d, states.day, FIELD_CONFIGS.day)
    } else {
      // Both specified: either matches is standard quartz behavior
      dayMatch = matchesField(d, states.day, FIELD_CONFIGS.day) && matchesField(w, states.week, FIELD_CONFIGS.week)
    }

    if (!dayMatch) {
      cur.setDate(cur.getDate() + 1)
      cur.setHours(0, 0, 0, 0)
      continue
    }

    const hr = cur.getHours()
    if (!matchesField(hr, states.hour, FIELD_CONFIGS.hour)) {
      cur.setHours(hr + 1, 0, 0, 0)
      continue
    }

    const m = cur.getMinutes()
    if (!matchesField(m, states.minute, FIELD_CONFIGS.minute)) {
      cur.setMinutes(m + 1, 0, 0)
      continue
    }

    const s = cur.getSeconds()
    if (!matchesField(s, states.second, FIELD_CONFIGS.second)) {
      cur.setSeconds(s + 1)
      continue
    }

    // Match found!
    results.push(new Date(cur.getTime()))
    cur.setSeconds(cur.getSeconds() + 1)
  }

  return results
}

// Generate friendly human-readable Chinese description
export function describeCron(expr: string): string {
  const states = parseCron(expr)
  if (!states) return '格式错误：请输入有效的 5、6 或 7 字段 Cron 表达式'

  const { second, minute, hour, day, month, week, year } = states

  // Common quick templates
  if (
    second.type === 'step' &&
    minute.type === 'all' &&
    hour.type === 'all' &&
    day.type === 'all' &&
    month.type === 'all'
  ) {
    return `每隔 ${second.step} 秒执行一次`
  }

  if (
    second.type === 'specific' &&
    second.specific.length === 1 &&
    second.specific[0] === 0 &&
    minute.type === 'step' &&
    hour.type === 'all' &&
    day.type === 'all' &&
    month.type === 'all'
  ) {
    return `每隔 ${minute.step} 分钟执行一次`
  }

  if (
    second.type === 'specific' &&
    second.specific.length === 1 &&
    second.specific[0] === 0 &&
    minute.type === 'specific' &&
    minute.specific.length === 1 &&
    hour.type === 'step' &&
    day.type === 'all' &&
    month.type === 'all'
  ) {
    return `每隔 ${hour.step} 小时的第 ${pad2(minute.specific[0])} 分执行一次`
  }

  if (
    second.type === 'specific' &&
    minute.type === 'specific' &&
    hour.type === 'specific' &&
    day.type === 'all' &&
    month.type === 'all' &&
    (week.type === 'range' && week.start === 1 && week.end === 5)
  ) {
    const h = pad2(hour.specific[0] ?? 0)
    const m = pad2(minute.specific[0] ?? 0)
    const s = pad2(second.specific[0] ?? 0)
    return `每周一至周五 (工作日) ${h}:${m}:${s} 执行`
  }

  if (
    second.type === 'specific' &&
    minute.type === 'specific' &&
    hour.type === 'specific' &&
    day.type === 'all' &&
    month.type === 'all' &&
    (week.type === 'none' || week.type === 'all')
  ) {
    const h = pad2(hour.specific[0] ?? 0)
    const m = pad2(minute.specific[0] ?? 0)
    const s = pad2(second.specific[0] ?? 0)
    return `每天 ${h}:${m}:${s} 执行一次`
  }

  if (
    second.type === 'specific' &&
    minute.type === 'specific' &&
    hour.type === 'specific' &&
    (day.type === 'none' || day.type === 'all') &&
    month.type === 'all' &&
    week.type === 'specific'
  ) {
    const weekMap: Record<number, string> = {
      1: '周一', 2: '周二', 3: '周三', 4: '周四', 5: '周五', 6: '周六', 7: '周日',
    }
    const days = week.specific.map((w) => weekMap[w] || `周${w}`).join('、')
    const h = pad2(hour.specific[0] ?? 0)
    const m = pad2(minute.specific[0] ?? 0)
    const s = pad2(second.specific[0] ?? 0)
    return `每周 (${days}) ${h}:${m}:${s} 执行`
  }

  if (
    second.type === 'specific' &&
    minute.type === 'specific' &&
    hour.type === 'specific' &&
    day.type === 'specific' &&
    month.type === 'all' &&
    (week.type === 'none' || week.type === 'all')
  ) {
    const days = day.specific.join('、')
    const h = pad2(hour.specific[0] ?? 0)
    const m = pad2(minute.specific[0] ?? 0)
    const s = pad2(second.specific[0] ?? 0)
    return `每月 ${days} 号 ${h}:${m}:${s} 执行`
  }

  return `按表达式执行: ${expr.trim()}`
}
