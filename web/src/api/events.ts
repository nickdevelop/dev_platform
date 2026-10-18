import { apiRequest } from './client'
import type { InteractionSnapshot, TurnPayload } from './sessions'

export interface SubscriptionSnapshot {
  sessionId: string
  turn: TurnPayload | null
  interactions: InteractionSnapshot
  resync: boolean
}

export function eventBusPath(uuid: string): string {
  return `/api/events/${encodeURIComponent(uuid)}`
}

export interface SubscribeEventBusSessionBody {
  projectId: number
  sessionId: string
  lastEventId?: number
}

// POST /api/events/{uuid}/subscriptions
export function subscribeEventBusSession(
  uuid: string,
  projectId: number,
  sessionId: string,
  lastEventId?: number,
  csrfToken?: string,
): Promise<SubscriptionSnapshot> {
  const body: SubscribeEventBusSessionBody = {
    projectId,
    sessionId,
  }
  if (lastEventId !== undefined && lastEventId > 0) {
    body.lastEventId = lastEventId
  }
  return apiRequest<SubscriptionSnapshot>('POST', `/events/${encodeURIComponent(uuid)}/subscriptions`, {
    body,
    csrfToken,
  })
}

// DELETE /api/events/{uuid}/subscriptions/{sessionId}
export function unsubscribeEventBusSession(
  uuid: string,
  sessionId: string,
  csrfToken?: string,
): Promise<{ success: boolean }> {
  return apiRequest<{ success: boolean }>(
    'DELETE',
    `/events/${encodeURIComponent(uuid)}/subscriptions/${encodeURIComponent(sessionId)}`,
    {
      csrfToken,
    },
  )
}

// GET /api/events/{uuid}/subscriptions
export function listEventBusSubscriptions(uuid: string): Promise<{ subscriptions: string[] }> {
  return apiRequest<{ subscriptions: string[] }>('GET', `/events/${encodeURIComponent(uuid)}/subscriptions`)
}
