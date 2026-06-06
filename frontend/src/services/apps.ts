import { httpClient } from '@/lib/http-client'

export type AppIntegration = {
  id: string
  name: string
  provider: string
  desc: string
  connected: boolean
}

type AppsResponse = {
  items: AppIntegration[]
}

export async function getApps(): Promise<AppIntegration[]> {
  const response = await httpClient.get<AppsResponse>('/apps')
  return response.data.items
}
