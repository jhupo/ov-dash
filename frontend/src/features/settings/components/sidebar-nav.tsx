import { type JSX } from 'react'
import { Link, useLocation } from '@tanstack/react-router'
import { cn } from '@/lib/utils'
import { buttonVariants } from '@/components/ui/button'

type SidebarNavProps = React.HTMLAttributes<HTMLElement> & {
  items: {
    href: string
    title: string
    icon: JSX.Element
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
        <Link
          key={item.href}
          to={item.href}
          className={cn(
            buttonVariants({ variant: 'ghost', size: 'sm' }),
            pathname === item.href
              ? 'bg-muted hover:bg-accent'
              : 'hover:bg-accent',
            'h-9 shrink-0 justify-start'
          )}
        >
          <span className='me-2'>{item.icon}</span>
          {item.title}
        </Link>
      ))}
    </nav>
  )
}
