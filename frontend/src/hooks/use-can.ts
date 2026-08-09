import { useCallback } from 'react'
import { type AuthUser } from '@/services/auth'
import { type Capability } from '@/services/module-registry'
import { useAuthStore } from '@/stores/auth-store'

export function useCan() {
  const user = useAuthStore((state) => state.auth.user)

  return useCallback(
    (capability: Capability) => roleAllows(user, capability),
    [user]
  )
}

export function roleAllows(
  user: Pick<AuthUser, 'role' | 'capabilities'> | null | undefined,
  capability: Capability
) {
  if (user?.role.trim().toLowerCase() === 'admin') {
    return true
  }
  return user?.capabilities.includes(capability) ?? false
}
