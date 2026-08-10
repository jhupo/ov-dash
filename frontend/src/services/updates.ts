import { httpClient } from '@/lib/http-client'

export type InstalledRelease = {
  release_id: string
  version: string
  sequence: number
  schema: number
  committed_at: string
}

export type ReleaseArtifact = {
  filename: string
  checksum_filename: string
  signature_filename: string
}

export type ReleaseCandidate = {
  schema_version: number
  release_id: string
  version: string
  sequence: number
  published_at: string
  minimum_version: string
  artifacts: Record<string, ReleaseArtifact>
  database: {
    from_schema: number
    to_schema: number
  }
}

export type UpdateOperationState =
  | 'requested'
  | 'downloaded'
  | 'verified'
  | 'switching'
  | 'committed'
  | 'failed'

export type UpdateOperation = {
  id: string
  release_id: string
  state: UpdateOperationState
  revision: number
  created_at: string
  updated_at: string
  last_error?: string
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
