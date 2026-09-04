import { apiRequest } from './client'

export type GenerationStatus = 'queued' | 'running' | 'succeeded' | 'failed' | 'cancelled'
export type GenerationStage =
  | 'queued'
  | 'transforming_images'
  | 'saving_results'
  | 'completed'

export interface GenerationRun {
  id: string
  gameId: string
  gameVersionId: string
  attemptNumber: number
  triggerType: 'initial' | 'user_retry'
  status: GenerationStatus
  stage: GenerationStage
  progress: number
  errorCode: string | null
  errorMessage: string | null
  retryable: boolean
  cancelRequested: boolean
  createdAt: string
  updatedAt: string
  startedAt: string | null
  completedAt: string | null
}

export interface AdminGenerationRun extends GenerationRun {
  executionCount: number
  traceId: string
  adminMessage: string | null
  sanitizedDetails: Record<string, unknown> | null
}

export function submitGeneration(gameId: string, versionId: string, idempotencyKey: string, csrfToken: string) {
  return apiRequest<GenerationRun>(`/games/${gameId}/generation-runs`, {
    method: 'POST',
    headers: { 'X-CSRF-Token': csrfToken, 'Idempotency-Key': idempotencyKey },
    body: JSON.stringify({ versionId }),
  })
}

export function listGenerationRuns(gameId: string) {
  return apiRequest<{ items: GenerationRun[] }>(`/games/${gameId}/generation-runs`)
}

export function getGenerationRun(gameId: string, runId: string) {
  return apiRequest<GenerationRun>(`/games/${gameId}/generation-runs/${runId}`)
}

export function cancelGeneration(gameId: string, runId: string, csrfToken: string) {
  return apiRequest<GenerationRun>(`/games/${gameId}/generation-runs/${runId}/cancel`, {
    method: 'POST',
    headers: { 'X-CSRF-Token': csrfToken },
  })
}

export function listAdminGenerationRuns(status = '') {
  const query = status ? `?status=${encodeURIComponent(status)}` : ''
  return apiRequest<{ items: AdminGenerationRun[] }>(`/admin/generation-runs${query}`)
}
