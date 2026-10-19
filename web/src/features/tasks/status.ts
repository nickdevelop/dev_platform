// Task status and type helpers for UI display
import type { RunStatus, ScheduleType, TaskStatus, TaskType } from '../../api/tasks'

export function getTaskTypeLabel(type: TaskType): string {
  switch (type) {
    case 'flow':
      return '流程任务'
    case 'standard':
    default:
      return '普通任务'
  }
}

export function getScheduleTypeLabel(type: ScheduleType): string {
  switch (type) {
    case 'once':
      return '定时执行'
    case 'cron':
      return '周期执行'
    case 'manual':
    default:
      return '手动执行'
  }
}

export function getRunStatusBadgeClass(status?: RunStatus | string): string {
  switch (status) {
    case 'running':
      return 'badge-running'
    case 'success':
      return 'badge-success'
    case 'failed':
      return 'badge-failed'
    default:
      return 'badge-idle'
  }
}

export function getRunStatusLabel(status?: RunStatus | string): string {
  switch (status) {
    case 'running':
      return '运行中'
    case 'success':
      return '执行成功'
    case 'failed':
      return '执行失败'
    default:
      return '未执行'
  }
}

export function formatDateTime(dateStr?: string): string {
  if (!dateStr) return '-'
  try {
    const d = new Date(dateStr)
    if (isNaN(d.getTime())) return dateStr
    return d.toLocaleString('zh-CN', {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
      hour12: false,
    })
  } catch {
    return dateStr
  }
}
