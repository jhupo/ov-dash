import { httpClient } from '@/lib/http-client'
import { type ChatUser } from '@/features/chats/data/chat-types'

type ChatsResponse = {
  items: ChatUser[]
}

export async function getChats(): Promise<ChatUser[]> {
  const response = await httpClient.get<ChatsResponse>('/chats')
  return response.data.items
}
