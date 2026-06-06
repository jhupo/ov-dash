import { httpClient } from '@/lib/http-client'
import { type User } from '@/features/users/data/schema'

type UsersResponse = {
  items: User[]
}

export async function getUsers(): Promise<User[]> {
  const response = await httpClient.get<UsersResponse>('/users')
  return response.data.items
}
