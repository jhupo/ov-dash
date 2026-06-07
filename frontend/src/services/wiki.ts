import { httpClient } from '@/lib/http-client'

export type WikiPageType =
  | 'document'
  | 'machine'
  | 'link'
  | 'runbook'
  | 'troubleshooting'

export type WikiResourceType = 'machine' | 'link' | 'credential' | 'note'

export type WikiResource = {
  id: string
  page_id: string
  resource_type: WikiResourceType
  title: string
  host: string
  port: string
  url: string
  username: string
  password: string
  note: string
  sort_order: number
  created_at: string
  updated_at: string
}

export type WikiPage = {
  id: string
  parent_id: string
  title: string
  page_type: WikiPageType
  category: string
  summary: string
  content_md: string
  tags: string
  resources: WikiResource[]
  created_by: string
  updated_by: string
  created_at: string
  updated_at: string
}

export type WikiRevision = {
  id: string
  page_id: string
  version: number
  title: string
  page_type: WikiPageType
  category: string
  summary: string
  content_md: string
  tags: string
  resources: WikiResource[]
  created_by: string
  created_at: string
}

export type SaveWikiResourcePayload = {
  id?: string
  resource_type: WikiResourceType
  title: string
  host: string
  port: string
  url: string
  username: string
  password: string
  note: string
  sort_order: number
}

export type SaveWikiPagePayload = {
  parent_id?: string
  title: string
  page_type: WikiPageType
  category: string
  summary: string
  content_md: string
  tags: string
  resources: SaveWikiResourcePayload[]
}

type WikiPagesResponse = {
  items: WikiPage[]
}

type WikiRevisionsResponse = {
  items: WikiRevision[]
}

export async function getWikiPages(): Promise<WikiPage[]> {
  const response = await httpClient.get<WikiPagesResponse>('/wiki/pages')
  return response.data.items
}

export async function getWikiPage(id: string): Promise<WikiPage> {
  const response = await httpClient.get<WikiPage>(
    `/wiki/pages/${encodeURIComponent(id)}`
  )
  return response.data
}

export async function createWikiPage(
  payload: SaveWikiPagePayload
): Promise<WikiPage> {
  const response = await httpClient.post<WikiPage>('/wiki/pages', payload)
  return response.data
}

export async function updateWikiPage(
  id: string,
  payload: SaveWikiPagePayload
): Promise<WikiPage> {
  const response = await httpClient.put<WikiPage>(
    `/wiki/pages/${encodeURIComponent(id)}`,
    payload
  )
  return response.data
}

export async function deleteWikiPage(id: string): Promise<void> {
  await httpClient.delete(`/wiki/pages/${encodeURIComponent(id)}`)
}

export async function getWikiRevisions(id: string): Promise<WikiRevision[]> {
  const response = await httpClient.get<WikiRevisionsResponse>(
    `/wiki/pages/${encodeURIComponent(id)}/revisions`
  )
  return response.data.items
}
