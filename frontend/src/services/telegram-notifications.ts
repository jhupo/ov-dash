import { httpClient } from '@/lib/http-client'

export type TelegramNotificationSettings = {
  id: string
  enabled: boolean
  has_bot_token: boolean
  has_inbound_token: boolean
  updated_at: string
}

export type UserTelegramNotificationSettings = {
  user_id: string
  username: string
  email: string
  full_name: string
  enabled: boolean
  chat_id: string
  updated_at: string
}

type TelegramUsersResponse = {
  items: UserTelegramNotificationSettings[]
}

export type UpdateTelegramNotificationSettingsPayload = {
  enabled: boolean
  bot_token?: string
  inbound_token?: string
  clear_bot_token?: boolean
  clear_inbound_token?: boolean
}

export type UpdateUserTelegramNotificationSettingsPayload = {
  enabled: boolean
  chat_id: string
}

export async function getTelegramNotificationSettings(): Promise<TelegramNotificationSettings> {
  const response = await httpClient.get<TelegramNotificationSettings>(
    '/telegram-notifications/settings'
  )
  return response.data
}

export async function updateTelegramNotificationSettings(
  payload: UpdateTelegramNotificationSettingsPayload
): Promise<TelegramNotificationSettings> {
  const response = await httpClient.put<TelegramNotificationSettings>(
    '/telegram-notifications/settings',
    payload
  )
  return response.data
}

export async function getUserTelegramNotificationSettings(): Promise<
  UserTelegramNotificationSettings[]
> {
  const response = await httpClient.get<TelegramUsersResponse>(
    '/telegram-notifications/users'
  )
  return response.data.items
}

export async function updateUserTelegramNotificationSettings(
  userId: string,
  payload: UpdateUserTelegramNotificationSettingsPayload
): Promise<UserTelegramNotificationSettings> {
  const response = await httpClient.put<UserTelegramNotificationSettings>(
    `/telegram-notifications/users/${encodeURIComponent(userId)}`,
    payload
  )
  return response.data
}
