export interface HealthResponse {
  status: 'ok' | 'unavailable'
  service: string
  dependencies?: Record<string, 'ok' | 'unavailable'>
}

export async function getReadiness(
  fetcher: typeof fetch = fetch,
  baseURL = import.meta.env.VITE_API_BASE_URL ?? '/api',
): Promise<HealthResponse> {
  const response = await fetcher(`${baseURL}/health/ready`, {
    headers: { Accept: 'application/json' },
    credentials: 'include',
  })

  if (!response.ok) {
    throw new Error(`API readiness request failed with status ${response.status}`)
  }
  return (await response.json()) as HealthResponse
}

