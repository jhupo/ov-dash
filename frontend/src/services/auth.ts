import { type Capability } from '@/services/module-registry'
import { httpClient } from '@/lib/http-client'

export type AuthUser = {
  id?: string
  name?: string
  email: string
  role: string
  capabilities: Capability[]
  avatar?: string
}

type LoginRequest = {
  email: string
  password: string
}

type PasswordRequest = {
  current_password: string
  new_password: string
}

type AuthResponse = {
  user?: AuthUser
  token?: string
  access_token?: string
}

function normalizeUser(data: AuthResponse | AuthUser): AuthUser {
  const user = 'user' in data && data.user ? data.user : (data as AuthUser)

  return {
    ...user,
    email: user.email,
    name: user.name || user.email,
  }
}

export async function login(payload: LoginRequest): Promise<{
  user: AuthUser
  token: string
}> {
  const response = await httpClient.post<AuthResponse>('/auth/login', payload)

  return {
    user: normalizeUser(response.data),
    token: response.data.access_token || response.data.token || 'session',
  }
}

export async function logout(): Promise<void> {
  await httpClient.post('/auth/logout')
}

export async function getCurrentUser(): Promise<AuthUser> {
  const response = await httpClient.get<AuthResponse | AuthUser>('/auth/me')
  return normalizeUser(response.data)
}

export async function changePassword(payload: PasswordRequest): Promise<void> {
  await httpClient.put('/auth/password', payload)
}
