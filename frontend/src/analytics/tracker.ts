import {
  recordCreatorEvent,
  recordPlayEvent,
  type CreatorPageName,
  type PublicPlayAnalyticsEventName,
} from '@/api/analytics'
import { useAuthStore } from '@/stores/auth'

function eventMetadata() {
  return {
    clientEventId: globalThis.crypto.randomUUID(),
    occurredAt: new Date().toISOString(),
  }
}

export async function trackCreatorEvent(page: CreatorPageName): Promise<void> {
  try {
    const metadata = eventMetadata()
    const csrfToken = await useAuthStore().ensureCSRF()
    await recordCreatorEvent(
      {
        eventName: 'creator.page_viewed',
        ...metadata,
        properties: { page },
      },
      csrfToken,
    )
  } catch {
    // Analytics is best-effort and must never surface a user-facing failure.
  }
}

export async function trackPlayEvent(eventName: PublicPlayAnalyticsEventName): Promise<void> {
  try {
    await recordPlayEvent({
      eventName,
      ...eventMetadata(),
      properties: { mode: 'public' },
    })
  } catch {
    // Analytics is best-effort and must never affect public game state.
  }
}
