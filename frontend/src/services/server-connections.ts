import { httpClient } from '@/lib/http-client'

export type ServerAuthType = 'password' | 'key'

export type ServerConnection = {
  id: string
  name: string
  group_name: string
  region: string
  host: string
  port: number
  username: string
  auth_type: ServerAuthType
  has_password: boolean
  has_private_key: boolean
  connection_hint: string
  expires_at: string | null
  created_at: string
  updated_at: string
}

export type SaveServerConnectionPayload = {
  id?: string
  name: string
  group_name: string
  region: string
  host: string
  port: number
  username: string
  auth_type: ServerAuthType
  password?: string
  private_key?: string
  expires_at?: string
  clear_secret?: boolean
}

type ListServerConnectionsResponse = {
  items: ServerConnection[]
}

export async function listServerConnections(): Promise<ServerConnection[]> {
  const response =
    await httpClient.get<ListServerConnectionsResponse>('/server-connections')
  return response.data.items
}

export async function saveServerConnection(
  payload: SaveServerConnectionPayload
): Promise<ServerConnection> {
  const { id, ...body } = payload
  const response = id
    ? await httpClient.put<ServerConnection>(`/server-connections/${id}`, body)
    : await httpClient.post<ServerConnection>('/server-connections', body)
  return response.data
}

export async function deleteServerConnection(id: string): Promise<void> {
  await httpClient.delete(`/server-connections/${id}`)
}

export async function requestServerShellTicket(
  id: string
): Promise<{ ticket: string; expires_at: string }> {
  const response = await httpClient.post<{ ticket: string; expires_at: string }>(
    `/server-connections/${id}/ssh/ticket`
  )
  return response.data
}
