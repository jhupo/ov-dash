import { createFileRoute } from '@tanstack/react-router'
import { ServerStatus } from '@/features/server-status'

export const Route = createFileRoute('/_authenticated/server-status/')({
  component: ServerStatus,
})
