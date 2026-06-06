import { create } from 'zustand'
import { getCookie, removeCookie, setCookie } from '@/lib/cookies'
import { type AuthUser } from '@/services/auth'

const ACCESS_TOKEN = 'thisisjustarandomstring'
const AUTH_USER = 'ovdash-auth-user'

interface AuthState {
  auth: {
    user: AuthUser | null
    setUser: (user: AuthUser | null) => void
    accessToken: string
    setAccessToken: (accessToken: string) => void
    resetAccessToken: () => void
    reset: () => void
  }
}

function readJsonCookie<T>(name: string, fallback: T): T {
  const value = getCookie(name)

  if (!value) {
    return fallback
  }

  try {
    return JSON.parse(value) as T
  } catch {
    removeCookie(name)
    return fallback
  }
}

export const useAuthStore = create<AuthState>()((set) => {
  const initToken = readJsonCookie(ACCESS_TOKEN, '')
  const initUser = readJsonCookie<AuthUser | null>(AUTH_USER, null)

  return {
    auth: {
      user: initUser,
      setUser: (user) =>
        set((state) => {
          if (user) {
            setCookie(AUTH_USER, JSON.stringify(user))
          } else {
            removeCookie(AUTH_USER)
          }

          return { ...state, auth: { ...state.auth, user } }
        }),
      accessToken: initToken,
      setAccessToken: (accessToken) =>
        set((state) => {
          setCookie(ACCESS_TOKEN, JSON.stringify(accessToken))
          return { ...state, auth: { ...state.auth, accessToken } }
        }),
      resetAccessToken: () =>
        set((state) => {
          removeCookie(ACCESS_TOKEN)
          return { ...state, auth: { ...state.auth, accessToken: '' } }
        }),
      reset: () =>
        set((state) => {
          removeCookie(ACCESS_TOKEN)
          removeCookie(AUTH_USER)
          return {
            ...state,
            auth: { ...state.auth, user: null, accessToken: '' },
          }
        }),
    },
  }
})
