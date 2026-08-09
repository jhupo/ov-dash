import { httpClient } from '@/lib/http-client'

export type InstalledRelease = {
  release_id: string
  version: string
  sequence: number
  schema: number
  images: Record<string, string>
  release_env?: string
  committed_at: string
}

export type ReleaseCandidate = {
  schema_version: number
  release_id: string
  version: string
  sequence: number
  published_at: string
  expires_at: string
  minimum_version: string
  images: Record<string, string>
  database: {
    from_schema: number
    to_schema: number
    strategy: string
    transactional: boolean
    backup_required: boolean
  }
  health: {
    timeout_seconds: number
    stability_seconds: number
  }
}

export type UpdateOperationState =
  | 'requested'
  | 'downloaded'
  | 'verified'
  | 'preflight'
  | 'quiescing'
  | 'backup'
  | 'migrating'
  | 'switching'
  | 'health_checking'
  | 'committed'
  | 'rolling_back'
  | 'rolled_back'
  | 'failed'
  | 'rollback_failed'
  | 'manual_intervention'

export type UpdateOperation = {
  id: string
  release_id: string
  state: UpdateOperationState
  revision: number
  created_at: string
  updated_at: string
  last_error?: string
  recovery_reason?: string
}

export type UpdateStatusResponse = {
  current: InstalledRelease
  operation: UpdateOperation | null
}

export type UpdateCheckResponse = {
  current: InstalledRelease
  candidate: ReleaseCandidate
  has_update: boolean
}

export async function getUpdateStatus(): Promise<UpdateStatusResponse> {
  const response = await httpClient.get<UpdateStatusResponse>('/updates')
  return response.data
}

export async function checkUpdate(): Promise<UpdateCheckResponse> {
  const response = await httpClient.post<UpdateCheckResponse>('/updates/check')
  return response.data
}

export async function applyUpdate(releaseId: string): Promise<UpdateOperation> {
  const response = await httpClient.post<UpdateOperation>('/updates/apply', {
    release_id: releaseId,
  })
  return response.data
}

export async function getUpdateOperation(
  operationId: string
): Promise<UpdateOperation> {
  const response = await httpClient.get<UpdateOperation>(
    `/updates/operations/${operationId}`
  )
  return response.data
}
