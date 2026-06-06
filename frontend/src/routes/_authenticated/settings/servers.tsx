import { createFileRoute } from '@tanstack/react-router'
import { SettingsServers } from '@/features/settings/servers'

export const Route = createFileRoute('/_authenticated/settings/servers')({
  component: SettingsServers,
})
