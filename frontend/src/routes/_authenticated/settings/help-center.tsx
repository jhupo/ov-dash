import { createFileRoute } from '@tanstack/react-router'
import { SettingsHelpCenter } from '@/features/settings/help-center'

export const Route = createFileRoute('/_authenticated/settings/help-center')({
  component: SettingsHelpCenter,
})
