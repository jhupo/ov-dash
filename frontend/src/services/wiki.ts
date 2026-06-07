import { httpClient } from '@/lib/http-client'

export type WikiPageType =
  | 'document'
  | 'machine'
  | 'link'
  | 'runbook'
  | 'troubleshooting'

export type WikiPage = {
  id: string
  parent_id: string
  title: string
  page_type: WikiPageType
  category: string
  summary: string
  content_md: string
  link_label: string
  link_url: string
  machine_host: string
  machine_port: string
  machine_username: string
  tags: string
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
  link_label: string
  link_url: string
  machine_host: string
  machine_port: string
  machine_username: string
  tags: string
  created_by: string
  created_at: string
}

export type SaveWikiPagePayload = {
  parent_id?: string
  title: string
  page_type: WikiPageType
  category: string
  summary: string
  content_md: string
  link_label: string
  link_url: string
  machine_host: string
  machine_port: string
  machine_username: string
  tags: string
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
