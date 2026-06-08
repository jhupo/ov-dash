import { createFileRoute, redirect } from '@tanstack/react-router'
import { getCurrentUser } from '@/services/auth'
import { canAccessModule, getModuleForPath } from '@/services/module-registry'
import { useAuthStore } from '@/stores/auth-store'
import { roleAllows } from '@/hooks/use-can'
import { AuthenticatedLayout } from '@/components/layout/authenticated-layout'

export const Route = createFileRoute('/_authenticated')({
  beforeLoad: async ({ location }) => {
    let user: Awaited<ReturnType<typeof getCurrentUser>>
    try {
      user = await getCurrentUser()
    } catch {
      useAuthStore.getState().auth.reset()
      throw redirect({
        to: '/sign-in',
        search: { redirect: location.href },
      })
    }

    useAuthStore.getState().auth.setUser(user)
    const module = getModuleForPath(location.pathname)
    if (
      !canAccessModule(module, (capability) =>
        roleAllows(user.role, capability)
      )
    ) {
      throw redirect({
        to: '/403',
      })
    }
  },
  component: AuthenticatedLayout,
})
