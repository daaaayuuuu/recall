export interface APIEnvelope<T> {
  data: T
  requestId: string
}

interface APIErrorEnvelope {
  error?: {
    code?: string
    message?: string
    fields?: Record<string, string>
  }
  requestId?: string
}

export class APIError extends Error {
  readonly status: number
  readonly code: string
  readonly fields: Record<string, string>
  readonly requestId?: string

  constructor(
    status: number,
    code: string,
    message: string,
    fields: Record<string, string> = {},
    requestId?: string,
  ) {
    super(message)
    this.name = 'APIError'
    this.status = status
    this.code = code
    this.fields = fields
    this.requestId = requestId
  }
}

export const API_BASE_URL = import.meta.env.VITE_API_V1_BASE_URL ?? '/api/v1'

export function apiResourceURL(path: string): string {
  return `${API_BASE_URL.replace(/\/$/, '')}${path}`
}

export async function apiRequest<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers)
  const isFormData = typeof globalThis.FormData !== 'undefined' && init.body instanceof globalThis.FormData
  if (init.body !== undefined && !isFormData && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json')
  }

  const response = await fetch(`${API_BASE_URL}${path}`, {
    ...init,
    headers,
    credentials: 'include',
  })
  const payload = (await response.json().catch(() => ({}))) as APIEnvelope<T> & APIErrorEnvelope
  if (!response.ok) {
    throw new APIError(
      response.status,
      payload.error?.code ?? 'REQUEST_FAILED',
      payload.error?.message ?? '请求失败，请稍后重试',
      payload.error?.fields ?? {},
      payload.requestId,
    )
  }
  return payload.data
}
