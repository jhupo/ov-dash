import { type ColumnDef } from '@tanstack/react-table'
import { Badge } from '@/components/ui/badge'
import { Checkbox } from '@/components/ui/checkbox'
import { DataTableColumnHeader } from '@/components/data-table'
import { labels, priorities, statuses } from '../data/data'
import { type Task } from '../data/schema'
import { DataTableRowActions } from './data-table-row-actions'

export const tasksColumns: ColumnDef<Task>[] = [
  {
    id: 'select',
    header: ({ table }) => (
      <Checkbox
        checked={
          table.getIsAllPageRowsSelected() ||
          (table.getIsSomePageRowsSelected() && 'indeterminate')
        }
        onCheckedChange={(value) => table.toggleAllPageRowsSelected(!!value)}
        aria-label='全选'
        className='translate-y-0.5'
      />
    ),
    cell: ({ row }) => (
      <Checkbox
        checked={row.getIsSelected()}
        onCheckedChange={(value) => row.toggleSelected(!!value)}
        aria-label='选择行'
        className='translate-y-0.5'
      />
    ),
    enableSorting: false,
    enableHiding: false,
  },
  {
    accessorKey: 'id',
    header: ({ column }) => (
      <DataTableColumnHeader column={column} title='任务' />
    ),
    cell: ({ row }) => {
      const id = String(row.getValue('id'))
      return (
        <div className='w-24 truncate font-mono text-xs' title={id}>
          {id.replace(/^srvcol_/, '')}
        </div>
      )
    },
    enableSorting: false,
    enableHiding: false,
  },
  {
    accessorKey: 'title',
    header: ({ column }) => (
      <DataTableColumnHeader column={column} title='标题' />
    ),
    meta: {
      className: 'ps-1 min-w-[28rem]',
      tdClassName: 'ps-4',
    },
    cell: ({ row }) => {
      const label = labels.find((label) => label.value === row.original.label)

      return (
        <div className='flex min-w-0 flex-col gap-1'>
          <div className='flex min-w-0 items-center gap-2'>
            {label && (
              <Badge variant='outline' className='shrink-0'>
                {label.label}
              </Badge>
            )}
            <span className='min-w-0 truncate font-medium'>
              {row.getValue('title')}
            </span>
          </div>
          {row.original.description && (
            <span className='truncate text-xs text-muted-foreground'>
              {row.original.description}
            </span>
          )}
        </div>
      )
    },
  },
  {
    accessorKey: 'assignee',
    header: ({ column }) => (
      <DataTableColumnHeader column={column} title='来源' />
    ),
    cell: ({ row }) => (
      <div className='max-w-48 truncate text-sm text-muted-foreground'>
        {row.original.assignee || '-'}
      </div>
    ),
  },
  {
    accessorKey: 'status',
    header: ({ column }) => (
      <DataTableColumnHeader column={column} title='状态' />
    ),
    meta: { className: 'ps-1', tdClassName: 'ps-4' },
    cell: ({ row }) => {
      const status = statuses.find(
        (status) => status.value === row.getValue('status')
      )

      if (!status) {
        return null
      }

      return (
        <div className='flex w-25 items-center gap-2'>
          {status.icon && (
            <status.icon className='size-4 text-muted-foreground' />
          )}
          <span>{status.label}</span>
        </div>
      )
    },
    filterFn: (row, id, value) => {
      return value.includes(row.getValue(id))
    },
  },
  {
    accessorKey: 'priority',
    header: ({ column }) => (
      <DataTableColumnHeader column={column} title='优先级' />
    ),
    meta: { className: 'ps-1', tdClassName: 'ps-3' },
    cell: ({ row }) => {
      const priority = priorities.find(
        (priority) => priority.value === row.getValue('priority')
      )

      if (!priority) {
        return null
      }

      return (
        <div className='flex items-center gap-2'>
          {priority.icon && (
            <priority.icon className='size-4 text-muted-foreground' />
          )}
          <span>{priority.label}</span>
        </div>
      )
    },
    filterFn: (row, id, value) => {
      return value.includes(row.getValue(id))
    },
  },
  {
    id: 'actions',
    cell: ({ row }) => <DataTableRowActions row={row} />,
  },
]
