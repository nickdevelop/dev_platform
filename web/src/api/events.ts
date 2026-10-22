import { get, mutate } from './client'
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
): Promise<SubscriptionSnapshot> {
  const body: SubscribeEventBusSessionBody = {
    projectId,
    sessionId,
  }
  if (lastEventId !== undefined && lastEventId > 0) {
    body.lastEventId = lastEventId
  }
  return mutate<SubscriptionSnapshot>('POST', `/events/${encodeURIComponent(uuid)}/subscriptions`, body)
}

// DELETE /api/events/{uuid}/subscriptions/{sessionId}
export function unsubscribeEventBusSession(
  uuid: string,
  sessionId: string,
): Promise<{ success: boolean }> {
  return mutate<{ success: boolean }>(
    'DELETE',
    `/events/${encodeURIComponent(uuid)}/subscriptions/${encodeURIComponent(sessionId)}`,
  )
}

// GET /api/events/{uuid}/subscriptions
export function listEventBusSubscriptions(uuid: string): Promise<{ subscriptions: string[] }> {
  return get<{ subscriptions: string[] }>(`/events/${encodeURIComponent(uuid)}/subscriptions`)
}
