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
  collect_interval_seconds: number
  next_collect_at: string
  collector_installed: boolean
  collect_status: 'pending' | 'collecting' | 'ok' | 'error'
  collect_error: string
  last_collected_at: string | null
  metric: ServerMetric | null
  created_at: string
  updated_at: string
}

export type ServerMetric = {
  server_id: string
  cpu_percent: number
  cpu_cores: number
  latency_ms: number
  memory_used_bytes: number
  memory_total_bytes: number
  swap_used_bytes: number
  swap_total_bytes: number
  disk_used_bytes: number
  disk_total_bytes: number
  network_rx_bytes: number
  network_tx_bytes: number
  network_rx_rate_bps: number
  network_tx_rate_bps: number
  load1: number
  load5: number
  load15: number
  tcp_connections: number
  udp_connections: number
  process_count: number
  uptime_seconds: number
  architecture: string
  virtualization: string
  os_name: string
  cpu_model: string
  gpu_model: string
  region: string
  raw: Record<string, unknown>
  collected_at: string
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
  collect_interval_seconds?: number
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

export async function updateServerAgent(id: string): Promise<void> {
  await httpClient.post(`/server-connections/${id}/agent/update`)
}

export async function touchServerMonitor(): Promise<void> {
  await httpClient.post('/server-connections/monitor/touch')
}

type MetricsResponse = {
  items: ServerMetric[]
}

export async function listServerMetrics(
  id: string,
  range: string
): Promise<ServerMetric[]> {
  const response = await httpClient.get<MetricsResponse>(
    `/server-connections/${id}/metrics`,
    { params: { range } }
  )
  return response.data.items
}
