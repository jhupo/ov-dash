import { httpClient } from '@/lib/http-client'

export type AuditLog = {
  id: string
  actor: {
    id: string
    email: string
    role: string
  }
  action: string
  resource: string
  resource_id: string
  result: string
  message: string
  metadata: Record<string, unknown>
  ip_address: string
  user_agent: string
  created_at: string
}

export async function listAuditLogs(limit = 50): Promise<AuditLog[]> {
  const response = await httpClient.get<{ items: AuditLog[] }>('/audit-logs', {
    params: { limit },
  })
  return response.data.items
}
