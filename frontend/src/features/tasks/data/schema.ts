import { z } from 'zod'

export const taskSchema = z.object({
  id: z.string(),
  title: z.string(),
  status: z.string(),
  label: z.string(),
  priority: z.string(),
  description: z.string().optional(),
  assignee: z.string().optional(),
  createdAt: z.string().optional(),
  updatedAt: z.string().optional(),
})

export type Task = z.infer<typeof taskSchema>
