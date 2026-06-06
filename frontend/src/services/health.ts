import { httpClient } from '@/lib/http-client'

export type BackendServiceStatus = 'ok' | 'degraded' | 'down' | 'unknown'

export interface BackendHealthResponse {
  status: BackendServiceStatus | string
  checks?: Record<string, BackendServiceStatus | string>
  services?: Record<string, BackendServiceStatus | string>
}

export async function getBackendHealth(signal?: AbortSignal) {
  const response = await httpClient.get<BackendHealthResponse>('/health', {
    signal,
  })

  return response.data
}
