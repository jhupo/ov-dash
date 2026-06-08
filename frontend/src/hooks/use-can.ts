import { useCallback } from 'react'
import { type Capability } from '@/services/module-registry'
import { useAuthStore } from '@/stores/auth-store'

const readCapabilities = [
  'dashboard:read',
  'platform:read',
  'tasks:read',
  'apps:read',
  'chats:read',
  'users:read',
  'wiki:read',
  'settings:read',
  'proxy:read',
  'notifications:read',
  'updates:read',
  'jobs:read',
  'servers:read',
] satisfies Capability[]

const roleGrants: Record<string, Capability[]> = {
  viewer: readCapabilities,
  operator: [
    ...readCapabilities,
    'tasks:write',
    'wiki:write',
    'proxy:write',
    'notifications:write',
    'jobs:create',
    'jobs:manage',
    'servers:write',
    'servers:ssh',
  ],
}

export function useCan() {
  const role = useAuthStore((state) => state.auth.user?.role)

  return useCallback(
    (capability: Capability) => roleAllows(role, capability),
    [role]
  )
}

export function roleAllows(
  role: string | string[] | undefined,
  capability: Capability
) {
  const roles = normalizeRoles(role)
  if (roles.includes('admin')) {
    return true
  }
  return roles.some((item) => roleGrants[item]?.includes(capability))
}

function normalizeRoles(role: string | string[] | undefined) {
  if (!role) {
    return []
  }
  const roles = Array.isArray(role) ? role : [role]
  return roles.map((item) => item.trim().toLowerCase()).filter(Boolean)
}
