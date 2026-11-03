import { get, mutate } from './client'
import type { InteractionSnapshot, TurnPayload } from './sessions'

export interface EventEnvelope<T = unknown> {
  id?: number
  busId?: string
  channel: 'sessions' | 'chat' | 'tasks' | 'system' | string
  targetId: string
  eventType: string
  timestamp: number
  payload: T
}

export interface SubscriptionSnapshot {
  sessionId: string
  turn: TurnPayload | null
  interactions: InteractionSnapshot
  resync: boolean
}

export interface SubscribeBusBody {
  channel: string
  targetId: string
  projectId?: number
  lastEventId?: number
  extra?: unknown
}

export interface SubscribeBusResponse<T = unknown> {
  channel: string
  targetId: string
  snapshot?: T
}

export function eventBusPath(uuid: string): string {
  return `/api/events/bus/${encodeURIComponent(uuid)}`
}

// POST /api/events/bus/{uuid}/subscriptions
export function subscribeEventBus<T = unknown>(
  uuid: string,
  body: SubscribeBusBody,
): Promise<SubscribeBusResponse<T>> {
  return mutate<SubscribeBusResponse<T>>('POST', `/events/bus/${encodeURIComponent(uuid)}/subscriptions`, body)
}

// DELETE /api/events/bus/{uuid}/subscriptions/{channel}/{targetId}
export function unsubscribeEventBus(
  uuid: string,
  channel: string,
  targetId: string,
): Promise<{ success: boolean }> {
  return mutate<{ success: boolean }>(
    'DELETE',
    `/events/bus/${encodeURIComponent(uuid)}/subscriptions/${encodeURIComponent(channel)}/${encodeURIComponent(targetId)}`,
  )
}

// GET /api/events/bus/{uuid}/subscriptions
export function listEventBusSubscriptions(uuid: string): Promise<{ subscriptions: Record<string, string[]> }> {
  return get<{ subscriptions: Record<string, string[]> }>(`/events/bus/${encodeURIComponent(uuid)}/subscriptions`)
}

// Convenience wrapper for session subscription
export async function subscribeEventBusSession(
  uuid: string,
  projectId: number,
  sessionId: string,
  lastEventId?: number,
): Promise<SubscriptionSnapshot> {
  const res = await subscribeEventBus<SubscriptionSnapshot>(uuid, {
    channel: 'sessions',
    targetId: sessionId,
    projectId,
    lastEventId,
  })
  if (res && res.snapshot) {
    return res.snapshot
  }
  if (res && (res as any).sessionId) {
    return res as unknown as SubscriptionSnapshot
  }
  return {
    sessionId,
    turn: null,
    interactions: { permissions: [], forms: [] },
    resync: false,
  }
}

// Convenience wrapper for session unsubscription
export function unsubscribeEventBusSession(
  uuid: string,
  sessionId: string,
): Promise<{ success: boolean }> {
  return unsubscribeEventBus(uuid, 'sessions', sessionId)
}
