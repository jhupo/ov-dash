import { httpClient } from '@/lib/http-client'

export type JobRecord = {
  id: string
  queue_name: string
  type: string
  payload: Record<string, unknown>
  status: string
  attempts: number
  max_attempts: number
  last_error: string
  cancel_requested: boolean
  cancel_requested_at?: string
  next_run_at?: string
  started_at?: string
  completed_at?: string
  dead_at?: string
  canceled_at?: string
  created_at: string
  updated_at: string
}

export type JobEvent = {
  id: number
  job_id: string
  event_type: string
  message: string
  metadata: Record<string, unknown>
  created_at: string
}

export async function listJobs(limit = 50): Promise<JobRecord[]> {
  const response = await httpClient.get<{ items: JobRecord[] }>('/jobs', {
    params: { limit },
  })
  return response.data.items
}

export async function getJob(id: string): Promise<JobRecord> {
  const response = await httpClient.get<JobRecord>(`/jobs/${id}`)
  return response.data
}

export async function listJobEvents(id: string): Promise<JobEvent[]> {
  const response = await httpClient.get<{ items: JobEvent[] }>(
    `/jobs/${id}/events`
  )
  return response.data.items
}

export async function cancelJob(id: string): Promise<void> {
  await httpClient.post(`/jobs/${id}/cancel`)
}
