import { apiRequest } from './client'

export type CreatorPageName =
  | 'create'
  | 'games'
  | 'game-edit'
  | 'game-preview'
  | 'game-share'
  | 'generation-progress'
  | 'settings'
export type CreatorAnalyticsEventName = 'creator.page_viewed'
export type PublicPlayAnalyticsEventName = 'play.completed' | 'play.replayed'

export type BehaviorEventName =
  | CreatorAnalyticsEventName
  | 'creator.registered'
  | 'creator.logged_in'
  | 'game.created'
  | 'game.version_created'
  | 'asset.uploaded'
  | 'generation.submitted'
  | 'generation.succeeded'
  | 'generation.failed'
  | 'share.created'
  | 'share.opened'
  | 'play.started'
  | PublicPlayAnalyticsEventName

export type BehaviorEventSource = 'frontend' | 'api' | 'worker'
export type BehaviorEventActorType = 'creator' | 'receiver' | 'system'

export interface AdminBehaviorEvent {
  id: string
  eventName: BehaviorEventName
  source: BehaviorEventSource
  actorType: BehaviorEventActorType
  creatorId: string | null
  loginId: string | null
  userSessionId: string | null
  gameId: string | null
  gameVersionId: string | null
  generationRunId: string | null
  shareId: string | null
  playSessionId: string | null
  requestId: string | null
  properties: Record<string, unknown>
  occurredAt: string | null
  createdAt: string
}

export interface AdminBehaviorEventPage {
  items: AdminBehaviorEvent[]
  nextCursor: string | null
}

export interface AdminBehaviorEventFilters {
  eventName?: BehaviorEventName
  creatorId?: string
  loginId?: string
  gameId?: string
  source?: BehaviorEventSource
  from?: string
  to?: string
  cursor?: string
  limit?: number
}

interface AnalyticsEventRequest<TEventName extends string, TProperties> {
  eventName: TEventName
  clientEventId: string
  occurredAt?: string
  properties: TProperties
}

export type CreatorAnalyticsEventRequest = AnalyticsEventRequest<
  CreatorAnalyticsEventName,
  { page: CreatorPageName }
>

export type PublicPlayAnalyticsEventRequest = AnalyticsEventRequest<
  PublicPlayAnalyticsEventName,
  { mode: 'public' }
>

export interface AnalyticsEventWriteResponse {
  eventId: string
  duplicate: boolean
}

export function recordCreatorEvent(request: CreatorAnalyticsEventRequest, csrfToken: string) {
  return apiRequest<AnalyticsEventWriteResponse>('/analytics/events', {
    method: 'POST',
    headers: { 'X-CSRF-Token': csrfToken },
    body: JSON.stringify(request),
  })
}

export function recordPlayEvent(request: PublicPlayAnalyticsEventRequest) {
  return apiRequest<AnalyticsEventWriteResponse>('/public/play-sessions/current/events', {
    method: 'POST',
    body: JSON.stringify(request),
  })
}

export function listAdminBehaviorEvents(filters: AdminBehaviorEventFilters = {}) {
  const query = new URLSearchParams()
  if (filters.eventName) query.set('eventName', filters.eventName)
  if (filters.creatorId) query.set('creatorId', filters.creatorId)
  if (filters.loginId) query.set('loginId', filters.loginId)
  if (filters.gameId) query.set('gameId', filters.gameId)
  if (filters.source) query.set('source', filters.source)
  if (filters.from) query.set('from', filters.from)
  if (filters.to) query.set('to', filters.to)
  if (filters.cursor) query.set('cursor', filters.cursor)
  query.set('limit', String(filters.limit ?? 50))

  return apiRequest<AdminBehaviorEventPage>(`/admin/behavior-events?${query.toString()}`)
}
