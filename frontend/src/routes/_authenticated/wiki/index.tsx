import { createFileRoute } from '@tanstack/react-router'
import { Wiki } from '@/features/wiki'

export const Route = createFileRoute('/_authenticated/wiki/')({
  component: Wiki,
})
