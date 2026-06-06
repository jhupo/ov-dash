import { httpClient } from '@/lib/http-client'

export type ProxySettings = {
  id: string
  enabled: boolean
  scheme: 'socks5'
  host: string
  port: number
  username: string
  has_password: boolean
  updated_at: string
}

export type UpdateProxySettingsPayload = {
  enabled: boolean
  host: string
  port: number
  username: string
  password?: string
  clear_password?: boolean
}

export async function getProxySettings(): Promise<ProxySettings> {
  const response = await httpClient.get<ProxySettings>('/proxy-settings')
  return response.data
}

export async function updateProxySettings(
  payload: UpdateProxySettingsPayload
): Promise<ProxySettings> {
  const response = await httpClient.put<ProxySettings>(
    '/proxy-settings',
    payload
  )
  return response.data
}
