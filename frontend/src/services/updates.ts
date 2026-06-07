import { httpClient } from '@/lib/http-client'

export type UpdateStatus = {
  currentVersion: string
  currentCommit: string
  latestVersion: string
  hasUpdate: boolean
  checkedAt: string
  message: string
  enabled: boolean
  updating: boolean
}

export type UpdateRun = {
  startedAt: string
  endedAt?: string
  version: string
  status: 'running' | 'ready' | 'restarting' | 'success' | 'error'
  message: string
  progress: number
}

type StatusResponse = {
  status: UpdateStatus
  update: UpdateRun | null
}

type CheckResponse = {
  status: UpdateStatus
}

type ApplyResponse = {
  update: UpdateRun
}

export async function getUpdateStatus(): Promise<StatusResponse> {
  const response = await httpClient.get<StatusResponse>('/updates')
  return response.data
}

export async function checkUpdate(): Promise<UpdateStatus> {
  const response = await httpClient.post<CheckResponse>('/updates/check')
  return response.data.status
}

export async function applyUpdate(): Promise<UpdateRun> {
  const response = await httpClient.post<ApplyResponse>('/updates/apply')
  return response.data.update
}

export async function restartUpdate(): Promise<UpdateRun> {
  const response = await httpClient.post<ApplyResponse>('/updates/restart')
  return response.data.update
}
