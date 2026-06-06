const DEFAULT_API_BASE_URL = '/api/v1'

function normalizeBaseUrl(value: string | undefined): string {
  const baseUrl = value?.trim() || DEFAULT_API_BASE_URL

  if (baseUrl === '/') {
    return ''
  }

  return baseUrl.replace(/\/+$/, '')
}

function readBoolean(value: string | undefined): boolean {
  return value?.toLowerCase() === 'true'
}

export const apiConfig = {
  baseURL: normalizeBaseUrl(import.meta.env.VITE_API_BASE_URL),
  timeoutMs: 30_000,
  withCredentials: readBoolean(import.meta.env.VITE_API_WITH_CREDENTIALS),
} as const
