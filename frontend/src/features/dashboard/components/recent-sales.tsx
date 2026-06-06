import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar'
import { type RecentSale } from '@/services/dashboard'

type RecentSalesProps = {
  items: RecentSale[]
}

function formatAmount(amount: number) {
  return new Intl.NumberFormat('zh-CN', {
    style: 'currency',
    currency: 'USD',
  }).format(amount / 100)
}

export function RecentSales({ items }: RecentSalesProps) {
  if (!items.length) {
    return (
      <div className='py-10 text-center text-sm text-muted-foreground'>
        暂无销售记录。
      </div>
    )
  }

  return (
    <div className='space-y-8'>
      {items.map((item) => (
        <div key={`${item.email}-${item.amount}`} className='flex items-center gap-4'>
          <Avatar className='h-9 w-9'>
            <AvatarImage alt={item.name} />
            <AvatarFallback>{item.name.slice(0, 2).toUpperCase()}</AvatarFallback>
          </Avatar>
          <div className='flex flex-1 flex-wrap items-center justify-between'>
            <div className='space-y-1'>
              <p className='text-sm leading-none font-medium'>{item.name}</p>
              <p className='text-sm text-muted-foreground'>{item.email}</p>
            </div>
            <div className='font-medium'>{formatAmount(item.amount)}</div>
          </div>
        </div>
      ))}
    </div>
  )
}
