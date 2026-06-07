import { Link, useLocation } from '@tanstack/react-router'
import { type LucideIcon } from 'lucide-react'
import { cn } from '@/lib/utils'
import { buttonVariants } from '@/components/ui/button'

type SidebarNavProps = React.HTMLAttributes<HTMLElement> & {
  items: {
    href: string
    title: string
    icon: LucideIcon
  }[]
}

export function SidebarNav({ className, items, ...props }: SidebarNavProps) {
  const { pathname } = useLocation()

  return (
    <nav
      className={cn(
        'mx-auto flex w-full max-w-4xl gap-2 overflow-x-auto border-b pb-2',
        className
      )}
      {...props}
    >
      {items.map((item) => (
        <SidebarNavLink
          key={item.href}
          item={item}
          active={pathname === item.href}
        />
      ))}
    </nav>
  )
}

function SidebarNavLink({
  active,
  item,
}: {
  active: boolean
  item: SidebarNavProps['items'][number]
}) {
  const Icon = item.icon

  return (
    <Link
      to={item.href}
      className={cn(
        buttonVariants({ variant: 'ghost', size: 'sm' }),
        active ? 'bg-muted hover:bg-accent' : 'hover:bg-accent',
        'h-9 shrink-0 justify-start'
      )}
    >
      <Icon className='me-2 size-[18px]' />
      {item.title}
    </Link>
  )
}
