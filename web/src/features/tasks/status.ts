// Task status presentation shared by the task list column and the detail pane:
// the badge class and the readable label for every task status, plus the filter
// options, so both columns always agree on one rendering.
import type { TaskStatus } from '../../api/tasks'

export const statusOptions: TaskStatus[] = [
  'draft',
  'in_progress',
  'ready_for_execution',
  'executing',
  'completed',
  'failed',
]

export function getStatusBadgeClass(status: TaskStatus): string {
  switch (status) {
    case 'ready_for_execution':
      return 'badge-ready'
    case 'executing':
      return 'badge-executing'
    case 'completed':
      return 'badge-completed'
    case 'failed':
      return 'badge-failed'
    case 'in_progress':
      return 'badge-progress'
    default:
      return 'badge-draft'
  }
}

export function getStatusLabel(status: TaskStatus): string {
  switch (status) {
    case 'draft':
      return '草稿'
    case 'in_progress':
      return '进行中'
    case 'ready_for_execution':
      return '就绪待执行'
    case 'executing':
      return '执行中'
    case 'completed':
      return '已完成'
    case 'failed':
      return '执行失败'
    default:
      return status
  }
}
