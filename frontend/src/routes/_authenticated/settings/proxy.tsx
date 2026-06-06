import { createFileRoute } from '@tanstack/react-router'
import { SettingsProxy } from '@/features/settings/proxy'

export const Route = createFileRoute('/_authenticated/settings/proxy')({
  component: SettingsProxy,
})
