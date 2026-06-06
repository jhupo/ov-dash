import { httpClient } from '@/lib/http-client'
import { type Task } from '@/features/tasks/data/schema'

type TasksResponse = {
  items: Task[]
}

export async function getTasks(): Promise<Task[]> {
  const response = await httpClient.get<TasksResponse>('/tasks')
  return response.data.items
}

export async function deleteTasks(ids: string[]): Promise<void> {
  await httpClient.delete('/tasks', { data: { ids } })
}
